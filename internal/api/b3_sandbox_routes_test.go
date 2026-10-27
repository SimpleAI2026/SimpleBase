package api

// b3_sandbox_routes_test.go：沙盒的 E2B 兼容 API 路由与鉴权。
// 旧的 /v1/projects/:projectID/sandboxes/** 已整体删除，本文件同时锁定「旧路径不存在」。
//
//   1. 路由集合恰好等于清单（控制面、envd 双挂载、ext）；
//   2. 沙盒不可用时 E2B 路径返回 503（不是 SPA 的 index.html），ext/capabilities 仍可答；
//   3. 凭据：X-API-Key 与 Bearer 都认；缺失 401，错误体是 E2B 形状；
//   4. 项目解析：头 / 单项目 Key / 多项目缺头 400 / 越权 403；
//   5. 只读 Key：读通、写 403；GET 详情里没有 envdAccessToken；
//   6. writable=false：写路由 503；
//   7. 系统项目写：403。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
)

type contextT = context.Context

// fakeRouterSandbox 实现 SandboxService。行存在内存里，足以覆盖 HTTP 层的分支。
type fakeRouterSandbox struct {
	available   bool
	interactive bool
	rows        map[string]sandbox.Sandbox
	files       map[string][]byte
	envs        map[string]map[string]string
	createdIn   []sandbox.CreateInput
	startedProc []sandbox.StartSpec
}

func newFakeRouterSandbox(avail bool) *fakeRouterSandbox {
	return &fakeRouterSandbox{available: avail, rows: map[string]sandbox.Sandbox{}, files: map[string][]byte{},
		envs: map[string]map[string]string{}}
}

func (f *fakeRouterSandbox) Available() bool   { return f.available }
func (f *fakeRouterSandbox) Interactive() bool { return f.interactive }
func (f *fakeRouterSandbox) Capabilities() sandbox.Capabilities {
	return sandbox.Capabilities{Available: f.available, Backend: "fake", Images: []string{"host"}, DefaultImage: "host",
		CPUs: 1, MemoryMiB: 256, TimeoutDefaultS: 300, TimeoutMaxS: 1800, Terminal: f.interactive}
}
func (f *fakeRouterSandbox) Create(_ contextT, projectID string, in sandbox.CreateInput, actor string) (sandbox.Sandbox, error) {
	f.createdIn = append(f.createdIn, in)
	name := in.Name
	if name == "" {
		name = "sbx-aaaa"
	}
	exp := time.Now().Add(5 * time.Minute)
	sb := sandbox.Sandbox{ID: "sb-" + name, ProjectID: projectID, Name: name, Source: in.Source, Image: "host",
		CPUs: 1, MemoryMiB: 256, Status: "running", Metadata: in.Metadata, CreatedAt: time.Now(), ExpiresAt: &exp, IdleTimeoutS: 300}
	f.rows[sb.ID] = sb
	return sb, nil
}
func (f *fakeRouterSandbox) List(_ contextT, projectID, _, _, _ string, _ int) ([]sandbox.Sandbox, string, error) {
	var out []sandbox.Sandbox
	for _, r := range f.rows {
		if r.ProjectID == projectID {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, "", nil
}
func (f *fakeRouterSandbox) Get(_ contextT, projectID, id string, _ bool) (sandbox.Sandbox, error) {
	if r, ok := f.rows[id]; ok && r.ProjectID == projectID {
		return r, nil
	}
	return sandbox.Sandbox{}, sandbox.ErrNotFound
}
func (f *fakeRouterSandbox) GetByID(_ contextT, id string) (sandbox.Sandbox, error) {
	if r, ok := f.rows[id]; ok {
		return r, nil
	}
	return sandbox.Sandbox{}, sandbox.ErrNotFound
}
func (f *fakeRouterSandbox) Start(_ contextT, projectID, id string) (sandbox.Sandbox, error) {
	r, err := f.Get(nil, projectID, id, false)
	if err != nil {
		return r, err
	}
	r.Status = "running"
	f.rows[id] = r
	return r, nil
}
func (f *fakeRouterSandbox) Stop(_ contextT, projectID, id string) (sandbox.Sandbox, error) {
	r, err := f.Get(nil, projectID, id, false)
	if err != nil {
		return r, err
	}
	r.Status = "stopped"
	f.rows[id] = r
	return r, nil
}
func (f *fakeRouterSandbox) Delete(_ contextT, projectID, id string) error {
	if _, err := f.Get(nil, projectID, id, false); err != nil {
		return err
	}
	delete(f.rows, id)
	return nil
}
func (f *fakeRouterSandbox) SetExpiry(_ contextT, projectID, id string, s int64) (sandbox.Sandbox, error) {
	r, err := f.Get(nil, projectID, id, false)
	if err != nil {
		return r, err
	}
	exp := time.Now().Add(time.Duration(s) * time.Second)
	r.ExpiresAt = &exp
	f.rows[id] = r
	return r, nil
}
func (f *fakeRouterSandbox) ExtendExpiry(ctx contextT, projectID, id string, s int64) (sandbox.Sandbox, error) {
	return f.SetExpiry(ctx, projectID, id, s)
}
func (f *fakeRouterSandbox) ReadFile(_ contextT, projectID, id, path string) (sandbox.FileContent, error) {
	if _, err := f.Get(nil, projectID, id, false); err != nil {
		return sandbox.FileContent{}, err
	}
	b, ok := f.files[id+path]
	if !ok {
		return sandbox.FileContent{}, vm.ErrFileNotFound
	}
	return sandbox.FileContent{Content: b}, nil
}
func (f *fakeRouterSandbox) WriteFile(_ contextT, projectID, id, path string, data []byte) error {
	if _, err := f.Get(nil, projectID, id, false); err != nil {
		return err
	}
	f.files[id+path] = data
	return nil
}
func (f *fakeRouterSandbox) RemoveFile(_ contextT, projectID, id, path string) error {
	if _, err := f.Get(nil, projectID, id, false); err != nil {
		return err
	}
	delete(f.files, id+path)
	return nil
}
func (f *fakeRouterSandbox) ListDir(_ contextT, projectID, id, dir string) ([]vm.FileEntry, error) {
	if _, err := f.Get(nil, projectID, id, false); err != nil {
		return nil, err
	}
	var out []vm.FileEntry
	prefix := id + strings.TrimSuffix(dir, "/") + "/"
	for k, v := range f.files {
		if strings.HasPrefix(k, prefix) && !strings.Contains(strings.TrimPrefix(k, prefix), "/") {
			name := strings.TrimPrefix(k, prefix)
			out = append(out, vm.FileEntry{Name: name, Path: strings.TrimPrefix(k, id), Kind: "file", Size: int64(len(v))})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (f *fakeRouterSandbox) StartProc(_ contextT, _, _ string, spec sandbox.StartSpec) (*sandbox.ProcHandle, error) {
	f.startedProc = append(f.startedProc, spec)
	return nil, vm.ErrNotSupported
}
func (f *fakeRouterSandbox) Procs(string) []sandbox.ProcInfo { return nil }
func (f *fakeRouterSandbox) FindProc(string, string, int, string) (*sandbox.ProcHandle, bool) {
	return nil, false
}
func (f *fakeRouterSandbox) SetEnvs(id string, env map[string]string) { f.envs[id] = env }
func (f *fakeRouterSandbox) Envs(id string) map[string]string {
	out := map[string]string{}
	for k, v := range f.envs[id] {
		out[k] = v
	}
	return out
}
func (f *fakeRouterSandbox) ClearEnvs(id string) { delete(f.envs, id) }

// e2bRouteSet 是沙盒的完整路由清单。envd 路由在根与 /e2b/envd 各挂一份。
func e2bRouteSet() map[string]bool {
	want := map[string]bool{}
	add := func(m, p string) { want[m+" "+p] = true }
	add("GET", "/e2b/ext/capabilities")
	add("POST", "/e2b/sandboxes")
	add("POST", "/e2b/v2/sandboxes")
	add("GET", "/e2b/sandboxes")
	add("GET", "/e2b/v2/sandboxes")
	add("GET", "/e2b/sandboxes/:sandboxID")
	add("DELETE", "/e2b/sandboxes/:sandboxID")
	add("POST", "/e2b/sandboxes/:sandboxID/timeout")
	add("POST", "/e2b/sandboxes/:sandboxID/pause")
	add("POST", "/e2b/sandboxes/:sandboxID/resume")
	add("POST", "/e2b/sandboxes/:sandboxID/connect")
	add("POST", "/e2b/v2/sandboxes/:sandboxID/connect")
	for _, base := range []string{"", "/e2b/envd"} {
		add("GET", base+"/health")
		add("GET", base+"/envs")
		add("GET", base+"/metrics")
		add("GET", base+"/files")
		add("POST", base+"/files")
		for _, m := range []string{"Stat", "ListDir", "MakeDir", "Move", "Remove", "WatchDir", "CreateWatcher", "GetWatcherEvents", "RemoveWatcher"} {
			add("POST", base+"/filesystem.Filesystem/"+m)
		}
		for _, m := range []string{"Start", "Connect", "List", "SendInput", "SendSignal", "CloseStdin", "Update", "StreamInput"} {
			add("POST", base+"/process.Process/"+m)
		}
	}
	return want
}

func isE2BRoute(p string) bool {
	return strings.HasPrefix(p, "/e2b") || p == "/files" || p == "/envs" || p == "/health" || p == "/metrics" ||
		strings.HasPrefix(p, "/process.Process/") || strings.HasPrefix(p, "/filesystem.Filesystem/")
}

func routerWith(t *testing.T, sb SandboxService) (*echo.Echo, string, Dependencies) {
	t.Helper()
	deps, key := fullRouterDeps(t)
	deps.Sandbox = sb
	return NewRouter(deps), key, deps
}

func call(e *echo.Echo, method, path, key string, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// 路由集合恰好等于清单（Echo 会自动补 HEAD，不计）。
func TestE2BRouteSetLocked(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	sb.interactive = true
	deps, _ := fullRouterDeps(t)
	deps.Sandbox = sb
	deps.Terminal = &TerminalBridge{}
	e := NewRouter(deps)
	want := e2bRouteSet()
	// 终端的两条 ext 路由只在终端可用时存在。
	want["POST /e2b/ext/sandboxes/:sandboxID/terminal-tickets"] = true
	want["GET /e2b/ext/sandboxes/:sandboxID/terminal"] = true
	got := map[string]bool{}
	for _, r := range e.Routes() {
		// Echo 给每个 Group 自动登记 echo_route_not_found 占位，不是业务路由。
		if !isE2BRoute(r.Path) || strings.Contains(r.Path, "/*") || r.Method == "echo_route_not_found" {
			continue
		}
		if strings.HasPrefix(r.Path, "/e2b/sandboxes/:sandboxID/") || strings.HasPrefix(r.Path, "/e2b/v2/sandboxes/:sandboxID/") {
			switch r.Path[strings.LastIndex(r.Path, "/")+1:] {
			case "fork", "snapshots", "logs", "metrics", "network", "refreshes":
				continue // 明确不支持的端点：统一 501，另测
			}
		}
		got[r.Method+" "+r.Path] = true
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing route %s", k)
		}
	}
	for k := range got {
		if !want[k] {
			t.Errorf("unexpected route %s", k)
		}
	}
}

// 旧的 /v1/projects/:projectID/sandboxes/** 一条都不能剩下。
func TestLegacySandboxRoutesRemoved(t *testing.T) {
	deps, _ := fullRouterDeps(t)
	deps.Sandbox = newFakeRouterSandbox(true)
	e := NewRouter(deps)
	for _, r := range e.Routes() {
		if strings.Contains(r.Path, "/sandboxes") && strings.HasPrefix(r.Path, "/v1/") {
			t.Errorf("legacy route still registered: %s %s", r.Method, r.Path)
		}
	}
}

func TestUnavailableSandboxAnswers503NotSPA(t *testing.T) {
	e, key, _ := routerWith(t, newFakeRouterSandbox(false))
	rec := call(e, "GET", "/e2b/sandboxes", key, "", nil)
	if rec.Code != http.StatusServiceUnavailable || strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("want 503 JSON, got %d %s", rec.Code, rec.Body.String())
	}
	var body e2bErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != 503 || body.ErrorCode != "sandbox_unavailable" {
		t.Fatalf("body = %s (%v)", rec.Body.String(), err)
	}
	// capabilities 仍可答，带 available:false。
	rec = call(e, "GET", "/e2b/ext/capabilities", key, "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"available":false`) {
		t.Fatalf("capabilities: %d %s", rec.Code, rec.Body.String())
	}
}

// ---- 凭据与错误体 ----

func TestE2BCredentialsAndErrorShape(t *testing.T) {
	e, key, _ := routerWith(t, newFakeRouterSandbox(true))
	// 无凭据：401，E2B 形状（没有 request_id 包装）。
	rec := call(e, "GET", "/e2b/sandboxes", "", "", nil)
	if rec.Code != 401 {
		t.Fatalf("no creds: %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, wrapped := body["error"]; wrapped || body["code"] != float64(401) || body["message"] == nil {
		t.Fatalf("auth failure must be E2B-shaped, got %s", rec.Body.String())
	}
	// X-API-Key 与 Bearer 都认。
	if rec = call(e, "GET", "/e2b/sandboxes", key, "", nil); rec.Code != 200 {
		t.Fatalf("X-API-Key: %d %s", rec.Code, rec.Body.String())
	}
	if rec = call(e, "GET", "/e2b/sandboxes", "", "", map[string]string{"Authorization": "Bearer " + key}); rec.Code != 200 {
		t.Fatalf("Bearer: %d %s", rec.Code, rec.Body.String())
	}
	if rec = call(e, "GET", "/e2b/sandboxes", "wrong", "", nil); rec.Code != 401 {
		t.Fatalf("bad key: %d", rec.Code)
	}
	// 未知的 /e2b 路径：E2B 形状的 404，不是 SPA。
	rec = call(e, "GET", "/e2b/nope", key, "", nil)
	if rec.Code != 404 || strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("unknown path: %d %s", rec.Code, rec.Body.String())
	}
	// 不支持的 E2B 端点：501 + unsupported_feature。
	rec = call(e, "POST", "/e2b/sandboxes/sb-x/fork", key, "{}", nil)
	if rec.Code != 501 || !strings.Contains(rec.Body.String(), "unsupported_feature") {
		t.Fatalf("fork: %d %s", rec.Code, rec.Body.String())
	}
}

func TestConnectErrorShapeOnRPCPaths(t *testing.T) {
	e, key, _ := routerWith(t, newFakeRouterSandbox(true))
	// Connect 路径的错误体是 {code: "not_found", message}，不是 E2B REST 的数字 code。
	rec := call(e, "POST", "/filesystem.Filesystem/ListDir", key, `{"path":"/workspace"}`, map[string]string{"E2b-Sandbox-Id": "sb-missing"})
	if rec.Code != 404 {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["code"] != "not_found" {
		t.Fatalf("connect error body = %s", rec.Body.String())
	}
}

// ---- 创建 / 项目解析 / 权限 ----

func TestCreateSandboxMapsE2BBody(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	rec := call(e, "POST", "/e2b/v2/sandboxes", key,
		`{"templateID":"base","timeout":120,"metadata":{"team":"a","name":"demo"},"envVars":{"FOO":"bar"}}`, nil)
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	for _, k := range []string{"sandboxID", "templateID", "clientID", "envdVersion", "envdAccessToken"} {
		if out[k] == nil || out[k] == "" {
			t.Errorf("response missing %s: %s", k, rec.Body.String())
		}
	}
	if out["domain"] != nil {
		t.Errorf("domain must be null: %v", out["domain"])
	}
	in := sb.createdIn[0]
	if in.Name != "demo" || in.TimeoutS != 120 || in.Metadata["team"] != "a" || !in.Start {
		t.Fatalf("CreateInput = %+v", in)
	}
	if _, has := in.Metadata["name"]; has {
		t.Fatal("the reserved name key must be lifted out of metadata")
	}
	if in.Source != "api" {
		t.Fatalf("API Key callers must be recorded as source=api, got %q", in.Source)
	}
	if sb.envs["sb-demo"]["FOO"] != "bar" {
		t.Fatal("envVars must be kept in memory for later process.Start")
	}
}

func TestCreateRejectsUnsupportedFields(t *testing.T) {
	e, key, _ := routerWith(t, newFakeRouterSandbox(true))
	for field, body := range map[string]string{
		"allow_internet_access": `{"templateID":"base","allow_internet_access":true}`,
		"network":               `{"templateID":"base","network":{"allowOut":["1.1.1.1"]}}`,
		"iam":                   `{"templateID":"base","iam":{"tokens":{"a":{}}}}`,
		"volumeMounts":          `{"templateID":"base","volumeMounts":[{"name":"v","path":"/v"}]}`,
		"mcp":                   `{"templateID":"base","mcp":{"x":1}}`,
		"autoPause":             `{"templateID":"base","autoPause":true}`,
		"autoResume":            `{"templateID":"base","autoResume":{"enabled":true}}`,
	} {
		rec := call(e, "POST", "/e2b/v2/sandboxes", key, body, nil)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), field) {
			t.Errorf("%s: want 400 naming the field, got %d %s", field, rec.Code, rec.Body.String())
		}
	}
	// 显式关闭 / 空值是合法的：SDK 默认会带这些。
	rec := call(e, "POST", "/e2b/v2/sandboxes", key, `{"templateID":"base","allow_internet_access":false,"autoPause":false,"autoResume":{"enabled":false},"network":{}}`, nil)
	if rec.Code != 201 {
		t.Fatalf("benign defaults rejected: %d %s", rec.Code, rec.Body.String())
	}
	if rec = call(e, "POST", "/e2b/v2/sandboxes", key, `{not json`, nil); rec.Code != 400 {
		t.Fatalf("bad json: %d", rec.Code)
	}
}

func TestProjectResolution(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	deps, key := fullRouterDeps(t)
	deps.Sandbox = sb
	// 授权两个项目的 Key。
	deps.Auth = auth.NewService(&memAuthRepo{rec: auth.APIKeyRecord{ID: "k2", KeyHash: deps.Auth.HashKey(key), TenantID: "tenant-1",
		ProjectIDs: []string{"proj-1", "proj-2"}, Permissions: []auth.Permission{auth.DatabaseRead, auth.DatabaseWrite}}}, "secret")
	e := NewRouter(deps)

	if rec := call(e, "GET", "/e2b/sandboxes", key, "", nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), "X-SimpleBase-Project") {
		t.Fatalf("multi-project without header: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(e, "GET", "/e2b/sandboxes", key, "", map[string]string{"X-SimpleBase-Project": "proj-2"}); rec.Code != 200 {
		t.Fatalf("with header: %d %s", rec.Code, rec.Body.String())
	}
	// 头指向无权限的项目：403，而不是静默用别的项目。
	if rec := call(e, "GET", "/e2b/sandboxes", key, "", map[string]string{"X-SimpleBase-Project": "proj-9"}); rec.Code != 403 {
		t.Fatalf("unauthorized project: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSingleProjectKeyNeedsNoHeader(t *testing.T) {
	e, key, _ := routerWith(t, newFakeRouterSandbox(true))
	if rec := call(e, "GET", "/e2b/sandboxes", key, "", nil); rec.Code != 200 {
		t.Fatalf("single-project key: %d %s", rec.Code, rec.Body.String())
	}
}

func TestReadOnlyKeyCannotWriteAndGetsNoEnvdToken(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	deps, key := fullRouterDeps(t)
	deps.Sandbox = sb
	deps.Auth = auth.NewService(&memAuthRepo{rec: auth.APIKeyRecord{ID: "ro", KeyHash: deps.Auth.HashKey(key), TenantID: "tenant-1",
		ProjectIDs: []string{"proj-1"}, Permissions: []auth.Permission{auth.DatabaseRead}}}, "secret")
	e := NewRouter(deps)
	row, _ := sb.Create(nil, "proj-1", sandbox.CreateInput{Name: "ro-box"}, "u")

	if rec := call(e, "GET", "/e2b/sandboxes", key, "", nil); rec.Code != 200 {
		t.Fatalf("read: %d", rec.Code)
	}
	rec := call(e, "GET", "/e2b/sandboxes/"+row.ID, key, "", nil)
	if rec.Code != 200 {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	// 越权回归：只读 Key 不能通过详情拿到可写的数据面令牌。
	if strings.Contains(rec.Body.String(), "envdAccessToken") {
		t.Fatalf("read-only key must not receive an envd access token: %s", rec.Body.String())
	}
	for _, w := range []struct{ m, p, b string }{
		{"POST", "/e2b/v2/sandboxes", `{"templateID":"base"}`},
		{"DELETE", "/e2b/sandboxes/" + row.ID, ""},
		{"POST", "/e2b/sandboxes/" + row.ID + "/timeout", `{"timeout":10}`},
		{"POST", "/e2b/sandboxes/" + row.ID + "/pause", ""},
		{"POST", "/e2b/v2/sandboxes/" + row.ID + "/connect", `{"timeout":10}`},
	} {
		if rec := call(e, w.m, w.p, key, w.b, nil); rec.Code != 403 {
			t.Errorf("%s %s with a read-only key: want 403, got %d %s", w.m, w.p, rec.Code, rec.Body.String())
		}
	}
	// envd 写也被拒。
	rec = call(e, "POST", "/filesystem.Filesystem/Remove", key, `{"path":"/workspace/a"}`, map[string]string{"E2b-Sandbox-Id": row.ID})
	if rec.Code != 403 {
		t.Fatalf("envd write with a read-only key: %d %s", rec.Code, rec.Body.String())
	}
}

func TestWriterTokenAppearsInGetOnlyForWriters(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	row, _ := sb.Create(nil, "proj-1", sandbox.CreateInput{Name: "rw-box"}, "u")
	rec := call(e, "GET", "/e2b/sandboxes/"+row.ID, key, "", nil)
	if !strings.Contains(rec.Body.String(), "envdAccessToken") {
		t.Fatalf("a writer should get a token: %s", rec.Body.String())
	}
}

func TestReadOnlyInstanceRejectsWrites(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	deps, key := fullRouterDeps(t)
	deps.Sandbox = sb
	deps.Config.Instance.Writable = false
	e := NewRouter(deps)
	row, _ := sb.Create(nil, "proj-1", sandbox.CreateInput{Name: "ro-inst"}, "u")
	for _, w := range []struct{ m, p, b string }{
		{"POST", "/e2b/v2/sandboxes", `{"templateID":"base"}`},
		{"DELETE", "/e2b/sandboxes/" + row.ID, ""},
		{"POST", "/e2b/sandboxes/" + row.ID + "/pause", ""},
		{"POST", "/e2b/sandboxes/" + row.ID + "/timeout", `{"timeout":5}`},
	} {
		if rec := call(e, w.m, w.p, key, w.b, nil); rec.Code != 503 {
			t.Errorf("%s %s on a read-only instance: want 503, got %d %s", w.m, w.p, rec.Code, rec.Body.String())
		}
	}
	// 读仍然可用。
	if rec := call(e, "GET", "/e2b/sandboxes", key, "", nil); rec.Code != 200 {
		t.Fatalf("read on read-only instance: %d", rec.Code)
	}
}

func TestSystemProjectWritesRejected(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	deps, key := fullRouterDeps(t)
	deps.Sandbox = sb
	deps.Auth = auth.NewService(&memAuthRepo{rec: auth.APIKeyRecord{ID: "adm", KeyHash: deps.Auth.HashKey(key), TenantID: "tenant-1",
		ProjectIDs: []string{catalog.ReservedSystemProjectID}, Permissions: []auth.Permission{auth.DatabaseRead, auth.DatabaseWrite, auth.ProjectAdmin}}}, "secret")
	e := NewRouter(deps)
	hdr := map[string]string{"X-SimpleBase-Project": catalog.ReservedSystemProjectID}
	rec := call(e, "POST", "/e2b/v2/sandboxes", key, `{"templateID":"base"}`, hdr)
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "system_project_protected") {
		t.Fatalf("system project create: %d %s", rec.Code, rec.Body.String())
	}
	if len(sb.createdIn) != 0 {
		t.Fatal("nothing may be created in the system project")
	}
}

// 用其它项目的 Key 访问：看不到本项目沙盒；沙盒不存在与跨项目一律 404。
func TestCrossProjectSandboxLooksMissing(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	other, _ := sb.Create(nil, "proj-2", sandbox.CreateInput{Name: "theirs"}, "u")
	rec := call(e, "GET", "/e2b/sandboxes/"+other.ID, key, "", nil)
	missing := call(e, "GET", "/e2b/sandboxes/sb-nope", key, "", nil)
	if rec.Code != 404 || missing.Code != 404 || rec.Body.String() != missing.Body.String() {
		t.Fatalf("cross-project must be indistinguishable from missing: %d %s vs %d %s", rec.Code, rec.Body.String(), missing.Code, missing.Body.String())
	}
	if rec = call(e, "DELETE", "/e2b/sandboxes/"+other.ID, key, "", nil); rec.Code != 404 {
		t.Fatalf("cross-project delete: %d", rec.Code)
	}
}

// ---- 控制面行为 ----

func TestControlPlaneLifecycle(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	rec := call(e, "POST", "/e2b/v2/sandboxes", key, `{"templateID":"base","metadata":{"k":"v"}}`, nil)
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	id := created["sandboxID"].(string)

	// 列表与 metadata 过滤。
	rec = call(e, "GET", "/e2b/v2/sandboxes?metadata=k%3Dv", key, "", nil)
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != 200 || len(list) != 1 || list[0]["state"] != "running" {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Total-Running") != "1" {
		t.Fatalf("X-Total-Running = %q", rec.Header().Get("X-Total-Running"))
	}
	rec = call(e, "GET", "/e2b/v2/sandboxes?metadata=k%3Dother", key, "", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Fatalf("metadata filter did not exclude: %s", rec.Body.String())
	}

	// timeout 204；负数 400。
	if rec = call(e, "POST", "/e2b/sandboxes/"+id+"/timeout", key, `{"timeout":60}`, nil); rec.Code != 204 {
		t.Fatalf("timeout: %d %s", rec.Code, rec.Body.String())
	}
	if rec = call(e, "POST", "/e2b/sandboxes/"+id+"/timeout", key, `{"timeout":-1}`, nil); rec.Code != 400 {
		t.Fatalf("negative timeout: %d", rec.Code)
	}
	if rec = call(e, "POST", "/e2b/sandboxes/"+id+"/timeout", key, `{}`, nil); rec.Code != 400 {
		t.Fatalf("missing timeout: %d", rec.Code)
	}

	// pause → 列表里 state=paused；再 pause 409；connect 恢复 201。
	if rec = call(e, "POST", "/e2b/sandboxes/"+id+"/pause", key, "", nil); rec.Code != 204 {
		t.Fatalf("pause: %d %s", rec.Code, rec.Body.String())
	}
	if rec = call(e, "POST", "/e2b/sandboxes/"+id+"/pause", key, "", nil); rec.Code != 409 {
		t.Fatalf("pause twice: %d", rec.Code)
	}
	rec = call(e, "GET", "/e2b/v2/sandboxes?state=paused", key, "", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0]["state"] != "paused" {
		t.Fatalf("paused list: %s", rec.Body.String())
	}
	if rec = call(e, "POST", "/e2b/v2/sandboxes/"+id+"/connect", key, `{"timeout":120}`, nil); rec.Code != 201 {
		t.Fatalf("connect (resume): %d %s", rec.Code, rec.Body.String())
	}
	if rec = call(e, "POST", "/e2b/v2/sandboxes/"+id+"/connect", key, `{"timeout":120}`, nil); rec.Code != 200 {
		t.Fatalf("connect (already running): %d", rec.Code)
	}

	// 详情字段。
	rec = call(e, "GET", "/e2b/sandboxes/"+id, key, "", nil)
	var d map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &d)
	if d["allowInternetAccess"] != false || d["lifecycle"] == nil || d["cpuCount"] == nil || d["x_simplebase"] == nil {
		t.Fatalf("detail = %s", rec.Body.String())
	}

	// 删除 204，再取 404。
	if rec = call(e, "DELETE", "/e2b/sandboxes/"+id, key, "", nil); rec.Code != 204 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec = call(e, "GET", "/e2b/sandboxes/"+id, key, "", nil); rec.Code != 404 {
		t.Fatalf("after delete: %d", rec.Code)
	}
}

func TestListValidationAndPagination(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	for _, n := range []string{"a1", "a2", "a3"} {
		call(e, "POST", "/e2b/v2/sandboxes", key, `{"metadata":{"name":"`+n+`"}}`, nil)
	}
	for q, why := range map[string]string{"limit=0": "limit", "limit=101": "limit", "limit=x": "limit", "state=weird": "state",
		"order=sideways": "order", "startedAfter=yesterday": "startedAfter", "metadata=novalue": "metadata", "nextToken=zzz": "nextToken"} {
		if rec := call(e, "GET", "/e2b/v2/sandboxes?"+q, key, "", nil); rec.Code != 400 {
			t.Errorf("%s (%s): want 400, got %d %s", q, why, rec.Code, rec.Body.String())
		}
	}
	rec := call(e, "GET", "/e2b/v2/sandboxes?limit=2&order=asc", key, "", nil)
	var page []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	next := rec.Header().Get("X-Next-Token")
	if len(page) != 2 || next == "" {
		t.Fatalf("first page: %d items, next=%q", len(page), next)
	}
	rec = call(e, "GET", "/e2b/v2/sandboxes?limit=2&order=asc&nextToken="+next, key, "", nil)
	var page2 []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &page2)
	if len(page2) != 1 || rec.Header().Get("X-Next-Token") != "" {
		t.Fatalf("last page: %d items, next=%q", len(page2), rec.Header().Get("X-Next-Token"))
	}
	if page[0]["sandboxID"] == page2[0]["sandboxID"] || page[1]["sandboxID"] == page2[0]["sandboxID"] {
		t.Fatal("pages overlap")
	}
}

// ---- envd 令牌 ----

func TestEnvdAccessTokenBinding(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, _, deps := routerWith(t, sb)
	a, _ := sb.Create(nil, "proj-1", sandbox.CreateInput{Name: "boxa"}, "u")
	b, _ := sb.Create(nil, "proj-1", sandbox.CreateInput{Name: "boxb"}, "u")
	signer := newEnvdTokenSigner(deps.Config.Auth.APIKeyHashSecret)
	tokA := signer.Issue("proj-1", a.ID, time.Now().Add(time.Minute))

	// 令牌 + 匹配的沙盒 ID：通过。
	rec := call(e, "GET", "/health", "", "", map[string]string{"X-Access-Token": tokA, "E2b-Sandbox-Id": a.ID})
	if rec.Code != 204 {
		t.Fatalf("matching token: %d %s", rec.Code, rec.Body.String())
	}
	// 令牌属于 a，请求指向 b：当作凭据无效。
	rec = call(e, "GET", "/health", "", "", map[string]string{"X-Access-Token": tokA, "E2b-Sandbox-Id": b.ID})
	if rec.Code != 401 {
		t.Fatalf("token for another sandbox: %d %s", rec.Code, rec.Body.String())
	}
	// 篡改 / 过期 / 垃圾。
	expired := signer.Issue("proj-1", a.ID, time.Now().Add(-time.Minute))
	for name, tok := range map[string]string{"expired": expired, "garbage": "abc.def", "tampered": tokA + "x", "empty-sig": strings.Split(tokA, ".")[0] + "."} {
		rec = call(e, "GET", "/health", "", "", map[string]string{"X-Access-Token": tok, "E2b-Sandbox-Id": a.ID})
		if rec.Code != 401 {
			t.Errorf("%s token accepted: %d", name, rec.Code)
		}
	}
	// 别的密钥签的令牌无效。
	foreign := newEnvdTokenSigner("another-secret").Issue("proj-1", a.ID, time.Now().Add(time.Minute))
	if rec = call(e, "GET", "/health", "", "", map[string]string{"X-Access-Token": foreign, "E2b-Sandbox-Id": a.ID}); rec.Code != 401 {
		t.Fatalf("foreign token: %d", rec.Code)
	}
	// 缺 E2b-Sandbox-Id。
	if rec = call(e, "GET", "/health", "", "", map[string]string{"X-Access-Token": tokA}); rec.Code != 400 {
		t.Fatalf("missing sandbox id: %d", rec.Code)
	}
	// 令牌只授权数据面：不能调控制面。
	if rec = call(e, "GET", "/e2b/sandboxes", "", "", map[string]string{"X-Access-Token": tokA}); rec.Code != 401 {
		t.Fatalf("envd token on the control plane: %d", rec.Code)
	}
	// 令牌也不能用于另一个项目的沙盒：项目取自令牌，而不是请求头。
	other, _ := sb.Create(nil, "proj-2", sandbox.CreateInput{Name: "other"}, "u")
	if rec = call(e, "GET", "/health", "", "", map[string]string{"X-Access-Token": tokA, "E2b-Sandbox-Id": other.ID}); rec.Code != 401 {
		t.Fatalf("token reused on another project's sandbox: %d", rec.Code)
	}
}

func TestEnvdTokenSignerDomainSeparation(t *testing.T) {
	s := newEnvdTokenSigner("secret")
	tok := s.Issue("p", "s", time.Now().Add(time.Minute))
	c, err := s.Verify(tok, time.Now())
	if err != nil || c.Project != "p" || c.Sandbox != "s" {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := s.Verify(tok, time.Now().Add(2*time.Minute)); err == nil {
		t.Fatal("expired token accepted")
	}
	if _, err := s.Verify("", time.Now()); err == nil {
		t.Fatal("empty token accepted")
	}
}

// keys 返回 map 的排序键，供其它路由测试复用。
func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
