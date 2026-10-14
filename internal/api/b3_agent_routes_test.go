package api

// b3_agent_routes_test.go：planv6.0 B3 agent/schedule 路由块表征测试。
// 在提取挂载函数前锁定旧实现行为：
//   1. agent + schedule 路由集合恰好等于清单；
//   2. deps.System == nil 时整块不挂载（404）；
//   3. 只读权限（DatabaseRead）下写路由 403，读路由可达；
//   4. agent 块全部 require（不带 delegable），委托令牌一律 403；
//   5. writable=false 时写路由被 writeGate 拦截 503；
//   6. POST /agent-threads/:threadID/runs 保持 DatabaseRead + writeGate
//      （创建 run 只需读权限 + 写门禁，B3 不得改成 DatabaseWrite）；
//   7. 旧 /agent-schedules 写路由（含 /run）已合并到 /cron-jobs，
//      统一挂载为 DatabaseWrite + writeGate 并返回 410（兼容层）。
// status 断言取自当前实现。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// agentBlockRoutes 是 agent 块 + schedule 块的完整路由清单。
var agentBlockRoutes = []struct{ method, path string }{
	{http.MethodGet, "/v1/projects/:projectID/agents/modules"},
	{http.MethodGet, "/v1/projects/:projectID/agents"},
	{http.MethodGet, "/v1/projects/:projectID/agents/models"},
	{http.MethodPost, "/v1/projects/:projectID/agents"},
	{http.MethodGet, "/v1/projects/:projectID/agents/:agentID"},
	{http.MethodPatch, "/v1/projects/:projectID/agents/:agentID"},
	{http.MethodDelete, "/v1/projects/:projectID/agents/:agentID"},
	{http.MethodGet, "/v1/projects/:projectID/agent-threads"},
	{http.MethodPost, "/v1/projects/:projectID/agent-threads"},
	{http.MethodGet, "/v1/projects/:projectID/agent-threads/:threadID"},
	{http.MethodPatch, "/v1/projects/:projectID/agent-threads/:threadID"},
	{http.MethodDelete, "/v1/projects/:projectID/agent-threads/:threadID"},
	{http.MethodGet, "/v1/projects/:projectID/agent-threads/:threadID/messages"},
	{http.MethodGet, "/v1/projects/:projectID/agent-threads/:threadID/runs"},
	{http.MethodPost, "/v1/projects/:projectID/agent-threads/:threadID/runs"},
	{http.MethodPost, "/v1/projects/:projectID/agent-runs/:runID/cancel"},
	{http.MethodPost, "/v1/projects/:projectID/agent-runs/:runID/confirmations/:callID"},
	{http.MethodGet, "/v1/projects/:projectID/agent-runs/:runID/events"},
	{http.MethodGet, "/v1/projects/:projectID/agent-schedules"},
	{http.MethodPost, "/v1/projects/:projectID/agent-schedules"},
	{http.MethodGet, "/v1/projects/:projectID/agent-schedules/:scheduleID"},
	{http.MethodPatch, "/v1/projects/:projectID/agent-schedules/:scheduleID"},
	{http.MethodDelete, "/v1/projects/:projectID/agent-schedules/:scheduleID"},
	{http.MethodGet, "/v1/projects/:projectID/agent-schedules/:scheduleID/runs"},
	{http.MethodPost, "/v1/projects/:projectID/agent-schedules/:scheduleID/run"},
}

// agentWriteOps 是 agent/schedule 块中标 DatabaseWrite 的路由。
var agentWriteOps = []struct{ method, path string }{
	{http.MethodPost, "/v1/projects/proj-1/agents"},
	{http.MethodPatch, "/v1/projects/proj-1/agents/a1"},
	{http.MethodDelete, "/v1/projects/proj-1/agents/a1"},
	{http.MethodPost, "/v1/projects/proj-1/agent-threads"},
	{http.MethodPatch, "/v1/projects/proj-1/agent-threads/t1"},
	{http.MethodDelete, "/v1/projects/proj-1/agent-threads/t1"},
	{http.MethodPost, "/v1/projects/proj-1/agent-runs/r1/cancel"},
	{http.MethodPost, "/v1/projects/proj-1/agent-runs/r1/confirmations/c1"},
	{http.MethodPost, "/v1/projects/proj-1/agent-schedules"},
	{http.MethodPatch, "/v1/projects/proj-1/agent-schedules/s1"},
	{http.MethodDelete, "/v1/projects/proj-1/agent-schedules/s1"},
	{http.MethodPost, "/v1/projects/proj-1/agent-schedules/s1/run"},
}

// agentReadOps 是块内 DatabaseRead 只读路由（无 handler 外副作用）。
var agentReadOps = []struct{ method, path string }{
	{http.MethodGet, "/v1/projects/proj-1/agents/modules"},
	{http.MethodGet, "/v1/projects/proj-1/agents"},
	{http.MethodGet, "/v1/projects/proj-1/agents/models"},
	{http.MethodGet, "/v1/projects/proj-1/agents/a1"},
	{http.MethodGet, "/v1/projects/proj-1/agent-threads"},
	{http.MethodGet, "/v1/projects/proj-1/agent-threads/t1"},
	{http.MethodGet, "/v1/projects/proj-1/agent-threads/t1/messages"},
	{http.MethodGet, "/v1/projects/proj-1/agent-threads/t1/runs"},
	{http.MethodGet, "/v1/projects/proj-1/agent-runs/r1/events"},
	{http.MethodGet, "/v1/projects/proj-1/agent-schedules"},
	{http.MethodGet, "/v1/projects/proj-1/agent-schedules/s1"},
	{http.MethodGet, "/v1/projects/proj-1/agent-schedules/s1/runs"},
}

// TestB3AgentRouteSetLocked：真实 Routes() 中 agent/schedule 路由集合
// 恰好等于清单，无多无少（/run 作为兼容层写路由始终挂载）。
func TestB3AgentRouteSetLocked(t *testing.T) {
	deps, _ := fullRouterDeps(t)
	e := NewRouter(deps)

	want := map[string]bool{}
	for _, r := range agentBlockRoutes {
		want[r.method+" "+r.path] = true
	}
	got := map[string]bool{}
	for _, r := range e.Routes() {
		if !strings.Contains(r.Path, "/agents") && !strings.Contains(r.Path, "/agent-threads") &&
			!strings.Contains(r.Path, "/agent-runs") && !strings.Contains(r.Path, "/agent-schedules") {
			continue
		}
		got[r.Method+" "+r.Path] = true
	}
	if len(got) != len(want) {
		t.Fatalf("agent route count: got %d want %d\nextra: %v", len(got), len(want), diffKeys(got, want))
	}
	for k := range want {
		if !got[k] {
			t.Fatalf("missing route %s; got=%v", k, keys(got))
		}
	}
}

// TestB3AgentRoutesAbsentWithoutSystem：System == nil 时整块不挂载。
// 断言用 Routes() 集合（不发起请求）：v1 组带 auth 中间件，未认证请求
// 会先被组级伪路由（/v1/projects/:projectID/*）拦成 401，无法区分挂载。
func TestB3AgentRoutesAbsentWithoutSystem(t *testing.T) {
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
	for _, r := range agentBlockRoutes {
		if mounted[r.method+" "+r.path] {
			t.Fatalf("%s %s: route must not mount without System", r.method, r.path)
		}
	}
}

// TestB3AgentScheduleWriteRoutesGone：旧 /agent-schedules 写路由（含 /run）
// 在有写权限的实例上统一返回 410 agent_schedules_merged。
func TestB3AgentScheduleWriteRoutesGone(t *testing.T) {
	deps, key := fullRouterDeps(t)
	e := NewRouter(deps)

	for _, op := range []struct{ method, path string }{
		{http.MethodPost, "/v1/projects/proj-1/agent-schedules"},
		{http.MethodPatch, "/v1/projects/proj-1/agent-schedules/s1"},
		{http.MethodDelete, "/v1/projects/proj-1/agent-schedules/s1"},
		{http.MethodPost, "/v1/projects/proj-1/agent-schedules/s1/run"},
	} {
		req := httptest.NewRequest(op.method, op.path, strings.NewReader("{}"))
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusGone || !strings.Contains(rec.Body.String(), "agent_schedules_merged") {
			t.Fatalf("%s %s: want 410 agent_schedules_merged, got %d %s", op.method, op.path, rec.Code, rec.Body.String())
		}
	}
}

// TestB3AgentWriteRoutesForbiddenWithReadOnlyPermission：
// DatabaseRead 身份访问写路由返回 403，读路由可达。
func TestB3AgentWriteRoutesForbiddenWithReadOnlyPermission(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Auth = auth.NewService(&memAuthRepo{rec: auth.APIKeyRecord{
		ID: "key-ro", KeyHash: deps.Auth.HashKey(key), TenantID: "tenant-1",
		ProjectIDs:  []string{"proj-1"},
		Permissions: []auth.Permission{auth.DatabaseRead},
	}}, "secret")
	e := NewRouter(deps)

	for _, op := range agentWriteOps {
		req := httptest.NewRequest(op.method, op.path, strings.NewReader("{}"))
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s: want 403, got %d %s", op.method, op.path, rec.Code, rec.Body.String())
		}
	}
	for _, op := range agentReadOps {
		req := httptest.NewRequest(op.method, op.path, nil)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		// 只断言通过权限中间件：403=权限拒绝，404=资源不存在（handler 内
		// 正常业务路径，如 agent 不存在），二者都不该发生的是 403。
		if rec.Code == http.StatusForbidden {
			t.Fatalf("%s %s: read route must pass permission gate, got %d %s", op.method, op.path, rec.Code, rec.Body.String())
		}
	}
}

// TestB3AgentCreateRunKeepsReadPermission：POST /agent-threads/:id/runs
// 是 DatabaseRead + writeGate（不是 DatabaseWrite）：DatabaseRead 身份
// 在 writable 实例上必须通过权限中间件（非 403）。
func TestB3AgentCreateRunKeepsReadPermission(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Auth = auth.NewService(&memAuthRepo{rec: auth.APIKeyRecord{
		ID: "key-ro", KeyHash: deps.Auth.HashKey(key), TenantID: "tenant-1",
		ProjectIDs:  []string{"proj-1"},
		Permissions: []auth.Permission{auth.DatabaseRead},
	}}, "secret")
	e := NewRouter(deps)

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/agent-threads/t1/runs", strings.NewReader(`{"content":"hi"}`))
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("create run with DatabaseRead must pass permission gate, got %d %s", rec.Code, rec.Body.String())
	}
}

// TestB3AgentRoutesRejectDelegatedTokens：agent/schedule 块全部 require
// （无 delegable），委托令牌一律 403（读路由也不放行）。
func TestB3AgentRoutesRejectDelegatedTokens(t *testing.T) {
	env := newSkillRouterEnv(t)
	tok := env.delegationToken(t)

	all := append(append([]struct{ method, path string }{}, agentWriteOps...), agentReadOps...)
	for _, op := range all {
		req := httptest.NewRequest(op.method, op.path, strings.NewReader("{}"))
		req.Header.Set(echoContentType, "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		env.echo.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s: delegated token must be rejected on agent routes, got %d %s", op.method, op.path, rec.Code, rec.Body.String())
		}
	}
}

// TestB3AgentWriteGateOnReadOnlyInstance：Writable=false 时写路由 503
// readonly_instance。
func TestB3AgentWriteGateOnReadOnlyInstance(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Config.Instance.Writable = false
	e := NewRouter(deps)

	for _, op := range agentWriteOps {
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

// diffKeys 返回 got 中不在 want 里的 key（诊断用）。
func diffKeys(got, want map[string]bool) []string {
	var out []string
	for k := range got {
		if !want[k] {
			out = append(out, k)
		}
	}
	return out
}
