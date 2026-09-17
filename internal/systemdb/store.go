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
	s.mu.Unlock()

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
