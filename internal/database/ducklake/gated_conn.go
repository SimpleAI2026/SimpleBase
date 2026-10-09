package ducklake

import (
	"context"
	"database/sql/driver"
	"sync/atomic"
)

// gatedConn 在 DuckDB 连接外包一层写锁。database/sql 先借出连接，
// 再进入 Exec/Begin；锁在这里获取，和 sync 的「先 Conn 再 holdWriteGate」
// 是同一顺序。
type gatedConn struct {
	raw     driver.Conn
	gate    *writeGate
	execer  driver.ExecerContext
	queryer driver.QueryerContext
	beginTx driver.ConnBeginTx
	prepCtx driver.ConnPrepareContext
	checker driver.NamedValueChecker
	// txHeld 表示这条连接上的显式事务已经持有写锁。
	// 事务内的后续语句走同一条连接，不能再次加锁（同一 goroutine 会死锁）。
	txHeld atomic.Bool
}

func newGatedConn(raw driver.Conn, gate *writeGate) driver.Conn {
	if gate == nil {
		return raw
	}
	g := &gatedConn{raw: raw, gate: gate}
	g.execer, _ = raw.(driver.ExecerContext)
	g.queryer, _ = raw.(driver.QueryerContext)
	g.beginTx, _ = raw.(driver.ConnBeginTx)
	g.prepCtx, _ = raw.(driver.ConnPrepareContext)
	g.checker, _ = raw.(driver.NamedValueChecker)
	return g
}

func (c *gatedConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.raw.Prepare(query)
	if err != nil {
		return nil, err
	}
	return newGatedStmt(c, query, stmt), nil
}

func (c *gatedConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if c.prepCtx == nil {
		return c.Prepare(query)
	}
	stmt, err := c.prepCtx.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return newGatedStmt(c, query, stmt), nil
}

func (c *gatedConn) Close() error { return c.raw.Close() }

func (c *gatedConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *gatedConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if c.beginTx == nil {
		return nil, driver.ErrSkip
	}
	release, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	c.txHeld.Store(true)
	tx, err := c.beginTx.BeginTx(ctx, opts)
	if err != nil {
		c.txHeld.Store(false)
		release()
		return nil, err
	}
	return &gatedTx{tx: tx, release: func() {
		c.txHeld.Store(false)
		release()
	}}, nil
}

func (c *gatedConn) CheckNamedValue(nv *driver.NamedValue) error {
	if c.checker == nil {
		return driver.ErrSkip
	}
	return c.checker.CheckNamedValue(nv)
}

func (c *gatedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.execer == nil {
		return nil, driver.ErrSkip
	}
	if !isMutatingSQL(query) {
		return c.execer.ExecContext(ctx, query, args)
	}
	release, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	var res driver.Result
	err = c.retry(ctx, func() error {
		var execErr error
		res, execErr = c.execer.ExecContext(ctx, query, args)
		return execErr
	})
	return res, err
}

func (c *gatedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if c.queryer == nil {
		return nil, driver.ErrSkip
	}
	if !isMutatingSQL(query) {
		return c.queryer.QueryContext(ctx, query, args)
	}
	release, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	var rows driver.Rows
	err = c.retry(ctx, func() error {
		var qerr error
		rows, qerr = c.queryer.QueryContext(ctx, query, args)
		return qerr
	})
	return rows, err
}

// acquire 取得写锁。ctx 已由 holdWriteGate 持有同一把锁时不再加锁。
func (c *gatedConn) acquire(ctx context.Context) (func(), error) {
	if c.gate == nil || c.txHeld.Load() || ctx.Value(writeGateHeld{}) == c.gate {
		return func() {}, nil
	}
	c.gate.lock()
	return func() { c.gate.unlock() }, nil
}

func (c *gatedConn) retry(ctx context.Context, fn func() error) error {
	if c.gate == nil || !c.gate.retryConflicts {
		return fn()
	}
	return RetryOnConflict(ctx, fn)
}

type gatedTx struct {
	tx      driver.Tx
	release func()
}

func (t *gatedTx) Commit() error {
	defer t.unlock()
	return t.tx.Commit()
}

func (t *gatedTx) Rollback() error {
	defer t.unlock()
	return t.tx.Rollback()
}

func (t *gatedTx) unlock() {
	if t.release != nil {
		t.release()
		t.release = nil
	}
}

type gatedStmt struct {
	driver.Stmt
	conn    *gatedConn
	query   string
	execer  driver.StmtExecContext
	queryer driver.StmtQueryContext
}

func newGatedStmt(conn *gatedConn, query string, stmt driver.Stmt) driver.Stmt {
	g := &gatedStmt{Stmt: stmt, conn: conn, query: query}
	g.execer, _ = stmt.(driver.StmtExecContext)
	g.queryer, _ = stmt.(driver.StmtQueryContext)
	return g
}

func (s *gatedStmt) Exec(args []driver.Value) (driver.Result, error) {
	if !isMutatingSQL(s.query) {
		return s.Stmt.Exec(args)
	}
	release, err := s.conn.acquire(context.Background())
	if err != nil {
		return nil, err
	}
	defer release()
	var res driver.Result
	err = s.conn.retry(context.Background(), func() error {
		var execErr error
		res, execErr = s.Stmt.Exec(args)
		return execErr
	})
	return res, err
}

func (s *gatedStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if s.execer == nil {
		return nil, driver.ErrSkip
	}
	if !isMutatingSQL(s.query) {
		return s.execer.ExecContext(ctx, args)
	}
	release, err := s.conn.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	var res driver.Result
	err = s.conn.retry(ctx, func() error {
		var execErr error
		res, execErr = s.execer.ExecContext(ctx, args)
		return execErr
	})
	return res, err
}

func (s *gatedStmt) Query(args []driver.Value) (driver.Rows, error) {
	if !isMutatingSQL(s.query) {
		return s.Stmt.Query(args)
	}
	release, err := s.conn.acquire(context.Background())
	if err != nil {
		return nil, err
	}
	defer release()
	var rows driver.Rows
	err = s.conn.retry(context.Background(), func() error {
		var qerr error
		rows, qerr = s.Stmt.Query(args)
		return qerr
	})
	return rows, err
}

func (s *gatedStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if s.queryer == nil {
		return nil, driver.ErrSkip
	}
	if !isMutatingSQL(s.query) {
		return s.queryer.QueryContext(ctx, args)
	}
	release, err := s.conn.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	var rows driver.Rows
	err = s.conn.retry(ctx, func() error {
		var qerr error
		rows, qerr = s.queryer.QueryContext(ctx, args)
		return qerr
	})
	return rows, err
}
