package datahttp

import (
	"time"

	"context"
	"database/sql"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

// timedLease 包装 SQLLease，把每次 DB 操作耗时计入 httpkit.StageDBQueue/httpkit.StageDBExec。
type timedLease struct {
	inner SQLLease
}

// WrapTimedLease 在启用分段计时且 ctx 上存在 timer 时包装 lease；
// 否则原样返回（no-op）。
func WrapTimedLease(ctx context.Context, lease SQLLease) SQLLease {
	if lease == nil || httpkit.StageTimerFrom(ctx) == nil {
		return lease
	}
	return &timedLease{inner: lease}
}

// observeDB 包裹一次 DB 调用：排队增量记 httpkit.StageDBQueue，其余记 httpkit.StageDBExec。
func (t *timedLease) observeDB(ctx context.Context, conn *sql.DB, op func() error) error {
	timer := httpkit.StageTimerFrom(ctx)
	if timer == nil {
		return op()
	}
	var before sql.DBStats
	if conn != nil {
		before = conn.Stats()
	}
	start := time.Now()
	err := op()
	elapsed := time.Since(start)
	if conn != nil {
		if waited := conn.Stats().WaitDuration - before.WaitDuration; waited > 0 {
			timer.Observe(httpkit.StageDBQueue, waited)
			elapsed -= waited
		}
	}
	timer.Observe(httpkit.StageDBExec, elapsed)
	return err
}

func (t *timedLease) Release() { t.inner.Release() }

func (t *timedLease) Query(ctx context.Context, stmt godatabase.Statement, maxRows int) (godatabase.QueryResult, error) {
	var res godatabase.QueryResult
	err := t.observeDB(ctx, t.inner.Raw(), func() error {
		var qerr error
		res, qerr = t.inner.Query(ctx, stmt, maxRows)
		return qerr
	})
	if err == nil {
		if timer := httpkit.StageTimerFrom(ctx); timer != nil {
			timer.SetMeta("rows", int64(len(res.Rows)))
		}
	}
	return res, err
}

func (t *timedLease) Execute(ctx context.Context, stmt godatabase.Statement) (godatabase.QueryResult, error) {
	var res godatabase.QueryResult
	err := t.observeDB(ctx, t.inner.Raw(), func() error {
		var xerr error
		res, xerr = t.inner.Execute(ctx, stmt)
		return xerr
	})
	return res, err
}

func (t *timedLease) Batch(ctx context.Context, stmts []godatabase.Statement, transactional bool) ([]godatabase.QueryResult, error) {
	var res []godatabase.QueryResult
	err := t.observeDB(ctx, t.inner.Raw(), func() error {
		var berr error
		res, berr = t.inner.Batch(ctx, stmts, transactional)
		return berr
	})
	return res, err
}

func (t *timedLease) Raw() *sql.DB { return t.inner.Raw() }

// NotifyWrite 计入 httpkit.StageOnWrite（写后同步链路）。
func (t *timedLease) NotifyWrite(ctx context.Context) {
	timer := httpkit.StageTimerFrom(ctx)
	if timer == nil {
		t.inner.NotifyWrite(ctx)
		return
	}
	scope := timer.StageScope(httpkit.StageOnWrite)
	defer scope.Done()
	t.inner.NotifyWrite(ctx)
}

// stageAuthObserver 把 auth.AuthObserver 事件写入请求级 httpkit.StageTimer
// （planv5.0 §4 P0.1）。子阶段只细分 httpkit.StageAuth，不进入 httpkit.PerfStages 归因求和，
// 与既有 httpkit.StageAuth 总口径不重叠双算；缓存命中状态记录为 meta。
//
// 观察者是服务级单例：每次回调从 ctx 提取当前请求的 httpkit.StageTimer，

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
	Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (*registry.Lease, error)
	CloseDatabase(ctx context.Context, databaseID string) error
}

// dbServiceAdapter 把 CatalogService + RegistryService 适配为 DatabaseService。
type dbServiceAdapter struct {
	catalog  CatalogService
	registry RegistryService
	purger   catalog.StoragePurger // 可为 nil（本地模式）
}

// NewDatabaseServiceAdapter 构造 DatabaseService。purger 用于删库时清理项目数据库文件。
func NewDatabaseServiceAdapter(cat CatalogService, reg RegistryService, purger catalog.StoragePurger) DatabaseService {
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
	timer := httpkit.StageTimerFrom(ctx)
	scope := timer.StageScope(httpkit.StageCatalog)
	defer scope.Done()
	return a.catalog.GetDatabase(ctx, principal, projectID, databaseID)
}

func (a *sqlServiceAdapter) Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (SQLLease, error) {
	// 系统库：桥接到 systemdb 常驻连接（只读查询），不经 registry。
	if catalog.IsSystemDatabase(db) && a.system != nil && mode == godatabase.ReadOnly {
		return WrapTimedLease(ctx, &systemLeaseAdapter{conn: a.system.DB()}), nil
	}
	timer := httpkit.StageTimerFrom(ctx)
	acquireScope := timer.StageScope(httpkit.StageAcquire)
	l, err := a.registry.Acquire(ctx, db, mode)
	acquireScope.Done()
	if err != nil {
		return nil, err
	}
	return WrapTimedLease(ctx, &sqlLeaseAdapter{lease: l}), nil
}

// ListDatabases 使 sqlServiceAdapter 同时满足 DataService。
// §7.2 M1：catalog 列表查询计入 catalog_lookup 段。
func (a *sqlServiceAdapter) ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error) {
	timer := httpkit.StageTimerFrom(ctx)
	scope := timer.StageScope(httpkit.StageCatalog)
	defer scope.Done()
	return a.catalog.ListDatabases(ctx, principal, projectID, page)
}

// systemLeaseAdapter 包装系统库常驻 *sql.DB 为只读租约。
// Release 不关闭连接（连接由 systemdb.Store 管理生命周期）。
type systemLeaseAdapter struct {
	conn *sql.DB
}

func (s *systemLeaseAdapter) Release() {}

func (s *systemLeaseAdapter) Query(ctx context.Context, stmt godatabase.Statement, maxRows int) (godatabase.QueryResult, error) {
	return godatabase.Query(ctx, s.conn, stmt, maxRows)
}

func (s *systemLeaseAdapter) Execute(ctx context.Context, stmt godatabase.Statement) (godatabase.QueryResult, error) {
	return godatabase.QueryResult{}, catalog.ErrSystemProtected
}

func (s *systemLeaseAdapter) Batch(ctx context.Context, stmts []godatabase.Statement, transactional bool) ([]godatabase.QueryResult, error) {
	return nil, catalog.ErrSystemProtected
}

// Raw 返回系统库常驻连接（只读用途；写由 Execute/Batch 层拒绝）。
func (s *systemLeaseAdapter) Raw() *sql.DB { return s.conn }

// NotifyWrite 对系统库是 no-op（系统库写已被拒绝，不存在外部写路径）。
func (s *systemLeaseAdapter) NotifyWrite(ctx context.Context) {}

// sqlLeaseAdapter 包装 registry.Lease，暴露 Handle 的 Query/Execute/Batch。
type sqlLeaseAdapter struct {
	lease *registry.Lease
}

func (s *sqlLeaseAdapter) Release() {
	s.lease.Release()
}

func (s *sqlLeaseAdapter) Query(ctx context.Context, stmt godatabase.Statement, maxRows int) (godatabase.QueryResult, error) {
	return s.lease.Handle.Query(ctx, stmt, maxRows)
}

func (s *sqlLeaseAdapter) Execute(ctx context.Context, stmt godatabase.Statement) (godatabase.QueryResult, error) {
	return s.lease.Handle.Execute(ctx, stmt)
}

func (s *sqlLeaseAdapter) Batch(ctx context.Context, stmts []godatabase.Statement, transactional bool) ([]godatabase.QueryResult, error) {
	return s.lease.Handle.Batch(ctx, stmts, transactional)
}

// Raw 返回底层 *sql.DB（KV 仓库自管事务用；连接生命周期由 Handle 管理）。
func (s *sqlLeaseAdapter) Raw() *sql.DB { return s.lease.Handle.Conn() }

// NotifyWrite 触发 Handle 的 onWrite 回调（CatalogSyncer.MarkDirty 等）。
func (s *sqlLeaseAdapter) NotifyWrite(ctx context.Context) { s.lease.Handle.NotifyWrite(ctx) }
