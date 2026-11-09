package sbcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// ── 有状态的假 SimpleBase 服务 ──────────────────────────────

type fakeDB struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	DataModel string `json:"data_model"`
	hasMig    bool
	migs      []string
}

type fakeFnVersion struct{ source string }

type fakeFn struct {
	description string
	active      int64
	versions    []fakeFnVersion
}

type fakeCloud struct {
	t       *testing.T
	mu      sync.Mutex
	n       int
	dbs     []*fakeDB
	colls   map[string][]string
	docs    map[string]bool
	funcs   map[string]*fakeFn
	cron    []map[string]any
	batches []string
	calls   []string
	tokens  map[string]bool
	failSQL bool
	failFn  bool
	engine  string
}

func newFakeCloud(t *testing.T) *fakeCloud {
	return &fakeCloud{t: t, colls: map[string][]string{}, docs: map[string]bool{}, funcs: map[string]*fakeFn{}, tokens: map[string]bool{}}
}

func (f *fakeCloud) reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeCloud) errorf(w http.ResponseWriter, status int, code string) {
	f.reply(w, status, map[string]any{"error": map[string]any{"code": code, "message": code, "request_id": "rid"}})
}

func (f *fakeCloud) db(id string) *fakeDB {
	for _, d := range f.dbs {
		if d.ID == id {
			return d
		}
	}
	return nil
}

func (f *fakeCloud) fnDTO(name string) map[string]any {
	fn := f.funcs[name]
	pick := fn.active
	if pick == 0 {
		pick = int64(len(fn.versions))
	}
	return map[string]any{
		"name": name, "description": fn.description, "active_version": fn.active,
		"latest_version": len(fn.versions), "source": fn.versions[pick-1].source,
	}
}

func (f *fakeCloud) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	f.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")] = true
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	path := strings.Trim(r.URL.Path, "/")
	if path == "v1/projects" { // 项目枚举：dev 用来比对数据库引擎
		f.reply(w, 200, map[string]any{"projects": []map[string]any{{"id": "fixtur01", "engine": f.engine}}})
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, "v1/projects/"), "/"), "/")
	if len(parts) < 2 {
		f.errorf(w, 404, "not_found")
		return
	}
	rest := parts[1:] // 去掉 projectID
	var in map[string]any
	_ = json.Unmarshal(body, &in)
	switch {
	case rest[0] == "databases" && len(rest) == 1 && r.Method == http.MethodGet:
		f.reply(w, 200, map[string]any{"databases": f.dbs, "next_cursor": ""})
	case rest[0] == "databases" && len(rest) == 1 && r.Method == http.MethodPost:
		f.n++
		d := &fakeDB{ID: fmt.Sprintf("db-%d", f.n), Name: in["name"].(string), DataModel: in["data_model"].(string)}
		f.dbs = append(f.dbs, d)
		f.reply(w, 201, d)
	case rest[0] == "databases" && len(rest) >= 3:
		d := f.db(rest[1])
		if d == nil {
			f.errorf(w, 404, "not_found")
			return
		}
		f.database(w, r, d, rest[2:], in)
	case rest[0] == "gofunctions":
		f.function(w, r, rest[1:], in)
	case rest[0] == "cron-jobs":
		f.cronJobs(w, r, rest[1:], in)
	default:
		f.errorf(w, 404, "not_found")
	}
}

func (f *fakeCloud) database(w http.ResponseWriter, r *http.Request, d *fakeDB, rest []string, in map[string]any) {
	switch {
	case rest[0] == "execute":
		if strings.Contains(in["sql"].(string), "CREATE TABLE IF NOT EXISTS _migrations") {
			d.hasMig = true
		}
		if strings.HasPrefix(in["sql"].(string), "INSERT INTO _migrations") {
			args, _ := in["args"].([]any)
			d.migs = append(d.migs, args[0].(string))
		}
		f.reply(w, 200, map[string]any{"rows_affected": 0})
	case rest[0] == "query":
		if !d.hasMig {
			f.errorf(w, 404, "table_not_found")
			return
		}
		rows := [][]any{}
		for _, m := range d.migs {
			rows = append(rows, []any{m})
		}
		f.reply(w, 200, map[string]any{"columns": []string{"name"}, "rows": rows, "row_count": len(rows)})
	case rest[0] == "schema" && len(rest) == 1:
		tables := []map[string]any{}
		if d.hasMig {
			tables = append(tables, map[string]any{"name": "_migrations", "columns": []any{}})
		}
		f.reply(w, 200, map[string]any{"tables": tables})
	case rest[0] == "batch":
		stmts, _ := in["statements"].([]any)
		for _, s := range stmts {
			if f.failSQL && strings.Contains(s.(map[string]any)["sql"].(string), "FAIL_SQL") {
				f.reply(w, 200, map[string]any{"results": []any{}, "error": map[string]any{"failed_index": 0, "code": "sql_failed", "message": "boom"}})
				return
			}
		}
		last := stmts[len(stmts)-1].(map[string]any)
		marker := last["args"].([]any)[0].(string)
		d.migs = append(d.migs, marker)
		f.batches = append(f.batches, marker)
		items := make([]map[string]any, len(stmts))
		for i := range items {
			items[i] = map[string]any{"index": i}
		}
		f.reply(w, 200, map[string]any{"results": items})
	case rest[0] == "data" && rest[1] == "collections" && len(rest) == 2 && r.Method == http.MethodGet:
		f.reply(w, 200, map[string]any{"collections": append([]string{}, f.colls[d.ID]...), "items": []any{}})
	case rest[0] == "data" && rest[1] == "collections" && len(rest) == 2 && r.Method == http.MethodPost:
		f.colls[d.ID] = append(f.colls[d.ID], in["name"].(string))
		f.reply(w, 201, map[string]any{"name": in["name"]})
	case rest[0] == "data" && len(rest) == 5 && rest[3] == "documents" && r.Method == http.MethodGet:
		if !f.docs[d.ID+"/"+rest[2]+"/"+rest[4]] {
			f.errorf(w, 404, "not_found")
			return
		}
		f.reply(w, 200, map[string]any{"id": rest[4], "data": map[string]any{}})
	case rest[0] == "data" && len(rest) == 4 && rest[3] == "documents" && r.Method == http.MethodPost:
		// 与真实服务端一致：忽略传入的 id，总是生成新 id。
		f.n++
		f.docs[d.ID+"/"+rest[2]+"/"+fmt.Sprintf("doc-%d", f.n)] = true
		f.reply(w, 201, in)
	default:
		f.errorf(w, 404, "not_found")
	}
}

func (f *fakeCloud) function(w http.ResponseWriter, r *http.Request, rest []string, in map[string]any) {
	if len(rest) == 0 {
		if r.Method != http.MethodPost {
			f.errorf(w, 404, "not_found")
			return
		}
		name := in["name"].(string)
		fn := &fakeFn{description: fmt.Sprint(in["description"]), versions: []fakeFnVersion{{in["source"].(string)}}}
		if a, _ := in["activate"].(bool); a {
			fn.active = 1
		}
		f.funcs[name] = fn
		f.reply(w, 201, map[string]any{"name": name, "latest_version": 1, "active_version": fn.active})
		return
	}
	fn := f.funcs[rest[0]]
	if fn == nil {
		f.errorf(w, 404, "not_found")
		return
	}
	switch {
	case len(rest) == 1 && r.Method == http.MethodGet:
		f.reply(w, 200, f.fnDTO(rest[0]))
	case len(rest) == 1 && r.Method == http.MethodPatch:
		fn.description = in["description"].(string)
		f.reply(w, 200, f.fnDTO(rest[0]))
	case rest[1] == "versions" && len(rest) == 2 && r.Method == http.MethodGet:
		vs := []map[string]any{}
		for i := range fn.versions {
			vs = append(vs, map[string]any{"version": i + 1})
		}
		f.reply(w, 200, map[string]any{"active_version": fn.active, "versions": vs})
	case rest[1] == "versions" && len(rest) == 2 && r.Method == http.MethodPost:
		fn.versions = append(fn.versions, fakeFnVersion{in["source"].(string)})
		if a, _ := in["activate"].(bool); a {
			fn.active = int64(len(fn.versions))
		}
		f.reply(w, 201, map[string]any{"name": rest[0], "latest_version": len(fn.versions)})
	case rest[1] == "versions" && len(rest) == 3 && r.Method == http.MethodGet:
		v, _ := strconv.Atoi(rest[2])
		f.reply(w, 200, map[string]any{"version": v, "source": fn.versions[v-1].source})
	case rest[1] == "versions" && len(rest) == 4 && rest[3] == "test":
		ok := !(f.failFn && strings.Contains(fn.versions[atoi(rest[2])-1].source, "FAIL_TEST"))
		f.reply(w, 200, map[string]any{"ok": ok, "error": map[bool]any{true: nil, false: "试跑失败"}[ok]})
	case rest[1] == "versions" && len(rest) == 4 && rest[3] == "activate":
		fn.active = int64(atoi(rest[2]))
		f.reply(w, 200, map[string]any{"active_version": fn.active})
	default:
		f.errorf(w, 404, "not_found")
	}
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func (f *fakeCloud) cronJobs(w http.ResponseWriter, r *http.Request, rest []string, in map[string]any) {
	switch {
	case len(rest) == 0 && r.Method == http.MethodGet:
		f.reply(w, 200, map[string]any{"jobs": f.cron})
	case len(rest) == 0 && r.Method == http.MethodPost:
		f.n++
		job := map[string]any{"id": fmt.Sprintf("job-%d", f.n), "target_kind": "function"}
		for k, v := range in {
			job[k] = v
		}
		f.cron = append(f.cron, job)
		f.reply(w, 201, job)
	case len(rest) == 1 && r.Method == http.MethodPatch:
		for _, j := range f.cron {
			if j["id"] == rest[0] {
				// update 是全量替换（与服务端一致）：body 里没有的字段按零值写入。
				j["description"] = ""
				for k, v := range in {
					j[k] = v
				}
				f.reply(w, 200, j)
				return
			}
		}
		f.errorf(w, 404, "not_found")
	default:
		f.errorf(w, 404, "not_found")
	}
}

// ── 测试夹具 ────────────────────────────────────────────────

func devFixture(t *testing.T, name string) string {
	t.Helper()
	dst := t.TempDir()
	src := filepath.Join("testdata", "dev", name)
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

type devRun struct {
	code int
	out  devOutput
	raw  string
	err  string
}

type devOutput struct {
	OK   bool       `json:"ok"`
	Data *devResult `json:"data"`
}

func runDevCLI(t *testing.T, endpoint string, args ...string) devRun {
	t.Helper()
	env := []string{"HOME=" + t.TempDir(), "SIMPLEBASE_URL=" + endpoint, "SIMPLEBASE_API_KEY=dev-secret-key", "SIMPLEBASE_PROJECT_ID=fixtur01"}
	var out, errb bytes.Buffer
	code := RunIO(context.Background(), append([]string{"dev"}, args...), env, &out, &errb, strings.NewReader(""))
	res := devRun{code: code, raw: out.String(), err: errb.String()}
	_ = json.Unmarshal(out.Bytes(), &res.out)
	return res
}

func (r devRun) changed() []string {
	var out []string
	if r.out.Data == nil {
		return out
	}
	for _, a := range r.out.Data.Actions {
		if a.Verb != "skip" {
			out = append(out, a.Verb+" "+a.Kind+" "+a.Name)
		}
	}
	return out
}

// 用 127.0.0.1 当 local，用 localhost.（不在本地主机集合内的别名）模拟云端。
func cloudURL(local string) string {
	return strings.Replace(local, "127.0.0.1", "localhost.", 1)
}

func devWrite(t *testing.T, ws, rel, content string) {
	t.Helper()
	p := filepath.Join(ws, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type devExpect struct {
	Errors   []string       `json:"errors"`
	Warnings []string       `json:"warnings"`
	Counts   map[string]int `json:"counts"`
	Plan     []string       `json:"plan"`
}

func loadExpect(t *testing.T, name string) devExpect {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "dev", name, "expect.json"))
	if err != nil {
		t.Fatal(err)
	}
	var e devExpect
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatal(err)
	}
	return e
}

// ── fixture 对账：校验结论与重放计划 ─────────────────────────

func TestDevFixtureValidation(t *testing.T) {
	for _, name := range []string{"basic", "invalid"} {
		exp := loadExpect(t, name)
		w, err := loadDevWorkspace(filepath.Join("testdata", "dev", name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// 严格逐条比对：错误与警告的文本、顺序、条数都要一致，
		// 避免两端新增/改动提示时静默漂移。
		if strings.Join(w.errors, "\n") != strings.Join(exp.Errors, "\n") {
			t.Fatalf("%s: 错误不一致\nwant: %v\n got: %v", name, exp.Errors, w.errors)
		}
		if strings.Join(w.warnings, "\n") != strings.Join(exp.Warnings, "\n") {
			t.Fatalf("%s: 警告不一致\nwant: %v\n got: %v", name, exp.Warnings, w.warnings)
		}
		if exp.Counts != nil {
			n := 0
			for _, d := range w.databases {
				n += len(d.migrations)
			}
			got := map[string]int{"databases": len(w.databases), "migrations": n, "collections": len(w.colls),
				"functions": len(w.functions), "cron_jobs": len(w.cronJobs)}
			for k, v := range exp.Counts {
				if got[k] != v {
					t.Fatalf("%s: %s=%d want %d", name, k, got[k], v)
				}
			}
		}
	}
}

func TestDevPlanMatchesFixtureAndWritesNothing(t *testing.T) {
	cloud := newFakeCloud(t)
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	exp := loadExpect(t, "basic")

	res := runDevCLI(t, srv.URL, "plan", "--dir", ws)
	if res.code != 0 || res.out.Data == nil {
		t.Fatalf("plan: %d %s %s", res.code, res.raw, res.err)
	}
	if got := res.changed(); strings.Join(got, "|") != strings.Join(exp.Plan, "|") {
		t.Fatalf("plan actions\n got %v\nwant %v", got, exp.Plan)
	}
	if len(cloud.dbs) != 0 || len(cloud.funcs) != 0 || len(cloud.cron) != 0 {
		t.Fatalf("plan 不应写入远端：%+v", cloud)
	}
	if _, err := os.Stat(filepath.Join(ws, ".simplebase")); !os.IsNotExist(err) {
		t.Fatalf("plan 不应写 .simplebase：%v", err)
	}
}

// ── 幂等、迁移与函数语义（对应 sbdev.py 的 7 条验收）──────────

func TestDevApplyIdempotentLocal(t *testing.T) {
	cloud := newFakeCloud(t)
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")

	first := runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if first.code != 0 {
		t.Fatalf("first apply: %d %s %s", first.code, first.raw, first.err)
	}
	if first.out.Data.Changes != 6 || first.out.Data.EnvHint != ".simplebase/env.local" {
		t.Fatalf("first apply data: %+v", first.out.Data)
	}
	fn := cloud.funcs["hello"]
	if fn.active != 1 || len(fn.versions) != 1 {
		t.Fatalf("hello 应已生效 v1：%+v", fn)
	}
	second := runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if second.code != 0 || len(second.changed()) != 0 || second.out.Data.Changes != 0 {
		t.Fatalf("second apply must be all skip: %d %v %s", second.code, second.changed(), second.err)
	}
	if len(cloud.funcs["hello"].versions) != 1 || strings.Join(cloud.batches, ",") != "001_init.sql" {
		t.Fatalf("重放产生了多余写入：versions=%d batches=%v", len(cloud.funcs["hello"].versions), cloud.batches)
	}

	// 新增 002 只执行 002
	devWrite(t, ws, "migrations/002_more.sql", "CREATE TABLE IF NOT EXISTS extra (id VARCHAR);\n")
	third := runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if third.code != 0 || len(third.changed()) != 1 || strings.Join(cloud.batches, ",") != "001_init.sql,002_more.sql" {
		t.Fatalf("002: %d %v %v", third.code, third.changed(), cloud.batches)
	}
}

// examples 的 community / ops-assistant 在 collection 库上同时跑迁移 SQL 并存集合文档，
// 显式声明 data_model=collection 的库带 migrations 不能被拒绝。
func TestDevCollectionDatabaseMayRunMigrations(t *testing.T) {
	cloud := newFakeCloud(t)
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	m, _ := os.ReadFile(filepath.Join(ws, "simplebase.json"))
	devWrite(t, ws, "simplebase.json", strings.Replace(string(m),
		`{ "name": "docs", "data_model": "collection" }`,
		`{ "name": "docs", "data_model": "collection", "migrations": "migrations" }`, 1))
	r := runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if r.code != 0 {
		t.Fatalf("collection 库带 migrations: %d %s %s", r.code, r.raw, r.err)
	}
	if strings.Join(cloud.batches, ",") != "001_init.sql,001_init.sql" {
		t.Fatalf("两个库各自应用一次：%v", cloud.batches)
	}
}

func TestDevModifiedMigrationRejected(t *testing.T) {
	cloud := newFakeCloud(t)
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	if r := runDevCLI(t, srv.URL, "apply", "--dir", ws); r.code != 0 {
		t.Fatalf("apply: %s", r.err)
	}
	devWrite(t, ws, "migrations/001_init.sql", "CREATE TABLE IF NOT EXISTS notes (id VARCHAR, extra VARCHAR);\n")
	r := runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if r.code != exitUsage || !strings.Contains(r.err, "migration_modified") {
		t.Fatalf("want migration_modified, got %d %s", r.code, r.err)
	}
	if len(cloud.batches) != 1 {
		t.Fatalf("不应再执行迁移：%v", cloud.batches)
	}
}

func TestDevFunctionVersioning(t *testing.T) {
	cloud := newFakeCloud(t)
	cloud.failFn = true
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	if r := runDevCLI(t, srv.URL, "apply", "--dir", ws); r.code != 0 {
		t.Fatalf("apply: %s", r.err)
	}
	src, _ := os.ReadFile(filepath.Join(ws, "functions/hello.go"))

	// 源码变化 → 新版本，试跑后才生效
	devWrite(t, ws, "functions/hello.go", string(src)+"\n// v2\n")
	if r := runDevCLI(t, srv.URL, "apply", "--dir", ws); r.code != 0 {
		t.Fatalf("v2: %s", r.err)
	}
	if fn := cloud.funcs["hello"]; len(fn.versions) != 2 || fn.active != 2 {
		t.Fatalf("v2 状态：%+v", fn)
	}
	// 试跑失败 → 旧版本保持生效；重跑复用未生效的同源码版本
	devWrite(t, ws, "functions/hello.go", string(src)+"\n// FAIL_TEST\n")
	r := runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if r.code != exitAPI || !strings.Contains(r.err, "试跑失败") {
		t.Fatalf("want 试跑失败: %d %s", r.code, r.err)
	}
	if fn := cloud.funcs["hello"]; fn.active != 2 || len(fn.versions) != 3 {
		t.Fatalf("失败后状态：%+v", fn)
	}
	runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if len(cloud.funcs["hello"].versions) != 3 {
		t.Fatalf("重跑应复用同源码版本，实际 %d 个", len(cloud.funcs["hello"].versions))
	}
}

func TestDevNewFunctionTestedBeforeActivation(t *testing.T) {
	cloud := newFakeCloud(t)
	cloud.failFn = true
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	src, _ := os.ReadFile(filepath.Join(ws, "functions/hello.go"))
	devWrite(t, ws, "functions/hello.go", string(src)+"\n// FAIL_TEST\n")
	r := runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if r.code != exitAPI {
		t.Fatalf("want exit 2, got %d %s", r.code, r.err)
	}
	if cloud.funcs["hello"].active != 0 {
		t.Fatal("试跑失败的新函数不能被激活")
	}
}

func TestDevFailedMigrationKeepsProgress(t *testing.T) {
	cloud := newFakeCloud(t)
	cloud.failSQL = true
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	devWrite(t, ws, "migrations/002_bad.sql", "SELECT 'FAIL_SQL';\n")
	r := runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if r.code != exitAPI || !strings.Contains(r.err, "002_bad.sql") {
		t.Fatalf("want migration failure: %d %s", r.code, r.err)
	}
	if strings.Join(cloud.batches, ",") != "001_init.sql" {
		t.Fatalf("batches=%v", cloud.batches)
	}
	devWrite(t, ws, "migrations/002_bad.sql", "CREATE TABLE IF NOT EXISTS t (id VARCHAR);\n")
	if r := runDevCLI(t, srv.URL, "apply", "--dir", ws); r.code != 0 {
		t.Fatalf("resume: %s", r.err)
	}
	if strings.Join(cloud.batches, ",") != "001_init.sql,002_bad.sql" {
		t.Fatalf("batches=%v", cloud.batches)
	}
}

func TestDevCronCreateSkipUpdate(t *testing.T) {
	cloud := newFakeCloud(t)
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if len(cloud.cron) != 1 {
		t.Fatalf("cron: %+v", cloud.cron)
	}
	m, _ := os.ReadFile(filepath.Join(ws, "simplebase.json"))
	devWrite(t, ws, "simplebase.json", strings.Replace(string(m), `"0 3 * * *"`, `"0 4 * * *"`, 1))
	r := runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if r.code != 0 || len(r.changed()) != 1 || !strings.Contains(r.raw, "cron_expr") || cloud.cron[0]["cron_expr"] != "0 4 * * *" {
		t.Fatalf("update: %d %v %s", r.code, r.changed(), r.raw)
	}

	// 已过期的 once 任务跳过
	devWrite(t, ws, "simplebase.json", strings.Replace(string(m), `"cron_jobs": [`,
		`"cron_jobs": [{"name":"old","schedule_kind":"once","run_at":"2020-01-01T00:00:00Z","func_file":"hello","func_export":"Hello"},`, 1))
	r = runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if r.code != 0 || len(cloud.cron) != 1 || !strings.Contains(r.raw, "已过期") {
		t.Fatalf("expired once: %d %s", r.code, r.raw)
	}
}

func TestDevSeedOnlyWithFlag(t *testing.T) {
	cloud := newFakeCloud(t)
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if strings.Join(cloud.batches, ",") != "001_init.sql" || len(cloud.docs) != 0 {
		t.Fatalf("无 --seed 不应写种子：%v %v", cloud.batches, cloud.docs)
	}
	r := runDevCLI(t, srv.URL, "apply", "--dir", ws, "--seed")
	if r.code != 0 || strings.Join(cloud.batches, ",") != "001_init.sql,seed:seed/dev.sql" || len(cloud.docs) != 1 {
		t.Fatalf("--seed: %d %v %v %s", r.code, cloud.batches, cloud.docs, r.err)
	}
	if again := runDevCLI(t, srv.URL, "apply", "--dir", ws, "--seed"); len(again.changed()) != 0 {
		t.Fatalf("种子应按记账去重：%v", again.changed())
	}
	if len(cloud.docs) != 1 {
		t.Fatalf("种子文档不应重复写入（服务端忽略 id）：%v", cloud.docs)
	}
}

// ── 远端确认与双端口 local/cloud ──────────────────────────────

func TestDevRemoteRequiresYesAndTwoTargets(t *testing.T) {
	local, cloud := newFakeCloud(t), newFakeCloud(t)
	localSrv, cloudSrv := httptest.NewServer(local), httptest.NewServer(cloud)
	defer localSrv.Close()
	defer cloudSrv.Close()
	cloudEndpoint := cloudURL(cloudSrv.URL)
	ws := devFixture(t, "basic")

	if r := runDevCLI(t, localSrv.URL, "apply", "--dir", ws); r.code != 0 {
		t.Fatalf("local apply: %s", r.err)
	}
	// 云端未带 --yes：退出码 3、输出计划、不写入
	r := runDevCLI(t, cloudEndpoint, "apply", "--dir", ws)
	if r.code != exitConfirm || !strings.Contains(r.err, "confirmation_required") || !strings.Contains(r.err, "--yes") {
		t.Fatalf("want exit 3: %d %s", r.code, r.err)
	}
	if r.out.Data == nil || r.out.Data.Changes != 6 || len(cloud.dbs) != 0 {
		t.Fatalf("exit 3 应输出计划且不写入：%s dbs=%d", r.raw, len(cloud.dbs))
	}
	// 带 --yes：写入，且 lock 按 endpoint#project 分区
	if r := runDevCLI(t, cloudEndpoint, "apply", "--dir", ws, "--yes"); r.code != 0 {
		t.Fatalf("cloud apply: %d %s", r.code, r.err)
	}
	if len(cloud.dbs) != 2 || cloud.funcs["hello"].active != 1 {
		t.Fatalf("cloud 未重放完整：dbs=%d", len(cloud.dbs))
	}
	// 再次不带 --yes：没有变更时直接成功
	if r := runDevCLI(t, cloudEndpoint, "apply", "--dir", ws); r.code != 0 || r.out.Data.Changes != 0 {
		t.Fatalf("cloud 无变更应直接成功：%d %s", r.code, r.err)
	}

	lockRaw, err := os.ReadFile(filepath.Join(ws, ".simplebase", "lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lock devLock
	if err := json.Unmarshal(lockRaw, &lock); err != nil {
		t.Fatal(err)
	}
	lk, ck := "local#fixtur01", cloudEndpoint+"#fixtur01" // 本地分区键不带端口
	if lock.Targets[lk] == nil || lock.Targets[ck] == nil {
		t.Fatalf("lock 缺少分区：%v", lock.Targets)
	}
	if lock.Targets[lk].Databases["main"].ID == "" || lock.Targets[lk].Databases["main"].ID == "x" {
		t.Fatalf("local 库 ID 缺失")
	}
	// 环境提示文件按 label 区分且不含密钥
	for _, name := range []string{"env.local", "env.localhost"} {
		b, err := os.ReadFile(filepath.Join(ws, ".simplebase", name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(string(b), "dev-secret-key") || !strings.Contains(string(b), "SIMPLEBASE_DATABASE_ID_MAIN=") {
			t.Fatalf("%s 内容不正确：%s", name, b)
		}
	}
	if strings.Contains(string(lockRaw), "dev-secret-key") {
		t.Fatal("lock 不应含密钥")
	}
}

func TestDevSystemDatabaseProtected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":"system_project_protected","message":"system project is read-only"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"databases":[],"next_cursor":""}`))
	}))
	defer srv.Close()
	ws := devFixture(t, "basic")
	r := runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if r.code != exitAPI || !strings.Contains(r.err, "system_project_protected") {
		t.Fatalf("系统项目写入应被后端拒绝并透传错误码：%d %s", r.code, r.err)
	}
}

// ── 校验失败、凭据与 Agent 模式 ───────────────────────────────

func TestDevInvalidWorkspaceExitsUsage(t *testing.T) {
	cloud := newFakeCloud(t)
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	r := runDevCLI(t, srv.URL, "apply", "--dir", filepath.Join("testdata", "dev", "invalid"))
	if r.code != exitUsage || !strings.Contains(r.err, "workspace_invalid") {
		t.Fatalf("want exit 1: %d %s", r.code, r.err)
	}
	if len(cloud.calls) != 0 {
		t.Fatalf("校验失败不应访问远端：%v", cloud.calls)
	}
}

func TestDevRejectsEnvAndEscapingPaths(t *testing.T) {
	ws := devFixture(t, "basic")
	m, _ := os.ReadFile(filepath.Join(ws, "simplebase.json"))
	for _, file := range []string{".env", "../outside.go", "/etc/passwd"} {
		devWrite(t, ws, "simplebase.json", strings.Replace(string(m), "functions/hello.go", file, 1))
		w, err := loadDevWorkspace(ws)
		if err != nil || len(w.errors) == 0 {
			t.Fatalf("%s 应被拒绝：%v %v", file, err, w)
		}
	}
}

func TestDevCredentialsStayOutOfArtifacts(t *testing.T) {
	cloud := newFakeCloud(t)
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	r := runDevCLI(t, srv.URL, "apply", "--dir", ws)
	if r.code != 0 {
		t.Fatal(r.err)
	}
	if !cloud.tokens["dev-secret-key"] {
		t.Fatalf("SIMPLEBASE_API_KEY 应作为 Bearer 发出：%v", cloud.tokens)
	}
	if strings.Contains(r.raw, "dev-secret-key") || strings.Contains(r.err, "dev-secret-key") {
		t.Fatal("输出不应含密钥")
	}
	_ = filepath.Walk(filepath.Join(ws, ".simplebase"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), "dev-secret-key") {
				t.Fatalf("%s 含密钥", p)
			}
		}
		return nil
	})
}

func TestDevAgentModeRejected(t *testing.T) {
	cloud := newFakeCloud(t)
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	env := []string{"HOME=" + t.TempDir(), "SIMPLEBASE_URL=" + srv.URL, "SIMPLEBASE_TOKEN=t", "SIMPLEBASE_PROJECT_ID=fixtur01", "SIMPLEBASE_AGENT_RUN=1"}
	for _, sub := range []string{"plan", "apply", "status"} {
		var out, errb bytes.Buffer
		code := RunIO(context.Background(), []string{"dev", sub, "--dir", ws}, env, &out, &errb, strings.NewReader(""))
		if code != exitUsage || !strings.Contains(errb.String(), "file_not_allowed") {
			t.Fatalf("dev %s 在助手模式应被拒绝：%d %s", sub, code, errb.String())
		}
	}
	if len(cloud.calls) != 0 {
		t.Fatalf("助手模式不应访问远端：%v", cloud.calls)
	}
}

// examples/lib/init/run.mjs 与 skills/simplebase 都把这四份清单交给 `dev apply`：它们必须零校验错误
// （once 任务已写死 run_at），且显式声明 data_model=collection，两种入口建出的库类型一致。
func TestDevExamplesManifestsLoad(t *testing.T) {
	for _, name := range []string{"shop", "jd", "iot-telemetry", "community", "ops-assistant"} {
		w, err := loadDevWorkspace(filepath.Join("..", "..", "examples", name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(w.errors) != 0 {
			t.Fatalf("%s: 意外的校验错误：%v", name, w.errors)
		}
		if len(w.databases) != 1 || len(w.functions) == 0 {
			t.Fatalf("%s: 应恰好一个库（项目单库）且有函数：%d 库", name, len(w.databases))
		}
		if d := w.databases[0]; d.dataModel != "collection" || len(d.migrations) == 0 {
			t.Fatalf("%s: 库应为 collection 且保留迁移：%+v", name, d)
		}
	}
}

func TestDevStatus(t *testing.T) {
	cloud := newFakeCloud(t)
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	runDevCLI(t, srv.URL, "apply", "--dir", ws)
	env := []string{"HOME=" + t.TempDir(), "SIMPLEBASE_URL=" + srv.URL, "SIMPLEBASE_TOKEN=t", "SIMPLEBASE_PROJECT_ID=fixtur01"}
	var out, errb bytes.Buffer
	if code := RunIO(context.Background(), []string{"dev", "status", "--dir", ws}, env, &out, &errb, strings.NewReader("")); code != 0 {
		t.Fatalf("status: %d %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "local#fixtur01") || !strings.Contains(out.String(), "001_init.sql") {
		t.Fatalf("status 输出：%s", out.String())
	}
}

// dev validate / dev status 只读工作区：没有连接配置也要能用（CI 里只装二进制时用 validate）。
func TestDevValidateWithoutCredentials(t *testing.T) {
	ws := devFixture(t, "basic")
	env := []string{"HOME=" + t.TempDir()}
	var out, errb bytes.Buffer
	code := RunIO(context.Background(), []string{"dev", "validate", "--dir", ws}, env, &out, &errb, strings.NewReader(""))
	if code != 0 {
		t.Fatalf("dev validate 应无凭据可用：%d %s", code, errb.String())
	}
	var got struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("输出不是 JSON 信封：%s", out.String())
	}
	if got.Data["functions"].(float64) != 1 {
		t.Fatalf("functions 计数不对：%v", got.Data)
	}

	bad := filepath.Join("testdata", "dev", "invalid")
	out.Reset()
	errb.Reset()
	code = RunIO(context.Background(), []string{"dev", "validate", "--dir", bad}, env, &out, &errb, strings.NewReader(""))
	if code != exitUsage || !strings.Contains(errb.String(), "workspace_invalid") {
		t.Fatalf("invalid 应退出码 1：%d %s", code, errb.String())
	}

	out.Reset()
	errb.Reset()
	if code := RunIO(context.Background(), []string{"dev", "status", "--dir", ws}, env, &out, &errb, strings.NewReader("")); code != 0 {
		t.Fatalf("dev status 应无凭据可用：%d %s", code, errb.String())
	}
}

// 引号、$$ 与注释里的 `;` 不能把语句拆错；字面量里的关键字不能误报。
func TestDevSQLSplitter(t *testing.T) {
	got := devSplitSQL("INSERT INTO t (id, note) VALUES ('a;b', \"c;d\");\n-- 注释;\nUPDATE t SET n = $$x;y$$;\n")
	if len(got) != 2 {
		t.Fatalf("应拆成 2 条：%q", got)
	}
	if !strings.Contains(got[1], "$$x;y$$") {
		t.Fatalf("$$ 内容被破坏：%q", got[1])
	}
	if masked := devMaskSQL("INSERT INTO t (v) VALUES ('PRIMARY KEY; ATTACH');"); len(masked) != 1 ||
		devNoDuckLake.MatchString(masked[0]) || strings.Contains(masked[0], "ATTACH") {
		t.Fatalf("字面量应被脱敏：%q", masked)
	}
}

// cron 的 description 写了（含空串）才同步：显式清空能生效，没写则保留远端值。
func TestDevCronDescriptionClearedWhenExplicit(t *testing.T) {
	cloud := newFakeCloud(t)
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	m := filepath.Join(ws, "simplebase.json")
	b, err := os.ReadFile(m)
	if err != nil {
		t.Fatal(err)
	}
	withDesc := strings.Replace(string(b), `"enabled": true`, `"enabled": true, "description": "夜间汇总"`, 1)
	devWrite(t, ws, "simplebase.json", withDesc)
	if r := runDevCLI(t, srv.URL, "apply", "--dir", ws); r.code != 0 {
		t.Fatal(r.err)
	}
	if got := cloud.cron[0]["description"]; got != "夜间汇总" {
		t.Fatalf("描述未写入：%v", got)
	}
	// 显式写空串 → 清空
	devWrite(t, ws, "simplebase.json", strings.Replace(withDesc, `"description": "夜间汇总"`, `"description": ""`, 1))
	if r := runDevCLI(t, srv.URL, "apply", "--dir", ws); r.code != 0 {
		t.Fatal(r.err)
	}
	if got := cloud.cron[0]["description"]; got != "" {
		t.Fatalf("描述未清空：%v", got)
	}
	// 去掉字段 → 保留远端值（此处已是空串，先设回来再验证）
	devWrite(t, ws, "simplebase.json", withDesc)
	if r := runDevCLI(t, srv.URL, "apply", "--dir", ws); r.code != 0 {
		t.Fatal(r.err)
	}
	devWrite(t, ws, "simplebase.json", strings.Replace(withDesc, `, "description": "夜间汇总"`, ``, 1))
	if r := runDevCLI(t, srv.URL, "apply", "--dir", ws); r.code != 0 {
		t.Fatal(r.err)
	}
	if got := cloud.cron[0]["description"]; got != "夜间汇总" {
		t.Fatalf("未写 description 应保留远端值：%v", got)
	}
}

// 引擎是项目级设置，迁移只按一种方言写：两个环境引擎不同时 plan/apply 要给出警告。
func TestDevEngineMismatchWarns(t *testing.T) {
	cloud := newFakeCloud(t)
	cloud.engine = "duckdb"
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	if r := runDevCLI(t, srv.URL, "apply", "--dir", ws); r.code != 0 {
		t.Fatal(r.err)
	}
	// 同一工作区换到「另一个环境」，引擎不同
	cloud.engine = "sqlite"
	r := runDevCLI(t, cloudURL(srv.URL), "plan", "--dir", ws)
	if r.code != 0 {
		t.Fatal(r.err)
	}
	found := false
	for _, w := range r.out.Data.Warnings {
		if strings.Contains(w, "引擎") && strings.Contains(w, "sqlite") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应给出引擎不一致警告：%v", r.out.Data.Warnings)
	}
}

// 函数 description 与 cron 同语义：不写保留远端值，写空串显式清空。
func TestDevFunctionDescriptionClearedWhenExplicit(t *testing.T) {
	cloud := newFakeCloud(t)
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	ws := devFixture(t, "basic")
	m := filepath.Join(ws, "simplebase.json")
	b, err := os.ReadFile(m)
	if err != nil {
		t.Fatal(err)
	}
	withDesc := strings.Replace(string(b), `"description": "示例函数"`, `"description": "旧描述"`, 1)
	devWrite(t, ws, "simplebase.json", withDesc)
	if r := runDevCLI(t, srv.URL, "apply", "--dir", ws); r.code != 0 {
		t.Fatal(r.err)
	}
	if got := cloud.funcs["hello"].description; got != "旧描述" {
		t.Fatalf("描述未写入：%q", got)
	}
	// 去掉字段 → 保留远端值
	noDesc := strings.Replace(withDesc, `"description": "旧描述",`, ``, 1)
	devWrite(t, ws, "simplebase.json", noDesc)
	if r := runDevCLI(t, srv.URL, "apply", "--dir", ws); r.code != 0 {
		t.Fatal(r.err)
	}
	if got := cloud.funcs["hello"].description; got != "旧描述" {
		t.Fatalf("未写 description 应保留远端值：%q", got)
	}
	// 显式写空串 → 清空
	devWrite(t, ws, "simplebase.json", strings.Replace(withDesc, `"description": "旧描述"`, `"description": ""`, 1))
	if r := runDevCLI(t, srv.URL, "apply", "--dir", ws); r.code != 0 {
		t.Fatal(r.err)
	}
	if got := cloud.funcs["hello"].description; got != "" {
		t.Fatalf("描述未清空：%q", got)
	}
}

// plan 与 apply 的动作序列必须一致（apply 的 detail 多一段「→ db-N」，比较时忽略 detail）。
// 覆盖空环境、远端已有同名库、lock 失效、只有记账表、带 --seed 五种情况。
func TestDevPlanMatchesApplyActions(t *testing.T) {
	type setup func(ws string, c *fakeCloud)
	cases := map[string]setup{
		"fresh": nil,
		"remote-db-exists": func(_ string, c *fakeCloud) {
			c.dbs = []*fakeDB{{ID: "db-pre", Name: "main", DataModel: "sql"}}
		},
		"stale-lock": func(ws string, _ *fakeCloud) {
			lock := devLock{Version: 1, Targets: map[string]*devLockEntry{
				"local#fixtur01": {Databases: map[string]devLockDB{"main": {ID: "gone"}}}}}
			if err := saveDevLock(&devWorkspace{root: ws}, &lock); err != nil {
				t.Fatal(err)
			}
		},
		"migrations-table-only": func(_ string, c *fakeCloud) {
			c.dbs = []*fakeDB{{ID: "db-pre", Name: "main", DataModel: "sql", hasMig: true}}
		},
		"seed": nil,
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			seed := name == "seed"
			// plan：在给定远端状态下预测
			ws := devFixture(t, "basic")
			c := newFakeCloud(t)
			if setup != nil {
				setup(ws, c)
			}
			srv := httptest.NewServer(c)
			defer srv.Close()
			planArgs := []string{"plan", "--dir", ws}
			if seed {
				planArgs = append(planArgs, "--seed")
			}
			plan := runDevCLI(t, srv.URL, planArgs...)
			if plan.code != 0 {
				t.Fatalf("plan: %s", plan.err)
			}
			// apply：相同初始状态（清掉 plan 产生的调用与 lock 写入）
			ws2 := devFixture(t, "basic")
			c2 := newFakeCloud(t)
			if setup != nil {
				setup(ws2, c2)
			}
			srv2 := httptest.NewServer(c2)
			defer srv2.Close()
			applyArgs := []string{"apply", "--dir", ws2}
			if seed {
				applyArgs = append(applyArgs, "--seed")
			}
			applied := runDevCLI(t, srv2.URL, append(applyArgs, "--yes")...)
			if applied.code != 0 {
				t.Fatalf("apply: %s", applied.err)
			}
			want := plan.changed()
			got := applied.changed()
			if strings.Join(want, "|") != strings.Join(got, "|") {
				t.Fatalf("plan 与 apply 动作不一致\nplan : %v\napply: %v", want, got)
			}
		})
	}
}
