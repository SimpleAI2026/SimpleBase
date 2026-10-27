package dbbridge

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// memFactory 用内存 sqlite 充当数据库引擎，只验证 registry 与 catalog 经适配层的协作。
type memFactory struct{}

func (memFactory) Open(ctx context.Context, _ godatabase.Spec, _ godatabase.AccessMode) (*sql.DB, error) {
	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func newAlwaysOpenCatalog(t *testing.T) (*catalog.Service, catalog.Repository, auth.Principal, string, string) {
	t.Helper()
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	for _, stmt := range []string{
		`CREATE TABLE sys_tenants (id VARCHAR NOT NULL, name VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE sys_projects (id VARCHAR NOT NULL, tenant_id VARCHAR NOT NULL, name VARCHAR NOT NULL, database_path VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE sys_databases (
			id VARCHAR NOT NULL, tenant_id VARCHAR NOT NULL, project_id VARCHAR NOT NULL, name VARCHAR NOT NULL,
			kind VARCHAR NOT NULL, status VARCHAR NOT NULL, storage_prefix VARCHAR NOT NULL, format_version BIGINT NOT NULL,
			deleted_at TIMESTAMP, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL,
			data_model VARCHAR NOT NULL DEFAULT 'collection')`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	repo := catalog.NewSQLRepository(raw)
	keys := objectstore.KeyBuilder{RootPrefix: "simplebase"}
	svc := catalog.NewService(repo, keys, nil, objectstore.DuckLakeStorage{}, nil)
	tenantID := "tenant001"
	projectID := "regprj01"
	now := time.Now().UTC()
	ctx := context.Background()
	if err := repo.CreateTenant(ctx, catalog.Tenant{ID: tenantID, Name: "t", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateProject(ctx, catalog.Project{ID: projectID, TenantID: tenantID, Name: "p", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	principal := auth.Principal{
		APIKeyID:   "k",
		TenantID:   tenantID,
		ProjectIDs: map[string]struct{}{projectID: {}},
	}
	return svc, repo, principal, tenantID, projectID
}

func TestPlan_CloseIdleThenQueryKeepsCatalogReady(t *testing.T) {
	svc, _, principal, tenantID, projectID := newAlwaysOpenCatalog(t)
	ctx := context.Background()
	db, err := svc.CreateDatabase(ctx, catalog.CreateDatabaseInput{
		TenantID: tenantID, ProjectID: projectID, Name: "live",
	})
	if err != nil {
		t.Fatal(err)
	}
	if db.Status != catalog.DatabaseReady {
		t.Fatalf("create status=%s", db.Status)
	}

	raw := registry.New(memFactory{}, ResolverFrom(svc), registry.Options{Writable: true, IdleTimeout: time.Millisecond}, nil, nil)
	reg := RegistryFrom(raw)
	lease, err := reg.Acquire(ctx, db, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lease.Handle.Query(ctx, godatabase.Statement{SQL: "SELECT 1"}, 1); err != nil {
		t.Fatal(err)
	}
	lease.Release()
	time.Sleep(5 * time.Millisecond)
	if err := raw.CloseIdle(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}

	got, err := svc.GetDatabase(ctx, principal, projectID, db.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != catalog.DatabaseReady || got.DeletedAt != nil {
		t.Fatalf("after idle: %+v", got)
	}

	lease2, err := reg.Acquire(ctx, got, godatabase.ReadWrite)
	if err != nil {
		t.Fatalf("re-acquire after idle: %v", err)
	}
	if _, err := lease2.Handle.Query(ctx, godatabase.Statement{SQL: "SELECT 1"}, 1); err != nil {
		t.Fatalf("query after idle: %v", err)
	}
	lease2.Release()

	again, err := svc.GetDatabase(ctx, principal, projectID, db.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != catalog.DatabaseReady || again.DeletedAt != nil {
		t.Fatalf("after re-query: %+v", again)
	}
}

// ---- 适配层自身的保护性用例 ----

func TestSpecOfKeepsSystemKindAndStatus(t *testing.T) {
	db := catalog.Database{
		ID: "i", TenantID: "t", ProjectID: "p", Name: "ignored", Kind: catalog.DatabaseKindSystem,
		Status: catalog.DatabaseDeleting, DatabasePath: "p/database.duckdb", DataModel: "sql",
	}
	s := SpecOf(db)
	if !s.IsSystem() || catalog.IsSystemDatabase(db) != s.IsSystem() {
		t.Fatal("system kind must survive conversion (system DB protection depends on it)")
	}
	if s.Status != godatabase.StatusDeleting || s.ID != "i" || s.TenantID != "t" || s.ProjectID != "p" || s.DatabasePath != "p/database.duckdb" {
		t.Fatalf("spec = %+v", s)
	}
	if SpecOf(catalog.Database{Kind: catalog.DatabaseKindUser}).IsSystem() {
		t.Fatal("user database must not be system")
	}
	if SpecOf(catalog.Database{}).IsSystem() {
		t.Fatal("empty kind must be treated as user")
	}
}

// 状态字符串必须与库常量一致：registry 靠它拒绝 deleting/deleted/degraded。
func TestStatusValuesStayInSync(t *testing.T) {
	pairs := map[catalog.DatabaseStatus]godatabase.Status{
		catalog.DatabaseCreating: godatabase.StatusCreating,
		catalog.DatabaseReady:    godatabase.StatusReady,
		catalog.DatabaseDegraded: godatabase.StatusDegraded,
		catalog.DatabaseDeleting: godatabase.StatusDeleting,
		catalog.DatabaseDeleted:  godatabase.StatusDeleted,
	}
	for c, d := range pairs {
		if string(c) != string(d) {
			t.Errorf("catalog %q != database %q", c, d)
		}
	}
}

func TestRoundTripWriteGate(t *testing.T) {
	var got catalog.Database
	gate := WriteGate(func(db catalog.Database) error { got = db; return errors.New("lost") })
	err := gate(godatabase.Spec{ID: "i", ProjectID: "p", Kind: godatabase.KindSystem})
	if err == nil || got.ID != "i" || got.ProjectID != "p" || got.Kind != catalog.DatabaseKindSystem {
		t.Fatalf("err=%v got=%+v", err, got)
	}
	if WriteGate(nil) != nil {
		t.Fatal("nil gate must stay nil")
	}
}

// typed-nil 陷阱：nil 指针装进接口会得到非 nil 接口，库内 `x != nil` 判断失效。
func TestNilHostValuesBecomeUntypedNil(t *testing.T) {
	if LoggerFrom(nil) != nil {
		t.Fatal("LoggerFrom(nil) must be an untyped nil interface")
	}
	if ObserverFrom(nil) != nil {
		t.Fatal("ObserverFrom(nil) must be an untyped nil interface")
	}
	if ResolverFrom(nil) != nil {
		t.Fatal("ResolverFrom(nil) must be an untyped nil interface")
	}
	if RegistryFrom(nil) != nil {
		t.Fatal("RegistryFrom(nil) must be an untyped nil interface (api uses `reg == nil` to detect read-only)")
	}
}

func TestResolverEnforcesTenantOwnership(t *testing.T) {
	svc, _, _, _, projectID := newAlwaysOpenCatalog(t)
	r := ResolverFrom(svc)
	if _, err := r.ResolveDatabasePath(context.Background(), "other-tenant", projectID); err == nil {
		t.Fatal("cross-tenant resolve must be rejected by catalog")
	}
}

// ObserverFrom 必须把库的每个指标点映射到与重构前相同的 Prometheus 指标与标签。
func TestObserverMapsToPrometheusMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := observability.NewMetrics(reg)
	o := ObserverFrom(m)

	o.OpenHandleDelta(1)
	o.OpenHandleDelta(1)
	o.OpenHandleDelta(-1)
	o.CacheBytes(1234)
	o.CacheEvicted()
	o.CatalogLocalAhead()
	o.CatalogSync("ok", 2*time.Second, false)
	o.CatalogSync("error", time.Second, true)
	o.CatalogSyncLag("db1", 7)
	o.CatalogSyncConflict("db1")
	o.CatalogSize("db1", 4096)

	if v := readMetric(t, m.DBOpenHandles); v != 1 {
		t.Errorf("DBOpenHandles = %v, want 1", v)
	}
	if v := readMetric(t, m.CacheBytes); v != 1234 {
		t.Errorf("CacheBytes = %v", v)
	}
	if v := readMetric(t, m.CacheEvictions); v != 1 {
		t.Errorf("CacheEvictions = %v", v)
	}
	if v := readMetric(t, m.CatalogLocalAhead); v != 1 {
		t.Errorf("CatalogLocalAhead = %v", v)
	}
	if v := readMetric(t, m.CatalogSyncFailures); v != 1 {
		t.Errorf("CatalogSyncFailures = %v, want 1 (only the failed sync)", v)
	}
	if v := readMetric(t, m.CatalogSyncTotal.WithLabelValues("ok")); v != 1 {
		t.Errorf("CatalogSyncTotal{ok} = %v", v)
	}
	if v := readMetric(t, m.CatalogSyncTotal.WithLabelValues("error")); v != 1 {
		t.Errorf("CatalogSyncTotal{error} = %v", v)
	}
	if v := readMetric(t, m.CatalogSyncLag.WithLabelValues("db1")); v != 7 {
		t.Errorf("CatalogSyncLag = %v", v)
	}
	if v := readMetric(t, m.CatalogSyncConflicts.WithLabelValues("db1")); v != 1 {
		t.Errorf("CatalogSyncConflicts = %v", v)
	}
	if v := readMetric(t, m.CatalogSizeBytes.WithLabelValues("db1")); v != 4096 {
		t.Errorf("CatalogSizeBytes = %v", v)
	}
}

// 单个指标字段为 nil 时必须静默跳过（与重构前的字段级 nil 判断一致），不得 panic。
func TestObserverToleratesNilMetricFields(t *testing.T) {
	o := ObserverFrom(&observability.Metrics{})
	o.OpenHandleDelta(1)
	o.CatalogSync("ok", time.Second, true)
	o.CatalogSyncLag("d", 1)
	o.CatalogSyncConflict("d")
	o.CatalogSize("d", 1)
	o.CatalogLocalAhead()
}

func TestLoggerFromPassesThroughAndCatalogOfRoundTrips(t *testing.T) {
	zl := observability.NewLogger("debug", "json", io.Discard)
	if LoggerFrom(zl) == nil {
		t.Fatal("non-nil logger must stay non-nil")
	}
	LoggerFrom(zl).Info("x") // 不得 panic

	in := godatabase.Spec{ID: "i", TenantID: "t", ProjectID: "p", Kind: godatabase.KindSystem, Status: godatabase.StatusReady, DatabasePath: "p/database.duckdb"}
	if got := SpecOf(CatalogOf(in)); got != in {
		t.Fatalf("round trip: %+v != %+v", got, in)
	}
	if !IsSystem(in) {
		t.Fatal("IsSystem")
	}
}

// readMetric 读取单个 Counter / Gauge 的当前值（不依赖 prometheus/testutil，避免改 go.mod）。
func readMetric(t *testing.T, c prometheus.Collector) float64 {
	t.Helper()
	ch := make(chan prometheus.Metric, 1)
	c.Collect(ch)
	close(ch)
	m, ok := <-ch
	if !ok {
		t.Fatal("collector produced no metric")
	}
	var out dto.Metric
	if err := m.Write(&out); err != nil {
		t.Fatal(err)
	}
	switch {
	case out.Counter != nil:
		return out.Counter.GetValue()
	case out.Gauge != nil:
		return out.Gauge.GetValue()
	}
	t.Fatal("unsupported metric type")
	return 0
}
