// 文件说明：agent_kernel_helpers_test.go — 留在 api 的路由级用例（经 NewRouter）需要的夹具：
// 内存库、SSE 帧解析、可脚本化 ChatClient、SQL / 数据服务假实现。
// handler 级 harness 已随实现迁到 internal/api/agenthttp，数据面假实现的主体在 internal/api/datahttp。
//
// 用法
//
//	openMemoryDB(t) / openAgentStore(t)        // 测试独占的 sqlite 内存库 / 系统库
//	scriptLLM / stepDelta / toolDeltas / textDeltas   // 可脚本化的 ChatClient 与帧构造
//	parseSSEFrame / readSSE / sseEv             // SSE 响应解析
//	fakeSQLService / fakeSQLLease / fakeDataService / testSQLLimits   // 数据面端口假实现（与 datahttp 测试同形）
//
// 调用架构
//
//	[上游] agent_skills_router_e2e_test.go、oai_e2e_test.go、router_stack_test.go、mcp_builtin_admin_test.go 等
//	         │ 同包调用
//	         ▼
//	[本文件] internal/api/agent_kernel_helpers_test.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api/httpkit、internal/api/datahttp（类型别名）、godatabase、gocloudagent
//
// 主要符号
//
//	openMemoryDB / openAgentStore / scriptLLM / stepDelta / toolDeltas / parseSSEFrame / readSSE
//	fakeSQLService / fakeSQLLease / fakeDataService / testSQLLimits

package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	_ "github.com/uglyer/go-sqlite3"
)

var memoryDBSeqKernel atomic.Uint64

// openMemoryDB 打开本测试独占的 sqlite 内存库。
// 裸 `:memory:` 下每个连接都是一份空库。流式 run 占着第一条连接时，
// 轮询 ListAgentRuns 会另开连接，于是永远看不到刚写入的 run。
func openMemoryDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:sbktest%d?mode=memory&cache=shared", memoryDBSeqKernel.Add(1))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxIdleConns(4)
	pin, err := db.Conn(context.Background())
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pin.Close()
		_ = db.Close()
	})
	return db
}

func pathMatchesProject(path, project string) bool {
	if strings.HasPrefix(path, "/go/") {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		return len(parts) >= 2 && parts[1] == project
	}
	return strings.Contains(path, "/projects/"+project+"/") || strings.HasSuffix(path, "/projects/"+project)
}

func skillWrite(method, path string, body []byte) bool {
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

func skillPayload(method, path string) (int, string) {
	switch {
	case method == http.MethodPost && strings.HasSuffix(path, "/databases"):
		return http.StatusCreated, `{"id":"shop","name":"shop","status":"ready"}`
	case strings.Contains(path, "/schema/tables"):
		return http.StatusCreated, `{"name":"orders"}`
	case strings.Contains(path, "/execute"):
		return http.StatusOK, `{"rows_affected":1}`
	case strings.Contains(path, "/query"):
		return http.StatusOK, `{"columns":["amount"],"rows":[[10]],"row_count":1}`
	case strings.Contains(path, "/kv"):
		return http.StatusOK, `{"ok":true}`
	case strings.Contains(path, "/trigger"):
		return http.StatusAccepted, `{"status":"accepted"}`
	case strings.Contains(path, "/cron-jobs"):
		return http.StatusCreated, `{"id":"job1"}`
	case strings.HasPrefix(path, "/go/"):
		return http.StatusOK, `{"result":"hi"}`
	case strings.Contains(path, "/gofunctions"):
		return http.StatusCreated, `{"name":"hello"}`
	case strings.Contains(path, "/s3/objects"):
		return http.StatusCreated, `{"key":"readme","size":5}`
	default:
		return http.StatusOK, `{"ok":true}`
	}
}

func writeAPI(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": code}})
}

func toolDeltas(id string, argv []string) []cloudagent.StreamDelta {
	raw, _ := json.Marshal(map[string]any{"argv": argv})
	s := string(raw)
	cut := len(s) / 2
	return []cloudagent.StreamDelta{
		{ToolCall: &cloudagent.ToolCallDelta{Index: 0, ID: id, Name: "simplebase", ArgsDelta: s[:cut]}},
		{ToolCall: &cloudagent.ToolCallDelta{Index: 0, ArgsDelta: s[cut:]}},
		{Finish: true},
	}
}

// toolStdinDeltas 构造带内联 stdin 的工具调用（planv6.0 D3：--file -）。
func toolStdinDeltas(id string, argv []string, stdin string) []cloudagent.StreamDelta {
	raw, _ := json.Marshal(map[string]any{"argv": argv, "stdin": stdin})
	s := string(raw)
	cut := len(s) / 2
	return []cloudagent.StreamDelta{
		{ToolCall: &cloudagent.ToolCallDelta{Index: 0, ID: id, Name: "simplebase", ArgsDelta: s[:cut]}},
		{ToolCall: &cloudagent.ToolCallDelta{Index: 0, ArgsDelta: s[cut:]}},
		{Finish: true},
	}
}

func textDeltas(text string) []cloudagent.StreamDelta {
	return []cloudagent.StreamDelta{{Content: text, Finish: true}}
}

// mustListAgents 返回某项目的全部助手（测试辅助）。
func mustListAgents(t *testing.T, store *systemdb.Store) []systemdb.CloudAgent {
	t.Helper()
	list, err := store.ListCloudAgents(context.Background(), "testprj1")
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func hasType(evs []sseEv, typ string) bool {
	for _, ev := range evs {
		if ev.Type == typ {
			return true
		}
	}
	return false
}

func typesOf(evs []sseEv) string {
	var b strings.Builder
	for _, ev := range evs {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(ev.Type)
	}
	return b.String()
}

func lastReason(evs []sseEv) string {
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == "end" {
			reason, _ := evs[i].Raw["reason"].(string)
			return reason
		}
	}
	return ""
}

func joinResults(evs []sseEv) string {
	var b strings.Builder
	for _, ev := range evs {
		if ev.Type == "tool_result" || ev.Type == "error" {
			b.WriteString(ev.Data)
		}
	}
	return b.String()
}

func resultIsError(evs []sseEv, code string) bool {
	for _, ev := range evs {
		if ev.Type != "tool_result" {
			continue
		}
		if ev.Raw["is_error"] != true {
			continue
		}
		if strings.Contains(ev.Data, code) {
			return true
		}
	}
	return false
}

func callAndRun(t *testing.T, evs []sseEv) (string, string) {
	t.Helper()
	for i := len(evs) - 1; i >= 0; i-- {
		ev := evs[i]
		if ev.Type == "confirmation_required" || ev.Type == "tool_start" || ev.Type == "run" {
			callID, _ := ev.Raw["call_id"].(string)
			runID, _ := ev.Raw["run_id"].(string)
			if ev.Type == "run" && runID != "" && callID == "" {
				continue
			}
			if runID != "" {
				return callID, runID
			}
		}
	}
	// run id is on every event written by streamRun.
	for _, ev := range evs {
		if runID, _ := ev.Raw["run_id"].(string); runID != "" {
			callID, _ := ev.Raw["call_id"].(string)
			return callID, runID
		}
	}
	t.Fatalf("no run id in %s", typesOf(evs))
	return "", ""
}

type sseEv struct {
	ID   int64
	Type string
	Raw  map[string]any
	Data string
}

type skillHit struct {
	Method string
	Path   string
	Status int
}

type skillData struct {
	mu     sync.Mutex
	hits   []skillHit
	reg    *auth.DelegationRegistry
	secret string
}

type stepDelta struct {
	deltas []cloudagent.StreamDelta
	i      int
}

func (d *skillData) reset() {
	d.mu.Lock()
	d.hits = nil
	d.mu.Unlock()
}
func (d *skillData) count(method, pathPart string, only2xx bool) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, h := range d.hits {
		if h.Method == method && strings.Contains(h.Path, pathPart) && (!only2xx || (h.Status >= 200 && h.Status < 300)) {
			n++
		}
	}
	return n
}
func (d *skillData) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	claims, err := auth.VerifyJWT(d.secret, token, time.Now())
	if err != nil || d.reg.Revoked(claims.JWTID, time.Now()) {
		writeAPI(w, http.StatusUnauthorized, "invalid_token")
		d.record(r.Method, r.URL.Path, http.StatusUnauthorized)
		return
	}
	principal, err := auth.PrincipalFromDelegation(claims)
	if err != nil || claims.ProjectID == "" || !pathMatchesProject(r.URL.Path, claims.ProjectID) {
		writeAPI(w, http.StatusForbidden, "forbidden")
		d.record(r.Method, r.URL.Path, http.StatusForbidden)
		return
	}
	if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "simplebase-system") {
		writeAPI(w, http.StatusForbidden, "system_database_protected")
		d.record(r.Method, r.URL.Path, http.StatusForbidden)
		return
	}
	if skillWrite(r.Method, r.URL.Path, body) {
		if _, ok := principal.Permissions[auth.DatabaseWrite]; !ok {
			writeAPI(w, http.StatusForbidden, "forbidden")
			d.record(r.Method, r.URL.Path, http.StatusForbidden)
			return
		}
	}
	if strings.Contains(string(body), "slow") {
		d.record(r.Method, r.URL.Path, 0)
		<-r.Context().Done()
		return
	}
	status, payload := skillPayload(r.Method, r.URL.Path)
	d.record(r.Method, r.URL.Path, status)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(payload))
}
func (d *skillData) record(method, path string, status int) {
	d.mu.Lock()
	d.hits = append(d.hits, skillHit{method, path, status})
	d.mu.Unlock()
}

func (s *stepDelta) NextDelta() (cloudagent.StreamDelta, error) {
	if s.i >= len(s.deltas) {
		return cloudagent.StreamDelta{}, io.EOF
	}
	d := s.deltas[s.i]
	s.i++
	return d, nil
}
func (s *stepDelta) Next() (string, bool, error) {
	d, err := s.NextDelta()
	if err != nil {
		return "", true, err
	}
	return d.Content, d.Finish, nil
}

// openAgentStore 打开带系统 schema 的内存系统库（单连接；与 agenthttp 测试夹具同形）。
func openAgentStore(t *testing.T) *systemdb.Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return systemdb.NewStoreForTest(db)
}

// serviceCallLog 记录 fakeSQLService 的调用次数，供表征测试断言门禁顺序。
type serviceCallLog struct {
	get     int
	acquire int
}

// fakeSQLService 是 SQLService 的内存假实现。
type fakeSQLService struct {
	db           catalog.Database
	getErr       error
	acquireErr   error
	acquiredMode godatabase.AccessMode
	lease        *fakeSQLLease
	calls        *serviceCallLog
}

func (f *fakeSQLService) GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	if f.calls != nil {
		f.calls.get++
	}
	if f.getErr != nil {
		return catalog.Database{}, f.getErr
	}
	return f.db, nil
}
func (f *fakeSQLService) Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (SQLLease, error) {
	if f.calls != nil {
		f.calls.acquire++
	}
	if f.acquireErr != nil {
		return nil, f.acquireErr
	}
	f.acquiredMode = mode
	if f.lease == nil {
		f.lease = &fakeSQLLease{}
	}
	return f.lease, nil
}

// fakeSQLLease 模拟 SQLLease，可预设查询结果或错误。
type fakeSQLLease struct {
	released       bool
	queryResult    godatabase.QueryResult
	queryErr       error
	executeResult  godatabase.QueryResult
	executeErr     error
	batchResults   []godatabase.QueryResult
	batchErr       error
	lastQueryStmt  godatabase.Statement
	queryStmts     []godatabase.Statement
	queryQueue     []godatabase.QueryResult
	lastExecStmt   godatabase.Statement
	execStmts      []godatabase.Statement
	lastBatchStmts []godatabase.Statement
	raw            *sql.DB
	notifyCount    int
}

func (l *fakeSQLLease) Query(ctx context.Context, stmt godatabase.Statement, maxRows int) (godatabase.QueryResult, error) {
	l.lastQueryStmt = stmt
	l.queryStmts = append(l.queryStmts, stmt)
	if len(l.queryQueue) > 0 {
		result := l.queryQueue[0]
		l.queryQueue = l.queryQueue[1:]
		return result, nil
	}
	return l.queryResult, l.queryErr
}
func (l *fakeSQLLease) Execute(ctx context.Context, stmt godatabase.Statement) (godatabase.QueryResult, error) {
	l.lastExecStmt = stmt
	l.execStmts = append(l.execStmts, stmt)
	return l.executeResult, l.executeErr
}
func (l *fakeSQLLease) Batch(ctx context.Context, stmts []godatabase.Statement, transactional bool) ([]godatabase.QueryResult, error) {
	l.lastBatchStmts = stmts
	return l.batchResults, l.batchErr
}

func testSQLLimits() SQLLimits {
	return SQLLimits{
		QueryTimeout:       int64(5 * time.Second),
		MaxQueryRows:       100,
		MaxConcurrent:      10,
		MaxBatchStatements: 10,
		MaxSQLBytes:        65536,
		MaxRequestBytes:    1 << 20,
	}
}

type fakeDataService struct {
	dbs          []catalog.Database
	listErr      error
	getErr       error
	acquireErr   error
	lastGetID    string
	listCalled   bool
	acquiredMode godatabase.AccessMode
	lease        *fakeSQLLease
}

func (f *fakeDataService) GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	f.lastGetID = databaseID
	if f.getErr != nil {
		return catalog.Database{}, f.getErr
	}
	for _, db := range f.dbs {
		if db.ID == databaseID {
			return db, nil
		}
	}
	return catalog.Database{}, catalog.ErrNotFound
}
func (f *fakeDataService) ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error) {
	f.listCalled = true
	if f.listErr != nil {
		return nil, "", f.listErr
	}
	if page.Limit > 0 && page.Limit < len(f.dbs) {
		return f.dbs[:page.Limit], "", nil
	}
	return f.dbs, "", nil
}
func (f *fakeDataService) Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (SQLLease, error) {
	if f.acquireErr != nil {
		return nil, f.acquireErr
	}
	f.acquiredMode = mode
	if f.lease == nil {
		f.lease = &fakeSQLLease{}
	}
	return f.lease, nil
}

func (l *fakeSQLLease) Release() { l.released = true }

// Raw/NotifyWrite 为 KV 服务预留；测试假实现返回 nil/no-op。
func (l *fakeSQLLease) Raw() *sql.DB { return l.raw }

func (l *fakeSQLLease) NotifyWrite(ctx context.Context) { l.notifyCount++ }

// fakeDBService 是 DatabaseService 的内存假实现，用于 handler 测试。
type fakeDBService struct {
	dbs map[string]catalog.Database // key = databaseID

	createErr error
	getErr    error
	listErr   error
	deleteErr error
}

func (f *fakeDBService) CreateDatabase(ctx context.Context, in catalog.CreateDatabaseInput) (catalog.Database, error) {
	if f.createErr != nil {
		return catalog.Database{}, f.createErr
	}
	db := catalog.Database{
		ID:        "db-" + in.Name,
		TenantID:  in.TenantID,
		ProjectID: in.ProjectID,
		Name:      in.Name,
		DataModel: in.DataModel,
		Status:    catalog.DatabaseReady,
	}
	f.dbs[db.ID] = db
	return db, nil
}
func (f *fakeDBService) GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	if f.getErr != nil {
		return catalog.Database{}, f.getErr
	}
	db, ok := f.dbs[databaseID]
	if !ok {
		return catalog.Database{}, catalog.ErrNotFound
	}
	return db, nil
}
func (f *fakeDBService) ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error) {
	if f.listErr != nil {
		return nil, "", f.listErr
	}
	var out []catalog.Database
	for _, db := range f.dbs {
		if db.ProjectID == projectID {
			out = append(out, db)
		}
	}
	return out, "", nil
}
func (f *fakeDBService) BeginDeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	if f.deleteErr != nil {
		return catalog.Database{}, f.deleteErr
	}
	db, ok := f.dbs[databaseID]
	if !ok {
		return catalog.Database{}, catalog.ErrNotFound
	}
	db.Status = catalog.DatabaseDeleting
	f.dbs[databaseID] = db
	return db, nil
}
func (f *fakeDBService) DeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	db, err := f.BeginDeleteDatabase(ctx, principal, projectID, databaseID)
	if err != nil {
		return catalog.Database{}, err
	}
	db.Status = catalog.DatabaseDeleted
	if f.dbs != nil {
		f.dbs[databaseID] = db
	}
	return db, nil
}
func newFakeDBService() *fakeDBService {
	return &fakeDBService{dbs: make(map[string]catalog.Database)}
}

// doRequest 发送 JSON 请求并返回记录器（与 datahttp 测试同形）。
func doRequest(e *echo.Echo, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func (s *stepDelta) Close() error { return nil }
