// 文件说明：oai_routes_test.go — OpenAI 兼容出口的路由级契约（路径、权限、认证）；handler 单元测试在 internal/api/llmhttp。
//
// 用法
//
//	go test ./internal/api -run 'TestOpenAI'
//
// 调用架构
//
//	[上游] 本包测试
//	         │ 挂真实路由树
//	         ▼
//	[本文件] internal/api/oai_routes_test.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api/llmhttp（经 NewRouter 挂载）
//
// 主要符号
//
//	TestOpenAIRoutePaths / TestOpenAIRoutesRequireLLMInvoke / TestOpenAIRoutesRequireAuth
//	openAIRoutes(all map[string]bool) []string

package api

import (
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// 锁定双 v1 路径：前一段是 SimpleBase 的 API 版本，后一段是 OpenAI SDK 约定的资源前缀。
// 防止后来者把 ".../projects/:id/v1/chat/completions" "顺手修正"成单段 v1——
// 那会让官方 SDK 在 base_url=".../projects/<id>/v1" 时拼出 404 路径。
func TestOpenAIRoutePaths(t *testing.T) {
	deps, key := fullRouterDeps(t)
	e := NewRouter(deps)

	registered := map[string]bool{}
	for _, r := range e.Routes() {
		registered[r.Method+" "+r.Path] = true
	}
	for _, want := range []string{
		"POST /v1/projects/:projectID/v1/chat/completions",
		"GET /v1/projects/:projectID/v1/models",
	} {
		if !registered[want] {
			t.Fatalf("route %q not registered; got %v", want, openAIRoutes(registered))
		}
	}
	// 旧端点已删除，不保留兼容期。
	for _, gone := range []string{
		"POST /v1/projects/:projectID/llm/chat",
		"POST /v1/projects/:projectID/llm/stream",
		"GET /v1/projects/:projectID/llm/providers",
	} {
		if registered[gone] {
			t.Fatalf("legacy route %q must be removed", gone)
		}
	}
	// /llm/settings 是模型配置读写，不在清理范围，必须仍在。
	for _, kept := range []string{
		"GET /v1/projects/:projectID/llm/settings",
		"PUT /v1/projects/:projectID/llm/settings",
	} {
		if !registered[kept] {
			t.Fatalf("route %q is settings, not the chat protocol, and must be kept", kept)
		}
	}

	// 端到端：带 Key 能通；旧路径不再可达（POST 落到 SPA fallback 的 405/404）。
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/v1/models", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("models status=%d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"object":"list"`) {
		t.Fatalf("models body=%s", rec.Body.String())
	}

	old := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/llm/chat", strings.NewReader(`{"messages":[{"role":"user","content":"x"}]}`))
	old.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	old.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	oldRec := httptest.NewRecorder()
	e.ServeHTTP(oldRec, old)
	if oldRec.Code == http.StatusOK {
		t.Fatalf("legacy /llm/chat must not be served any more: %s", oldRec.Body.String())
	}
}

func openAIRoutes(all map[string]bool) []string {
	var out []string
	for r := range all {
		if strings.Contains(r, "/v1/chat") || strings.Contains(r, "/v1/models") || strings.Contains(r, "/llm/") {
			out = append(out, r)
		}
	}
	return out
}

// 新端点要求 llm:invoke，而不是 database:read：只有读权限的 Key 不能调用模型。
func TestOpenAIRoutesRequireLLMInvoke(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Auth = auth.NewService(&memAuthRepo{rec: auth.APIKeyRecord{
		ID: "key-ro", KeyHash: deps.Auth.HashKey(key), TenantID: "tenant-1",
		ProjectIDs: []string{"proj-1"}, Permissions: []auth.Permission{auth.DatabaseRead},
	}}, "secret")
	e := NewRouter(deps)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/projects/proj-1/v1/models", ""},
		{http.MethodPost, "/v1/projects/proj-1/v1/chat/completions", `{"messages":[{"role":"user","content":"x"}]}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s with database:read only: status=%d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

// 无凭证必须 401，而不是落进 SPA fallback。
func TestOpenAIRoutesRequireAuth(t *testing.T) {
	deps, _ := fullRouterDeps(t)
	e := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/v1/models", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
}
