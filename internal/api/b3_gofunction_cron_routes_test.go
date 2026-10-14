package api

// b3_gofunction_cron_routes_test.go：planv6.0 B3 gofunction/cron 路由块
// 表征测试。在提取挂载函数前锁定旧实现行为：
//   1. gofunction + cron 路由集合恰好等于清单；
//   2. System == nil 时整块不挂载（Routes() 集合断言）；
//   3. 只读权限下写路由 403，读路由通过权限门；
//   4. 两块均为 delegable：委托令牌可到达 handler（非 403）；
//   5. writable=false 时写路由被 writeGate 拦截 503。
// status 断言取自当前实现。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// gofnCronBlockRoutes 是 gofunction + cron 块的完整路由清单。
var gofnCronBlockRoutes = []struct{ method, path string }{
	{http.MethodGet, "/v1/projects/:projectID/gofunctions"},
	{http.MethodPost, "/v1/projects/:projectID/gofunctions"},
	{http.MethodGet, "/v1/projects/:projectID/gofunctions/:name"},
	{http.MethodPatch, "/v1/projects/:projectID/gofunctions/:name"},
	{http.MethodDelete, "/v1/projects/:projectID/gofunctions/:name"},
	{http.MethodGet, "/v1/projects/:projectID/gofunctions/:name/versions"},
	{http.MethodPost, "/v1/projects/:projectID/gofunctions/:name/versions"},
	{http.MethodGet, "/v1/projects/:projectID/gofunctions/:name/versions/:ver"},
	{http.MethodPost, "/v1/projects/:projectID/gofunctions/:name/versions/:ver/activate"},
	{http.MethodPost, "/v1/projects/:projectID/gofunctions/:name/versions/:ver/test"},
	{http.MethodGet, "/v1/projects/:projectID/cron-jobs"},
	{http.MethodPost, "/v1/projects/:projectID/cron-jobs"},
	{http.MethodGet, "/v1/projects/:projectID/cron-jobs/:jobID"},
	{http.MethodPatch, "/v1/projects/:projectID/cron-jobs/:jobID"},
	{http.MethodDelete, "/v1/projects/:projectID/cron-jobs/:jobID"},
	{http.MethodGet, "/v1/projects/:projectID/cron-jobs/:jobID/runs"},
	{http.MethodPost, "/v1/projects/:projectID/cron-jobs/:jobID/trigger"},
}

// gofnCronWriteOps 是两块中标 DatabaseWrite 的路由（含 writeGate）。
var gofnCronWriteOps = []struct{ method, path string }{
	{http.MethodPost, "/v1/projects/proj-1/gofunctions"},
	{http.MethodPatch, "/v1/projects/proj-1/gofunctions/f1"},
	{http.MethodDelete, "/v1/projects/proj-1/gofunctions/f1"},
	{http.MethodPost, "/v1/projects/proj-1/gofunctions/f1/versions"},
	{http.MethodPost, "/v1/projects/proj-1/gofunctions/f1/versions/1/activate"},
	{http.MethodPost, "/v1/projects/proj-1/gofunctions/f1/versions/1/test"},
	{http.MethodPost, "/v1/projects/proj-1/cron-jobs"},
	{http.MethodPatch, "/v1/projects/proj-1/cron-jobs/j1"},
	{http.MethodDelete, "/v1/projects/proj-1/cron-jobs/j1"},
	{http.MethodPost, "/v1/projects/proj-1/cron-jobs/j1/trigger"},
}

// gofnCronReadOps 是两块的只读路由。
var gofnCronReadOps = []struct{ method, path string }{
	{http.MethodGet, "/v1/projects/proj-1/gofunctions"},
	{http.MethodGet, "/v1/projects/proj-1/gofunctions/f1"},
	{http.MethodGet, "/v1/projects/proj-1/gofunctions/f1/versions"},
	{http.MethodGet, "/v1/projects/proj-1/gofunctions/f1/versions/1"},
	{http.MethodGet, "/v1/projects/proj-1/cron-jobs"},
	{http.MethodGet, "/v1/projects/proj-1/cron-jobs/j1"},
	{http.MethodGet, "/v1/projects/proj-1/cron-jobs/j1/runs"},
}

// TestB3GofnCronRouteSetLocked：真实 Routes() 中两块路由集合恰好等于清单。
func TestB3GofnCronRouteSetLocked(t *testing.T) {
	deps, _ := fullRouterDeps(t)
	e := NewRouter(deps)

	want := map[string]bool{}
	for _, r := range gofnCronBlockRoutes {
		want[r.method+" "+r.path] = true
	}
	got := map[string]bool{}
	for _, r := range e.Routes() {
		if !strings.Contains(r.Path, "/gofunctions") && !strings.Contains(r.Path, "/cron-jobs") {
			continue
		}
		got[r.Method+" "+r.Path] = true
	}
	if len(got) != len(want) {
		t.Fatalf("gofn/cron route count: got %d want %d\nextra: %v\nmissing: %v",
			len(got), len(want), diffKeys(got, want), diffKeys(want, got))
	}
	for k := range want {
		if !got[k] {
			t.Fatalf("missing route %s", k)
		}
	}
}

// TestB3GofnCronRoutesAbsentWithoutSystem：System == nil 时整块不挂载。
func TestB3GofnCronRoutesAbsentWithoutSystem(t *testing.T) {
	deps, _ := fullRouterDeps(t)
	deps.System = nil
	e := NewRouter(deps)
	mounted := map[string]bool{}
	for _, r := range e.Routes() {
		if r.Name == "echo_route_not_found" {
			continue
		}
		mounted[r.Method+" "+r.Path] = true
	}
	for _, r := range gofnCronBlockRoutes {
		if mounted[r.method+" "+r.path] {
			t.Fatalf("%s %s: route must not mount without System", r.method, r.path)
		}
	}
}

// TestB3GofnCronWriteRoutesForbiddenWithReadOnlyPermission：
// DatabaseRead 身份访问写路由返回 403，读路由通过权限门。
func TestB3GofnCronWriteRoutesForbiddenWithReadOnlyPermission(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Auth = auth.NewService(&memAuthRepo{rec: auth.APIKeyRecord{
		ID: "key-ro", KeyHash: deps.Auth.HashKey(key), TenantID: "tenant-1",
		ProjectIDs:  []string{"proj-1"},
		Permissions: []auth.Permission{auth.DatabaseRead},
	}}, "secret")
	e := NewRouter(deps)

	for _, op := range gofnCronWriteOps {
		req := httptest.NewRequest(op.method, op.path, strings.NewReader("{}"))
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s: want 403, got %d %s", op.method, op.path, rec.Code, rec.Body.String())
		}
	}
	for _, op := range gofnCronReadOps {
		req := httptest.NewRequest(op.method, op.path, nil)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code == http.StatusForbidden {
			t.Fatalf("%s %s: read route must pass permission gate, got %d %s", op.method, op.path, rec.Code, rec.Body.String())
		}
	}
}

// TestB3GofnCronDelegatedAccess：两块均为 delegable，委托令牌（RoleUser
// 权限集含 DatabaseRead/Write）可到达 handler（非 403）。
func TestB3GofnCronDelegatedAccess(t *testing.T) {
	env := newSkillRouterEnv(t)
	tok := env.delegationToken(t)

	all := append(append([]struct{ method, path string }{}, gofnCronWriteOps...), gofnCronReadOps...)
	for _, op := range all {
		req := httptest.NewRequest(op.method, op.path, strings.NewReader("{}"))
		req.Header.Set(echoContentType, "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		env.echo.ServeHTTP(rec, req)
		if rec.Code == http.StatusForbidden {
			t.Fatalf("%s %s: delegable route must not 403 for delegated token, got %d %s", op.method, op.path, rec.Code, rec.Body.String())
		}
	}
}

// TestB3GofnCronWriteGateOnReadOnlyInstance：Writable=false 时写路由
// 503 readonly_instance。
func TestB3GofnCronWriteGateOnReadOnlyInstance(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Config.Instance.Writable = false
	e := NewRouter(deps)

	for _, op := range gofnCronWriteOps {
		req := httptest.NewRequest(op.method, op.path, strings.NewReader("{}"))
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "readonly_instance") {
			t.Fatalf("%s %s: want 503 readonly_instance, got %d %s", op.method, op.path, rec.Code, rec.Body.String())
		}
	}
}
