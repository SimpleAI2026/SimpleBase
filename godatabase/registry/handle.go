// 文件说明：handle.go — 进程内每个 logical database 的唯一 writer/connection
//
// 用法
//
//	(h *Handle) LastUsed() time.Time  // LastUsed 返回最后一次被访问的时间
//	(h *Handle) ActiveCount() int64  // ActiveCount 返回当前活跃引用数
//	同包调用：stateReady → registry.go；Handle → registry.go；newHandle() → registry.go
//
// 调用架构
//
//	[上游] godatabase/kv, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/registry/handle.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase
//	[同包] evictor.go, registry.go
//	[关键引用] stateReady ← registry.go
//
// 主要符号
//
//	type Handle struct {…}
//	(h *Handle) LastUsed() time.Time
//	(h *Handle) ActiveCount() int64
//	(h *Handle) Conn() *sql.DB
//	(h *Handle) NotifyWrite(ctx context.Context)
//	(h *Handle) Query(ctx context.Context, stmt godatabase.Statement, maxRows int) ...
//	... 另有 12 个顶层符号

package registry

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
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
	Database godatabase.Spec

	conn *sql.DB

	mode      godatabase.AccessMode
	state     atomic.Uint32
	active    atomic.Int64
	lastUsed  atomic.Int64 // UnixNano
	closeOnce sync.Once

	// onWrite 在写成功后回调（CatalogSyncer.MarkDirty 等）；只读 Query 不触发。
	onWrite func(ctx context.Context, h *Handle) error
	// beforeClose 在关闭连接前回调（Flush catalog 等）。
	beforeClose func(ctx context.Context, h *Handle) error
}

func newHandle(db godatabase.Spec, conn *sql.DB, mode godatabase.AccessMode) *Handle {
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

// notifyWrite 在写成功后回调 onWrite（CatalogSyncer.MarkDirty 等）。
func (h *Handle) notifyWrite(ctx context.Context) {
	h.touch()
	if h.onWrite != nil {
		_ = h.onWrite(ctx, h)
	}
}

// NotifyWrite 通知本 handle 上发生了一次外部写提交（不走 Execute/Batch 的
// 路径，例如 kv 包自管事务），触发与 Execute/Batch 相同的 onWrite 回调。
func (h *Handle) NotifyWrite(ctx context.Context) {
	h.notifyWrite(ctx)
}

// Query 委托给 internal/database.Query。
func (h *Handle) Query(ctx context.Context, stmt godatabase.Statement, maxRows int) (godatabase.QueryResult, error) {
	return godatabase.Query(ctx, h.conn, stmt, maxRows)
}

// Execute 委托给 internal/database.Execute，成功后触发 onWrite。
func (h *Handle) Execute(ctx context.Context, stmt godatabase.Statement) (godatabase.QueryResult, error) {
	res, err := godatabase.Execute(ctx, h.conn, stmt)
	if err != nil {
		return res, err
	}
	h.notifyWrite(ctx)
	return res, nil
}

// Batch 委托给 internal/database.Batch，成功后触发 onWrite。
func (h *Handle) Batch(ctx context.Context, stmts []godatabase.Statement, transactional bool) ([]godatabase.QueryResult, error) {
	res, err := godatabase.Batch(ctx, h.conn, stmts, transactional)
	if err != nil {
		return res, err
	}
	h.notifyWrite(ctx)
	return res, nil
}

// closeLocked 关闭底层连接，只执行一次。调用方必须已将 state 置为 closing/closed
// 且确认不会再有新的 Acquire 命中该 handle。
// §3.6：beforeClose（Flush catalog）失败必须传播给调用方——静默吞掉会让
// 缓存淘汰继续删除本地目录，放大未同步损失。连接仍会关闭，错误如实返回。
func (h *Handle) closeLocked() error {
	var err error
	h.closeOnce.Do(func() {
		h.state.Store(uint32(stateClosed))
		if h.beforeClose != nil {
			err = h.beforeClose(context.Background(), h)
		}
		if cerr := h.conn.Close(); err == nil {
			err = cerr
		}
	})
	return err
}

// closeNoFlushLocked 关闭连接但跳过 beforeClose 回调（失租路径：
// multi-instance-consistency-plan §4.6 要求失租后 Close 不发起任何 PUT）。
func (h *Handle) closeNoFlushLocked() error {
	var err error
	h.closeOnce.Do(func() {
		h.state.Store(uint32(stateClosed))
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
