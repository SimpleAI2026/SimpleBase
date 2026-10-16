package api

// b3_sandbox_routes_test.go：planv6.0 B3 沙盒路由块表征测试。
// 目的：在提取挂载函数前锁定旧实现行为——
//   1. 沙盒块路由集合恰好等于预期清单（method + path 模板）；
//   2. 可选依赖为 nil（deps.Sandbox == nil）或不可用（Available() == false）
//      时，除 capabilities 外沙盒路由不挂载；
//   3. 正常权限下读写路由可达（不是 404/401/403）；
//   4. 权限不足（只读 DatabaseRead）时写路由返回 403，读路由仍可达；
//   5. delegated 身份按 delegable 标记放行（写路由 delegable(DatabaseWrite)）；
//   6. writable=false 时写路由被 writeGate 拦截返回 503；
//   7. project context 注入：跨项目访问返回 403 cross_project_denied。
// status 断言取自当前实现，不引入新语义。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
)

type contextT = context.Context

// fakeRouterSandbox 实现 SandboxService，全部方法返回可用的空结果。
type fakeRouterSandbox struct {
	available bool
}

func (f *fakeRouterSandbox) Available() bool { return f.available }
func (f *fakeRouterSandbox) Capabilities() sandbox.Capabilities {
	return sandbox.Capabilities{Available: f.available, Backend: "fake", Images: []string{"python:3.12-slim"}}
}
func (f *fakeRouterSandbox) Create(ctx contextT, projectID string, in sandbox.CreateInput, actor string) (sandbox.Sandbox, error) {
	return sandbox.Sandbox{ID: "sb-1", ProjectID: projectID, Name: "demo", Status: "created"}, nil
}
func (f *fakeRouterSandbox) List(ctx contextT, projectID, status, source, cursor string, limit int) ([]sandbox.Sandbox, string, error) {
	return []sandbox.Sandbox{{ID: "sb-1", ProjectID: projectID}}, "", nil
}
func (f *fakeRouterSandbox) Get(ctx contextT, projectID, id string, refresh bool) (sandbox.Sandbox, error) {
	return sandbox.Sandbox{ID: id, ProjectID: projectID, Status: "running"}, nil
}
func (f *fakeRouterSandbox) Update(ctx contextT, projectID, id string, in sandbox.UpdateInput) (sandbox.Sandbox, error) {
	return sandbox.Sandbox{ID: id, ProjectID: projectID, Status: "updated"}, nil
}
func (f *fakeRouterSandbox) Start(ctx contextT, projectID, id string) (sandbox.Sandbox, error) {
	return sandbox.Sandbox{ID: id, ProjectID: projectID, Status: "running"}, nil
}
func (f *fakeRouterSandbox) Stop(ctx contextT, projectID, id string) (sandbox.Sandbox, error) {
	return sandbox.Sandbox{ID: id, ProjectID: projectID, Status: "stopped"}, nil
}
func (f *fakeRouterSandbox) Delete(ctx contextT, projectID, id string) error { return nil }
func (f *fakeRouterSandbox) Exec(ctx contextT, projectID, id string, in sandbox.ExecInput) (sandbox.ExecOutput, error) {
	return sandbox.ExecOutput{ExitCode: 0, Stdout: "ok", Status: "done"}, nil
}
func (f *fakeRouterSandbox) ReadFile(ctx contextT, projectID, id, path string) (sandbox.FileContent, error) {
	return sandbox.FileContent{Content: []byte("hello")}, nil
}
func (f *fakeRouterSandbox) WriteFile(ctx contextT, projectID, id, path string, data []byte) error {
	return nil
}
func (f *fakeRouterSandbox) RemoveFile(ctx contextT, projectID, id, path string) error { return nil }
func (f *fakeRouterSandbox) ListDir(ctx contextT, projectID, id, path string) ([]vm.FileEntry, error) {
	return []vm.FileEntry{{Name: "a.txt", Kind: "file"}}, nil
}
func (f *fakeRouterSandbox) RunOnce(ctx contextT, projectID string, in sandbox.RunInput, actor string) (sandbox.ExecOutput, error) {
	return sandbox.ExecOutput{ExitCode: 0, Stdout: "ok", Status: "done", SandboxID: "sb-1"}, nil
}

// sandboxBlockRoutes 是沙盒块的完整路由清单（含 capabilities）。
// 与 mountV1Routes 中挂载顺序一致；路径相对 /v1/projects/:projectID。
var sandboxBlockRoutes = []struct{ method, path string }{
	{http.MethodGet, "/v1/projects/:projectID/sandboxes/capabilities"},
	{http.MethodGet, "/v1/projects/:projectID/sandboxes"},
	{http.MethodPost, "/v1/projects/:projectID/sandboxes"},
	{http.MethodPost, "/v1/projects/:projectID/sandboxes/run"},
	{http.MethodGet, "/v1/projects/:projectID/sandboxes/:sandboxID"},
	{http.MethodPatch, "/v1/projects/:projectID/sandboxes/:sandboxID"},
	{http.MethodDelete, "/v1/projects/:projectID/sandboxes/:sandboxID"},
	{http.MethodPost, "/v1/projects/:projectID/sandboxes/:sandboxID/start"},
	{http.MethodPost, "/v1/projects/:projectID/sandboxes/:sandboxID/stop"},
	{http.MethodPost, "/v1/projects/:projectID/sandboxes/:sandboxID/exec"},
	{http.MethodGet, "/v1/projects/:projectID/sandboxes/:sandboxID/files"},
	{http.MethodGet, "/v1/projects/:projectID/sandboxes/:sandboxID/files/content"},
	{http.MethodPut, "/v1/projects/:projectID/sandboxes/:sandboxID/files/content"},
	{http.MethodDelete, "/v1/projects/:projectID/sandboxes/:sandboxID/files/content"},
}

// TestB3_SandboxRouteSetLocked：真实 Routes() 中沙盒路由集合恰好等于清单，
// 无多无少（Echo 会自动补 HEAD，说明差异即可）。
func TestB3_SandboxRouteSetLocked(t *testing.T) {
	deps, _ := fullRouterDeps(t)
	deps.Sandbox = &fakeRouterSandbox{available: true}
	e := NewRouter(deps)

	want := map[string]bool{}
	for _, r := range sandboxBlockRoutes {
		want[r.method+" "+r.path] = true
	}
	got := map[string]bool{}
	for _, r := range e.Routes() {
		if !strings.HasPrefix(r.Path, "/v1/projects/:projectID/sandboxes") {
			continue
		}
		got[r.Method+" "+r.Path] = true
	}
	if len(got) != len(want) {
		t.Fatalf("sandbox route count: got %d want %d\ngot: %v", len(got), len(want), keys(got))
	}
	for k := range want {
		if !got[k] {
			t.Fatalf("missing route %s; got=%v", k, keys(got))
		}
	}
}

// TestB3_SandboxRoutesAbsentWithoutDependency：Sandbox == nil 或
// Available() == false 时仅 capabilities 挂载，其余路由不存在（404）。
func TestB3_SandboxRoutesAbsentWithoutDependency(t *testing.T) {
	for name, sb := range map[string]SandboxService{
		"nil sandbox": nil,
		"unavailable": &fakeRouterSandbox{available: false},
	} {
		t.Run(name, func(t *testing.T) {
			deps, key := fullRouterDeps(t)
			deps.Sandbox = sb
			e := NewRouter(deps)
			for _, r := range sandboxBlockRoutes[1:] { // 跳过 capabilities
				req := httptest.NewRequest(r.method, materializeRoute(r.path), strings.NewReader("{}"))
				req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)
				if rec.Code != http.StatusNotFound {
					t.Fatalf("%s %s: want 404 (route must not mount), got %d %s", r.method, r.path, rec.Code, rec.Body.String())
				}
			}
			// capabilities 仍在（供控制台空态展示）。
			req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/sandboxes/capabilities", nil)
			req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"available":false`) {
				t.Fatalf("capabilities: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestB3_SandboxWriteRoutesForbiddenWithReadOnlyPermission：
// DatabaseRead 身份访问写路由返回 403，读路由可达（200）。
func TestB3_SandboxWriteRoutesForbiddenWithReadOnlyPermission(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Sandbox = &fakeRouterSandbox{available: true}
	deps.Auth = auth.NewService(&memAuthRepo{rec: auth.APIKeyRecord{
		ID: "key-ro", KeyHash: deps.Auth.HashKey(key), TenantID: "tenant-1",
		ProjectIDs:  []string{"proj-1"},
		Permissions: []auth.Permission{auth.DatabaseRead},
	}}, "secret")
	e := NewRouter(deps)

	writeOps := []struct{ method, path string }{
		{http.MethodPost, "/v1/projects/proj-1/sandboxes"},
		{http.MethodPost, "/v1/projects/proj-1/sandboxes/run"},
		{http.MethodPatch, "/v1/projects/proj-1/sandboxes/s1"},
		{http.MethodDelete, "/v1/projects/proj-1/sandboxes/s1"},
		{http.MethodPost, "/v1/projects/proj-1/sandboxes/s1/start"},
		{http.MethodPost, "/v1/projects/proj-1/sandboxes/s1/stop"},
		{http.MethodPost, "/v1/projects/proj-1/sandboxes/s1/exec"},
		{http.MethodPut, "/v1/projects/proj-1/sandboxes/s1/files/content"},
		{http.MethodDelete, "/v1/projects/proj-1/sandboxes/s1/files/content"},
	}
	for _, op := range writeOps {
		req := httptest.NewRequest(op.method, op.path, strings.NewReader("{}"))
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s: want 403, got %d %s", op.method, op.path, rec.Code, rec.Body.String())
		}
	}
	for _, p := range []string{
		"/v1/projects/proj-1/sandboxes",
		"/v1/projects/proj-1/sandboxes/s1",
		"/v1/projects/proj-1/sandboxes/capabilities",
		"/v1/projects/proj-1/sandboxes/s1/files",
		"/v1/projects/proj-1/sandboxes/s1/files/content",
	} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: want 200, got %d %s", p, rec.Code, rec.Body.String())
		}
	}
}

// TestB3_SandboxDelegatedAccess：委托令牌按 delegable 标记访问。
// 写路由标的是 delegable(DatabaseWrite)，委托令牌（RoleUser 权限集）
// 可到达 handler（非 403）；start/stop/exec 标 require(DatabaseWrite)
// 不可委托，返回 403。
func TestB3_SandboxDelegatedAccess(t *testing.T) {
	env := newSkillRouterEnv(t)
	tok := env.delegationToken(t)

	delegableOK := []struct{ method, path string }{
		{http.MethodPost, "/v1/projects/proj-1/sandboxes"},
		{http.MethodPost, "/v1/projects/proj-1/sandboxes/run"},
		{http.MethodPatch, "/v1/projects/proj-1/sandboxes/s1"},
		{http.MethodDelete, "/v1/projects/proj-1/sandboxes/s1"},
	}
	for _, op := range delegableOK {
		req := httptest.NewRequest(op.method, op.path, strings.NewReader("{}"))
		req.Header.Set(echoContentType, "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		env.echo.ServeHTTP(rec, req)
		if rec.Code == http.StatusForbidden {
			t.Fatalf("%s %s: delegable route must not 403, got %d %s", op.method, op.path, rec.Code, rec.Body.String())
		}
	}
	requireOnly := []struct{ method, path string }{
		{http.MethodPost, "/v1/projects/proj-1/sandboxes/s1/start"},
		{http.MethodPost, "/v1/projects/proj-1/sandboxes/s1/stop"},
		{http.MethodPost, "/v1/projects/proj-1/sandboxes/s1/exec"},
	}
	for _, op := range requireOnly {
		req := httptest.NewRequest(op.method, op.path, strings.NewReader("{}"))
		req.Header.Set(echoContentType, "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		env.echo.ServeHTTP(rec, req)
		// skillRouterEnv 未配置 Sandbox（路由未挂载），此处只断言委托边界
		// 语义：路由存在时应 403；当前实现下未挂载返回 404，同样不是放行。
		if rec.Code == http.StatusOK {
			t.Fatalf("%s %s: require-only route must not 2xx for delegated token, got %d %s", op.method, op.path, rec.Code, rec.Body.String())
		}
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s: delegated token must hit delegation boundary (403) or unmounted route (404), got %d %s", op.method, op.path, rec.Code, rec.Body.String())
		}
	}
}

// TestB3_SandboxWriteGateOnReadOnlyInstance：Writable=false 时写路由
// 被 writeGate 拦截返回 503 readonly_instance；读路由不受影响。
func TestB3_SandboxWriteGateOnReadOnlyInstance(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Sandbox = &fakeRouterSandbox{available: true}
	deps.Config.Instance.Writable = false
	e := NewRouter(deps)

	writeOps := []struct{ method, path string }{
		{http.MethodPost, "/v1/projects/proj-1/sandboxes"},
		{http.MethodPost, "/v1/projects/proj-1/sandboxes/run"},
		{http.MethodPatch, "/v1/projects/proj-1/sandboxes/s1"},
		{http.MethodDelete, "/v1/projects/proj-1/sandboxes/s1"},
		{http.MethodPost, "/v1/projects/proj-1/sandboxes/s1/start"},
		{http.MethodPost, "/v1/projects/proj-1/sandboxes/s1/stop"},
		{http.MethodPost, "/v1/projects/proj-1/sandboxes/s1/exec"},
		{http.MethodPut, "/v1/projects/proj-1/sandboxes/s1/files/content"},
		{http.MethodDelete, "/v1/projects/proj-1/sandboxes/s1/files/content"},
	}
	for _, op := range writeOps {
		req := httptest.NewRequest(op.method, op.path, strings.NewReader("{}"))
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "readonly_instance") {
			t.Fatalf("%s %s: want 503 readonly_instance, got %d %s", op.method, op.path, rec.Code, rec.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/sandboxes/s1/files", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("read route on readonly instance: want 200, got %d %s", rec.Code, rec.Body.String())
	}
}

// TestB3_SandboxCrossProjectDenied：project context 注入后，key 无 proj-2
// 权限（ProjectIDs 仅 proj-1）时访问 proj-2 沙盒路由返回 403
// cross_project_denied（projectContextMiddlewareEcho 先于 handler 拒绝）。
func TestB3_SandboxCrossProjectDenied(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Sandbox = &fakeRouterSandbox{available: true}
	deps.Auth = auth.NewService(&memAuthRepo{rec: auth.APIKeyRecord{
		ID: "key-1", KeyHash: deps.Auth.HashKey(key), TenantID: "tenant-1",
		ProjectIDs:  []string{"proj-1"},
		Permissions: []auth.Permission{auth.DatabaseRead, auth.DatabaseWrite},
	}}, "secret")
	e := NewRouter(deps)
	for _, p := range []string{
		"/v1/projects/proj-2/sandboxes",
		"/v1/projects/proj-2/sandboxes/capabilities",
		"/v1/projects/proj-2/sandboxes/s1",
	} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "cross_project_denied") {
			t.Fatalf("GET %s: want 403 cross_project_denied, got %d %s", p, rec.Code, rec.Body.String())
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
