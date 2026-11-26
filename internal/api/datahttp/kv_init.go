package datahttp

import (
	"context"
	"database/sql"
	"fmt"

	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/kv"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

type KVInitializer struct {
	reg    RegistryService
	logger observability.Logger
}

// NewKVInitializer 构造 KV 初始化器。reg 为 nil 时返回 nil（readonly 实例）。
// NewKVInitializer 构造 KV 初始化器（满足 cataloghttp.KVInitializer 端口）；reg 为 nil 时返回 nil。
func NewKVInitializer(reg RegistryService, logger observability.Logger) *KVInitializer {
	if reg == nil {
		return nil
	}
	return &KVInitializer{reg: reg, logger: logger}
}

// InitKV 初始化指定库的 kv schema 与 6 张系统表。系统库跳过（无 KV 语义且禁写）。
// 失败时返回错误并记 warn；调用方（建项目）不因此失败——首次 KV 写会兜底补建。
func (k *KVInitializer) InitKV(ctx context.Context, db catalog.Database) error {
	if catalog.IsSystemDatabase(db) {
		return nil
	}
	err := k.ensure(ctx, db)
	if err != nil && k.logger != nil {
		k.logger.Warn("kv system tables init failed",
			zap.String("database_id", db.ID), zap.String("err", err.Error()))
		return err
	}
	if err == nil && k.logger != nil {
		k.logger.Info("kv system tables initialized", zap.String("database_id", db.ID))
	}
	return err
}

func (k *KVInitializer) ensure(ctx context.Context, db catalog.Database) error {
	lease, err := k.reg.Acquire(ctx, db, godatabase.ReadWrite)
	if err != nil {
		return fmt.Errorf("api: acquire database for kv init: %w", err)
	}
	defer lease.Release()

	if err := kv.EnsureSchema(ctx, lease.Handle.Conn()); err != nil {
		return err
	}
	lease.Handle.NotifyWrite(ctx)
	return nil
}

// kvServiceAdapter 把 catalog.Service + RegistryService 适配为项目级 KVService。
type kvServiceAdapter struct {
	catalog  *catalog.Service
	registry RegistryService
}

// NewKVServiceAdapter 构造 KVService。cat 为实际 *catalog.Service（内部入口无 principal）。
func NewKVServiceAdapter(cat *catalog.Service, reg RegistryService) KVService {
	return &kvServiceAdapter{catalog: cat, registry: reg}
}

func (a *kvServiceAdapter) GetKVDatabase(ctx context.Context, tenantID, projectID string) (catalog.Database, error) {
	timer := httpkit.StageTimerFrom(ctx)
	scope := timer.StageScope(httpkit.StageCatalog)
	defer scope.Done()
	return a.catalog.ResolveProjectStorage(ctx, tenantID, projectID)
}

func (a *kvServiceAdapter) CreateKVDatabase(ctx context.Context, tenantID, projectID string) (catalog.Database, error) {
	timer := httpkit.StageTimerFrom(ctx)
	scope := timer.StageScope(httpkit.StageCatalog)
	defer scope.Done()
	return a.catalog.ResolveProjectStorage(ctx, tenantID, projectID)
}

func (a *kvServiceAdapter) Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (SQLLease, error) {
	timer := httpkit.StageTimerFrom(ctx)
	acquireScope := timer.StageScope(httpkit.StageAcquire)
	l, err := a.registry.Acquire(ctx, db, mode)
	acquireScope.Done()
	if err != nil {
		return nil, err
	}
	return WrapTimedLease(ctx, &kvLeaseAdapter{lease: l}), nil
}

// kvLeaseAdapter 包装 registry.Lease 为 KV handler 所需的 SQLLease。
// KV 只使用 Raw 自管事务与 NotifyWrite；Query/Execute/Batch 以委托实现
// 满足接口（KV 分发器不会调用它们）。
type kvLeaseAdapter struct {
	lease *registry.Lease
}

func (s *kvLeaseAdapter) Release() { s.lease.Release() }

func (s *kvLeaseAdapter) Query(ctx context.Context, stmt godatabase.Statement, maxRows int) (godatabase.QueryResult, error) {
	return s.lease.Handle.Query(ctx, stmt, maxRows)
}

func (s *kvLeaseAdapter) Execute(ctx context.Context, stmt godatabase.Statement) (godatabase.QueryResult, error) {
	return s.lease.Handle.Execute(ctx, stmt)
}

func (s *kvLeaseAdapter) Batch(ctx context.Context, stmts []godatabase.Statement, transactional bool) ([]godatabase.QueryResult, error) {
	return s.lease.Handle.Batch(ctx, stmts, transactional)
}

// Raw 返回底层 *sql.DB（KV 仓库自管事务用；连接生命周期由 Handle 管理）。
func (s *kvLeaseAdapter) Raw() *sql.DB { return s.lease.Handle.Conn() }

// NotifyWrite 触发 Handle 的 onWrite 回调（CatalogSyncer.MarkDirty 等）。
func (s *kvLeaseAdapter) NotifyWrite(ctx context.Context) { s.lease.Handle.NotifyWrite(ctx) }
