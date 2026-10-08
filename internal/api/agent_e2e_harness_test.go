package api

// agent_e2e_harness_test.go：云 Agent 全链路测试基建（planv4.1 §5.1）。
// 真实链路：router(handler) → cloudagent.Runtime → llmgateway.Service → HTTP fake upstream。
// 不访问外网、无需真实 key；上游为 internal/testutil/fakellm。

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

// harnessDB 实现 cloudagent.DatabaseAccess，记录调用。
type harnessDB struct {
	mu        sync.Mutex
	listCalls int
	sqlCalls  int
	blockFor  time.Duration // list_databases 阻塞时长（测心跳）
	databases []cloudagent.DatabaseInfo
	sqlError  string // 非空时 ReadOnlyQuery 返回该错误（sqlguard 拒绝模拟）
}

func (f *harnessDB) ListDatabases(context.Context, auth.Principal, string) ([]cloudagent.DatabaseInfo, error) {
	f.mu.Lock()
	f.listCalls++
	block := f.blockFor
	f.mu.Unlock()
	if block > 0 {
		time.Sleep(block)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.databases != nil {
		return f.databases, nil
	}
	return []cloudagent.DatabaseInfo{{ID: "d1", Name: "orders", Status: "ready"}, {ID: "d2", Name: "users", Status: "ready"}}, nil
}

func (f *harnessDB) ListCollections(context.Context, auth.Principal, string, string) ([]string, error) {
	return []string{"t1"}, nil
}

func (f *harnessDB) ReadOnlyQuery(_ context.Context, _ auth.Principal, _, _, sqlText string, _ int) (cloudagent.SQLResult, error) {
	f.mu.Lock()
	f.sqlCalls++
	errText := f.sqlError
	f.mu.Unlock()
	if errText != "" {
		return cloudagent.SQLResult{}, &sqlguardError{msg: errText}
	}
	return cloudagent.SQLResult{Columns: []string{"id"}, Rows: [][]any{{1}}, RowCount: 1}, nil
}

type sqlguardError struct{ msg string }

func (e *sqlguardError) Error() string { return e.msg }

// harnessLogs 实现 cloudagent.LogAccess。
type harnessLogs struct {
	mu    sync.Mutex
	calls int
}

func (f *harnessLogs) SearchLogs(context.Context, string, string, string, int) ([]systemdb.LogEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return []systemdb.LogEvent{{Level: "error", Logger: "app", Message: "boom"}}, nil
}

func (f *harnessLogs) LevelStats(context.Context, string) ([]systemdb.LogLevelCount, error) {
	return []systemdb.LogLevelCount{{Level: "error", Count: 3}}, nil
}

// agentHarness 聚合全链路依赖。
type agentHarness struct {
	store   *systemdb.Store
	echo    *echo.Echo
	runtime *cloudagent.Runtime
	fake    *fakellm.Server
	db      *harnessDB
	logs    *harnessLogs
}

// newAgentHarness 构造全链路测试环境：
// fake upstream URL → FallbackResolver → llmgateway.Service → Runtime → 真实 handler。
// thinkingInterval 为 streamRun 心跳注入间隔（仅测试）；0 使用默认。
func newAgentHarness(t *testing.T, scripts []fakellm.Script, def fakellm.Script) *agentHarness {
	t.Helper()
	fake := fakellm.New(scripts, def)
	t.Cleanup(fake.Close)

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.ApplySystemMigrations(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := systemdb.NewStoreForTest(db)

	resolver := llmgateway.NewFallbackResolver(nil, map[string]llmgateway.InstanceProvider{"openai": {
		APIKey: "test-key", BaseURL: fake.URL + "/v1", DefaultModel: "fake-model", AllowedModels: []string{"fake-model"},
	}})
	gw := llmgateway.NewService(resolver, nil, nil)

	hdb := &harnessDB{}
	hlogs := &harnessLogs{}
	rt := &cloudagent.Runtime{
		LLM: NewCloudAgentLLM(NewLLMService(gw)),
		DB:  hdb, Obj: nil, Logs: hlogs,
		ToolProtocol: "native",
		RunTimeout:   30 * time.Second,
	}

	e := echo.New()
	e.HideBanner = true
	h := &cloudAgentHandler{store: store, runtime: rt, llm: NewLLMService(gw)}
	registerAgentRoutes(e, h)
	return &agentHarness{store: store, echo: e, runtime: rt, fake: fake, db: hdb, logs: hlogs}
}

// registerAgentRoutes 挂载与生产一致的 agent 路由（免认证，上下文由 hReq 注入）。
func registerAgentRoutes(e *echo.Echo, h *cloudAgentHandler) {
	e.GET("/agents/modules", h.ListModules)
	e.GET("/agents", h.ListAgents)
	e.POST("/agents", h.CreateAgent)
	e.GET("/agents/:agentID", h.GetAgent)
	e.PATCH("/agents/:agentID", h.PatchAgent)
	e.DELETE("/agents/:agentID", h.DeleteAgent)
	e.GET("/agent-threads", h.ListThreads)
	e.POST("/agent-threads", h.CreateThread)
	e.GET("/agent-threads/:threadID", h.GetThread)
	e.PATCH("/agent-threads/:threadID", h.PatchThread)
	e.DELETE("/agent-threads/:threadID", h.DeleteThread)
	e.GET("/agent-threads/:threadID/messages", h.ListMessages)
	e.GET("/agent-threads/:threadID/runs", h.ListThreadRuns)
	e.POST("/agent-threads/:threadID/runs", h.CreateRun)
	e.POST("/agent-runs/:runID/cancel", h.CancelRun)
	e.GET("/agents/models", h.ListAgentModels)
}

// hReq 发送带项目/身份上下文的请求。
func (hs *agentHarness) req(method, path string, body any) *httptest.ResponseRecorder {
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, strings.NewReader(string(b)))
		r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	ctx := WithProject(r.Context(), ProjectContext{ID: catalog.DevProjectID, TenantID: catalog.ReservedTenantID})
	ctx = WithPrincipal(ctx, auth.Principal{APIKeyID: "k1", TenantID: catalog.ReservedTenantID})
	r = r.WithContext(ctx)
	rec := httptest.NewRecorder()
	hs.echo.ServeHTTP(rec, r)
	return rec
}

// createThread 建会话并返回 id。
func (hs *agentHarness) createThread(t *testing.T) string {
	t.Helper()
	rec := hs.req(http.MethodPost, "/agent-threads", map[string]any{"title": "t"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create thread: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.ID
}

// createRun 发起流式 run 并返回 SSE 事件流。
func (hs *agentHarness) createRun(t *testing.T, threadID, content string, extra map[string]any) []sseEvent {
	t.Helper()
	body := map[string]any{"content": content, "stream": true}
	for k, v := range extra {
		body[k] = v
	}
	rec := hs.req(http.MethodPost, "/agent-threads/"+threadID+"/runs", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("create run: %d %s", rec.Code, rec.Body.String())
	}
	events, err := parseSSE(rec.Body.String())
	if err != nil {
		t.Fatal(err)
	}
	return events
}

// runIDOf 从事件流提取 run_id。
func runIDOf(events []sseEvent) string {
	for _, ev := range events {
		if ev.Type == "run" {
			return ev.RunID
		}
	}
	return ""
}

// eventsOf 过滤指定类型事件。
func eventsOf(events []sseEvent, typ string) []sseEvent {
	out := make([]sseEvent, 0, 4)
	for _, ev := range events {
		if ev.Type == typ {
			out = append(out, ev)
		}
	}
	return out
}
