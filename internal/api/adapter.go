// adapter.go 提供 catalog.Service + registry.Registry 到 api.DatabaseService 的适配器。
//
// handler 只依赖 api.DatabaseService 接口；本文件把具体实现桥接到该接口，
// 使 handler 可测试且不直接依赖 catalog/registry 具体类型。
package api

import (
	"context"
	"database/sql"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/registry"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// CatalogService 是 handler 依赖的 catalog.Service 的最小接口。
type CatalogService interface {
	CreateDatabase(ctx context.Context, in catalog.CreateDatabaseInput) (catalog.Database, error)
	GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error)
	BeginDeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	DeleteDatabaseSync(ctx context.Context, principal auth.Principal, projectID, databaseID string, closer func(context.Context, string) error, purger catalog.StoragePurger) (catalog.Database, error)
	ResolveProjectTenant(ctx context.Context, projectID string) (string, error)
	ListProjects(ctx context.Context, principal auth.Principal) ([]catalog.Project, error)
	CreateProject(ctx context.Context, principal auth.Principal, in catalog.CreateProjectInput) (catalog.Project, error)
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
	purger   objectstore.Deleter // 可为 nil（DevMode）
}

// NewDatabaseServiceAdapter 构造 DatabaseService。purger 用于删库时同步清理平面 B。
func NewDatabaseServiceAdapter(cat CatalogService, reg RegistryService, purger objectstore.Deleter) DatabaseService {
	return &dbServiceAdapter{catalog: cat, registry: reg, purger: purger}
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

func (a *dbServiceAdapter) DeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	var purger catalog.StoragePurger
	if a.purger != nil {
		purger = a.purger
	}
	return a.catalog.DeleteDatabaseSync(ctx, principal, projectID, databaseID, a.registry.CloseDatabase, purger)
}

// sqlServiceAdapter 把 CatalogService + RegistryService 适配为 SQLService。
type sqlServiceAdapter struct {
	catalog  CatalogService
	registry RegistryService
	system   SystemStoreRef // 可为 nil；系统库桥接
}

// SystemStoreRef 是系统库常驻连接的最小引用（*systemdb.Store 满足）。
type SystemStoreRef interface {
	Meta() catalog.Database
	DB() *sql.DB
}

// NewSQLServiceAdapter 构造 SQLService。
// system 可为 nil（测试）；非 nil 时系统库的 Acquire 桥接到 systemdb 常驻连接，
// 不再经 registry 打开第二实例（用户 factory 的 CacheDir 与系统库不同，会得到空 catalog）。
func NewSQLServiceAdapter(cat CatalogService, reg RegistryService, system SystemStoreRef) SQLService {

	return &sqlServiceAdapter{catalog: cat, registry: reg, system: system}
}

func (a *sqlServiceAdapter) GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	return a.catalog.GetDatabase(ctx, principal, projectID, databaseID)
}

func (a *sqlServiceAdapter) Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (SQLLease, error) {
	// 系统库：桥接到 systemdb 常驻连接（只读查询），不经 registry。
	if catalog.IsSystemDatabase(db) && a.system != nil && mode == database.ReadOnly {
		return &systemLeaseAdapter{conn: a.system.DB()}, nil
	}
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

// systemLeaseAdapter 包装系统库常驻 *sql.DB 为只读租约。
// Release 不关闭连接（连接由 systemdb.Store 管理生命周期）。
type systemLeaseAdapter struct {
	conn *sql.DB
}

func (s *systemLeaseAdapter) Release() {}

func (s *systemLeaseAdapter) Query(ctx context.Context, stmt database.Statement, maxRows int) (database.QueryResult, error) {
	return database.Query(ctx, s.conn, stmt, maxRows)
}

func (s *systemLeaseAdapter) Execute(ctx context.Context, stmt database.Statement) (database.QueryResult, error) {
	return database.QueryResult{}, catalog.ErrSystemProtected
}

func (s *systemLeaseAdapter) Batch(ctx context.Context, stmts []database.Statement, transactional bool) ([]database.QueryResult, error) {
	return nil, catalog.ErrSystemProtected
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
