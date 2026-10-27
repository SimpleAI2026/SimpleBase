// 文件说明：registry.go — godatabase/registry 中 Registry、Options 等符号的实现
//
// 用法
//
//	New(factory godatabase.Factory, resolver godatabase.PathResolver, opts Options ...
//	(r *Registry) Acquire(ctx context.Context, db godatabase.Spec, mode ...  // Acquire 获取指定数据库的访问租约
//	(r *Registry) CloseDatabase(ctx context.Context, databaseID string) error  // CloseDatabase 主动关闭指定数据库的 handle（不删除 S3 数据）
//	同包调用：Registry → evictor.go, handle.go；CloseIdle() → evictor.go；Acquire() → handle.go
//
// 调用架构
//
//	[上游] godatabase/kv, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/registry/registry.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase
//	[同包] evictor.go, handle.go
//	[关键引用] Registry ← evictor.go, handle.go
//
// 主要符号
//
//	type Registry struct {…}
//	type Options struct {…}
//	New(factory godatabase.Factory, resolver godatabase.PathResolver, opts Options ...
//	(r *Registry) Acquire(ctx context.Context, db godatabase.Spec, mode ...
//	(r *Registry) CloseDatabase(ctx context.Context, databaseID string) error
//	(r *Registry) Remove(databaseID string)
//	... 另有 13 个顶层符号

package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"go.uber.org/zap"
)

// entry 是 Registry 内部的每库状态槽。它可能处于"正在打开"或"已就绪"两种阶段：
//   - opening != nil：有且只有一个 goroutine（创建者）在执行 factory.Open，
//     其余并发 Acquire 通过等待 opening channel 关闭来获知结果，不重复打开。
//   - handle != nil：已就绪，可直接复用。
type entry struct {
	handle  *Handle
	opening chan struct{}
}

// Registry 保证进程内每个 databaseID 只有一个 *sql.DB / writer。
type Registry struct {
	factory  godatabase.Factory
	resolver godatabase.PathResolver
	writable bool

	mu      sync.Mutex
	entries map[string]*entry

	idleTimeout time.Duration
	maxOpen     int // 0 表示不限制同时打开的数据库数量

	logger   godatabase.Logger
	observer godatabase.Observer

	closed bool
	// writeGate 在 ReadWrite Acquire 前调用；失租时拒绝新写（§4.6）。
	writeGate func(db godatabase.Spec) error
}

// Options 配置 Registry 行为。
type Options struct {
	IdleTimeout time.Duration // <=0 时使用默认 5 分钟
	MaxOpen     int           // <=0 表示不限制
	// Writable 表示本实例是否允许提供 ReadWrite 访问；对应 config.Instance.Writable。
	Writable bool
	// WriteGate 在 ReadWrite Acquire 前调用（multi-instance-consistency-plan §4.6）。
	// 返回非 nil 拒绝获取（例：写租约丢失/被抢占）。nil 表示门禁未启用。
	WriteGate func(db godatabase.Spec) error
}

// New 构造 Registry。factory 用于实际打开数据库连接；resolver 用于补全缺失的 DatabasePath，
// 可为 nil（跳过补全）；logger 与 observer 可为 nil（静默 / 不采集）。
func New(factory godatabase.Factory, resolver godatabase.PathResolver, opts Options, logger godatabase.Logger, observer godatabase.Observer) *Registry {
	idle := opts.IdleTimeout
	if idle <= 0 {
		idle = 5 * time.Minute
	}
	return &Registry{
		factory:     factory,
		resolver:    resolver,
		writable:    opts.Writable,
		writeGate:   opts.WriteGate,
		entries:     make(map[string]*entry),
		idleTimeout: idle,
		maxOpen:     opts.MaxOpen,
		logger:      logger,
		observer:    observer,
	}
}

// Acquire 获取指定数据库的访问租约。见 plan4.md「Acquire 精确算法」。
//
// 步骤：
//  1. 校验数据库状态与访问模式的合法性（deleting/deleted 拒绝；degraded 需恢复路径；
//     Writable=false 时拒绝 ReadWrite）。
//  2. 已有 ready handle：增加活跃计数并复用。
//  3. 否则由唯一的创建者调用 factory.Open；其他并发调用者等待结果，不重复打开、
//     不在锁内做网络 I/O。
func (r *Registry) Acquire(ctx context.Context, db godatabase.Spec, mode godatabase.AccessMode) (*Lease, error) {
	if !db.IsSystem() && db.DatabasePath == "" && db.ProjectID != "" && r.resolver != nil {
		path, err := r.resolver.ResolveDatabasePath(ctx, db.TenantID, db.ProjectID)
		if err != nil {
			return nil, err
		}
		db.DatabasePath = path
	}
	if err := r.validateAccess(db, mode); err != nil {
		return nil, err
	}
	key := registryKey(db)

	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return nil, godatabase.ErrRegistryClosed
		}
		e, ok := r.entries[key]
		if !ok {
			if r.maxOpen > 0 && len(r.entries) >= r.maxOpen {
				r.mu.Unlock()
				return nil, fmt.Errorf("registry: maximum open project databases reached")
			}
			e = &entry{opening: make(chan struct{})}
			r.entries[key] = e
			r.mu.Unlock()
			return r.openAndAcquire(ctx, db, mode, e)
		}
		if e.handle != nil && e.handle.isReady() {
			e.handle.active.Add(1)
			e.handle.touch()
			r.mu.Unlock()
			r.observeAcquire("hot", 0, nil)
			return r.newLease(e.handle), nil
		}
		if e.opening != nil {
			// 有其他 goroutine 正在打开：等待其完成，不持锁等待。
			opening := e.opening
			r.mu.Unlock()
			waitStart := time.Now()
			select {
			case <-opening:
				r.observeAcquire("opening_wait", time.Since(waitStart), nil)
				// 结果已产生，回到循环顶部重新读取 entry。
				continue
			case <-ctx.Done():
				r.observeAcquire("opening_wait", time.Since(waitStart), ctx.Err())
				return nil, ctx.Err()
			}
		}
		// e 存在但 handle 为 nil 且 opening 为 nil：说明上次打开失败且未清理，
		// 视为可重新尝试打开。
		e.opening = make(chan struct{})
		r.mu.Unlock()
		return r.openAndAcquire(ctx, db, mode, e)
	}
}

// openAndAcquire 由创建者 goroutine 调用：执行 factory.Open，
// 成功后把结果写回 entry 并唤醒等待者；失败则清理 entry 使下次可重试。
func (r *Registry) openAndAcquire(ctx context.Context, db godatabase.Spec, mode godatabase.AccessMode, e *entry) (*Lease, error) {
	start := time.Now()
	conn, err := r.factory.Open(ctx, db, mode)
	r.observeAcquire("factory_open", time.Since(start), err)

	r.mu.Lock()
	if err != nil {
		close(e.opening)
		e.opening = nil
		// 打开失败：删除 entry，允许下次 Acquire 重新尝试。
		delete(r.entries, registryKey(db))
		r.mu.Unlock()
		if r.logger != nil {
			r.logger.Error("registry: open database failed",
				zap.String("database_id", db.ID), zap.String("err", err.Error()))
		}
		return nil, fmt.Errorf("registry: open database %s: %w", db.ID, err)
	}

	handle := newHandle(db, conn, mode)
	if hook, ok := r.factory.(godatabase.WriteHookFactory); ok {
		hookFactory := hook
		handle.onWrite = func(ctx context.Context, h *Handle) error {
			return hookFactory.AfterWrite(ctx, h.Database, h.conn)
		}
		handle.beforeClose = func(ctx context.Context, h *Handle) error {
			return hookFactory.BeforeClose(ctx, registryKey(h.Database), h.conn)
		}
	}
	handle.active.Add(1)
	e.handle = handle
	close(e.opening)
	e.opening = nil
	r.mu.Unlock()

	if r.observer != nil {
		r.observer.OpenHandleDelta(1)
	}
	return r.newLease(handle), nil
}

func (r *Registry) observeAcquire(stage string, duration time.Duration, err error) {
	if r.logger == nil {
		return
	}
	fields := []zap.Field{zap.String("stage", stage), zap.Duration("duration", duration)}
	if err != nil {
		fields = append(fields, zap.Bool("error", true))
	}
	r.logger.Debug("registry acquire stage", fields...)
}

func (r *Registry) newLease(h *Handle) *Lease {
	return &Lease{
		Handle: h,
		release: func() {
			h.active.Add(-1)
			h.touch()
		},
	}
}

func registryKey(db godatabase.Spec) string {
	if db.ProjectID != "" {
		return db.ProjectID
	}
	return db.ID
}

// findEntryLocked 同时接受项目 ID 和默认 databaseID，物理条目始终按项目存储。
func (r *Registry) findEntryLocked(id string) (string, *entry, bool) {
	if e, ok := r.entries[id]; ok {
		return id, e, true
	}
	for key, e := range r.entries {
		if e.handle != nil && (e.handle.Database.ID == id || e.handle.Database.ProjectID == id) {
			return key, e, true
		}
	}
	return "", nil, false
}

// validateAccess 校验数据库状态与访问模式（不涉及 I/O）。
func (r *Registry) validateAccess(db godatabase.Spec, mode godatabase.AccessMode) error {
	switch db.Status {
	case godatabase.StatusDeleting, godatabase.StatusDeleted:
		return godatabase.ErrDatabaseDeleting
	case godatabase.StatusDegraded:
		// degraded 状态只允许恢复路径重新打开；本 Acquire 不区分调用者身份，
		// 恢复流程需通过专用路径调用（Plan 7），此处保守拒绝一般访问。
		return godatabase.ErrDatabaseNotReady
	}
	if mode == godatabase.ReadWrite && !r.writable {
		return godatabase.ErrWriterUnavailable
	}
	if mode == godatabase.ReadWrite && r.writeGate != nil {
		if err := r.writeGate(db); err != nil {
			return err
		}
	}
	return nil
}

// CloseDatabase 主动关闭指定数据库的 handle（不删除 S3 数据）。
// 若存在活跃引用，返回 error 而不强制关闭，避免破坏进行中的请求。
func (r *Registry) CloseDatabase(ctx context.Context, databaseID string) error {
	r.mu.Lock()
	key, e, ok := r.findEntryLocked(databaseID)
	if !ok || e.handle == nil {
		r.mu.Unlock()
		return nil
	}
	if e.handle.active.Load() > 0 {
		r.mu.Unlock()
		return fmt.Errorf("registry: project %s has active references", key)
	}
	delete(r.entries, key)
	handle := e.handle
	r.mu.Unlock()

	err := handle.closeLocked()
	if r.observer != nil {
		r.observer.OpenHandleDelta(-1)
	}
	return err
}

// Remove 失租释放句柄（multi-instance-consistency-plan §4.6 onLost 第 2 步）：
// 与 CloseDatabase 的区别是（a）跳过 beforeClose 回调（失租后禁止 Flush——
// CatalogSyncer 已置 CloseNoFlush，物理不 PUT），（b）不等待活跃引用，
// 立即摘除 entry 并把 handle 置 closing；进行中的请求用完旧 handle 后关闭。
func (r *Registry) Remove(databaseID string) {
	r.mu.Lock()
	key, e, ok := r.findEntryLocked(databaseID)
	if !ok || e.handle == nil {
		r.mu.Unlock()
		return
	}
	delete(r.entries, key)
	handle := e.handle
	handle.state.Store(uint32(stateClosing))
	r.mu.Unlock()

	go func() {
		// 等活跃引用清零后真正关闭（上限 30s，超时强制）。
		deadline := time.Now().Add(30 * time.Second)
		for handle.active.Load() > 0 && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		_ = handle.closeNoFlushLocked() // 失租：跳过 beforeClose（Flush）
		if r.observer != nil {
			r.observer.OpenHandleDelta(-1)
		}
	}()
}

// IsActive 返回指定 databaseID 是否在 registry 中有活跃引用（active>0）。
// CacheManager 仅在 IsActive==false 且 entry 不存在或 ready 时允许淘汰。
// 不做网络 I/O，不持锁等待。
func (r *Registry) IsActive(databaseID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, e, ok := r.findEntryLocked(databaseID)
	if !ok || e.handle == nil {
		return false
	}
	return e.handle.active.Load() > 0
}

// OpenedIDs 返回当前已打开且就绪（stateReady）的数据库 ID 列表。
// 供后台任务（如 KV TTL Sweeper）枚举已打开的库；不会打开新连接。
// 不做网络 I/O。
func (r *Registry) OpenedIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.entries))
	for id, e := range r.entries {
		if e.handle != nil && e.handle.isReady() {
			ids = append(ids, id)
		}
	}
	return ids
}

// Snapshot 返回指定 databaseID 已打开 handle 的只读快照（Database 元数据）。
// 未打开返回 false。不做网络 I/O。
func (r *Registry) Snapshot(databaseID string) (godatabase.Spec, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, e, ok := r.findEntryLocked(databaseID)
	if !ok || e.handle == nil || !e.handle.isReady() {
		return godatabase.Spec{}, false
	}
	return e.handle.Database, true
}

// OpenedProjects 返回已打开项目 ID 列表；KV 清扫等后台任务按项目枚举。
func (r *Registry) OpenedProjects() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.entries))
	for key, e := range r.entries {
		if e.handle != nil && e.handle.isReady() {
			out = append(out, key)
		}
	}
	return out
}

// CloseIdle 关闭所有 active==0 且超过 idleTimeout 未使用的 ready handle。
// 应由定时任务周期调用（建议每分钟一次）。
func (r *Registry) CloseIdle(ctx context.Context, now time.Time) error {
	var toClose []*Handle

	r.mu.Lock()
	for id, e := range r.entries {
		if e.handle == nil || !e.handle.isReady() {
			continue
		}
		if e.handle.active.Load() != 0 {
			continue
		}
		if now.Sub(e.handle.LastUsed()) < r.idleTimeout {
			continue
		}
		delete(r.entries, id)
		toClose = append(toClose, e.handle)
	}
	r.mu.Unlock()

	var firstErr error
	for _, h := range toClose {
		if err := h.closeLocked(); err != nil && firstErr == nil {
			firstErr = err
		}
		if r.observer != nil {
			r.observer.OpenHandleDelta(-1)
		}
	}
	return firstErr
}

// Shutdown 停止接受新的 Acquire，并尝试等待活跃引用清零后关闭全部 handle。
// 超时后仍强制关闭剩余 handle 并记录告警。
func (r *Registry) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	r.closed = true
	entries := make([]*entry, 0, len(r.entries))
	for _, e := range r.entries {
		if e.handle != nil {
			entries = append(entries, e)
		}
	}
	r.entries = make(map[string]*entry)
	r.mu.Unlock()

	var firstErr error
	for _, e := range entries {
		h := e.handle
		// 等待活跃引用清零，受 ctx 超时控制。
		for h.active.Load() > 0 {
			select {
			case <-ctx.Done():
				if r.logger != nil {
					r.logger.Warn("registry: shutdown timeout with active references, forcing close",
						zap.String("database_id", h.Database.ID))
				}
				goto forceClose
			case <-time.After(50 * time.Millisecond):
			}
		}
	forceClose:
		if err := h.closeLocked(); err != nil {
			if r.logger != nil {
				r.logger.Error("registry: close handle on shutdown failed",
					zap.String("database_id", h.Database.ID), zap.String("err", err.Error()))
			}
			if firstErr == nil {
				firstErr = err
			}
		}
		if r.observer != nil {
			r.observer.OpenHandleDelta(-1)
		}
	}
	return firstErr
}
