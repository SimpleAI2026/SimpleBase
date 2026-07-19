package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// entry 是 Registry 内部的每库状态槽。它可能处于"正在打开"或"已就绪"两种阶段：
//   - opening != nil：有且只有一个 goroutine（创建者）在执行 factory.Open，
//     其余并发 Acquire 通过等待 opening channel 关闭来获知结果，不重复打开。
//   - handle != nil：已就绪，可直接复用。
type entry struct {
	handle  *Handle
	opening chan struct{}
	openErr error
}

// Registry 保证进程内每个 databaseID 只有一个 *sql.DB / writer。
type Registry struct {
	factory  database.Factory
	catalog  *catalog.Service
	writable bool

	mu      sync.Mutex
	entries map[string]*entry

	idleTimeout time.Duration
	maxOpen     int // 0 表示不限制同时打开的数据库数量

	logger  observability.Logger
	metrics *observability.Metrics

	closed bool
}

// Options 配置 Registry 行为。
type Options struct {
	IdleTimeout time.Duration // <=0 时使用默认 5 分钟
	MaxOpen     int            // <=0 表示不限制
	// Writable 表示本实例是否允许提供 ReadWrite 访问；对应 config.Instance.Writable。
	Writable bool
}

// New 构造 Registry。factory 用于实际打开数据库连接；catalogSvc 用于状态校验与转换。
func New(factory database.Factory, catalogSvc *catalog.Service, opts Options, logger observability.Logger, metrics *observability.Metrics) *Registry {
	idle := opts.IdleTimeout
	if idle <= 0 {
		idle = 5 * time.Minute
	}
	return &Registry{
		factory:     factory,
		catalog:     catalogSvc,
		writable:    opts.Writable,
		entries:     make(map[string]*entry),
		idleTimeout: idle,
		maxOpen:     opts.MaxOpen,
		logger:      logger,
		metrics:     metrics,
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
func (r *Registry) Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (*Lease, error) {
	if err := r.validateAccess(db, mode); err != nil {
		return nil, err
	}

	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return nil, database.ErrRegistryClosed
		}
		e, ok := r.entries[db.ID]
		if !ok {
			// 没有任何记录：本 goroutine 成为创建者。
			e = &entry{opening: make(chan struct{})}
			r.entries[db.ID] = e
			r.mu.Unlock()
			return r.openAndAcquire(ctx, db, mode, e)
		}
		if e.handle != nil && e.handle.isReady() {
			e.handle.active.Add(1)
			e.handle.touch()
			r.mu.Unlock()
			return r.newLease(e.handle), nil
		}
		if e.opening != nil {
			// 有其他 goroutine 正在打开：等待其完成，不持锁等待。
			opening := e.opening
			r.mu.Unlock()
			select {
			case <-opening:
				// 结果已产生，回到循环顶部重新读取 entry。
				continue
			case <-ctx.Done():
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
func (r *Registry) openAndAcquire(ctx context.Context, db catalog.Database, mode database.AccessMode, e *entry) (*Lease, error) {
	conn, err := r.factory.Open(ctx, db, mode)

	r.mu.Lock()
	if err != nil {
		e.openErr = err
		close(e.opening)
		e.opening = nil
		// 打开失败：删除 entry，允许下次 Acquire 重新尝试。
		delete(r.entries, db.ID)
		r.mu.Unlock()
		if r.logger != nil {
			r.logger.Error("registry: open database failed",
				zap.String("database_id", db.ID), zap.String("err", err.Error()))
		}
		return nil, fmt.Errorf("registry: open database %s: %w", db.ID, err)
	}

	handle := newHandle(db, conn, mode)
	handle.active.Add(1)
	e.handle = handle
	close(e.opening)
	e.opening = nil
	r.mu.Unlock()

	if r.metrics != nil {
		r.metrics.DBOpenHandles.Inc()
	}
	return r.newLease(handle), nil
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

// validateAccess 校验数据库状态与访问模式（不涉及 I/O）。
func (r *Registry) validateAccess(db catalog.Database, mode database.AccessMode) error {
	switch db.Status {
	case catalog.DatabaseDeleting, catalog.DatabaseDeleted:
		return database.ErrDatabaseDeleting
	case catalog.DatabaseDegraded:
		// degraded 状态只允许恢复路径重新打开；本 Acquire 不区分调用者身份，
		// 恢复流程需通过专用路径调用（Plan 7），此处保守拒绝一般访问。
		return database.ErrDatabaseNotReady
	}
	if mode == database.ReadWrite && !r.writable {
		return database.ErrWriterUnavailable
	}
	return nil
}

// CloseDatabase 主动关闭指定数据库的 handle（不删除 S3 数据）。
// 若存在活跃引用，返回 error 而不强制关闭，避免破坏进行中的请求。
func (r *Registry) CloseDatabase(ctx context.Context, databaseID string) error {
	r.mu.Lock()
	e, ok := r.entries[databaseID]
	if !ok || e.handle == nil {
		r.mu.Unlock()
		return nil
	}
	if e.handle.active.Load() > 0 {
		r.mu.Unlock()
		return fmt.Errorf("registry: database %s has active references", databaseID)
	}
	delete(r.entries, databaseID)
	handle := e.handle
	r.mu.Unlock()

	err := handle.closeLocked()
	if r.metrics != nil {
		r.metrics.DBOpenHandles.Dec()
	}
	return err
}

// IsActive 返回指定 databaseID 是否在 registry 中有活跃引用（active>0）。
// CacheManager 仅在 IsActive==false 且 entry 不存在或 ready 时允许淘汰。
// 不做网络 I/O，不持锁等待。
func (r *Registry) IsActive(databaseID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[databaseID]
	if !ok || e.handle == nil {
		return false
	}
	return e.handle.active.Load() > 0
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
		if r.metrics != nil {
			r.metrics.DBOpenHandles.Dec()
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
		if r.metrics != nil {
			r.metrics.DBOpenHandles.Dec()
		}
	}
	return firstErr
}
