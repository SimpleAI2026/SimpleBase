package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/prometheus/client_golang/prometheus"
)

// TestSandboxHTTP_CrossProjectMatrix 覆盖 §8.1：项目 B 的凭据不能触达项目 A 的沙盒。
//   - B 的 Key 访问 A 的项目路径：项目中间件直接 403 cross_project_denied；
//   - 在 B 的项目路径下使用 A 的沙盒 id：所有资源路由一律 404 sandbox_not_found（不暴露存在性）。
func TestSandboxHTTP_CrossProjectMatrix(t *testing.T) {
	cfg := testConfig(true)
	cfg.DevMode = true
	cfg.S3 = config.S3Config{}
	cfg.Database.CacheDir = t.TempDir()
	cfg.Sandbox = config.SandboxConfig{Enabled: true, Backend: "fake", Image: "python:3.12-slim", CPUs: 1,
		MemoryMiB: 256, Network: "none", Workdir: "/workspace", MaxDuration: 30 * time.Minute,
		IdleTimeout: 5 * time.Minute, ExecTimeout: 30 * time.Second, ExecTimeoutMax: time.Minute,
		MaxFileBytes: 1 << 20, MaxOutputBytes: 65536, MaxPerProject: 2}
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
	defer ts.Close()

	ctx := context.Background()
	const projectA, projectB = catalog.DevProjectID, "0b0b0b0b-0000-4000-8000-00000000000b"
	if err := a.SystemStore().CatalogRepo().CreateProject(ctx, catalog.Project{ID: projectB,
		TenantID: catalog.ReservedTenantID, Name: "sandbox-other", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	const keyA, keyB = "sb_live_sandbox_matrix_a", "sb_live_sandbox_matrix_b"
	rw := []auth.Permission{auth.DatabaseRead, auth.DatabaseWrite}
	if err := auth.CreateAPIKey(ctx, a.SystemStore().DB(), "sandbox-matrix-a", projectA, a.auth.HashKey(keyA), rw, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := auth.CreateAPIKey(ctx, a.SystemStore().DB(), "sandbox-matrix-b", projectB, a.auth.HashKey(keyB), rw, time.Now()); err != nil {
		t.Fatal(err)
	}

	call := func(key, project, method, path, body string) (int, map[string]any) {
		t.Helper()
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		req, e := http.NewRequest(method, ts.URL+"/v1/projects/"+project+"/sandboxes"+path, reader)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+key)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		raw, e := io.ReadAll(resp.Body)
		if e != nil {
			t.Fatal(e)
		}
		var v map[string]any
		if len(raw) > 0 && raw[0] == '{' {
			if e = json.Unmarshal(raw, &v); e != nil {
				t.Fatalf("decode %q: %v", raw, e)
			}
		}
		return resp.StatusCode, v
	}
	errCode := func(v map[string]any) string {
		detail, _ := v["error"].(map[string]any)
		code, _ := detail["code"].(string)
		return code
	}

	code, created := call(keyA, projectA, "POST", "", `{"name":"owned-by-a"}`)
	if code != 201 {
		t.Fatalf("create in A: %d %v", code, created)
	}
	id, _ := created["id"].(string)
	file := "/files/content?path=" + url.QueryEscape("/workspace/secret.txt")
	if code, v := call(keyA, projectA, "PUT", "/"+id+file, `{"content":"only-a"}`); code != 204 {
		t.Fatalf("seed file: %d %v", code, v)
	}

	routes := []struct{ name, method, path, body string }{
		{"get", "GET", "/" + id, ""},
		{"refresh", "GET", "/" + id + "?refresh=1", ""},
		{"update", "PATCH", "/" + id, `{"name":"hijacked"}`},
		{"start", "POST", "/" + id + "/start", ""},
		{"stop", "POST", "/" + id + "/stop", ""},
		{"exec", "POST", "/" + id + "/exec", `{"command":"cat /workspace/secret.txt"}`},
		{"list files", "GET", "/" + id + "/files?path=%2Fworkspace", ""},
		{"read file", "GET", "/" + id + file, ""},
		{"write file", "PUT", "/" + id + file, `{"content":"overwritten"}`},
		{"delete file", "DELETE", "/" + id + file, ""},
		{"delete", "DELETE", "/" + id, ""},
	}
	for _, r := range routes {
		// B 的 Key 直接打 A 的项目路径：在项目中间件即被拒绝。
		if code, v := call(keyB, projectA, r.method, r.path, r.body); code != 403 || errCode(v) != "cross_project_denied" {
			t.Errorf("%s: key B on project A = %d %v, want 403 cross_project_denied", r.name, code, v)
		}
		// 在 B 的项目路径下猜 A 的沙盒 id：与不存在的 id 无法区分。
		if code, v := call(keyB, projectB, r.method, r.path, r.body); code != 404 || errCode(v) != "sandbox_not_found" {
			t.Errorf("%s: A's id under project B = %d %v, want 404 sandbox_not_found", r.name, code, v)
		}
		// 拥有租户内全部项目权限的管理 Key 也不能借 B 的路径操作 A 的资源。
		if code, v := call(systemdb.DevRawAPIKey, projectB, r.method, r.path, r.body); code != 404 || errCode(v) != "sandbox_not_found" {
			t.Errorf("%s: admin key with A's id under project B = %d %v, want 404", r.name, code, v)
		}
	}
	for _, r := range []struct{ name, method, path, body string }{
		{"list", "GET", "", ""},
		{"create", "POST", "", `{"name":"intruder"}`},
		{"run", "POST", "/run", `{"command":"echo no"}`},
		{"capabilities", "GET", "/capabilities", ""},
	} {
		if code, v := call(keyB, projectA, r.method, r.path, r.body); code != 403 || errCode(v) != "cross_project_denied" {
			t.Errorf("%s: key B on project A = %d %v, want 403 cross_project_denied", r.name, code, v)
		}
	}

	// A 的资源未被任何越权请求改动。
	if code, v := call(keyA, projectA, "GET", "/"+id, ""); code != 200 || v["name"] != "owned-by-a" || v["status"] != "running" {
		t.Fatalf("A's sandbox changed: %d %v", code, v)
	}
	if code, v := call(keyA, projectA, "GET", "/"+id+file, ""); code != 200 || v["content"] != "only-a" {
		t.Fatalf("A's file changed: %d %v", code, v)
	}
	// 列表、名称唯一性与数量上限均按项目隔离。
	if code, v := call(keyB, projectB, "GET", "", ""); code != 200 || len(v["sandboxes"].([]any)) != 0 {
		t.Fatalf("B list leaks: %d %v", code, v)
	}
	if code, v := call(keyB, projectB, "POST", "", `{"name":"owned-by-a"}`); code != 201 {
		t.Fatalf("same name in B: %d %v", code, v)
	}
	if code, v := call(keyB, projectB, "POST", "", `{"name":"b-second"}`); code != 201 {
		t.Fatalf("B second: %d %v", code, v)
	}
	if code, v := call(keyB, projectB, "POST", "", `{"name":"b-third"}`); code != 429 {
		t.Fatalf("B limit: %d %v", code, v)
	}
	if code, v := call(keyA, projectA, "POST", "", `{"name":"a-second"}`); code != 201 {
		t.Fatalf("A quota must not count B: %d %v", code, v)
	}
	if code, v := call(keyA, projectA, "GET", "", ""); code != 200 || len(v["sandboxes"].([]any)) != 2 {
		t.Fatalf("A list: %d %v", code, v)
	}
}
