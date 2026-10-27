package api

// agent_skills_router_e2e_test.go：云助手 Skill 样例集（planv6.0 §3.7）。
// 与 agent_skills_e2e_test.go 的区别：数据面打真实 api.NewRouter +
// 真实 AuthMiddlewareWithDelegation / writeGate / projectContextMiddleware，
// 鉴权链路的回归（SEC-03 白名单漂移、D2 递归、D3 文件隔离）在此暴露。
// 数据面依赖：内存 SQLite 系统库 + 内存对象存储 + fake catalog/fakeSQL。

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// skillRouterEnv 聚合真实 router 环境 + 云助手运行时。
type skillRouterEnv struct {
	deps     Dependencies
	echo     *echo.Echo
	srv      *httptest.Server
	store    *systemdb.Store
	rt       *gocloudagent.Runtime
	llm      *scriptLLM
	reg      *auth.DelegationRegistry
	dataHits *skillHitCounter
}

// skillHitCounter 统计数据面请求（透传计数，不改变响应）。
type skillHitCounter struct {
	mu   sync.Mutex
	hits []skillHit
}

func (s *skillHitCounter) record(method, path string, status int) {
	s.mu.Lock()
	s.hits = append(s.hits, skillHit{Method: method, Path: path, Status: status})
	s.mu.Unlock()
}

func (s *skillHitCounter) count(method, pathPart string, only2xx bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, h := range s.hits {
		if h.Method == method && strings.Contains(h.Path, pathPart) && (!only2xx || (h.Status >= 200 && h.Status < 300)) {
			n++
		}
	}
	return n
}

func (s *skillHitCounter) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.hits)
}

func (s *skillHitCounter) reset() {
	s.mu.Lock()
	s.hits = nil
	s.mu.Unlock()
}

// newSkillRouterEnv 构造真实 router 环境：
// - 系统库：内存 SQLite（真实迁移）；
// - catalog/SQL/data：fake 服务（鉴权链路是重点，数据面语义已有各自测试）；
// - 云助手：SkillsCLI + InProcessCLI + scriptLLM（与桩服务版相同）。
func newSkillRouterEnv(t *testing.T) *skillRouterEnv {
	t.Helper()

	rawKey := "sk-router-e2e-key"
	rawKeyB := "sk-router-e2e-key-b"
	authSvc0 := auth.NewService(&memAuthRepo{}, "secret")
	hash := authSvc0.HashKey(rawKey)
	hashB := authSvc0.HashKey(rawKeyB)
	authSvc := auth.NewService(&multiKeyRepo{recs: []auth.APIKeyRecord{
		{ID: "key-A", TenantID: "tenant-1", ProjectIDs: []string{"proj-1"},
			Permissions: []auth.Permission{auth.DatabaseRead, auth.DatabaseWrite, auth.DatabaseAdmin, auth.ProjectAdmin},
			KeyHash:     hash},
		{ID: "key-B", TenantID: "tenant-1", ProjectIDs: []string{"proj-1"},
			Permissions: []auth.Permission{auth.DatabaseRead, auth.DatabaseWrite},
			KeyHash:     hashB},
	}}, "secret")

	cat := &stubCatalog{tenant: "tenant-1", db: catalog.Database{ID: "db-1", ProjectID: "proj-1", Name: "shop", Status: catalog.DatabaseReady}}
	dbSvc := newFakeDBService()
	dbSvc.dbs["db-1"] = cat.db
	sqlSvc := &fakeSQLService{db: cat.db, lease: &fakeSQLLease{queryResult: godatabase.QueryResult{Columns: []string{"amount"}, Rows: [][]any{{10}}, RowsAffected: 1}}}

	sdb := openMemoryDB(t)
	if err := systemdb.EnsureSystemSchema(context.Background(), sdb); err != nil {
		t.Fatal(err)
	}
	store := systemdb.NewStoreForTest(sdb)

	delegations := auth.NewDelegationRegistry()
	counter := &skillHitCounter{}

	deps := Dependencies{
		Config: config.Config{
			HTTP:          config.HTTPConfig{Address: ":0"},
			Observability: config.ObservabilityConfig{MetricsPath: "/metrics"},
			Limits:        config.LimitsConfig{MaxRequestBytes: 1 << 20, MaxConcurrentGoRuns: 16},
			Instance:      config.InstanceConfig{Writable: true},
			Auth:          config.AuthConfig{APIKeyHashSecret: "test-secret"},
		},
		Logger:          observability.NewLogger("error", "json", nil),
		Health:          &fakeHealth{},
		Auth:            authSvc,
		Catalog:         cat,
		DatabaseHandler: NewDatabaseHandler(dbSvc, true),
		SQLHandler:      NewSQLHandler(sqlSvc, testSQLLimits(), true),
		DataHandler:     NewDataHandler(&fakeDataService{dbs: []catalog.Database{cat.db}, lease: &fakeSQLLease{}}, true),
		Usage:           &fakeUsage{},
		Audit:           &fakeAudit{},
		LLM:             &fakeLLMSvc{names: []string{"openai"}},
		S3FileStore:     objectstore.NewMemoryFileStore(),
		System:          store,
		Delegations:     delegations,
	}

	// 云助手运行时：CLI 打真实 router（经计数代理）。
	counterSrv := &countingProxy{inner: nil, counter: counter}
	e := NewRouter(deps)
	counterSrv.inner = e
	srv := httptest.NewServer(counterSrv)
	t.Cleanup(srv.Close)

	llm := &scriptLLM{}
	rt := &gocloudagent.Runtime{
		LLM: llm, SkillsCLI: true, CLI: agentbridge.InProcessCLI{},
		APIBaseURL: srv.URL, Policy: agentbridge.NewCommandPolicy(), Issuer: agentbridge.NewDelegationIssuer("test-secret", delegations),
		// Confirm 留空：由 NewRouter 注入统一的 confirmHub，
		// 保证 Runtime.Wait 与 handler.Resolve 用同一个 hub。
		ConfirmTimeout: 30 * time.Second, DelegationTTL: time.Minute,
		ToolProtocol: "native", RunTimeout: 2 * time.Minute, MaxIterations: 8,
	}
	deps.CloudAgent = rt
	// CloudAgent 在 NewRouter 内被引用；重新构造以挂载 agent 路由。
	e2 := NewRouter(deps)
	counterSrv.inner = e2

	env := &skillRouterEnv{
		deps: deps, echo: e2, srv: srv, store: store, rt: rt, llm: llm,
		reg: delegations, dataHits: counter,
	}
	// planv6.0 v2 D-C1a：v48 起助手默认只读。本文件验证的是「委托令牌 → 真实
	// 路由」这一段，因此显式开启写权限；助手级只读拒绝由 agent_skills_e2e 覆盖。
	seedWritableAgents(t, e2, store, "proj-1")
	return env
}

// seedWritableAgents 确保项目存在可写助手（router E2E 的写流程前置）。
func seedWritableAgents(t *testing.T, e *echo.Echo, store *systemdb.Store, projectID string) {
	t.Helper()
	// 经真实 handler 播种内置助手，保证与生产路径一致。
	seedBody, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/"+projectID+"/agents", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer sk-router-e2e-key")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	_ = seedBody
	list, err := store.ListCloudAgents(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range list {
		if !a.Writable {
			a.Writable = true
			if _, uerr := store.UpdateCloudAgent(context.Background(), a); uerr != nil {
				t.Fatal(uerr)
			}
		}
	}
}

// countingProxy 记录经过的数据面请求后转发给真实 router。
type countingProxy struct {
	inner   http.Handler
	counter *skillHitCounter
}

func (p *countingProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := &captureWriter{ResponseWriter: w, code: http.StatusOK}
	p.inner.ServeHTTP(rec, r)
	p.counter.record(r.Method, r.URL.Path, rec.code)
}

type captureWriter struct {
	http.ResponseWriter
	code int
}

func (w *captureWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *captureWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// multiKeyRepo 支持多个 API Key 的内存仓储（样例19 需要双主体）。
type multiKeyRepo struct {
	recs []auth.APIKeyRecord
}

func (m *multiKeyRepo) FindByHash(_ context.Context, hash string) (auth.APIKeyRecord, error) {
	for _, r := range m.recs {
		if r.KeyHash == hash {
			return r, nil
		}
	}
	return auth.APIKeyRecord{}, auth.ErrInvalidCredentials
}

// srReq 发送携带 API Key 的真实路由请求；key 为空用 key-A，"b" 用 key-B。
func (env *skillRouterEnv) reqWithKey(key, method, path string, body []byte) *httptest.ResponseRecorder {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, strings.NewReader(string(body)))
		r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	token := "sk-router-e2e-key"
	if key == "b" {
		token = "sk-router-e2e-key-b"
	}
	r.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	rec := httptest.NewRecorder()
	env.echo.ServeHTTP(rec, r)
	return rec
}

// srReq 发送携带 API Key 的真实路由请求。
func (env *skillRouterEnv) req(method, path string, body []byte) *httptest.ResponseRecorder {
	return env.reqWithKey("", method, path, body)
}

// srRun 对真实 router 发起流式 run，读到 end 返回全部事件。
// srRun 对真实 router 发起流式 run，读到 end 返回全部事件。
// 技能由 thread 绑定的助手 module 决定，请求体不再传 skills。
func (env *skillRouterEnv) run(t *testing.T, content string, module string, steps [][]gocloudagent.StreamDelta) []sseEvent {
	t.Helper()
	env.dataHits.reset()
	env.llm.reset(steps)
	thread := env.createThreadFor(t, module)
	body := map[string]any{"content": content, "stream": true}
	raw, _ := json.Marshal(body)
	rec := env.req(http.MethodPost, "/v1/projects/proj-1/agent-threads/"+thread+"/runs", raw)
	if rec.Code != http.StatusOK {
		t.Fatalf("run %d %s", rec.Code, rec.Body.String())
	}
	evs, err := parseSSE(rec.Body.String())
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

// createThreadFor 取某个 module 的内置助手并建绑定 thread。
func (env *skillRouterEnv) createThreadFor(t *testing.T, module string) string {
	t.Helper()
	list, err := env.store.ListCloudAgents(context.Background(), "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range list {
		if a.Module != module || a.BuiltinKey != module {
			continue
		}
		raw, _ := json.Marshal(map[string]any{"title": "t", "agent_id": a.ID})
		rec := env.req(http.MethodPost, "/v1/projects/proj-1/agent-threads", raw)
		if rec.Code != http.StatusCreated {
			t.Fatalf("thread %d %s", rec.Code, rec.Body.String())
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.ID
	}
	t.Fatalf("seeded agent for module %s not found", module)
	return ""
}

func (env *skillRouterEnv) createThread(t *testing.T) string {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"title": "t"})
	rec := env.req(http.MethodPost, "/v1/projects/proj-1/agent-threads", raw)
	if rec.Code != http.StatusCreated {
		t.Fatalf("thread %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.ID
}

// toolStdin 构造带 stdin 的 simplebase 工具参数（planv6.0 D3）。
func toolStdin(id string, argv []string, stdin string) []gocloudagent.StreamDelta {
	raw, _ := json.Marshal(map[string]any{"argv": argv, "stdin": stdin})
	s := string(raw)
	cut := len(s) / 2
	return []gocloudagent.StreamDelta{
		{ToolCall: &gocloudagent.ToolCallDelta{Index: 0, ID: id, Name: "simplebase", ArgsDelta: s[:cut]}},
		{ToolCall: &gocloudagent.ToolCallDelta{Index: 0, ArgsDelta: s[cut:]}},
		{Finish: true},
	}
}

// sseHelpers：lastReason/resultIsError 的 sseEvent 版本（与桩服务版参数类型不同）。
func sseLastReason(evs []sseEvent) string {
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == "end" {
			return evs[i].Reason
		}
	}
	return ""
}

func sseResultIsError(evs []sseEvent, code string) bool {
	for _, ev := range evs {
		if ev.Type != "tool_result" || !ev.IsError {
			continue
		}
		if strings.Contains(ev.Content, code) {
			return true
		}
	}
	return false
}

// 16：@云函数 create → invoke，HTTP 走真实 /gofunctions 与 /go/...，返回 2xx。
// 修复前失败：delegationAllowedResources 用 "functions" 匹配 "gofunctions" 路由 → 403。
func TestAgentSkillsRouterE2E_16_GoFunctionCreateInvoke(t *testing.T) {
	env := newSkillRouterEnv(t)
	src := base64.StdEncoding.EncodeToString([]byte("package main\ntype Req struct{ Name string }\nfunc Hello(req Req) string { return \"hi\" }"))
	evs := env.run(t, "写一个 Hello 云函数并调用", "gofunction",
		[][]gocloudagent.StreamDelta{
			toolStdin("c1", []string{"function", "create", "--name", "hello", "--file", "-", "--source-b64", src}, ""),
			toolDeltas("c2", []string{"function", "invoke", "--name", "hello", "--export", "Hello"}),
			textDeltas("hi"),
		})
	if env.dataHits.count(http.MethodPost, "/gofunctions", true) != 1 {
		t.Fatalf("create gofunction hits=%+v（P0-1：委托白名单与真实路由不匹配）", env.dataHits.hits)
	}
	if env.dataHits.count(http.MethodPost, "/go/", true) != 1 {
		t.Fatalf("invoke /go hits=%+v", env.dataHits.hits)
	}
	if sseLastReason(evs) != "stop" {
		t.Fatalf("events: %+v", evs)
	}
}

// 17：委托令牌直接 POST agent-threads/:id/runs → 403（D2：助手路由不可委托）。
// 修复前失败：白名单放行 agents/agent-threads/agent-runs → 递归与自我确认。
func TestAgentSkillsRouterE2E_17_DelegationCannotCallAgentRoutes(t *testing.T) {
	env := newSkillRouterEnv(t)
	thread := env.createThread(t)
	// 用运行时同款 HMAC 签发委托令牌。
	p := auth.Principal{
		UserID: "u-1", Role: auth.RoleUser, TenantID: "tenant-1",
		ProjectIDs:  map[string]struct{}{"proj-1": {}},
		Permissions: auth.PermissionsForRole(auth.RoleUser),
	}
	tok, err := auth.IssueDelegation("test-secret", p, "proj-1", "run-x", time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"content": "hi", "stream": false})
	paths := []string{
		"/v1/projects/proj-1/agent-threads/" + thread + "/runs",
		"/v1/projects/proj-1/agents",
		"/v1/projects/proj-1/agent-schedules",
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+tok)
		rec := httptest.NewRecorder()
		env.echo.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: expected 403, got %d %s（P0-2：助手路由可被委托令牌调用）", path, rec.Code, rec.Body.String())
		}
	}
	// 取消他人 run 同样拒绝（自我确认收口）。
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/agent-runs/r1/cancel", strings.NewReader("{}"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+tok)
	rec := httptest.NewRecorder()
	env.echo.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cancel: expected 403, got %d（P0-2）", rec.Code)
	}
}

// 18：助手模式 function create --file /etc/hosts → file_not_allowed，HTTP 0 次。
// 修复前失败：CLI 直接读宿主机文件并上传。
func TestAgentSkillsRouterE2E_18_HostFileForbiddenInAgentMode(t *testing.T) {
	env := newSkillRouterEnv(t)
	evs := env.run(t, "把宿主机文件传上去", "gofunction",
		[][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"function", "create", "--name", "evil", "--file", "/etc/hosts"}),
			textDeltas("no"),
		})
	if env.dataHits.total() != 0 {
		t.Fatalf("expected 0 HTTP calls, got %+v（P0-3：助手模式读宿主机文件）", env.dataHits.hits)
	}
	if !sseResultIsError(evs, "file_not_allowed") {
		t.Fatalf("expected file_not_allowed, results=%+v", evs)
	}
}

// 19：B 确认 A 发起的 run → 403；A 确认 → 200（D5 发起人校验）。
// key-A 持 ProjectAdmin，key-B 只有读写——B 不是发起人也不是 admin。
//
// planv6.0 v2 BUG-B4 修复后，确认项在 Wait 返回时即被回收，不再有「泄漏的
// pending 条目」。因此本用例必须用真实 HTTP 服务：run 在 StartRun 内阻塞于
// Confirm.Wait，确认请求才能在等待期间命中同一个 pending 条目并放行。
func TestAgentSkillsRouterE2E_19_InitiatorCheck(t *testing.T) {
	env := newSkillRouterEnv(t)
	env.dataHits.reset()
	thread := env.createThreadFor(t, "database")
	env.llm.reset([][]gocloudagent.StreamDelta{
		toolDeltas("c1", []string{"database", "delete", "--id", "db-1"}),
		textDeltas("done"),
	})
	body, _ := json.Marshal(map[string]any{"content": "删掉库", "stream": true})
	endpoint := env.srv.URL + "/v1/projects/proj-1/agent-threads/" + thread + "/runs"
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, "Bearer sk-router-e2e-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run %d", resp.StatusCode)
	}
	// 读 SSE 至 confirmation_required，拿到 runID 与 callID；此后 run 阻塞在等待确认。
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var runID, callID string
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev struct {
			Type   string `json:"type"`
			RunID  string `json:"run_id"`
			CallID string `json:"call_id"`
		}
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev) != nil {
			continue
		}
		if ev.Type == "confirmation_required" {
			runID, callID = ev.RunID, ev.CallID
			break
		}
	}
	if runID == "" || callID == "" {
		t.Fatal("未收到 confirmation_required")
	}
	// B（同项目写权限、无 admin、非发起人）确认 → 403。
	confirmBody, _ := json.Marshal(map[string]any{"approve": true})
	rec := env.reqWithKey("b", http.MethodPost, "/v1/projects/proj-1/agent-runs/"+runID+"/confirmations/"+callID, confirmBody)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("B confirm: expected 403, got %d %s（D5：非发起人可确认）", rec.Code, rec.Body.String())
	}
	// B 取消 A 的 run → 403。
	rec = env.reqWithKey("b", http.MethodPost, "/v1/projects/proj-1/agent-runs/"+runID+"/cancel", []byte("{}"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("B cancel: expected 403, got %d（D5：非发起人可取消）", rec.Code)
	}
	// A（发起人）确认 → 200，run 随后结束。
	rec = env.req(http.MethodPost, "/v1/projects/proj-1/agent-runs/"+runID+"/confirmations/"+callID, confirmBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("A confirm: expected 200, got %d %s", rec.Code, rec.Body.String())
	}
	for sc.Scan() {
		if strings.Contains(sc.Text(), `"type":"end"`) {
			break
		}
	}
}

// 20：project 技能的 allow 前缀（D4：只保留 quota / audit 只读两面）。
// `@` 只点名助手后，没有助手 module 对应 project，该技能不再有运行入口，
// 因此这里只校验前缀白名单，不再走真实 /quota 与 /audit 调用链。
func TestAgentSkillsRouterE2E_20_ProjectSkillPrefixes(t *testing.T) {
	if !gocloudagent.AllowArgv([]string{"project"}, []string{"quota", "get"}) ||
		!gocloudagent.AllowArgv([]string{"project"}, []string{"audit", "list"}) {
		t.Fatal("project skill 应放行 quota / audit")
	}
	if gocloudagent.AllowArgv([]string{"project"}, []string{"user", "list"}) {
		t.Fatal("user 命令不应被 project skill 放行（D4）")
	}
	if gocloudagent.AllowArgv([]string{"project"}, []string{"apikey", "create"}) {
		t.Fatal("apikey 命令不应被 project skill 放行（D4）")
	}
}

var _ = io.Discard
