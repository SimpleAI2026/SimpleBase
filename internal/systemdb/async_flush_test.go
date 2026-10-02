package systemdb

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/uglyer/go-sqlite3"
)

func newFlushTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ApplySystemMigrations(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return &Store{db: db}, db
}

// P1-B：日志 + 指标 + 聚合桶合入一次 flushAllAsync 调用全部落盘。
func TestFlushAllAsyncCombined(t *testing.T) {
	store, db := newFlushTestStore(t)
	ctx := context.Background()

	store.RecordLog(LogEvent{ProjectID: "p1", Logger: "http", Message: "POST /v1/x"})
	store.RecordMetric(MetricSample{ProjectID: "p1", Name: "http_requests", Value: 1})
	store.RecordMetric(MetricSample{ProjectID: "p1", Name: "custom_metric", Value: 2})

	if err := store.flushAllAsync(ctx); err != nil {
		t.Fatal(err)
	}
	events, err := store.QueryLogs(ctx, LogQuery{ProjectID: "p1", Limit: 10})
	if err != nil || len(events) != 1 {
		t.Fatalf("logs n=%d err=%v", len(events), err)
	}
	var metricRows int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_metric_samples`).Scan(&metricRows); err != nil {
		t.Fatal(err)
	}
	// custom_metric 1 行 + http_requests 聚合 1 行
	if metricRows != 2 {
		t.Fatalf("metric rows=%d want 2", metricRows)
	}
	// 缓冲已排空
	if len(store.drainLogs()) != 0 || len(store.drainMetrics()) != 0 {
		t.Fatal("buffers should be drained")
	}
	// 空批 no-op
	if err := store.flushAllAsync(ctx); err != nil {
		t.Fatal(err)
	}
}

// P1-B：任一批次失败时整体回滚并退回缓冲，下轮可重试。
func TestFlushAllAsyncRequeueOnError(t *testing.T) {
	store, db := newFlushTestStore(t)
	ctx := context.Background()

	store.RecordLog(LogEvent{ProjectID: "p1", Logger: "http", Message: "POST /v1/x"})
	store.RecordMetric(MetricSample{ProjectID: "p1", Name: "custom_metric", Value: 2})

	// 破坏日志表以制造写入失败；指标同事务回滚。
	if _, err := db.ExecContext(ctx, `DROP TABLE sys_log_events`); err != nil {
		t.Fatal(err)
	}
	if err := store.flushAllAsync(ctx); err == nil {
		t.Fatal("want error")
	}
	if got := len(store.drainLogs()); got != 1 {
		t.Fatalf("logs requeued=%d want 1", got)
	}
	if got := len(store.drainMetrics()); got != 1 {
		t.Fatalf("metrics requeued=%d want 1", got)
	}
	// 指标未落盘（事务已回滚）
	var metricRows int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_metric_samples`).Scan(&metricRows); err != nil {
		t.Fatal(err)
	}
	if metricRows != 0 {
		t.Fatalf("metric rows=%d want 0（应随事务回滚）", metricRows)
	}
}

// db 为 nil 时退回缓冲且不 panic。
func TestFlushAllAsyncNilDB(t *testing.T) {
	store := &Store{}
	store.RecordLog(LogEvent{ProjectID: "p1", Message: "x"})
	if err := store.flushAllAsync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := len(store.drainLogs()); got != 1 {
		t.Fatalf("logs requeued=%d want 1", got)
	}
}
