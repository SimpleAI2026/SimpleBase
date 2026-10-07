package app

import (
	"bytes"
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
	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
	"github.com/prometheus/client_golang/prometheus"
)

// TestSandboxHTTP_E2E 使用真实装配、认证、路由、系统库及 fake VM；不访问 Cloud。
func TestSandboxHTTP_E2E(t *testing.T) {
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
	base := ts.URL + "/v1/projects/" + catalog.DevProjectID + "/sandboxes"
	call := func(method, path, body, key string) (int, map[string]any) {
		t.Helper()
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		req, e := http.NewRequest(method, base+path, reader)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+systemdb.DevRawAPIKey)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		b, e := io.ReadAll(resp.Body)
		if e != nil {
			t.Fatal(e)
		}
		var v map[string]any
		if len(bytes.TrimSpace(b)) > 0 && b[0] == '{' {
			if e = json.Unmarshal(b, &v); e != nil {
				t.Fatalf("decode %q: %v", b, e)
			}
		}
		return resp.StatusCode, v
	}
	if code, c := call("GET", "/capabilities", "", ""); code != 200 || c["backend"] != "fake" {
		t.Fatalf("capabilities: %d %v", code, c)
	}
	code, r := call("POST", "", `{"name":"e2e"}`, "idem-1")
	if code != 201 || r["status"] != "pending" {
		t.Fatalf("create: %d %v", code, r)
	}
	id, _ := r["id"].(string)
	if id == "" {
		t.Fatal("missing id")
	}
	if code, r2 := call("POST", "", `{"name":"e2e"}`, "idem-1"); code != 200 || r2["id"] != id {
		t.Fatalf("idempotency: %d %v", code, r2)
	}
	if code, r2 := call("GET", "/"+id, "", ""); code != 200 || r2["status"] != "pending" {
		t.Fatalf("lazy: %d %v", code, r2)
	}
	readonlyKey := "sb_live_readonly_test_key"
	if err := auth.CreateAPIKey(context.Background(), a.SystemStore().DB(), "sandbox-readonly", catalog.DevProjectID,
		a.auth.HashKey(readonlyKey), []auth.Permission{auth.DatabaseRead}, time.Now()); err != nil {
		t.Fatal(err)
	}
	readonlyRequest := func(method, path string) int {
		req, err := http.NewRequest(method, base+path, strings.NewReader(`{"command":"echo no"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+readonlyKey)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if code := readonlyRequest("GET", "/"+id); code != 200 {
		t.Fatalf("readonly GET: %d", code)
	}
	if code := readonlyRequest("POST", "/"+id+"/exec"); code != 403 {
		t.Fatalf("readonly exec: %d", code)
	}
	if code := readonlyRequest("POST", "/run"); code != 403 {
		t.Fatalf("readonly run: %d", code)
	}
	filePath := "/" + id + "/files/content?path=" + url.QueryEscape("/workspace/a.txt")
	if code, r2 := call("PUT", filePath, `{"content":"hello"}`, ""); code != 204 {
		t.Fatalf("put: %d %v", code, r2)
	}
	if code, r2 := call("GET", filePath, "", ""); code != 200 || r2["content"] != "hello" {
		t.Fatalf("read: %d %v", code, r2)
	}
	if code, r2 := call("GET", "/"+id+"/files?path=%2Fworkspace", "", ""); code != 200 || len(r2["entries"].([]any)) != 1 {
		t.Fatalf("list files: %d %v", code, r2)
	}
	if code, r2 := call("POST", "/"+id+"/exec", `{"command":"cat a.txt"}`, ""); code != 200 || r2["stdout"] != "hello" {
		t.Fatalf("exec: %d %v", code, r2)
	}
	if code, r2 := call("POST", "/"+id+"/exec", `{"command":"exit 3"}`, ""); code != 200 || r2["exit_code"] != float64(3) {
		t.Fatalf("exit: %d %v", code, r2)
	}
	if code, r2 := call("POST", "/"+id+"/exec", `{"command":"sleep 2","timeout_s":1}`, ""); code != 200 || r2["timed_out"] != true {
		t.Fatalf("timeout: %d %v", code, r2)
	}
	if code, r2 := call("GET", "/"+id+"/files/content?path=%2Fetc%2Fpasswd", "", ""); code != 400 {
		t.Fatalf("path: %d %v", code, r2)
	}
	if code, r2 := call("POST", "", `{"name":"second"}`, ""); code != 201 {
		t.Fatalf("second: %d %v", code, r2)
	}
	if code, r2 := call("POST", "", `{"name":"third"}`, ""); code != 429 {
		t.Fatalf("limit: %d %v", code, r2)
	}
	code, page1 := call("GET", "?limit=1", "", "")
	cursor, _ := page1["next_cursor"].(string)
	if code != 200 || len(page1["sandboxes"].([]any)) != 1 || cursor == "" {
		t.Fatalf("page1: %d %v", code, page1)
	}
	code, page2 := call("GET", "?limit=1&cursor="+url.QueryEscape(cursor), "", "")
	if code != 200 || len(page2["sandboxes"].([]any)) != 1 || page2["next_cursor"] != nil {
		t.Fatalf("page2: %d %v", code, page2)
	}
	if a, b := page1["sandboxes"].([]any)[0].(map[string]any)["id"], page2["sandboxes"].([]any)[0].(map[string]any)["id"]; a == b {
		t.Fatalf("pages overlap: %v", a)
	}
	if code, r2 := call("GET", "?cursor=unknown", "", ""); code != 400 {
		t.Fatalf("bad cursor: %d %v", code, r2)
	}
	if code, r2 := call("GET", "?limit=abc", "", ""); code != 400 {
		t.Fatalf("bad limit: %d %v", code, r2)
	}
	if code, r2 := call("DELETE", "/"+id, "", ""); code != 204 {
		t.Fatalf("delete: %d %v", code, r2)
	}
	if code, r2 := call("GET", "/"+id, "", ""); code != 404 {
		t.Fatalf("gone: %d %v", code, r2)
	}
	if code, r2 := call("POST", "/run", `{"command":"echo done"}`, ""); code != 200 || r2["stdout"] != "done\n" {
		t.Fatalf("run: %d %v", code, r2)
	}
	if code, r2 := call("GET", "", "", ""); code != 200 || len(r2["sandboxes"].([]any)) != 1 {
		t.Fatalf("run cleanup: %d %v", code, r2)
	}
	client, err := gosdk.NewClient(gosdk.Options{URL: ts.URL, APIKey: systemdb.DevRawAPIKey, ProjectID: catalog.DevProjectID})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.RunSandbox(context.Background(), gosdk.SandboxRunInput{
		SandboxExecInput: gosdk.SandboxExecInput{Command: "cat /workspace/ci.txt"},
		Files:            []gosdk.SandboxRunFile{{Path: "/workspace/ci.txt", Content: "sdk-e2e"}},
	})
	if err != nil || result.Stdout != "sdk-e2e" {
		t.Fatalf("SDK run: %+v, %v", result, err)
	}
	if code, r2 := call("GET", "", "", ""); code != 200 || len(r2["sandboxes"].([]any)) != 1 {
		t.Fatalf("SDK run cleanup: %d %v", code, r2)
	}
	if code, r2 := call("POST", "/run", `{"command":"echo keep","keep":true}`, ""); code != 200 || r2["sandbox_id"] == "" {
		t.Fatalf("keep run: %d %v", code, r2)
	} else if keptID, ok := r2["sandbox_id"].(string); ok {
		if code, r3 := call("GET", "/"+keptID, "", ""); code != 200 || r3["source"] != "api" {
			t.Fatalf("keep resource: %d %v", code, r3)
		}
		if code, r3 := call("DELETE", "/"+keptID, "", ""); code != 204 {
			t.Fatalf("keep cleanup: %d %v", code, r3)
		}
	}
}
