package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

// e2bApp 起一个真实装配（认证、路由、系统库 SQLite + 迁移 v50、fake 驱动）的服务，
// 创建两个项目与对应的 Key。不访问任何外部服务。
type e2bApp struct {
	ts        *httptest.Server
	a         *App
	keyA, rok string // 项目 A 的读写 Key / 只读 Key
	keyB      string // 项目 B 的 Key
}

func newE2BApp(t *testing.T, max int) *e2bApp {
	t.Helper()
	cfg := testConfig(true)
	cfg.Storage = config.StorageConfig{Database: config.StorageModeLocal, Files: config.StorageModeLocal, DataDir: t.TempDir()}
	cfg.S3 = config.S3Config{}
	cfg.Database.CacheDir = t.TempDir()
	cfg.Sandbox = config.SandboxConfig{Enabled: true, Backend: "fake", Image: "python:3.12-slim", CPUs: 1,
		MemoryMiB: 256, Network: "none", Workdir: "/workspace", MaxDuration: 30 * time.Minute,
		IdleTimeout: 5 * time.Minute, ExecTimeout: 30 * time.Second, ExecTimeoutMax: time.Minute,
		MaxFileBytes: 1 << 20, MaxOutputBytes: 65536, MaxPerProject: max,
		Terminal: config.SandboxTerminalConfig{Enabled: true, MaxSessionsPerSandbox: 2, IdleTimeout: time.Minute}}
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	})
	ts := httptest.NewServer(a.Handler())
	t.Cleanup(ts.Close)

	ctx := context.Background()
	for _, p := range []catalog.Project{
		{ID: "sandprj1", TenantID: catalog.ReservedTenantID, Name: "sandbox-a", CreatedAt: time.Now().UTC()},
		{ID: "sandprj2", TenantID: catalog.ReservedTenantID, Name: "sandbox-b", CreatedAt: time.Now().UTC()},
	} {
		if err := a.SystemStore().CatalogRepo().CreateProject(ctx, p); err != nil && !errors.Is(err, catalog.ErrAlreadyExists) {
			t.Fatal(err)
		}
	}
	mk := func(name, project, key string, perms ...auth.Permission) string {
		if err := auth.CreateAPIKey(ctx, a.SystemStore().DB(), name, project, a.auth.HashKey(key), perms, time.Now()); err != nil {
			t.Fatal(err)
		}
		return key
	}
	return &e2bApp{ts: ts, a: a,
		keyA: mk("e2b-a", "sandprj1", "sb_live_e2b_a", auth.DatabaseRead, auth.DatabaseWrite),
		rok:  mk("e2b-ro", "sandprj1", "sb_live_e2b_ro", auth.DatabaseRead),
		keyB: mk("e2b-b", "sandprj2", "sb_live_e2b_b", auth.DatabaseRead, auth.DatabaseWrite)}
}

func (v *e2bApp) do(t *testing.T, key, method, path, body string, hdr map[string]string) (int, http.Header, []byte) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, v.ts.URL+path, r)
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, val := range hdr {
		req.Header.Set(k, val)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, b
}

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if len(bytes.TrimSpace(b)) > 0 && b[0] == '{' {
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("decode %q: %v", b, err)
		}
	}
	return m
}

// TestSandboxE2B_E2E 走完整的 E2B 生命周期：创建 → 文件 → 命令 → 暂停 / 恢复 → 删除。
func TestSandboxE2B_E2E(t *testing.T) {
	v := newE2BApp(t, 2)

	// 能力：fake 后端不是本机进程隔离，终端能力由驱动是否实现 Interactive 决定（fake 实现了）。
	// local/fake 都实现 Interactive，所以真实装配下 terminal 必须为 true：日志装饰器若吃掉该能力，
	// 这里会失败（曾出现过，见 vm/logged_test.go）。
	code, _, b := v.do(t, v.keyA, "GET", "/e2b/ext/capabilities", "", nil)
	caps := decode(t, b)
	if code != 200 || caps["available"] != true || caps["backend"] != "fake" {
		t.Fatalf("capabilities: %d %s", code, b)
	}
	if caps["terminal"] != true {
		t.Fatalf("terminal must be available with an Interactive backend, got %v (backend=%v)", caps["terminal"], caps["backend"])
	}

	// 创建：201，创建即运行；返回 envd 令牌。
	code, _, b = v.do(t, v.keyA, "POST", "/e2b/v2/sandboxes", `{"templateID":"base","timeout":600,"metadata":{"name":"e2e","team":"a"}}`, nil)
	created := decode(t, b)
	id, _ := created["sandboxID"].(string)
	tok, _ := created["envdAccessToken"].(string)
	if code != 201 || id == "" || tok == "" {
		t.Fatalf("create: %d %s", code, b)
	}
	ext, _ := created["x_simplebase"].(map[string]any)
	if ext["status"] != "running" || ext["source"] != "api" {
		t.Fatalf("x_simplebase = %v", ext)
	}

	// 用 envd 令牌（SDK 的方式）写文件、读回、列目录、执行命令。
	envd := map[string]string{"X-Access-Token": tok, "E2b-Sandbox-Id": id}
	if code, _, b = v.do(t, "", "POST", "/files?path=hello.txt", "hello", map[string]string{
		"X-Access-Token": tok, "E2b-Sandbox-Id": id, "Content-Type": "application/octet-stream"}); code != 200 {
		t.Fatalf("upload: %d %s", code, b)
	}
	if code, _, b = v.do(t, "", "GET", "/files?path=hello.txt", "", envd); code != 200 || string(b) != "hello" {
		t.Fatalf("download: %d %q", code, b)
	}
	code, _, b = v.do(t, "", "POST", "/filesystem.Filesystem/ListDir", `{"path":"/workspace"}`, envd)
	if code != 200 || !strings.Contains(string(b), `"name":"hello.txt"`) {
		t.Fatalf("ListDir: %d %s", code, b)
	}
	// process.Start 走流式（Connect envelope）。真实装配把驱动包了日志装饰器，
	// 装饰器必须透传 vm.Interactive，否则终端与流式执行会静默降级（vm/logged_test.go 有回归）。
	code, _, b = v.do(t, "", "POST", "/process.Process/Start", `{"process":{"cmd":"cat","args":["hello.txt"]}}`, envd)
	if code != 200 || !strings.Contains(string(b), "aGVsbG8") { // base64("hello")
		t.Fatalf("process.Start through the real assembly: %d %s", code, b)
	}

	// 列表带 metadata 过滤，名称 / 来源作为保留键出现。
	code, hdr, b := v.do(t, v.keyA, "GET", "/e2b/v2/sandboxes?metadata=team%3Da", "", nil)
	var list []map[string]any
	_ = json.Unmarshal(b, &list)
	if code != 200 || len(list) != 1 || hdr.Get("X-Total-Running") != "1" {
		t.Fatalf("list: %d %s", code, b)
	}
	md, _ := list[0]["metadata"].(map[string]any)
	if md["name"] != "e2e" || md["simplebase.source"] != "api" || md["team"] != "a" {
		t.Fatalf("metadata round-trip through the real system DB: %v", md)
	}

	// 暂停：进程丢失、文件保留；connect 恢复后文件还在。
	if code, _, b = v.do(t, v.keyA, "POST", "/e2b/sandboxes/"+id+"/pause", "", nil); code != 204 {
		t.Fatalf("pause: %d %s", code, b)
	}
	if code, _, _ = v.do(t, "", "GET", "/health", "", envd); code != 502 {
		t.Fatalf("a paused sandbox must report 502, got %d", code)
	}
	if code, _, b = v.do(t, v.keyA, "POST", "/e2b/v2/sandboxes/"+id+"/connect", `{"timeout":300}`, nil); code != 201 {
		t.Fatalf("resume: %d %s", code, b)
	}
	if code, _, b = v.do(t, "", "GET", "/files?path=hello.txt", "", envd); code != 200 || string(b) != "hello" {
		t.Fatalf("files must survive pause/resume: %d %q", code, b)
	}

	// 配额：上限 2。
	if code, _, b = v.do(t, v.keyA, "POST", "/e2b/v2/sandboxes", `{"templateID":"base"}`, nil); code != 201 {
		t.Fatalf("second: %d %s", code, b)
	}
	code, _, b = v.do(t, v.keyA, "POST", "/e2b/v2/sandboxes", `{"templateID":"base"}`, nil)
	if code != 429 || decode(t, b)["error_code"] != "sandbox_limit_exceeded" {
		t.Fatalf("limit: %d %s", code, b)
	}

	// 删除。
	if code, _, _ = v.do(t, v.keyA, "DELETE", "/e2b/sandboxes/"+id, "", nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _, _ = v.do(t, v.keyA, "GET", "/e2b/sandboxes/"+id, "", nil); code != 404 {
		t.Fatalf("after delete: %d", code)
	}
	// 删除后令牌也失效（按 ID 反查不到）。
	if code, _, _ = v.do(t, "", "GET", "/health", "", envd); code != 404 {
		t.Fatalf("token for a deleted sandbox: %d", code)
	}
}

func TestSandboxE2B_ReadOnlyAndLegacyGone(t *testing.T) {
	v := newE2BApp(t, 3)
	code, _, b := v.do(t, v.keyA, "POST", "/e2b/v2/sandboxes", `{"templateID":"base"}`, nil)
	id, _ := decode(t, b)["sandboxID"].(string)
	if code != 201 || id == "" {
		t.Fatalf("create: %d %s", code, b)
	}
	// 只读 Key：读通、写 403，详情里没有 envd 令牌。
	if code, _, _ = v.do(t, v.rok, "GET", "/e2b/sandboxes", "", nil); code != 200 {
		t.Fatalf("readonly list: %d", code)
	}
	code, _, b = v.do(t, v.rok, "GET", "/e2b/sandboxes/"+id, "", nil)
	if code != 200 || strings.Contains(string(b), "envdAccessToken") {
		t.Fatalf("readonly detail must not carry a token: %d %s", code, b)
	}
	for _, w := range [][3]string{{"POST", "/e2b/v2/sandboxes", `{"templateID":"base"}`}, {"DELETE", "/e2b/sandboxes/" + id, ""},
		{"POST", "/e2b/sandboxes/" + id + "/pause", ""}} {
		if code, _, b = v.do(t, v.rok, w[0], w[1], w[2], nil); code != 403 {
			t.Errorf("readonly %s %s: %d %s", w[0], w[1], code, b)
		}
	}
	// 旧路径已整体删除：不再有 JSON 接口，落进 SPA fallback（GET）或 404/405（写）。
	for _, p := range []string{"/v1/projects/sandprj1/sandboxes", "/v1/projects/sandprj1/sandboxes/capabilities", "/v1/projects/sandprj1/sandboxes/" + id} {
		code, _, b = v.do(t, v.keyA, "GET", p, "", nil)
		if code == 200 && strings.Contains(string(b), `"sandboxes"`) {
			t.Errorf("legacy API still answers on %s", p)
		}
	}
	if code, _, _ = v.do(t, v.keyA, "POST", "/v1/projects/sandprj1/sandboxes/run", `{"command":"echo"}`, nil); code == 200 {
		t.Error("legacy /run still answers")
	}
}

func TestSandboxE2B_CrossProject(t *testing.T) {
	v := newE2BApp(t, 3)
	code, _, b := v.do(t, v.keyA, "POST", "/e2b/v2/sandboxes", `{"templateID":"base"}`, nil)
	id, _ := decode(t, b)["sandboxID"].(string)
	if code != 201 {
		t.Fatalf("create: %d %s", code, b)
	}
	// 项目 B 的 Key 看不到 A 的沙盒：所有路由一律 404，与不存在无法区分。
	_, _, missing := v.do(t, v.keyB, "GET", "/e2b/sandboxes/sb-no-such", "", nil)
	for _, c := range [][3]string{{"GET", "/e2b/sandboxes/" + id, ""}, {"DELETE", "/e2b/sandboxes/" + id, ""},
		{"POST", "/e2b/sandboxes/" + id + "/pause", ""}, {"POST", "/e2b/sandboxes/" + id + "/timeout", `{"timeout":5}`},
		{"POST", "/e2b/v2/sandboxes/" + id + "/connect", `{}`}} {
		code, _, got := v.do(t, v.keyB, c[0], c[1], c[2], nil)
		if code != 404 || string(got) != string(missing) {
			t.Errorf("%s %s from project B: want an identical 404, got %d %s", c[0], c[1], code, got)
		}
	}
	// envd：B 的 Key + A 的沙盒 ID。
	if code, _, _ = v.do(t, v.keyB, "GET", "/files?path=x", "", map[string]string{"E2b-Sandbox-Id": id}); code != 404 {
		t.Errorf("envd cross-project: %d", code)
	}
	// B 的列表里没有 A 的沙盒。
	_, _, b = v.do(t, v.keyB, "GET", "/e2b/v2/sandboxes", "", nil)
	if strings.Contains(string(b), id) {
		t.Fatalf("project B can see project A's sandbox: %s", b)
	}
	// 越权访问项目：头指向别人的项目 → 403。
	if code, _, _ = v.do(t, v.keyB, "GET", "/e2b/sandboxes", "", map[string]string{"X-SimpleBase-Project": "sandprj1"}); code != 403 {
		t.Errorf("X-SimpleBase-Project pointing at another project: %d", code)
	}
}
