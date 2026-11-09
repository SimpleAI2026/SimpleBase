// 文件说明：db_stats_cache.go — 「库内用户表总行数」的缓存与后台刷新
//
// 用法
//
//	NewRowCountCache(opts RowCountCacheOptions) *RowCountCache
//	(c *RowCountCache) Get(dbID string) (int64, bool)  // Get 返回命中且新鲜的行数
//	(c *RowCountCache) RefreshAsync(dbs []catalog.Database)  // RefreshAsync 对未命中/过期的 ready 库发起后台有界并发刷新（每库去重，
//	同包调用：Get() → collection_meta_handler.go, collection_transfer.go 等；refresh() → auth_handler.go, error.go 等；finish() → llm_handler.go, oai_dto.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/db_stats_cache.go
//	         │ 依赖
//	         ▼
//	[下游] internal/catalog, internal/observability
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] Get() ← collection_meta_handler.go, collection_transfer.go, cronjob_handler.go
//
// 主要符号
//
//	type RowCountCacheOptions struct {…}
//	type RowCountCache struct {…}
//	NewRowCountCache(opts RowCountCacheOptions) *RowCountCache
//	(c *RowCountCache) Get(dbID string) (int64, bool)
//	(c *RowCountCache) RefreshAsync(dbs []catalog.Database)
//	(c *RowCountCache) RefreshSync(ctx context.Context, db catalog.Database, timeout ...
//	... 另有 6 个顶层符号

package api

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// errRowCountInflight 表示该库已有刷新在进行（调用方应省略字段而非等待）。
var errRowCountInflight = errors.New("api: row count refresh in flight")

// RowCountCacheOptions 配置 RowCountCache。
type RowCountCacheOptions struct {
	// TTL 缓存兜底寿命；<=0 默认 60s。
	TTL time.Duration
	// Workers 后台刷新并发上限；<=0 默认 2。
	Workers int
	// Count 实际统计函数（CountDatabaseRows）。
	Count func(ctx context.Context, db catalog.Database) (int64, error)
	// Watermark 可选：返回 (lastSynced, lag) 同步水位，用于写后失效。
	Watermark func(dbID string) (lastSynced, lag int64)
	Logger    observability.Logger
}

type rowCountEntry struct {
	count      int64
	lastSynced int64
	lag        int64
	fetchedAt  time.Time
	refreshing bool
}

// RowCountCache 是 per-DB 行数统计缓存；所有方法 nil 安全。
type RowCountCache struct {
	ttl       time.Duration
	count     func(ctx context.Context, db catalog.Database) (int64, error)
	watermark func(dbID string) (int64, int64)
	logger    observability.Logger

	mu      sync.Mutex
	entries map[string]*rowCountEntry
	sem     chan struct{}
}

// NewRowCountCache 构造缓存；Count 为 nil 时缓存退化为永远未命中。
func NewRowCountCache(opts RowCountCacheOptions) *RowCountCache {
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = 2
	}
	return &RowCountCache{
		ttl:       ttl,
		count:     opts.Count,
		watermark: opts.Watermark,
		logger:    opts.Logger,
		entries:   map[string]*rowCountEntry{},
		sem:       make(chan struct{}, workers),
	}
}

// Get 返回命中且新鲜的行数；未命中/过期/无缓存返回 ok=false。
func (c *RowCountCache) Get(dbID string) (int64, bool) {
	if c == nil || c.count == nil {
		return 0, false
	}
	c.mu.Lock()
	e := c.entries[dbID]
	if e == nil || e.refreshing && e.fetchedAt.IsZero() {
		c.mu.Unlock()
		return 0, false
	}
	count, fetchedAt := e.count, e.fetchedAt
	ls, lag := e.lastSynced, e.lag
	c.mu.Unlock()

	if time.Since(fetchedAt) > c.ttl {
		return 0, false
	}
	if curLS, curLag := c.watermarkOf(dbID); curLS != ls || curLag != lag {
		return 0, false // 写后水位已变：失效
	}
	return count, true
}

// RefreshAsync 对未命中/过期的 ready 库发起后台有界并发刷新（每库去重，
// 已在刷新的跳过）。不阻塞调用方；失败仅记日志，等待 TTL 后重试。
func (c *RowCountCache) RefreshAsync(dbs []catalog.Database) {
	if c == nil || c.count == nil {
		return
	}
	for _, db := range dbs {
		if db.Status != catalog.DatabaseReady {
			continue
		}
		if _, ok := c.Get(db.ID); ok {
			continue
		}
		if !c.markRefreshing(db.ID) {
			continue
		}
		go c.refresh(db)
	}
}

// RefreshSync 同步刷新单库（GetDatabase 单库路径），带超时与单飞去重。
// 已有刷新在进行时返回 errRowCountInflight（调用方省略字段即可）。
func (c *RowCountCache) RefreshSync(ctx context.Context, db catalog.Database, timeout time.Duration) (int64, error) {
	if c == nil || c.count == nil {
		return 0, errors.New("api: row count cache not configured")
	}
	if !c.markRefreshing(db.ID) {
		return 0, errRowCountInflight
	}
	ls, lag := c.watermarkOf(db.ID) // 统计前取水位：统计期间的写会让缓存下次失效
	cctx, cancel := context.WithTimeout(ctx, timeout)
	n, err := c.count(cctx, db)
	cancel()
	c.finish(db.ID, n, ls, lag, err)
	return n, err
}

func (c *RowCountCache) refresh(db catalog.Database) {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()
	ls, lag := c.watermarkOf(db.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	n, err := c.count(ctx, db)
	cancel()
	c.finish(db.ID, n, ls, lag, err)
	if err != nil && c.logger != nil {
		c.logger.Debug("row count background refresh failed",
			zap.String("database_id", db.ID), zap.Error(err))
	}
}

// markRefreshing 占位刷新标记；已在刷新返回 false。
func (c *RowCountCache) markRefreshing(dbID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[dbID]
	if e == nil {
		e = &rowCountEntry{}
		c.entries[dbID] = e
	}
	if e.refreshing {
		return false
	}
	e.refreshing = true
	return true
}

func (c *RowCountCache) finish(dbID string, n, ls, lag int64, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[dbID]
	if e == nil {
		return
	}
	e.refreshing = false
	if err == nil {
		e.count = n
		e.lastSynced = ls
		e.lag = lag
		e.fetchedAt = time.Now()
	}
}

func (c *RowCountCache) watermarkOf(dbID string) (int64, int64) {
	if c.watermark == nil {
		return 0, 0
	}
	return c.watermark(dbID)
}
