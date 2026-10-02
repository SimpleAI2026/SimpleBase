// stage_timing_lease.go 把租约的 Query/Execute/Batch 包装为计时版本，
// 将 plan §1 的 S7 拆分为 db_conn_queue（单连接排队）与 db_exec（执行+扫描）。
//
// 排队时间通过 *sql.DB.Stats() 的 WaitDuration 增量估算：每库
// MaxOpenConns=1（factory.go），Stats 变化即反映本语句前的排队。
// 计时关闭时 WrapTimedLease 原样返回，零开销。
package api

import (
	"context"
	"database/sql"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/database"
)

// timedLease 包装 SQLLease，把每次 DB 操作耗时计入 StageDBQueue/StageDBExec。
type timedLease struct {
	inner SQLLease
}

// WrapTimedLease 在启用分段计时且 ctx 上存在 timer 时包装 lease；
// 否则原样返回（no-op）。
func WrapTimedLease(ctx context.Context, lease SQLLease) SQLLease {
	if lease == nil || StageTimerFrom(ctx) == nil {
		return lease
	}
	return &timedLease{inner: lease}
}

// observeDB 包裹一次 DB 调用：排队增量记 StageDBQueue，其余记 StageDBExec。
func (t *timedLease) observeDB(ctx context.Context, conn *sql.DB, op func() error) error {
	timer := StageTimerFrom(ctx)
	if timer == nil {
		return op()
	}
	var before sql.DBStats
	if conn != nil {
		before = conn.Stats()
	}
	start := time.Now()
	err := op()
	elapsed := time.Since(start)
	if conn != nil {
		if waited := conn.Stats().WaitDuration - before.WaitDuration; waited > 0 {
			timer.Observe(StageDBQueue, waited)
			elapsed -= waited
		}
	}
	timer.Observe(StageDBExec, elapsed)
	return err
}

func (t *timedLease) Release() { t.inner.Release() }

func (t *timedLease) Query(ctx context.Context, stmt database.Statement, maxRows int) (database.QueryResult, error) {
	var res database.QueryResult
	err := t.observeDB(ctx, t.inner.Raw(), func() error {
		var qerr error
		res, qerr = t.inner.Query(ctx, stmt, maxRows)
		return qerr
	})
	if err == nil {
		if timer := StageTimerFrom(ctx); timer != nil {
			timer.SetMeta("rows", int64(len(res.Rows)))
		}
	}
	return res, err
}

func (t *timedLease) Execute(ctx context.Context, stmt database.Statement) (database.QueryResult, error) {
	var res database.QueryResult
	err := t.observeDB(ctx, t.inner.Raw(), func() error {
		var xerr error
		res, xerr = t.inner.Execute(ctx, stmt)
		return xerr
	})
	return res, err
}

func (t *timedLease) Batch(ctx context.Context, stmts []database.Statement, transactional bool) ([]database.QueryResult, error) {
	var res []database.QueryResult
	err := t.observeDB(ctx, t.inner.Raw(), func() error {
		var berr error
		res, berr = t.inner.Batch(ctx, stmts, transactional)
		return berr
	})
	return res, err
}

func (t *timedLease) Raw() *sql.DB { return t.inner.Raw() }

// NotifyWrite 计入 StageOnWrite（写后同步链路）。
func (t *timedLease) NotifyWrite(ctx context.Context) {
	timer := StageTimerFrom(ctx)
	if timer == nil {
		t.inner.NotifyWrite(ctx)
		return
	}
	scope := timer.StageScope(StageOnWrite)
	defer scope.Done()
	t.inner.NotifyWrite(ctx)
}
