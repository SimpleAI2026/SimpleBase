package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// ErrUnavailable 表示系统库连接不可用。
var ErrUnavailable = errors.New("system_store_unavailable")

// IsAdminProject 判断 projectID 是否为 admin（系统）项目；
// admin 项目下的日志/监控查询不按项目过滤，返回全系统数据。
func IsAdminProject(projectID string) bool {
	return projectID == catalog.ReservedSystemProjectID
}

// Store 持有系统 DuckLake 连接与生命周期。
type Store struct {
	db      *sql.DB
	meta    catalog.Database
	factory *ducklake.Factory
	locator Locator
	logger  observability.Logger

	mu     sync.Mutex
	closed bool

	metricsBuf []MetricSample
	logBuf     []LogEvent
	// metricAgg 是热指标的内存预聚合桶（§7.2 P3.3）。
	metricAgg map[string]metricAggregate
	// async 是后台刷写器（§7.2 P3.1：请求路径永不同步 flush）。
	async      *asyncFlusher
	asyncStop  func(ctx context.Context)
	droppedLog int64

	// 仪表盘读缓存（perf §1 P1-D：10s TTL + 单飞）。
	summaryCache *queryCache[MetricsSummary]
	trendCache   *queryCache[[]TrendPoint]

	defaultKeepDays int
	flushCancel     context.CancelFunc
}

// DB 返回底层连接（供仓储使用）。
func (s *Store) DB() *sql.DB { return s.db }

// initReadCaches 惰性初始化仪表盘读缓存（10s TTL）。
func (s *Store) initReadCaches() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.summaryCache == nil {
		s.summaryCache = newQueryCache[MetricsSummary](10 * time.Second)
	}
	if s.trendCache == nil {
		s.trendCache = newQueryCache[[]TrendPoint](10 * time.Second)
	}
}

// NewStoreForTest wraps an already-migrated *sql.DB (unit tests).
func NewStoreForTest(db *sql.DB) *Store {
	return &Store{db: db}
}

// Meta 返回系统库 catalog 行（kind=system）。
func (s *Store) Meta() catalog.Database { return s.meta }

// Locator 返回落盘定位器。
func (s *Store) Locator() Locator { return s.locator }

// CatalogRepo 返回面向 sys_* 表的 catalog.Repository。
func (s *Store) CatalogRepo() catalog.Repository {
	return catalog.NewSQLRepository(s.db, s.notifyWrite)
}

// AuthRepo 返回面向 sys_api_keys 的 auth.Repository。
func (s *Store) AuthRepo() auth.Repository {
	return auth.NewSQLAPIKeyRepository(s.db)
}

func (s *Store) notifyWrite(ctx context.Context) {
	if s == nil || s.factory == nil {
		return
	}
	_ = s.factory.AfterWrite(ctx, s.meta, s.db)
}

// Ping 检查系统库是否可查询。
func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return ErrUnavailable
	}
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	var n int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_migration_versions`).Scan(&n); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// Close 同步系统库并关闭连接。
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	cancel := s.flushCancel
	s.flushCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// 先停后台 flusher（含尾部排空），再同步收尾一次兜底。
	if s.asyncStop != nil {
		s.asyncStop(ctx)
	}
	_ = s.FlushMetrics(ctx)
	_ = s.FlushLogs(ctx)
	if s.factory != nil {
		_ = s.factory.BeforeClose(ctx, s.meta.ID, s.db)
	}
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *Store) defaultKeep() int {
	if s != nil && s.defaultKeepDays > 0 {
		return s.defaultKeepDays
	}
	return 14
}

// SetDefaultLogKeepDays sets the fallback used when sys_log_retention has no row.
func (s *Store) SetDefaultLogKeepDays(days int) {
	if s == nil {
		return
	}
	if days <= 0 {
		days = 14
	}
	s.mu.Lock()
	s.defaultKeepDays = days
	s.mu.Unlock()
}

// SeedGlobalRetention inserts a global sys_log_retention row when missing.
func (s *Store) SeedGlobalRetention(ctx context.Context, keepDays int) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if keepDays <= 0 {
		keepDays = 14
	}
	var existing int64
	err := s.db.QueryRowContext(ctx,
		`SELECT keep_days FROM sys_log_retention WHERE scope = 'global' LIMIT 1`).Scan(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sys_log_retention(scope, project_id, keep_days, updated_at) VALUES('global', NULL, ?, ?)`,
		int64(keepDays), time.Now().UTC())
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// StartPeriodicFlush starts tickers that flush log/metric buffers in addition to the size-64 path.
func (s *Store) StartPeriodicFlush(logEvery, metricsEvery time.Duration) {
	if s == nil {
		return
	}
	if logEvery <= 0 && metricsEvery <= 0 {
		return
	}
	// §7.2 P3：启动后台 flusher；Record* 满批只发信号，不再在请求路径写库。
	s.startAsyncFlusher()
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.flushCancel != nil {
		s.flushCancel()
	}
	s.flushCancel = cancel
	s.mu.Unlock()
	// perf §1 P1-B：周期 flush 也走合批入口（单事务 + 单次 notifyWrite），
	// 两个 ticker 各自兜底对应缓冲的时效，先醒的一方顺带刷空另一方。
	if logEvery > 0 {
		go s.flushLoop(ctx, logEvery, s.flushAllAsync)
	}
	if metricsEvery > 0 {
		go s.flushLoop(ctx, metricsEvery, s.flushAllAsync)
	}
	// §7.2 P6：排队观测——周期采样连接池水位与累计等待，暴露系统性瓶颈。
	go s.observeConnStats(ctx, metricsEvery)
}

// observeConnStats 周期采集 *sql.DB 统计并记录异常等待（诊断口径，
// 不落 Prometheus：systemdb 包不依赖 observability 注册器）。
func (s *Store) observeConnStats(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = 2 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	var lastWait time.Duration
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if s.db == nil {
				return
			}
			st := s.db.Stats()
			if st.WaitDuration > lastWait {
				// 有新的连接等待：记录增量供排障（logger 为 nil 时静默）。
				if s.logger != nil {
					s.logger.Warn("systemdb connection wait accumulated",
						zap.Int("open", st.OpenConnections),
						zap.Int("in_use", st.InUse),
						zap.Duration("wait_total", st.WaitDuration),
						zap.Duration("wait_delta", st.WaitDuration-lastWait),
					)
				}
				lastWait = st.WaitDuration
			}
		}
	}
}

// startAsyncFlusher 幂等启动后台刷写循环。
func (s *Store) startAsyncFlusher() {
	s.mu.Lock()
	if s.async != nil || s.closed {
		s.mu.Unlock()
		return
	}
	f := newAsyncFlusher(s.flushAllAsync)
	s.async = f
	s.mu.Unlock()
	s.asyncStop = f.start()
}

// asyncNotifyLocked 通知后台 flusher（不持有 mu 时调用）。
func (s *Store) asyncNotifyLocked() {
	if s == nil {
		return
	}
	s.mu.Lock()
	f := s.async
	s.mu.Unlock()
	if f != nil {
		f.notify()
	}
}

func (s *Store) flushLoop(ctx context.Context, every time.Duration, fn func(context.Context) error) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = fn(ctx)
		}
	}
}
