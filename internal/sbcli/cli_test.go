package sbcli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type cliHit struct {
	Method string
	Path   string
	Body   string
	Token  string
}

type cliStub struct {
	mu   sync.Mutex
	hits []cliHit
}

func (s *cliStub) reset() {
	s.mu.Lock()
	s.hits = nil
	s.mu.Unlock()
}

func (s *cliStub) all() []cliHit {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]cliHit(nil), s.hits...)
	return out
}

func (s *cliStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := readLimited(r)
	s.mu.Lock()
	s.hits = append(s.hits, cliHit{Method: r.Method, Path: r.URL.Path, Body: string(body), Token: strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")})
	s.mu.Unlock()
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	path := r.URL.Path
	fail := func(status int, code string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": code, "request_id": "rid"}})
	}
	if strings.Contains(path, "simplebase-system") && r.Method == http.MethodDelete {
		fail(http.StatusForbidden, "system_database_protected")
		return
	}
	if strings.Contains(path, "/execute") && strings.Contains(string(body), ";") {
		fail(http.StatusBadRequest, "multiple_statements")
		return
	}
	if strings.Contains(path, "/data/collections") && strings.Contains(path, "/sqldb/") {
		fail(http.StatusBadRequest, "data_model_mismatch")
		return
	}
	if strings.Contains(path, "/kv") && strings.Contains(string(body), `"NOPE"`) {
		fail(http.StatusBadRequest, "kv_unknown_command")
		return
	}
	if r.Method == http.MethodPost && strings.Contains(path, "/cron-jobs") && !strings.Contains(path, "/trigger") && strings.Contains(string(body), `"interval_seconds":10`) {
		fail(http.StatusBadRequest, "invalid_interval")
		return
	}
	if strings.Contains(path, "/gofunctions/missing") && r.Method == http.MethodDelete {
		fail(http.StatusNotFound, "not_found")
		return
	}
	if strings.Contains(path, "/sandboxes") && !strings.HasSuffix(path, "/capabilities") && tok == "nosandbox" {
		fail(http.StatusServiceUnavailable, "sandbox_unavailable")
		return
	}
	if strings.Contains(path, "/logs/retention") && r.Method == http.MethodPut && tok == "plain-user" {
		fail(http.StatusForbidden, "forbidden")
		return
	}
	if strings.Contains(path, "/api-keys") && r.Method == http.MethodPost && strings.Contains(string(body), "user:admin") && tok == "user-token" {
		fail(http.StatusForbidden, "forbidden")
		return
	}
	if tok == "ro-token" && cliWrite(r.Method, path, body) {
		fail(http.StatusServiceUnavailable, "writer_unavailable")
		return
	}
	if tok == "admin-token" && cliWrite(r.Method, path, body) {
		fail(http.StatusForbidden, "forbidden")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.Contains(path, "/query") && strings.Contains(string(body), "password"):
		_, _ = w.Write([]byte(`{"columns":["password","id"],"rows":[["s3cret","1"]],"row_count":1}`))
	case strings.Contains(path, "/api-keys") && r.Method == http.MethodPost:
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"k1","secret":"sekret-value","permissions":["database:read"]}`))
	case strings.Contains(path, "/presign"):
		_, _ = w.Write([]byte(`{"url":"https://files.example/signed-secret"}`))
	case strings.HasPrefix(path, "/v1/users") && r.Method == http.MethodPost:
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"u1","username":"ada","password":"hunter2"}`))
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/s3/objects"):
		_, _ = w.Write([]byte(`[]`))
	default:
		if r.Method == http.MethodPost && strings.HasSuffix(path, "/databases") {
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = w.Write([]byte(`{}`))
	}
}

func readLimited(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for len(buf) < 1<<20 {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			return buf, nil
		}
	}
	return buf, nil
}

func cliWrite(method, path string, body []byte) bool {
	if method == http.MethodGet || method == http.MethodHead {
		return false
	}
	if strings.Contains(path, "/query") || strings.HasPrefix(path, "/go/") {
		return false
	}
	if strings.Contains(path, "/kv") && strings.Contains(string(body), `"GET"`) {
		return false
	}
	return true
}

func runCLI(t *testing.T, env, args []string, stdin string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := RunIO(context.Background(), args, env, &out, &errb, strings.NewReader(stdin))
	return code, out.String(), errb.String()
}

func TestCLIConfigAndEnvelope(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "simplebase", "config.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"endpoint":"http://file.example","token":"file-token","project_id":"from-file","output":"json"}`)
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	stub := &cliStub{}
	srv := httptest.NewServer(stub)
	defer srv.Close()

	env := []string{"XDG_CONFIG_HOME=" + dir, "HOME=" + t.TempDir()}
	code, _, errb := runCLI(t, append(env, "SIMPLEBASE_URL=http://user:pass@127.0.0.1:9", "SIMPLEBASE_TOKEN=t", "SIMPLEBASE_PROJECT_ID=p"), []string{"database", "list"}, "")
	if code != 4 || !strings.Contains(errb, "http") {
		t.Fatalf("userinfo: %d %s", code, errb)
	}

	if err := os.Chmod(cfgPath, 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errb = runCLI(t, []string{"XDG_CONFIG_HOME=" + dir, "HOME=" + t.TempDir(), "SIMPLEBASE_URL=" + srv.URL, "SIMPLEBASE_TOKEN=user-token", "SIMPLEBASE_PROJECT_ID=p1"}, []string{"database", "list"}, "")
	if code != 4 || !strings.Contains(errb, "config_insecure") {
		t.Fatalf("mode: %d %s", code, errb)
	}
	if err := os.Chmod(cfgPath, 0o600); err != nil {
		t.Fatal(err)
	}

	stub.reset()
	var out string
	code, out, errb = runCLI(t, []string{
		"XDG_CONFIG_HOME=" + dir,
		"SIMPLEBASE_URL=" + srv.URL,
		"SIMPLEBASE_TOKEN=env-token",
		"SIMPLEBASE_PROJECT_ID=env-project",
	}, []string{"--endpoint", srv.URL, "--token", "flag-token", "--project", "p1", "--output", "json", "database", "list"}, "")
	if code != 0 || !strings.Contains(out, `"ok":true`) {
		t.Fatalf("flag priority: %d %s %s", code, out, errb)
	}
	hits := stub.all()
	if len(hits) != 1 || hits[0].Token != "flag-token" || !strings.Contains(hits[0].Path, "/projects/p1/databases") {
		t.Fatalf("hits %+v", hits)
	}

	stub.reset()
	code, _, errb = runCLI(t, []string{"HOME=" + t.TempDir(), "SIMPLEBASE_URL=" + srv.URL, "SIMPLEBASE_TOKEN=user-token", "SIMPLEBASE_PROJECT_ID=p1"}, []string{"nope", "thing"}, "")
	if code != 1 || len(stub.all()) != 0 || !strings.Contains(errb, `"ok":false`) {
		t.Fatalf("unknown: %d %s hits %d", code, errb, len(stub.all()))
	}

	code, _, errb = runCLI(t, []string{"HOME=" + t.TempDir()}, []string{"database", "list"}, "")
	if code != 4 {
		t.Fatalf("missing config %d %s", code, errb)
	}
}

func TestCLIResources(t *testing.T) {
	stub := &cliStub{}
	srv := httptest.NewServer(stub)
	defer srv.Close()
	home := t.TempDir()
	base := []string{"HOME=" + home, "SIMPLEBASE_URL=" + srv.URL, "SIMPLEBASE_TOKEN=user-token", "SIMPLEBASE_PROJECT_ID=p1", "SIMPLEBASE_OUTPUT=json"}
	src := filepath.Join(t.TempDir(), "hello.go")
	if err := os.WriteFile(src, []byte("package main\nfunc Hello() string { return \"hi\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	batch := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(batch, []byte(`[{"sql":"INSERT INTO t VALUES (1)","args":[]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(t.TempDir(), "readme.txt")
	if err := os.WriteFile(readme, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok := func(t *testing.T, args []string, extra ...string) string {
		t.Helper()
		env := append(append([]string{}, base...), extra...)
		code, out, errb := runCLI(t, env, args, "")
		if code != 0 {
			t.Fatalf("%v code %d %s %s", args, code, out, errb)
		}
		return out
	}
	fail := func(t *testing.T, token string, args []string, stdin, code string) {
		t.Helper()
		env := append([]string{"HOME=" + home, "SIMPLEBASE_URL=" + srv.URL, "SIMPLEBASE_TOKEN=" + token, "SIMPLEBASE_PROJECT_ID=p1"}, "SIMPLEBASE_OUTPUT=json")
		c, out, errb := runCLI(t, env, args, stdin)
		if c != 2 || !strings.Contains(errb, code) {
			t.Fatalf("%v got %d %s %s", args, c, out, errb)
		}
	}

	ok(t, []string{"database", "create", "--name", "shop", "--data-model", "sql"})
	ok(t, []string{"database", "get", "--id", "shop"})
	ok(t, []string{"database", "list"})
	fail(t, "user-token", []string{"database", "delete", "--id", "simplebase-system"}, "", "system_database_protected")
	fail(t, "admin-token", []string{"database", "create", "--name", "x"}, "", "forbidden")
	fail(t, "ro-token", []string{"database", "create", "--name", "x"}, "", "writer_unavailable")
	ok(t, []string{"database", "delete", "--id", "shop"})

	ok(t, []string{"schema", "create-table", "--database", "shop", "--name", "orders", "--columns", `[{"name":"id","type":"BIGINT"}]`})
	ok(t, []string{"schema", "add-column", "--database", "shop", "--table", "orders", "--column", `{"name":"amount","type":"DOUBLE"}`})
	ok(t, []string{"sql", "query", "--database", "shop", "--statement", "SELECT password FROM t"})
	ok(t, []string{"sql", "exec", "--database", "shop", "--statement", "INSERT INTO orders VALUES (1)"})
	fail(t, "user-token", []string{"sql", "exec", "--database", "shop", "--statement", "DELETE FROM t; DROP TABLE t"}, "", "multiple_statements")
	ok(t, []string{"sql", "batch", "--database", "shop", "--file", batch})
	fail(t, "user-token", []string{"collection", "list", "--database", "sqldb"}, "", "data_model_mismatch")
	ok(t, []string{"collection", "create", "--database", "shop", "--name", "Books"})
	ok(t, []string{"document", "insert", "--database", "shop", "--collection", "Books", "--json", `{"title":"a"}`})
	ok(t, []string{"document", "update", "--database", "shop", "--collection", "Books", "--id", "d1", "--json", `{"title":"b"}`})
	ok(t, []string{"document", "list", "--database", "shop", "--collection", "Books"})
	ok(t, []string{"document", "delete", "--database", "shop", "--collection", "Books", "--id", "d1"})

	ok(t, []string{"kv", "exec", "--command", "SET", "--arg", "a", "--arg", "1"})
	ok(t, []string{"kv", "exec", "--command", "GET", "--arg", "a"})
	ok(t, []string{"kv", "exec", "--command", "DEL", "--arg", "a"})
	fail(t, "admin-token", []string{"kv", "exec", "--command", "SET", "--arg", "a", "--arg", "1"}, "", "forbidden")
	fail(t, "user-token", []string{"kv", "exec", "--command", "NOPE"}, "", "kv_unknown_command")

	ok(t, []string{"object", "upload", "--key", "readme", "--file", readme})
	ok(t, []string{"object", "list", "--prefix", ""})
	fail(t, "ro-token", []string{"object", "upload", "--key", "readme", "--file", readme}, "", "writer_unavailable")
	fail(t, "ro-token", []string{"object", "delete", "--key", "readme"}, "", "writer_unavailable")

	ok(t, []string{"function", "create", "--name", "hello", "--file", src})
	ok(t, []string{"function", "version", "get", "--name", "hello", "--version", "1"})
	ok(t, []string{"function", "activate", "--name", "hello", "--version", "1"})
	ok(t, []string{"function", "invoke", "--name", "hello", "--export", "Hello"})
	ok(t, []string{"function", "test", "--name", "hello", "--version", "1", "--export", "Hello"})
	ok(t, []string{"function", "delete", "--name", "hello"})
	fail(t, "user-token", []string{"function", "delete", "--name", "missing"}, "", "not_found")

	ok(t, []string{"cron", "create", "--name", "nightly", "--schedule-kind", "cron", "--cron", "0 0 * * *"})
	ok(t, []string{"cron", "get", "--id", "job1"})
	ok(t, []string{"cron", "trigger", "--id", "job1"})
	ok(t, []string{"cron", "runs", "--id", "job1"})
	ok(t, []string{"cron", "delete", "--id", "job1"})
	fail(t, "user-token", []string{"cron", "create", "--schedule-kind", "interval", "--interval-seconds", "10"}, "", "invalid_interval")

	ok(t, []string{"sandbox", "capabilities"})
	ok(t, []string{"sandbox", "create", "--name", "box", "--image", "alpine"})
	ok(t, []string{"sandbox", "exec", "--id", "sb1", "--command", "echo hi"})
	ok(t, []string{"sandbox", "update", "--id", "sb1", "--name", "box2"})
	ok(t, []string{"sandbox", "run", "--command", "echo hi"})
	ok(t, []string{"sandbox", "files", "list", "--id", "sb1", "--path", "/workspace"})
	ok(t, []string{"sandbox", "delete", "--id", "sb1"})
	fail(t, "nosandbox", []string{"sandbox", "list"}, "", "sandbox_unavailable")
	code, out, errb := runCLI(t, []string{"HOME=" + home, "SIMPLEBASE_URL=" + srv.URL, "SIMPLEBASE_TOKEN=nosandbox", "SIMPLEBASE_PROJECT_ID=p1"}, []string{"sandbox", "capabilities"}, "")
	if code != 0 {
		t.Fatalf("capabilities %d %s %s", code, out, errb)
	}

	ok(t, []string{"log", "search", "--q", "boom"})
	fail(t, "plain-user", []string{"log", "retention", "set", "--days", "7"}, "", "forbidden")
	ok(t, []string{"log", "retention", "get"})

	code, out, errb = runCLI(t, []string{"HOME=" + home, "SIMPLEBASE_URL=" + srv.URL, "SIMPLEBASE_TOKEN=admin-token", "SIMPLEBASE_PROJECT_ID=p1"}, []string{"user", "list"}, "")
	if code != 0 {
		t.Fatalf("user list %d %s %s", code, out, errb)
	}
	fail(t, "admin-token", []string{"user", "create", "--username", "ada", "--role", "user"}, "hunter2\n", "forbidden")
	out = ok(t, []string{"user", "create", "--username", "ada", "--role", "user"})
	// password comes from empty stdin in ok(); dedicated call below checks redaction.
	_ = out
	code, out, errb = runCLI(t, base, []string{"user", "create", "--username", "ada", "--role", "user"}, "hunter2\n")
	if code != 0 || strings.Contains(out, "hunter2") || !strings.Contains(out, "***") {
		t.Fatalf("password leaked %d %s %s", code, out, errb)
	}
	ok(t, []string{"user", "delete", "--id", "u1"})

	ok(t, []string{"project", "list"})
	code, out, errb = runCLI(t, append(base, "SIMPLEBASE_AGENT_RUN=1"), []string{"apikey", "create", "--permissions", "database:read"}, "")
	if code != 0 || strings.Contains(out, "sekret-value") || !strings.Contains(out, "secret_withheld") {
		t.Fatalf("agent secret %d %s %s", code, out, errb)
	}
	code, out, errb = runCLI(t, base, []string{"apikey", "create", "--permissions", "database:read"}, "")
	if code != 0 || !strings.Contains(out, "sekret-value") {
		t.Fatalf("human secret %d %s %s", code, out, errb)
	}
	fail(t, "user-token", []string{"apikey", "create", "--permissions", "user:admin"}, "", "forbidden")
	ok(t, []string{"apikey", "revoke", "--id", "k1"})

	code, out, errb = runCLI(t, append(base, "SIMPLEBASE_AGENT_RUN=1"), []string{"object", "presign", "--key", "readme"}, "")
	if code != 0 || strings.Contains(out, "signed-secret") || !strings.Contains(out, "expires_at") {
		t.Fatalf("agent presign %d %s %s", code, out, errb)
	}
	code, out, errb = runCLI(t, base, []string{"object", "presign", "--key", "readme"}, "")
	if code != 0 || !strings.Contains(out, "signed-secret") {
		t.Fatalf("human presign %d %s %s", code, out, errb)
	}

	stub.reset()
	code, out, errb = runCLI(t, base, []string{"sql", "query", "--database", "shop", "--statement", "SELECT password FROM t"}, "")
	if code != 0 || strings.Contains(out, "s3cret") || !strings.Contains(out, "***") {
		t.Fatalf("redact %d %s %s", code, out, errb)
	}
}

func TestCLIInvokeStaysReadable(t *testing.T) {
	stub := &cliStub{}
	srv := httptest.NewServer(stub)
	defer srv.Close()
	code, out, errb := runCLI(t, []string{"HOME=" + t.TempDir(), "SIMPLEBASE_URL=" + srv.URL, "SIMPLEBASE_TOKEN=admin-token", "SIMPLEBASE_PROJECT_ID=p1"}, []string{"function", "invoke", "--name", "hello", "--export", "Hello"}, "")
	if code != 0 {
		t.Fatalf("admin invoke %d %s %s", code, out, errb)
	}
	if len(stub.all()) != 1 || !strings.HasPrefix(stub.all()[0].Path, "/go/") {
		t.Fatalf("invoke path %+v", stub.all())
	}
}
