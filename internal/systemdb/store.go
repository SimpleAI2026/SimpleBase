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

	defaultKeepDays int
	flushCancel     context.CancelFunc
}

// DB 返回底层连接（供仓储使用）。
func (s *Store) DB() *sql.DB { return s.db }

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
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.flushCancel != nil {
		s.flushCancel()
	}
	s.flushCancel = cancel
	s.mu.Unlock()
	if logEvery > 0 {
		go s.flushLoop(ctx, logEvery, s.FlushLogs)
	}
	if metricsEvery > 0 {
		go s.flushLoop(ctx, metricsEvery, s.FlushMetrics)
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
