// Package registry 实现进程内每个 logical database 的唯一 writer/connection
// manager（见 plan4.md）。它只保证单进程内单写：所有写接口必须通过
// Registry.Acquire(ReadWrite) 获取 Handle，禁止绕过 Registry 直接构造 DSN。
//
// 这不是分布式一致性方案；部署层必须保证全局只有一个可写实例运行。
package registry

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
)

// handleState 是 Handle 的内部生命周期状态。
type handleState uint32

const (
	stateReady handleState = iota
	stateClosing
	stateClosed
)

// Handle 持有某个 logical database 唯一的 *sql.DB 及其运行时状态。
type Handle struct {
	Database catalog.Database

	conn *sql.DB

	mode      database.AccessMode
	state     atomic.Uint32
	active    atomic.Int64
	lastUsed  atomic.Int64 // UnixNano
	closeOnce sync.Once

	// onWrite 在写成功后回调（CatalogSyncer.MarkDirty 等）；只读 Query 不触发。
	onWrite func(ctx context.Context, h *Handle) error
	// beforeClose 在关闭连接前回调（Flush catalog 等）。
	beforeClose func(ctx context.Context, h *Handle) error
}

func newHandle(db catalog.Database, conn *sql.DB, mode database.AccessMode) *Handle {
	h := &Handle{
		Database: db,
		conn:     conn,
		mode:     mode,
	}
	h.state.Store(uint32(stateReady))
	h.touch()
	return h
}

func (h *Handle) touch() {
	h.lastUsed.Store(time.Now().UnixNano())
}

func (h *Handle) isReady() bool {
	return handleState(h.state.Load()) == stateReady
}

// LastUsed 返回最后一次被访问的时间。
func (h *Handle) LastUsed() time.Time {
	return time.Unix(0, h.lastUsed.Load())
}

// ActiveCount 返回当前活跃引用数。
func (h *Handle) ActiveCount() int64 {
	return h.active.Load()
}

// Conn 返回底层 *sql.DB（供同步器 / 管理面使用）。
func (h *Handle) Conn() *sql.DB {
	return h.conn
}

// Query 委托给 internal/database.Query。
func (h *Handle) Query(ctx context.Context, stmt database.Statement, maxRows int) (database.QueryResult, error) {
	return database.Query(ctx, h.conn, stmt, maxRows)
}

// Execute 委托给 internal/database.Execute，成功后触发 onWrite。
func (h *Handle) Execute(ctx context.Context, stmt database.Statement) (database.QueryResult, error) {
	res, err := database.Execute(ctx, h.conn, stmt)
	if err != nil {
		return res, err
	}
	h.touch()
	if h.onWrite != nil {
		_ = h.onWrite(ctx, h)
	}
	return res, nil
}

// Batch 委托给 internal/database.Batch，成功后触发 onWrite。
func (h *Handle) Batch(ctx context.Context, stmts []database.Statement, transactional bool) ([]database.QueryResult, error) {
	res, err := database.Batch(ctx, h.conn, stmts, transactional)
	if err != nil {
		return res, err
	}
	h.touch()
	if h.onWrite != nil {
		_ = h.onWrite(ctx, h)
	}
	return res, nil
}

// closeLocked 关闭底层连接，只执行一次。调用方必须已将 state 置为 closing/closed
// 且确认不会再有新的 Acquire 命中该 handle。
func (h *Handle) closeLocked() error {
	var err error
	h.closeOnce.Do(func() {
		h.state.Store(uint32(stateClosed))
		if h.beforeClose != nil {
			_ = h.beforeClose(context.Background(), h)
		}
		err = h.conn.Close()
	})
	return err
}

// Lease 是调用方持有的一次数据库访问租约。使用完毕必须调用 Release。
type Lease struct {
	Handle *Handle

	released atomic.Bool
	release  func()
}

// Release 释放本次租约。幂等：多次调用只生效一次。
func (l *Lease) Release() {
	if l == nil {
		return
	}
	if l.released.CompareAndSwap(false, true) {
		l.release()
	}
}
