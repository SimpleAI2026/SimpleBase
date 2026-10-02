package kv

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Store 是绑定单个 DuckLake 数据库连接（*sql.DB）的 KV 仓库入口。
// 调用方必须经 Registry 租约拿到该连接，不得绕过 Registry 自行构造。
type Store struct {
	db *sql.DB

	// schemaMu 串行化建表；schemaKnown 缓存「kv schema 已存在」判定，
	// 避免每次读都查 information_schema。
	schemaMu    sync.Mutex
	schemaKnown atomic.Bool

	now     func() time.Time
	onWrite func(ctx context.Context)
	// observe 收到写事务分段耗时（exec/commit），计时装配注入。
	observe func(exec, commit time.Duration)
}

// Option 配置 Store。
type Option func(*Store)

// WithNowFunc 注入时钟（测试用固定时钟验证 TTL）。默认 time.Now。
func WithNowFunc(fn func() time.Time) Option {
	return func(s *Store) { s.now = fn }
}

// WithOnWrite 注入写提交回调（handler 侧接 registry.Handle.NotifyWrite，
// 保证 catalog debounce 同步链路与现有写路径一致）。
// 每个写事务 Commit 成功后调用一次；读路径不触发。
func WithOnWrite(fn func(ctx context.Context)) Option {
	return func(s *Store) { s.onWrite = fn }
}

// New 构造 Store。db 必须是经 Registry 获取的 DuckLake 连接
// （boot 时已 USE lake，kv schema 建在 lake catalog 内）。
func New(db *sql.DB, opts ...Option) *Store {
	s := &Store{db: db, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s
}

// nowMs 返回当前 unix 毫秒。
func (s *Store) nowMs() int64 { return s.now().UnixMilli() }

// HasSchema 报告 kv schema 是否已存在（结果缓存）。
// 读路径用它短路：不存在即空库（返回空列表 / ErrNotFound），
// 避免只读请求触发建表产生 DuckLake 快照副作用。
func (s *Store) HasSchema(ctx context.Context) (bool, error) {
	if s.schemaKnown.Load() {
		return true, nil
	}
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM information_schema.schemata WHERE schema_name = 'kv'`).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("kv: check schema: %w", err)
	}
	if n > 0 {
		s.schemaKnown.Store(true)
		return true, nil
	}
	return false, nil
}

// ensureSchema 惰性建表（首次写时执行一次；失败不缓存，下次写重试）。
func (s *Store) ensureSchema(ctx context.Context) error {
	if s.schemaKnown.Load() {
		return nil
	}
	s.schemaMu.Lock()
	defer s.schemaMu.Unlock()
	if s.schemaKnown.Load() {
		return nil
	}
	if err := ensureSchemaLocked(ctx, s.db); err != nil {
		return err
	}
	s.schemaKnown.Store(true)
	return nil
}

// Update 在单个读写事务内执行 fn（多步写操作的原子边界）。
// 首次调用会先确保 schema 存在；Commit 成功后触发 onWrite 回调一次。
// observe 非空时收到分段耗时（exec=fn 执行、commit=提交），供上层计时。
func (s *Store) Update(ctx context.Context, fn func(tx *Tx) error) error {
	if err := s.ensureSchema(ctx); err != nil {
		return err
	}
	var execCost time.Duration
	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("kv: begin tx: %w", err)
	}
	tx := &Tx{tx: sqlTx, now: s.now}
	execStart := time.Now()
	ferr := fn(tx)
	execCost = time.Since(execStart)
	if ferr != nil {
		_ = sqlTx.Rollback()
		return ferr
	}
	commitStart := time.Now()
	cerr := sqlTx.Commit()
	if s.observe != nil {
		s.observe(execCost, time.Since(commitStart))
	}
	if cerr != nil {
		return fmt.Errorf("kv: commit: %w", cerr)
	}
	if s.onWrite != nil {
		s.onWrite(ctx)
	}
	return nil
}

// ObserveCost 注册写事务分段观测（exec/commit）；仅供计时装配使用。
func (s *Store) ObserveCost(fn func(exec, commit time.Duration)) {
	s.observe = fn
}

// View 在单个事务内执行只读操作（一致性快照；多语句读避免中途变更）。
// 不创建 schema：调用方应先 HasSchema 短路空库。
func (s *Store) View(ctx context.Context, fn func(tx *Tx) error) error {
	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("kv: begin view: %w", err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	return fn(&Tx{tx: sqlTx, now: s.now})
}

// Tx 是事务版仓库入口。六个仓库方法返回绑定本事务的仓库。
type Tx struct {
	tx  *sql.Tx
	now func() time.Time
}

func (t *Tx) nowMs() int64 { return t.now().UnixMilli() }

func (t *Tx) Key() *KeyRepo       { return &KeyRepo{t: t} }
func (t *Tx) Str() *StringRepo    { return &StringRepo{t: t} }
func (t *Tx) Hash() *HashRepo     { return &HashRepo{t: t} }
func (t *Tx) List() *ListRepo     { return &ListRepo{t: t} }
func (t *Tx) Set() *SetRepo       { return &SetRepo{t: t} }
func (t *Tx) ZSet() *ZSetRepo     { return &ZSetRepo{t: t} }
