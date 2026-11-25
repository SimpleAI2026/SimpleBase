// 文件说明：policy_gate_test.go — internal/api 的单元测试（TestPolicyGateResourcesAndExpiry、TestPolicyGateRealtimeDefersCollectionScope 等）
//
// 用法
//
//	go test ./internal/api -run 'TestPolicyGateResourcesAndExpiry|TestPolicyGateRealtimeDefersCollectionScope|TestScopeSubset|TestRouteGuardsApplyPolicyGate'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/policy_gate_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestPolicyGateResourcesAndExpiry(t *testing.T)
//	TestPolicyGateRealtimeDefersCollectionScope(t *testing.T)
//	TestScopeSubset(t *testing.T)
//	TestRouteGuardsApplyPolicyGate(t *testing.T)
//	TestRouteGuardsWriteGateReadOnlyInstance(t *testing.T)

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

func TestPolicyGateResourcesAndExpiry(t *testing.T) {
	e := echo.New()
	called := false
	handler := policyGate(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})
	check := func(method, path string, policy *auth.KeyPolicy, status int) {
		req := httptest.NewRequest(method, path, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/projects/:projectID/databases/:databaseID/data/collections/:collection")
		c.SetParamNames("projectID", "databaseID", "collection")
		c.SetParamValues("p1", "db1", "orders")
		c.SetRequest(req.WithContext(WithPrincipal(req.Context(), auth.Principal{Scope: policy})))
		called = false
		_ = handler(c)
		if rec.Code != status || called != (status == http.StatusOK) {
			t.Fatalf("%s %s: status=%d called=%v", method, path, rec.Code, called)
		}
	}
	readOrders := &auth.KeyPolicy{Resources: []string{"collection:orders"}, Actions: []string{"read"}}
	check(http.MethodGet, "/", readOrders, http.StatusOK)
	check(http.MethodPost, "/", readOrders, http.StatusForbidden)
	check(http.MethodGet, "/", &auth.KeyPolicy{Resources: []string{"collection:other"}, Actions: []string{"read"}}, http.StatusForbidden)
	expired := time.Now().Add(-time.Minute)
	check(http.MethodGet, "/", &auth.KeyPolicy{Resources: []string{"collection:*"}, Actions: []string{"read"}, ExpiresAt: &expired}, http.StatusForbidden)
}

func TestPolicyGateRealtimeDefersCollectionScope(t *testing.T) {
	for _, test := range []struct {
		collection string
		status     int
	}{{"orders", http.StatusOK}, {"other", http.StatusForbidden}} {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/v1/projects/p1/realtime?collections="+test.collection, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/v1/projects/:projectID/realtime")
		c.SetRequest(req.WithContext(WithPrincipal(req.Context(), auth.Principal{Scope: &auth.KeyPolicy{
			Resources: []string{"collection:orders"}, Actions: []string{"read"},
		}})))
		handler := policyGate(func(c echo.Context) error {
			if err := (&RealtimeHandler{}).checkScope(c, splitRealtimeNames(c.QueryParam("collections"), true)); err != nil {
				return WriteError(c, err)
			}
			return c.NoContent(http.StatusOK)
		})
		_ = handler(c)
		if rec.Code != test.status {
			t.Fatalf("collection %s: status=%d want %d", test.collection, rec.Code, test.status)
		}
	}
}

func TestScopeSubset(t *testing.T) {
	parent := &auth.KeyPolicy{Resources: []string{"collection:orders"}, Actions: []string{"read"}, HiddenFields: []string{"secret"}}
	if !scopeSubset(parent, parent) {
		t.Fatal("same scope rejected")
	}
	if scopeSubset(parent, &auth.KeyPolicy{Resources: []string{"collection:*"}, Actions: []string{"read"}}) {
		t.Fatal("wider resource accepted")
	}
	if scopeSubset(parent, &auth.KeyPolicy{Resources: []string{"collection:orders"}, Actions: []string{"read", "write"}}) {
		t.Fatal("wider action accepted")
	}
}

// 受限 Key（Scope 非空）经 g.require / g.delegable 挂载的路由必须先过 policyGate。
// function-triggers 过去直接用 auth.Require，绕开了 Key 的资源 / 动作范围。
func TestRouteGuardsApplyPolicyGate(t *testing.T) {
	var deps Dependencies
	deps.Config.Instance.Writable = true
	g := newRouteGuards(deps)
	e := echo.New()
	ok := func(c echo.Context) error { return c.NoContent(http.StatusOK) }
	e.GET("/v1/projects/:projectID/function-triggers", ok, g.require(auth.ProjectAdmin))
	e.POST("/v1/projects/:projectID/function-triggers/:id/invoke", ok, g.require(auth.DatabaseWrite), g.writeGate)

	run := func(method, path string, scope *auth.KeyPolicy) int {
		req := httptest.NewRequest(method, path, nil)
		p := apiKeyPrincipal(auth.ProjectAdmin, auth.DatabaseWrite)
		p.Scope = scope
		req = req.WithContext(WithPrincipal(req.Context(), p))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}
	listPath := "/v1/projects/proj-1/function-triggers"
	invokePath := "/v1/projects/proj-1/function-triggers/t1/invoke"

	if got := run(http.MethodGet, listPath, nil); got != http.StatusOK {
		t.Fatalf("unrestricted key: status=%d want 200", got)
	}
	ordersOnly := &auth.KeyPolicy{Resources: []string{"collection:orders"}, Actions: []string{"read"}}
	if got := run(http.MethodGet, listPath, ordersOnly); got != http.StatusForbidden {
		t.Fatalf("scoped key on function-triggers list: status=%d want 403", got)
	}
	if got := run(http.MethodPost, invokePath, ordersOnly); got != http.StatusForbidden {
		t.Fatalf("scoped key on function-triggers invoke: status=%d want 403", got)
	}
	projectRead := &auth.KeyPolicy{Resources: []string{"project:*"}, Actions: []string{"read"}}
	if got := run(http.MethodPost, invokePath, projectRead); got != http.StatusForbidden {
		t.Fatalf("read-only scope must not invoke: status=%d want 403", got)
	}
}

// 只读实例：writeGate 返回 ErrWriterUnavailable 对应的 503。
func TestRouteGuardsWriteGateReadOnlyInstance(t *testing.T) {
	var deps Dependencies
	deps.Config.Instance.Writable = false
	g := newRouteGuards(deps)
	e := echo.New()
	e.POST("/x", func(c echo.Context) error { return c.NoContent(http.StatusOK) }, g.writeGate)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", rec.Code)
	}
}
