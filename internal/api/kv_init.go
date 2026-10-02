// kv_init.go 提供项目级 KV catalog 的初始化钩子（key-value-ducklake-plan §2）。
//
// 项目 KV 是 catalog 里一行 kind=kv 的内部记录，随「项目创建」同步建立
//（失败不回滚项目，只记 warn；下一次 KV 请求会补建）。本文件同时承担：
//   - KVService 适配器：把 catalog.Service + RegistryService 桥接为
//     api.KVService（GetKVDatabase / CreateKVDatabase / Acquire）；
//   - 项目 KV 初始化：EnsureSchema（幂等 CREATE ... IF NOT EXISTS），
//     保证新建项目后控制台与 HTTP 可直接写入，不存在「未初始化」中间态。
package api

import (
	"context"
	"database/sql"
	"fmt"

	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/kv"
	"github.com/linkxzhou/SimpleBase/internal/database/registry"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// KVInitializer 初始化指定库的 KV 系统表（幂等）。
type KVInitializer interface {
	InitKV(ctx context.Context, db catalog.Database) error
}

type kvInitializer struct {
	reg    RegistryService
	logger observability.Logger
}

// NewKVInitializer 构造 KV 初始化器。reg 为 nil 时返回 nil（readonly 实例）。
func NewKVInitializer(reg RegistryService, logger observability.Logger) KVInitializer {
	if reg == nil {
		return nil
	}
	return &kvInitializer{reg: reg, logger: logger}
}

// InitKV 初始化指定库的 kv schema 与 6 张系统表。系统库跳过（无 KV 语义且禁写）。
// 失败时返回错误并记 warn；调用方（建项目）不因此失败——首次 KV 写会兜底补建。
func (k *kvInitializer) InitKV(ctx context.Context, db catalog.Database) error {
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

func (k *kvInitializer) ensure(ctx context.Context, db catalog.Database) error {
	lease, err := k.reg.Acquire(ctx, db, database.ReadWrite)
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
	timer := StageTimerFrom(ctx)
	scope := timer.StageScope(StageCatalog)
	defer scope.Done()
	return a.catalog.GetKVDatabase(ctx, tenantID, projectID)
}

func (a *kvServiceAdapter) CreateKVDatabase(ctx context.Context, tenantID, projectID string) (catalog.Database, error) {
	timer := StageTimerFrom(ctx)
	scope := timer.StageScope(StageCatalog)
	defer scope.Done()
	return a.catalog.CreateKVDatabase(ctx, tenantID, projectID)
}

func (a *kvServiceAdapter) Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (SQLLease, error) {
	timer := StageTimerFrom(ctx)
	acquireScope := timer.StageScope(StageAcquire)
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

func (s *kvLeaseAdapter) Query(ctx context.Context, stmt database.Statement, maxRows int) (database.QueryResult, error) {
	return s.lease.Handle.Query(ctx, stmt, maxRows)
}

func (s *kvLeaseAdapter) Execute(ctx context.Context, stmt database.Statement) (database.QueryResult, error) {
	return s.lease.Handle.Execute(ctx, stmt)
}

func (s *kvLeaseAdapter) Batch(ctx context.Context, stmts []database.Statement, transactional bool) ([]database.QueryResult, error) {
	return s.lease.Handle.Batch(ctx, stmts, transactional)
}

// Raw 返回底层 *sql.DB（KV 仓库自管事务用；连接生命周期由 Handle 管理）。
func (s *kvLeaseAdapter) Raw() *sql.DB { return s.lease.Handle.Conn() }

// NotifyWrite 触发 Handle 的 onWrite 回调（CatalogSyncer.MarkDirty 等）。
func (s *kvLeaseAdapter) NotifyWrite(ctx context.Context) { s.lease.Handle.NotifyWrite(ctx) }
