package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
)

func TestMapE2BError(t *testing.T) {
	for _, tc := range []struct {
		err       error
		status    int
		errorCode string
		connect   string
	}{
		{sandbox.ErrNotFound, 404, "sandbox_not_found", "not_found"},
		{vm.ErrFileNotFound, 404, "file_not_found", "not_found"},
		{sandbox.ErrNameConflict, 409, "sandbox_name_conflict", "already_exists"},
		{sandbox.ErrLimitExceeded, 429, "sandbox_limit_exceeded", "resource_exhausted"},
		{sandbox.ErrInvalidSpec, 400, "invalid_request", "invalid_argument"},
		{sandbox.ErrInvalidPath, 400, "invalid_path", "invalid_argument"},
		{sandbox.ErrFileTooLarge, 413, "file_too_large", "resource_exhausted"},
		{sandbox.ErrBusy, 409, "sandbox_busy", "failed_precondition"},
		{sandbox.ErrGone, 410, "sandbox_gone", "not_found"},
		{vm.ErrNotSupported, 501, "unsupported_feature", "unimplemented"},
		{vm.ErrUnavailable, 503, "sandbox_unavailable", "unavailable"},
		{vm.ErrBackend, 502, "sandbox_backend_error", "internal"},
	} {
		got := mapE2BError(tc.err)
		if got.status != tc.status || got.errorCode != tc.errorCode || got.connect != tc.connect {
			t.Errorf("%v -> %+v, want %d/%s/%s", tc.err, got, tc.status, tc.errorCode, tc.connect)
		}
		// 包装过的错误同样命中，且文案是固定的，不含内部细节。
		wrapped := mapE2BError(errors.Join(errors.New("leak: /data/secret/path"), tc.err))
		if wrapped.status != tc.status || strings.Contains(wrapped.message, "secret") {
			t.Errorf("wrapped %v leaked or mismatched: %+v", tc.err, wrapped)
		}
	}
	if mapE2BError(errors.New("boom")).status != 0 {
		t.Fatal("unknown errors must not be mapped here")
	}
}

func TestE2BPathClassification(t *testing.T) {
	for p, want := range map[string]bool{
		"/e2b/sandboxes": true, "/e2b": true, "/e2b/envd/files": true, "/files": true, "/envs": true,
		"/process.Process/Start": true, "/filesystem.Filesystem/ListDir": true,
		"/v1/projects/p/databases": false, "/health/live": false, "/sandboxes": false, "/": false,
	} {
		if isE2BPath(p) != want {
			t.Errorf("isE2BPath(%q) = %v, want %v", p, !want, want)
		}
	}
	if !isConnectPath("/e2b/envd/process.Process/Start") || !isConnectPath("/filesystem.Filesystem/Stat") || isConnectPath("/files") {
		t.Fatal("connect path classification is wrong")
	}
}

func TestE2BPathResolution(t *testing.T) {
	for in, want := range map[string]string{"": "/workspace", "a.txt": "/workspace/a.txt", "/workspace/x": "/workspace/x",
		"../etc": "/etc", "sub/../b": "/workspace/b", "  c  ": "/workspace/c"} {
		if got := e2bPath("/workspace", in); got != want {
			t.Errorf("e2bPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---- envd: /files ----

func upload(e interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, key, id, target, content string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "ignored.txt")
	io.WriteString(fw, content)
	mw.Close()
	req := httptest.NewRequest("POST", "/files?path="+target, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-API-Key", key)
	req.Header.Set("E2b-Sandbox-Id", id)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestEnvdFilesRoundTrip(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	row, _ := sb.Create(nil, "proj-1", sandbox.CreateInput{Name: "fbox"}, "u")
	hdr := map[string]string{"E2b-Sandbox-Id": row.ID}

	rec := upload(e, key, row.ID, "notes/a.txt", "hello")
	if rec.Code != 200 {
		t.Fatalf("multipart upload: %d %s", rec.Code, rec.Body.String())
	}
	var entries []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &entries)
	if len(entries) != 1 || entries[0]["path"] != "/workspace/notes/a.txt" || entries[0]["size"] != "5" || entries[0]["type"] != "FILE_TYPE_FILE" {
		t.Fatalf("upload entry = %s", rec.Body.String())
	}
	rec = call(e, "GET", "/files?path=notes/a.txt", key, "", hdr)
	if rec.Code != 200 || rec.Body.String() != "hello" || rec.Header().Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("download: %d %q %s", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	// 纯字节上传需要 path。
	req := httptest.NewRequest("POST", "/files?path=/workspace/raw.bin", strings.NewReader("RAW"))
	req.Header.Set("X-API-Key", key)
	req.Header.Set("E2b-Sandbox-Id", row.ID)
	req.Header.Set("Content-Type", "application/octet-stream")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 200 || string(sb.files[row.ID+"/workspace/raw.bin"]) != "RAW" {
		t.Fatalf("octet-stream upload: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest("POST", "/files", strings.NewReader("x"))
	req.Header.Set("X-API-Key", key)
	req.Header.Set("E2b-Sandbox-Id", row.ID)
	req.Header.Set("Content-Type", "application/octet-stream")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("octet-stream without path: %d", rec.Code)
	}
	// 缺失文件 404；缺沙盒头 400；控制台副本路径同样可用。
	if rec = call(e, "GET", "/files?path=nope", key, "", hdr); rec.Code != 404 {
		t.Fatalf("missing file: %d", rec.Code)
	}
	if rec = call(e, "GET", "/files?path=notes/a.txt", key, "", nil); rec.Code != 400 {
		t.Fatalf("missing sandbox header: %d", rec.Code)
	}
	if rec = call(e, "GET", "/e2b/envd/files?path=notes/a.txt", key, "", hdr); rec.Code != 200 || rec.Body.String() != "hello" {
		t.Fatalf("console copy of /files: %d %s", rec.Code, rec.Body.String())
	}
}

func TestEnvdFilesEnforceSizeLimit(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	deps, key := fullRouterDeps(t)
	deps.Sandbox = sb
	deps.Config.Sandbox.MaxFileBytes = 8
	e := NewRouter(deps)
	row, _ := sb.Create(nil, "proj-1", sandbox.CreateInput{Name: "big"}, "u")
	if rec := upload(e, key, row.ID, "ok.txt", "12345678"); rec.Code != 200 {
		t.Fatalf("exactly at the limit: %d %s", rec.Code, rec.Body.String())
	}
	rec := upload(e, key, row.ID, "big.txt", "123456789")
	if rec.Code != 413 || !strings.Contains(rec.Body.String(), "file_too_large") {
		t.Fatalf("over the limit: %d %s", rec.Code, rec.Body.String())
	}
	if _, stored := sb.files[row.ID+"/workspace/big.txt"]; stored {
		t.Fatal("an oversized upload must not be stored")
	}
}

// 读到被截断的文件时返回 413，而不是静默给出残缺内容。
type truncatingSandbox struct{ *fakeRouterSandbox }

func (truncatingSandbox) ReadFile(contextT, string, string, string) (sandbox.FileContent, error) {
	return sandbox.FileContent{Content: []byte("partial"), Truncated: true}, nil
}

func TestEnvdDownloadOfTruncatedFileIs413(t *testing.T) {
	base := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, truncatingSandbox{base})
	row, _ := base.Create(nil, "proj-1", sandbox.CreateInput{Name: "trunc"}, "u")
	rec := call(e, "GET", "/files?path=x", key, "", map[string]string{"E2b-Sandbox-Id": row.ID})
	if rec.Code != 413 || strings.Contains(rec.Body.String(), "partial") {
		t.Fatalf("truncated file: %d %s", rec.Code, rec.Body.String())
	}
}

// ---- envd: filesystem ----

func rpc(e interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, key, id, method, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/filesystem.Filesystem/"+method, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("X-API-Key", key)
	req.Header.Set("E2b-Sandbox-Id", id)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestFilesystemRPC(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	row, _ := sb.Create(nil, "proj-1", sandbox.CreateInput{Name: "fs1"}, "u")
	sb.files[row.ID+"/workspace/a.txt"] = []byte("AAA")
	sb.files[row.ID+"/workspace/b.txt"] = []byte("B")

	rec := rpc(e, key, row.ID, "ListDir", `{"path":"/workspace"}`)
	var list struct {
		Entries []map[string]any `json:"entries"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != 200 || len(list.Entries) != 2 || list.Entries[0]["name"] != "a.txt" || list.Entries[0]["size"] != "3" {
		t.Fatalf("ListDir: %d %s", rec.Code, rec.Body.String())
	}
	if list.Entries[0]["permissions"] == "" || list.Entries[0]["mode"] == nil {
		t.Fatalf("entries must carry mode and permissions: %s", rec.Body.String())
	}
	// 空体也合法（路径取工作目录）。
	if rec = rpc(e, key, row.ID, "ListDir", ``); rec.Code != 200 {
		t.Fatalf("empty body: %d %s", rec.Code, rec.Body.String())
	}
	rec = rpc(e, key, row.ID, "Stat", `{"path":"a.txt"}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"a.txt"`) {
		t.Fatalf("Stat file: %d %s", rec.Code, rec.Body.String())
	}
	if rec = rpc(e, key, row.ID, "Stat", `{"path":"/workspace"}`); rec.Code != 200 || !strings.Contains(rec.Body.String(), "FILE_TYPE_DIRECTORY") {
		t.Fatalf("Stat workspace: %d %s", rec.Code, rec.Body.String())
	}
	if rec = rpc(e, key, row.ID, "Stat", `{"path":"ghost.txt"}`); rec.Code != 404 || !strings.Contains(rec.Body.String(), `"code":"not_found"`) {
		t.Fatalf("Stat missing: %d %s", rec.Code, rec.Body.String())
	}
	// Move = 读 + 写 + 删。
	rec = rpc(e, key, row.ID, "Move", `{"source":"a.txt","destination":"moved/c.txt"}`)
	if rec.Code != 200 || string(sb.files[row.ID+"/workspace/moved/c.txt"]) != "AAA" {
		t.Fatalf("Move: %d %s", rec.Code, rec.Body.String())
	}
	if _, still := sb.files[row.ID+"/workspace/a.txt"]; still {
		t.Fatal("Move must remove the source")
	}
	if rec = rpc(e, key, row.ID, "Remove", `{"path":"b.txt"}`); rec.Code != 200 {
		t.Fatalf("Remove: %d", rec.Code)
	}
	if _, still := sb.files[row.ID+"/workspace/b.txt"]; still {
		t.Fatal("Remove did not delete")
	}
	if rec = rpc(e, key, row.ID, "MakeDir", `{"path":"newdir"}`); rec.Code != 200 || !strings.Contains(rec.Body.String(), "FILE_TYPE_DIRECTORY") {
		t.Fatalf("MakeDir: %d %s", rec.Code, rec.Body.String())
	}
	if rec = rpc(e, key, row.ID, "MakeDir", `{"path":"/workspace"}`); rec.Code != 409 {
		t.Fatalf("MakeDir on the workspace root: %d", rec.Code)
	}
}

func TestFilesystemRPCRejectsBinaryProtoAndWatchers(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	row, _ := sb.Create(nil, "proj-1", sandbox.CreateInput{Name: "fs2"}, "u")
	req := httptest.NewRequest("POST", "/filesystem.Filesystem/ListDir", strings.NewReader("\x0a\x01/"))
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("X-API-Key", key)
	req.Header.Set("E2b-Sandbox-Id", row.ID)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 501 || !strings.Contains(rec.Body.String(), "unimplemented") {
		t.Fatalf("binary protobuf: %d %s", rec.Code, rec.Body.String())
	}
	for _, m := range []string{"WatchDir", "CreateWatcher", "GetWatcherEvents", "RemoveWatcher"} {
		if rec := rpc(e, key, row.ID, m, `{}`); rec.Code != 501 {
			t.Errorf("%s: want 501, got %d", m, rec.Code)
		}
	}
	if rec := rpc(e, key, row.ID, "ListDir", `{broken`); rec.Code != 400 {
		t.Fatalf("malformed JSON: %d", rec.Code)
	}
}

func TestEnvsAndHealth(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	row, _ := sb.Create(nil, "proj-1", sandbox.CreateInput{Name: "h1"}, "u")
	sb.envs[row.ID] = map[string]string{"A": "1"}
	hdr := map[string]string{"E2b-Sandbox-Id": row.ID}
	if rec := call(e, "GET", "/health", key, "", hdr); rec.Code != 204 {
		t.Fatalf("running sandbox: %d", rec.Code)
	}
	rec := call(e, "GET", "/envs", key, "", hdr)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"A":"1"}` {
		t.Fatalf("envs: %d %s", rec.Code, rec.Body.String())
	}
	sb.Stop(nil, "proj-1", row.ID)
	if rec := call(e, "GET", "/health", key, "", hdr); rec.Code != 502 {
		t.Fatalf("a paused sandbox is not running: want 502, got %d", rec.Code)
	}
	if rec := call(e, "GET", "/metrics", key, "", hdr); rec.Code != 501 {
		t.Fatalf("envd metrics: %d", rec.Code)
	}
}

// ---- process.* ----

func TestProcessStartRequiresInteractiveBackend(t *testing.T) {
	sb := newFakeRouterSandbox(true) // interactive=false
	e, key, _ := routerWith(t, sb)
	row, _ := sb.Create(nil, "proj-1", sandbox.CreateInput{Name: "p1"}, "u")
	req := httptest.NewRequest("POST", "/process.Process/Start", strings.NewReader(`{"process":{"cmd":"/bin/sh"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)
	req.Header.Set("E2b-Sandbox-Id", row.ID)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 501 || !strings.Contains(rec.Body.String(), "unimplemented") {
		t.Fatalf("non-interactive backend: %d %s", rec.Code, rec.Body.String())
	}
	if len(sb.startedProc) != 0 {
		t.Fatal("must not try to start a process on a backend that cannot")
	}
}

func TestProcessValidationAndNotFound(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	sb.interactive = true
	e, key, _ := routerWith(t, sb)
	row, _ := sb.Create(nil, "proj-1", sandbox.CreateInput{Name: "p2"}, "u")
	post := func(method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/process.Process/"+method, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", key)
		req.Header.Set("E2b-Sandbox-Id", row.ID)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	if rec := post("Start", `{"process":{}}`); rec.Code != 400 || !strings.Contains(rec.Body.String(), "invalid_argument") {
		t.Fatalf("missing cmd: %d %s", rec.Code, rec.Body.String())
	}
	for _, m := range []string{"SendInput", "SendSignal", "CloseStdin", "Update"} {
		if rec := post(m, `{"process":{"pid":4242}}`); rec.Code != 404 {
			t.Errorf("%s on an unknown pid: want 404, got %d %s", m, rec.Code, rec.Body.String())
		}
		if rec := post(m, `{"process":{}}`); rec.Code != 400 {
			t.Errorf("%s without a selector: want 400, got %d", m, rec.Code)
		}
	}
	if rec := post("Connect", `{"process":{"tag":"nope"}}`); rec.Code != 404 {
		t.Fatalf("Connect to a missing tag: %d", rec.Code)
	}
	if rec := post("StreamInput", `{}`); rec.Code != 501 {
		t.Fatalf("StreamInput: %d", rec.Code)
	}
	rec := post("List", `{}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"processes":[]`) {
		t.Fatalf("List: %d %s", rec.Code, rec.Body.String())
	}
}

func TestClampU16(t *testing.T) {
	if sandbox.ClampPTYDim(0, 80) != 80 || sandbox.ClampPTYDim(50, 80) != 50 || sandbox.ClampPTYDim(5000, 80) != 1000 {
		t.Fatal("clampU16")
	}
}

// ---- 控制台能力 ----

func TestCapabilitiesReportsTerminalAndSSH(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	sb.interactive = true
	deps, key := fullRouterDeps(t)
	deps.Sandbox = sb
	deps.Terminal = &TerminalBridge{Fingerprint: "SHA256:abc", SSHListen: "127.0.0.1:2222"}
	e := NewRouter(deps)
	rec := call(e, "GET", "/e2b/ext/capabilities", key, "", nil)
	var caps map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &caps)
	ssh, _ := caps["ssh"].(map[string]any)
	if rec.Code != 200 || caps["terminal"] != true || ssh["enabled"] != true || ssh["host_key_fingerprint"] != "SHA256:abc" {
		t.Fatalf("capabilities = %s", rec.Body.String())
	}
	if caps["envd_version"] == "" || caps["envd_version"] == nil {
		t.Fatal("capabilities must report the envd version")
	}
	// 终端关闭时 ssh 段不暴露地址。
	deps.Terminal = &TerminalBridge{Fingerprint: "SHA256:abc"}
	e = NewRouter(deps)
	rec = call(e, "GET", "/e2b/ext/capabilities", key, "", nil)
	if strings.Contains(rec.Body.String(), "127.0.0.1:2222") || !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("ssh closed: %s", rec.Body.String())
	}
}
