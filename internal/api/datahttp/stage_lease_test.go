// 文件说明：stage_lease_test.go — 带分段计时的 SQL lease 包装（自 internal/api/stage_timing_test.go 随实现迁入）

package datahttp

import (
	"context"
	"database/sql"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"testing"
	"time"

	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/godatabase"
)

// —— timedLease ——

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
	timer := httpkit.NewStageTimer(context.Background())
	ctx := timer.WithContext(context.Background())
	lease := WrapTimedLease(ctx, newFakeLease(t))
	if _, ok := lease.(*timedLease); !ok {
		t.Fatalf("want timedLease, got %T", lease)
	}
	if _, err := lease.Execute(ctx, godatabase.Statement{SQL: "CREATE TABLE t(id INT)"}); err != nil {
		t.Fatal(err)
	}
	res, err := lease.Query(ctx, godatabase.Statement{SQL: "SELECT 1 AS v"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("rows=%d", len(res.Rows))
	}
	if d := snapshotT(timer, httpkit.StageDBExec); d <= 0 {
		t.Fatal("db_exec not recorded")
	}
	if timer.Meta("rows") != 1 {
		t.Fatalf("rows meta=%d", timer.Meta("rows"))
	}

	if _, err := lease.Batch(ctx, []godatabase.Statement{{SQL: "INSERT INTO t VALUES (1)"}}, true); err != nil {
		t.Fatal(err)
	}
	lease.NotifyWrite(ctx) // 记入 on_write
	if d := snapshotT(timer, httpkit.StageOnWrite); d < 0 {
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

func (f *fakeLease) Query(ctx context.Context, stmt godatabase.Statement, maxRows int) (godatabase.QueryResult, error) {
	return godatabase.Query(ctx, f.db, stmt, maxRows)
}

func (f *fakeLease) Execute(ctx context.Context, stmt godatabase.Statement) (godatabase.QueryResult, error) {
	return godatabase.Execute(ctx, f.db, stmt)
}

func (f *fakeLease) Batch(ctx context.Context, stmts []godatabase.Statement, transactional bool) ([]godatabase.QueryResult, error) {
	return godatabase.Batch(ctx, f.db, stmts, transactional)
}

func (f *fakeLease) Raw() *sql.DB { return f.db }

func (f *fakeLease) NotifyWrite(ctx context.Context) {}

// —— observability 指标 ——

// snapshotT 是测试辅助：读取单段累计。
func snapshotT(t *httpkit.StageTimer, stage httpkit.Stage) time.Duration {
	spent, _ := t.Snapshot()
	return spent[stage]
}
