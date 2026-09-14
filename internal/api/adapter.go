// adapter.go 提供 catalog.Service + registry.Registry 到 api.DatabaseService 的适配器。
//
// handler 只依赖 api.DatabaseService 接口；本文件把具体实现桥接到该接口，
// 使 handler 可测试且不直接依赖 catalog/registry 具体类型。
package api

import (
	"context"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/registry"
)

// CatalogService 是 handler 依赖的 catalog.Service 的最小接口。
// 避免强制要求 *catalog.Service 具体类型，便于测试。
type CatalogService interface {
	CreateDatabase(ctx context.Context, in catalog.CreateDatabaseInput) (catalog.Database, error)
	GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error)
	BeginDeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	ResolveProjectTenant(ctx context.Context, projectID string) (string, error)
}

// RegistryService 是 handler 依赖的 registry 的最小接口。
type RegistryService interface {
	Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (*registry.Lease, error)
	CloseDatabase(ctx context.Context, databaseID string) error
}

// dbServiceAdapter 把 CatalogService + RegistryService 适配为 DatabaseService。
type dbServiceAdapter struct {
	catalog  CatalogService
	registry RegistryService
}

// NewDatabaseServiceAdapter 构造 DatabaseService 的实现。
func NewDatabaseServiceAdapter(cat CatalogService, reg RegistryService) DatabaseService {
	return &dbServiceAdapter{catalog: cat, registry: reg}
}

func (a *dbServiceAdapter) CreateDatabase(ctx context.Context, in catalog.CreateDatabaseInput) (catalog.Database, error) {
	return a.catalog.CreateDatabase(ctx, in)
}

func (a *dbServiceAdapter) GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	return a.catalog.GetDatabase(ctx, principal, projectID, databaseID)
}

func (a *dbServiceAdapter) ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error) {
	return a.catalog.ListDatabases(ctx, principal, projectID, page)
}

func (a *dbServiceAdapter) BeginDeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	return a.catalog.BeginDeleteDatabase(ctx, principal, projectID, databaseID)
}

func (a *dbServiceAdapter) Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (Lease, error) {
	l, err := a.registry.Acquire(ctx, db, mode)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (a *dbServiceAdapter) CloseDatabase(ctx context.Context, databaseID string) error {
	return a.registry.CloseDatabase(ctx, databaseID)
}

// sqlServiceAdapter 把 CatalogService + RegistryService 适配为 SQLService。
// 复用 dbServiceAdapter 的 catalog 查询能力，Acquire 返回带 SQL 执行能力的租约。
type sqlServiceAdapter struct {
	catalog  CatalogService
	registry RegistryService
}

// NewSQLServiceAdapter 构造 SQLService 的实现。
// catalog 和 registry 与 NewDatabaseServiceAdapter 共享同一实例。
func NewSQLServiceAdapter(cat CatalogService, reg RegistryService) SQLService {
	return &sqlServiceAdapter{catalog: cat, registry: reg}
}

func (a *sqlServiceAdapter) GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	return a.catalog.GetDatabase(ctx, principal, projectID, databaseID)
}

func (a *sqlServiceAdapter) Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (SQLLease, error) {
	l, err := a.registry.Acquire(ctx, db, mode)
	if err != nil {
		return nil, err
	}
	return &sqlLeaseAdapter{lease: l}, nil
}

// ListDatabases 使 sqlServiceAdapter 同时满足 DataService。
func (a *sqlServiceAdapter) ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error) {
	return a.catalog.ListDatabases(ctx, principal, projectID, page)
}

// sqlLeaseAdapter 包装 registry.Lease，暴露 Handle 的 Query/Execute/Batch。
type sqlLeaseAdapter struct {
	lease *registry.Lease
}

func (s *sqlLeaseAdapter) Release() {
	s.lease.Release()
}

func (s *sqlLeaseAdapter) Query(ctx context.Context, stmt database.Statement, maxRows int) (database.QueryResult, error) {
	return s.lease.Handle.Query(ctx, stmt, maxRows)
}

func (s *sqlLeaseAdapter) Execute(ctx context.Context, stmt database.Statement) (database.QueryResult, error) {
	return s.lease.Handle.Execute(ctx, stmt)
}

func (s *sqlLeaseAdapter) Batch(ctx context.Context, stmts []database.Statement, transactional bool) ([]database.QueryResult, error) {
	return s.lease.Handle.Batch(ctx, stmts, transactional)
}
