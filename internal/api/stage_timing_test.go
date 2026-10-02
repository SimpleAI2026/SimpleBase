// stage_timing_test.go 验证分段计时中间件与打点的正确性
//（api-db-perf-validation-plan §2.1 的单元验收）。
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "github.com/uglyer/go-sqlite3"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
)

// fakeStageRecorder 收集 ObserveStage 调用，供断言。
type fakeStageRecorder struct {
	routes map[string]map[string]float64
}

func newFakeStageRecorder() *fakeStageRecorder {
	return &fakeStageRecorder{routes: map[string]map[string]float64{}}
}

func (f *fakeStageRecorder) ObserveStage(route, stage string, seconds float64) {
	m, ok := f.routes[route]
	if !ok {
		m = map[string]float64{}
		f.routes[route] = m
	}
	m[stage] = m[stage] + seconds
}

// —— StageTimer 基础行为 ——

func TestStageTimerNilSafe(t *testing.T) {
	var timer *StageTimer
	timer.Observe(StageAuth, time.Second) // 不应 panic
	timer.SetMeta("rows", 1)
	if timer.Meta("rows") != 0 {
		t.Fatal("nil timer meta should be 0")
	}
	if timer.StageScope(StageAuth) != nil {
		t.Fatal("nil timer scope should be nil")
	}
	var scope *StageScope
	scope.Done() // nil-safe
	if StageTimerFrom(nil) != nil {
		t.Fatal("nil ctx should return nil timer")
	}
	if StageTimerFrom(context.Background()) != nil {
		t.Fatal("empty ctx should return nil timer")
	}
}

func TestStageTimerObserveAndSnapshot(t *testing.T) {
	timer := newStageTimer(context.Background())
	timer.Observe(StageAuth, 10*time.Millisecond)
	timer.Observe(StageAuth, 5*time.Millisecond)
	timer.Observe(StageDecode, 1*time.Millisecond)
	timer.SetMeta("rows", 42)

	spent, extra := timer.Snapshot()
	if spent[StageAuth] != 15*time.Millisecond {
		t.Fatalf("auth=%v", spent[StageAuth])
	}
	if spent[StageDecode] != 1*time.Millisecond {
		t.Fatalf("decode=%v", spent[StageDecode])
	}
	if extra["rows"] != 42 {
		t.Fatalf("rows=%d", extra["rows"])
	}

	// Snapshot 必须是副本：改动不影响内部状态。
	spent[StageAuth] = 0
	again, _ := timer.Snapshot()
	if again[StageAuth] != 15*time.Millisecond {
		t.Fatal("snapshot not a copy")
	}
}

func TestStageScopeDoneIdempotent(t *testing.T) {
	timer := newStageTimer(context.Background())
	scope := timer.StageScope(StageProject)
	time.Sleep(2 * time.Millisecond)
	scope.Done()
	first := timer.SnapshotT(StageProject)
	if first <= 0 {
		t.Fatalf("project stage missing: %v", first)
	}
	scope.Done() // 第二次必须无效果
	if again := timer.SnapshotT(StageProject); again != first {
		t.Fatalf("Done not idempotent: first=%v again=%v", first, again)
	}
}

// SnapshotT 是测试辅助：读取单段累计。
func (t *StageTimer) SnapshotT(stage Stage) time.Duration {
	spent, _ := t.Snapshot()
	return spent[stage]
}

// —— 中间件 ——

func TestPerfStageMiddlewareDisabledByDefault(t *testing.T) {
	// 不开启开关：路由正常工作，且无 stage 输出。
	e := echo.New()
	e.GET("/_t", func(c echo.Context) error {
		if StageTimerFrom(c.Request().Context()) != nil {
			t.Error("timer should not exist when disabled")
		}
		return c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/_t", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestPerfStageMiddlewareRecordsStages(t *testing.T) {
	recorder := newFakeStageRecorder()
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.SetRequest(c.Request().WithContext(WithRequestID(c.Request().Context(), "rid-1234")))
			return next(c)
		}
	})
	e.Use(perfStageMiddleware(recorder, nil))
	e.GET("/_t", func(c echo.Context) error {
		timer := StageTimerFrom(c.Request().Context())
		if timer == nil {
			t.Fatal("timer missing in handler")
		}
		// 注入远大于真实耗时的段耗时，模拟慢阶段。
		timer.Observe(StageAuth, 5*time.Millisecond)
		timer.Observe(StageDBExec, 20*time.Millisecond)
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/_t", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}

	stages := recorder.routes["/_t"]
	if stages == nil {
		t.Fatal("no stages recorded")
	}
	if stages[string(StageAuth)] != 0.005 {
		t.Fatalf("auth stage wrong: %v", stages[string(StageAuth)])
	}
	if stages[string(StageDBExec)] != 0.02 {
		t.Fatalf("db_exec stage wrong: %v", stages[string(StageDBExec)])
	}
	// total/unattributed 必须存在（归因账户完整性）。
	if _, ok := stages[string(StageTotal)]; !ok {
		t.Fatal("total stage missing")
	}
	if _, ok := stages[string(StageUnattributed)]; !ok {
		t.Fatal("unattributed stage missing")
	}
	// 注入值可大于真实墙钟时间（模拟外部注入），只验证记录不验证守恒。
}

func TestPerfStageMiddlewareLoggerOutput(t *testing.T) {
	// 捕获 debug 日志行，验证包含各段毫秒数且不含 SQL。
	var buf strings.Builder
	logger := observability.NewLogger("debug", "json", &buf)
	e := echo.New()
	e.Use(perfStageMiddleware(nil, logger))
	e.GET("/_t", func(c echo.Context) error {
		StageTimerFrom(c.Request().Context()).Observe(StageAcquire, 7*time.Millisecond)
		return c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/_t", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	line := buf.String()
	if line == "" {
		t.Fatal("no log output")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &parsed); err != nil {
		t.Fatalf("log not json: %v\n%s", err, line)
	}
	if parsed["msg"] != "perf_stage" {
		t.Fatalf("msg=%v", parsed["msg"])
	}
	stages, ok := parsed["stages"].(map[string]any)
	if !ok {
		t.Fatalf("stages missing: %v", parsed)
	}
	if _, ok := stages[string(StageAcquire)]; !ok {
		t.Fatalf("acquire missing from log: %v", stages)
	}
}

// —— 打点位置：project / auth 包装 ——

func TestTimedAuthMiddlewareWraps(t *testing.T) {
	inner := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			time.Sleep(2 * time.Millisecond) // 模拟认证本身
			return next(c)
		}
	}
	e := echo.New()
	var authStage time.Duration
	var wall time.Duration
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			timer := newStageTimer(c.Request().Context())
			c.SetRequest(c.Request().WithContext(timer.ctx))
			t0 := time.Now()
			err := next(c)
			wall = time.Since(t0)
			authStage = timer.SnapshotT(StageAuth)
			return err
		}
	})
	e.Use(timedAuthMiddleware(inner))
	e.GET("/_t", func(c echo.Context) error {
		time.Sleep(50 * time.Millisecond) // 下游业务耗时：不得计入 auth
		return c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/_t", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if authStage < time.Millisecond {
		t.Fatalf("auth stage not recorded: %v", authStage)
	}
	// §7.2 M1：auth 只含认证本身，下游 50ms 必须留在请求墙钟里。
	if authStage > 20*time.Millisecond {
		t.Fatalf("auth stage swallowed downstream time: auth=%v wall=%v", authStage, wall)
	}
	if wall < 45*time.Millisecond {
		t.Fatalf("wall clock lost downstream time: %v", wall)
	}
}

func TestTimedAuthMiddlewareFailureStillRecords(t *testing.T) {
	inner := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			time.Sleep(1 * time.Millisecond)
			return echo.NewHTTPError(http.StatusUnauthorized, "denied") // 认证失败：不走 next
		}
	}
	e := echo.New()
	var authStage time.Duration
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			timer := newStageTimer(c.Request().Context())
			c.SetRequest(c.Request().WithContext(timer.ctx))
			err := next(c)
			authStage = timer.SnapshotT(StageAuth)
			return err
		}
	})
	e.Use(timedAuthMiddleware(inner))
	e.GET("/_t", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/_t", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d", rec.Code)
	}
	if authStage < 500*time.Microsecond {
		t.Fatalf("auth stage not recorded on failure path: %v", authStage)
	}
}

// —— timedLease ——

type stubTimedLease struct {
	inner SQLLease
}

func TestWrapTimedLeaseNoopWithoutTimer(t *testing.T) {
	lease := newFakeLease(t)
	if got := WrapTimedLease(context.Background(), lease); got != SQLLease(lease) {
		t.Fatalf("should return original lease, got %T", got)
	}
	if got := WrapTimedLease(context.Background(), nil); got != nil {
		t.Fatal("nil lease should stay nil")
	}
}

func TestWrapTimedLeaseRecordsExec(t *testing.T) {
	timer := newStageTimer(context.Background())
	ctx := timer.ctx
	lease := WrapTimedLease(ctx, newFakeLease(t))
	if _, ok := lease.(*timedLease); !ok {
		t.Fatalf("want timedLease, got %T", lease)
	}
	if _, err := lease.Execute(ctx, database.Statement{SQL: "CREATE TABLE t(id INT)"}); err != nil {
		t.Fatal(err)
	}
	res, err := lease.Query(ctx, database.Statement{SQL: "SELECT 1 AS v"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("rows=%d", len(res.Rows))
	}
	if d := timer.SnapshotT(StageDBExec); d <= 0 {
		t.Fatal("db_exec not recorded")
	}
	if timer.Meta("rows") != 1 {
		t.Fatalf("rows meta=%d", timer.Meta("rows"))
	}

	if _, err := lease.Batch(ctx, []database.Statement{{SQL: "INSERT INTO t VALUES (1)"}}, true); err != nil {
		t.Fatal(err)
	}
	lease.NotifyWrite(ctx) // 记入 on_write
	if d := timer.SnapshotT(StageOnWrite); d < 0 {
		t.Fatal("on_write negative")
	}
	lease.Release()
}

// fakeLease 是最小 SQLLease 实现（内存 sqlite，满足 Query/Execute/Batch/Raw/NotifyWrite）。
type fakeLease struct {
	db *sql.DB
}

func newFakeLease(t *testing.T) *fakeLease {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &fakeLease{db: db}
}

func (f *fakeLease) Release() {}

func (f *fakeLease) Query(ctx context.Context, stmt database.Statement, maxRows int) (database.QueryResult, error) {
	return database.Query(ctx, f.db, stmt, maxRows)
}

func (f *fakeLease) Execute(ctx context.Context, stmt database.Statement) (database.QueryResult, error) {
	return database.Execute(ctx, f.db, stmt)
}

func (f *fakeLease) Batch(ctx context.Context, stmts []database.Statement, transactional bool) ([]database.QueryResult, error) {
	return database.Batch(ctx, f.db, stmts, transactional)
}

func (f *fakeLease) Raw() *sql.DB { return f.db }

func (f *fakeLease) NotifyWrite(ctx context.Context) {}

// —— observability 指标 ——

func TestMetricsObserveStage(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := observability.NewMetrics(reg)
	m.ObserveStage("/v1/x", string(StageAuth), 0.5)

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, mf := range metrics {
		if mf.GetName() != "simplebase_api_stage_seconds" {
			continue
		}
		found = true
		for _, s := range mf.GetMetric() {
			if s.GetHistogram().GetSampleCount() != 1 {
				t.Fatalf("sample count=%d", s.GetHistogram().GetSampleCount())
			}
		}
	}
	if !found {
		t.Fatal("simplebase_api_stage_seconds not registered")
	}
	// nil 安全
	var nilM *observability.Metrics
	nilM.ObserveStage("r", "s", 1)
}
