package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := ApplySystemMigrations(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStoreForTest(db)
}

func TestApplySystemMigrationsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := ApplySystemMigrations(ctx, db); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := ApplySystemMigrations(ctx, db); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	var n int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_migration_versions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if int(n) != MigrationCount() {
		t.Fatalf("versions=%d want %d", n, MigrationCount())
	}
}

func TestPlan_MigrateRetiredOpenCloseStatuses(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := ApplySystemMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	insert := func(id, status string, deleted bool) {
		t.Helper()
		var deletedAt any
		if deleted {
			deletedAt = now
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO sys_databases(
			id, tenant_id, project_id, name, kind, status, storage_prefix, format_version, deleted_at, created_at, updated_at)
			VALUES(?, 't', 'p', ?, 'user', ?, 'prefix', 1, ?, ?, ?)`,
			id, id, status, deletedAt, now, now); err != nil {
			t.Fatal(err)
		}
	}
	insert("closed", "closed", false)
	insert("opening", "opening", false)
	insert("closing", "closing", false)
	insert("recovering", "recovering", false)
	insert("degraded", "degraded", false)
	insert("creating", "creating", false)
	insert("deleted-closed", "closed", true)

	var dataModel string
	if err := db.QueryRowContext(ctx, `SELECT data_model FROM sys_databases WHERE id = ?`, "creating").Scan(&dataModel); err != nil {
		t.Fatal(err)
	}
	if dataModel != catalog.DataModelCollection {
		t.Fatalf("existing/default data_model=%q", dataModel)
	}

	if _, err := db.ExecContext(ctx, retireOpenCloseStatusSQL); err != nil {
		t.Fatal(err)
	}
	statusOf := func(id string) string {
		t.Helper()
		var status string
		var deletedAt sql.NullTime
		if err := db.QueryRowContext(ctx, `SELECT status, deleted_at FROM sys_databases WHERE id = ?`, id).Scan(&status, &deletedAt); err != nil {
			t.Fatal(err)
		}
		if id == "deleted-closed" {
			if !deletedAt.Valid || status != "closed" {
				t.Fatalf("soft-deleted closed row: status=%s deleted=%v", status, deletedAt.Valid)
			}
			return status
		}
		if deletedAt.Valid {
			t.Fatalf("%s unexpectedly soft-deleted", id)
		}
		return status
	}
	for _, id := range []string{"closed", "opening", "closing", "recovering"} {
		if got := statusOf(id); got != "ready" {
			t.Fatalf("%s status=%s", id, got)
		}
	}
	if got := statusOf("degraded"); got != "degraded" {
		t.Fatalf("degraded became %s", got)
	}
	if got := statusOf("creating"); got != "creating" {
		t.Fatalf("creating became %s", got)
	}
	statusOf("deleted-closed")
}

func TestLocatorRoundTrip(t *testing.T) {
	dir := t.TempDir()
	loc := Locator{DatabaseID: uuid.NewString(), TenantID: catalog.ReservedTenantID, Name: DefaultName, CreatedAt: time.Now().UTC()}
	if err := SaveLocator(dir, loc); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadLocator(dir)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.DatabaseID != loc.DatabaseID || got.TenantID != loc.TenantID {
		t.Fatalf("locator mismatch: %+v", got)
	}
}

func TestSQLRepositoryOnSystemSchema(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := ApplySystemMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := catalog.NewSQLRepository(db)
	tenantID := catalog.ReservedTenantID
	projectID := catalog.DevProjectID
	if err := repo.CreateTenant(ctx, catalog.Tenant{ID: tenantID, Name: "t", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateProject(ctx, catalog.Project{ID: projectID, TenantID: tenantID, Name: "商城后台", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	authSvc := auth.NewService(auth.NewSQLAPIKeyRepository(db), "secret")
	if err := auth.CreateAPIKey(ctx, db, catalog.DevAPIKeyID, projectID, authSvc.HashKey(DevRawAPIKey), []auth.Permission{auth.ProjectAdmin}, time.Now()); err != nil {
		t.Fatal(err)
	}
	rec, err := authSvc.Authenticate(ctx, DevRawAPIKey)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if rec.TenantID != tenantID || !rec.CanAccessProject(projectID) {
		t.Fatalf("principal mismatch: %+v", rec)
	}

	store := &Store{db: db}
	if err := store.UpsertS3Object(ctx, projectID, "a.txt", 3, "", "text/plain", time.Now()); err != nil {
		t.Fatal(err)
	}
	objs, err := store.ListS3Objects(ctx, projectID, "", 10)
	if err != nil || len(objs) != 1 {
		t.Fatalf("list s3: n=%d err=%v", len(objs), err)
	}
	sess, err := store.CreateLLMSession(ctx, LLMSession{ProjectID: projectID, Title: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendLLMMessage(ctx, LLMMessage{SessionID: sess.ID, ProjectID: projectID, Role: "user", Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	store.RecordMetric(MetricSample{ProjectID: projectID, Name: "http_requests", Value: 1})
	if err := store.FlushMetrics(ctx); err != nil {
		t.Fatal(err)
	}
	sum, err := store.MetricsSummary(ctx, projectID)
	if err != nil || sum.TotalRequests < 1 {
		t.Fatalf("summary=%+v err=%v", sum, err)
	}
	store.RecordLog(LogEvent{ProjectID: projectID, Level: "info", Logger: "http", Message: "GET /v1"})
	if err := store.FlushLogs(ctx); err != nil {
		t.Fatal(err)
	}
	events, err := store.QueryLogs(ctx, LogQuery{ProjectID: projectID, Limit: 10})
	if err != nil || len(events) != 1 {
		t.Fatalf("logs n=%d err=%v", len(events), err)
	}
	if err := store.PutLLMSettings(ctx, LLMSettings{ProjectID: projectID, DefaultProvider: "openai", DefaultModel: "gpt-4o-mini", Temperature: 0.2, MaxTokens: 512}); err != nil {
		t.Fatal(err)
	}
	st, err := store.GetLLMSettings(ctx, projectID)
	if err != nil || st.DefaultModel != "gpt-4o-mini" {
		t.Fatalf("settings %+v err=%v", st, err)
	}
	if err := store.SeedDefaultCloudAgents(ctx, projectID); err != nil {
		t.Fatal(err)
	}
	agents, err := store.ListCloudAgents(ctx, projectID)
	if err != nil || len(agents) != 4 {
		t.Fatalf("agents n=%d err=%v", len(agents), err)
	}
	if err := store.SeedDefaultCloudAgents(ctx, projectID); err != nil {
		t.Fatal(err)
	}
	agents, err = store.ListCloudAgents(ctx, projectID)
	if err != nil || len(agents) != 4 {
		t.Fatalf("agents idempotent n=%d err=%v", len(agents), err)
	}
	th, err := store.CreateAgentThread(ctx, AgentThread{ProjectID: projectID, Title: "t1"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreateAgentRun(ctx, AgentRun{ThreadID: th.ID, ProjectID: projectID, AgentID: agents[0].ID, Status: AgentRunRunning, StartedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendAgentMessage(ctx, AgentMessage{ThreadID: th.ID, ProjectID: projectID, Role: "user", Content: "list dbs", AgentID: agents[0].ID, RunID: run.ID}); err != nil {
		t.Fatal(err)
	}
	msgs, err := store.ListAgentMessages(ctx, projectID, th.ID, 10)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("agent msgs n=%d err=%v", len(msgs), err)
	}
	obj, err := store.GetS3Object(ctx, projectID, "a.txt")
	if err != nil || obj.Key != "a.txt" {
		t.Fatalf("get s3 %+v err=%v", obj, err)
	}
	stats, err := store.LogLevelStats(ctx, projectID)
	if err != nil || len(stats) == 0 {
		t.Fatalf("log stats %+v err=%v", stats, err)
	}
}

func TestBootstrapDuckLake(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "system", "dbs")
	f := &ducklake.Factory{CacheDir: cacheDir, Options: ducklake.DefaultOptions(), Syncer: ducklake.NewLocalSyncer()}
	store, err := Bootstrap(context.Background(), BootstrapInput{
		LocatorDir: filepath.Join(dir, "system"),
		Name:       DefaultName,
		Factory:    f,
		Keys:       objectstore.KeyBuilder{RootPrefix: "simplebase", Environment: "test"},
	})
	if err != nil {
		t.Fatalf("bootstrap (needs CGO + ducklake extension): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	meta := store.Meta()
	if meta.Kind != catalog.DatabaseKindSystem {
		t.Fatalf("kind=%s", meta.Kind)
	}
	got, err := store.CatalogRepo().GetDatabase(context.Background(), catalog.ReservedSystemProjectID, meta.ID)
	if err != nil {
		t.Fatalf("backfill missing: %v", err)
	}
	if got.Kind != catalog.DatabaseKindSystem {
		t.Fatalf("sys_databases kind=%s", got.Kind)
	}
	if _, _, err := LoadLocator(filepath.Join(dir, "system")); err != nil {
		t.Fatal(err)
	}
}

func TestIsAdminProjectAndStoreLifecycle(t *testing.T) {
	if !IsAdminProject(catalog.ReservedSystemProjectID) {
		t.Fatal("admin project")
	}
	if IsAdminProject(catalog.DevProjectID) || IsAdminProject("") {
		t.Fatal("non-admin")
	}

	var nilStore *Store
	if err := nilStore.Ping(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil ping: %v", err)
	}
	if err := nilStore.Close(); err != nil {
		t.Fatal(err)
	}
	nilStore.RecordLog(LogEvent{Message: "x"})
	nilStore.RecordMetric(MetricSample{Name: "n"})
	if err := nilStore.FlushLogs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := nilStore.FlushMetrics(context.Background()); err != nil {
		t.Fatal(err)
	}

	empty := &Store{}
	if err := empty.Ping(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no db ping: %v", err)
	}
	if err := empty.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	s := NewStoreForTest(raw)
	if s.DB() != raw {
		t.Fatal("DB()")
	}
	_ = s.Meta()
	_ = s.Locator()
	if s.CatalogRepo() == nil || s.AuthRepo() == nil {
		t.Fatal("repos")
	}
	s.notifyWrite(context.Background()) // factory nil
	if err := s.Ping(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unmigrated ping: %v", err)
	}

	if err := ApplySystemMigrations(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Ping(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("closed ping: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSystemDatabaseDataModelMigration(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := ApplySystemMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	insert := func(id, kind, model string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, `INSERT INTO sys_databases(
			id, tenant_id, project_id, name, kind, status, storage_prefix, format_version, created_at, updated_at, data_model)
			VALUES(?, 't', 'p', ?, ?, 'ready', 'prefix', 1, ?, ?, ?)`,
			id, id, kind, now, now, model); err != nil {
			t.Fatal(err)
		}
	}
	insert("sys", "system", "collection")
	insert("user-col", "user", "collection")
	insert("user-sql", "user", "sql")
	if _, err := db.ExecContext(ctx, `DELETE FROM sys_migration_versions WHERE version = 44`); err != nil {
		t.Fatal(err)
	}
	if err := ApplySystemMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := ApplySystemMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	modelOf := func(id string) string {
		t.Helper()
		var model string
		if err := db.QueryRowContext(ctx, `SELECT data_model FROM sys_databases WHERE id = ?`, id).Scan(&model); err != nil {
			t.Fatal(err)
		}
		return model
	}
	if modelOf("sys") != catalog.DataModelSQL {
		t.Fatalf("system model %s", modelOf("sys"))
	}
	if modelOf("user-col") != catalog.DataModelCollection || modelOf("user-sql") != catalog.DataModelSQL {
		t.Fatalf("user models changed: %s %s", modelOf("user-col"), modelOf("user-sql"))
	}
}

func TestApplySystemMigrationsNilAndSplit(t *testing.T) {
	if err := ApplySystemMigrations(context.Background(), nil); err == nil || !errors.Is(err, catalog.ErrMigrationFailed) {
		t.Fatalf("nil db: %v", err)
	}
	if MigrationCount() < 20 {
		t.Fatalf("migration count %d", MigrationCount())
	}
	if got := splitStatements("  a ; ; b ; "); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("split: %#v", got)
	}
	if got := splitStatements(""); len(got) != 0 {
		t.Fatalf("empty split: %#v", got)
	}
}

func TestLocatorErrors(t *testing.T) {
	dir := t.TempDir()
	if _, ok, err := LoadLocator(dir); err != nil || ok {
		t.Fatalf("missing file: ok=%v err=%v", ok, err)
	}
	if err := os.WriteFile(filepath.Join(dir, locatorFileName), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadLocator(dir); err == nil || !strings.Contains(err.Error(), "parse locator") {
		t.Fatalf("bad json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, locatorFileName), []byte(`{"database_id":"","tenant_id":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadLocator(dir); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing fields: %v", err)
	}

	// unreadable path: file where directory expected
	fileAsDir := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveLocator(fileAsDir, Locator{DatabaseID: "a", TenantID: "b"}); err == nil {
		t.Fatal("save into file should fail")
	}
}

func TestBootstrapValidationAndBackfill(t *testing.T) {
	ctx := context.Background()
	if _, err := Bootstrap(ctx, BootstrapInput{}); err == nil || !strings.Contains(err.Error(), "factory") {
		t.Fatalf("no factory: %v", err)
	}
	if _, err := Bootstrap(ctx, BootstrapInput{Factory: &ducklake.Factory{}}); err == nil || !strings.Contains(err.Error(), "locator dir") {
		t.Fatalf("no locator: %v", err)
	}

	// LoadLocator error bubbles
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, locatorFileName), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Bootstrap(ctx, BootstrapInput{Factory: &ducklake.Factory{}, LocatorDir: dir}); err == nil {
		t.Fatal("expected parse error")
	}

	s := newTestStore(t)
	meta := catalog.Database{
		ID: uuid.NewString(), TenantID: catalog.ReservedTenantID, ProjectID: catalog.ReservedSystemProjectID,
		Name: DefaultName, Kind: catalog.DatabaseKindSystem, Status: catalog.DatabaseReady,
		StoragePrefix: "p", FormatVersion: 2, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := backfillSystemRow(ctx, s, meta); err != nil {
		t.Fatal(err)
	}
	if err := backfillSystemRow(ctx, s, meta); err != nil {
		t.Fatal(err)
	}
	// same ID other project → GetDatabase NotFound, CreateDatabase AlreadyExists (ignored)
	other := meta
	other.ProjectID = uuid.NewString()
	other.ID = uuid.NewString()
	if err := s.CatalogRepo().CreateDatabase(ctx, catalog.Database{
		ID: other.ID, TenantID: meta.TenantID, ProjectID: "other-proj", Name: "x",
		Kind: catalog.DatabaseKindSystem, Status: catalog.DatabaseReady, StoragePrefix: "p",
		FormatVersion: 2, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	other.ProjectID = catalog.ReservedSystemProjectID
	if err := backfillSystemRow(ctx, s, other); err != nil {
		t.Fatal(err)
	}

	closed := NewStoreForTest(s.db)
	_ = s.db.Close()
	if err := backfillSystemRow(ctx, closed, meta); err == nil {
		t.Fatal("closed db backfill should fail")
	}
}

func TestSeedAndIsDuplicate(t *testing.T) {
	if err := Seed(context.Background(), SeedInput{}); err == nil || !strings.Contains(err.Error(), "store is required") {
		t.Fatalf("nil store: %v", err)
	}
	if isDuplicate(nil) || !isDuplicate(errors.New("UNIQUE")) || !isDuplicate(errors.New("duplicate")) || !isDuplicate(errors.New("already exists")) || isDuplicate(errors.New("other")) {
		t.Fatal("isDuplicate")
	}

	s := newTestStore(t)
	ctx := context.Background()
	if err := Seed(ctx, SeedInput{Store: s}); err != nil {
		t.Fatal(err)
	}
	if err := Seed(ctx, SeedInput{Store: s}); err != nil {
		t.Fatal(err)
	}

	authSvc := auth.NewService(s.AuthRepo(), "secret")
	keys := objectstore.KeyBuilder{RootPrefix: "simplebase", Environment: "test"}
	cat := catalog.NewService(s.CatalogRepo(), keys, nil, objectstore.DuckLakeStorage{}, nil)
	if err := Seed(ctx, SeedInput{Store: s, Auth: authSvc, Catalog: cat, DevMode: true}); err != nil {
		t.Fatal(err)
	}
	// idempotent DevMode
	if err := Seed(ctx, SeedInput{Store: s, Auth: authSvc, Catalog: cat, DevMode: true}); err != nil {
		t.Fatal(err)
	}
	n, err := s.CountCloudAgents(ctx, catalog.DevProjectID)
	if err != nil || n != 4 {
		t.Fatalf("seeded agents n=%d err=%v", n, err)
	}
	dbs, _, err := s.CatalogRepo().ListDatabasesByKind(ctx, catalog.DevProjectID, catalog.DatabaseKindKV, catalog.Page{Limit: 10})
	if err != nil || len(dbs) != 1 || dbs[0].Name != catalog.KVDatabaseName || dbs[0].Kind != catalog.DatabaseKindKV {
		t.Fatalf("seeded project kv db: %+v err=%v", dbs, err)
	}
}

func TestJSONHelpers(t *testing.T) {
	if s, err := marshalStringSlice(nil); err != nil || s != "[]" {
		t.Fatalf("marshal nil: %s err=%v", s, err)
	}
	got, err := unmarshalStringSlice("")
	if err != nil || len(got) != 0 {
		t.Fatalf("empty: %+v err=%v", got, err)
	}
	got, err = unmarshalStringSlice("null")
	if err != nil || got == nil {
		t.Fatalf("null: %+v err=%v", got, err)
	}
	if _, err := unmarshalStringSlice("{"); err == nil {
		t.Fatal("bad json")
	}
}

func assertUnavailable(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestSettingsCRUDAndUnavailable(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	if _, err := nilStore.GetGlobalSetting(ctx, "k"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.ListGlobalSettings(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.PutGlobalSetting(ctx, "k", "{}"))
	if _, err := nilStore.ListProjectSettings(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.PutProjectSetting(ctx, "p", "k", "{}"))

	s := newTestStore(t)
	missing, err := s.GetGlobalSetting(ctx, "theme")
	if err != nil || missing.Key != "theme" || missing.ValueJSON != "" {
		t.Fatalf("missing: %+v err=%v", missing, err)
	}
	if err := s.PutGlobalSetting(ctx, "theme", `{"dark":true}`); err != nil {
		t.Fatal(err)
	}
	if err := s.PutGlobalSetting(ctx, "theme", `{"dark":false}`); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetGlobalSetting(ctx, "theme")
	if err != nil || got.ValueJSON != `{"dark":false}` {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	list, err := s.ListGlobalSettings(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %+v err=%v", list, err)
	}
	if err := s.PutProjectSetting(ctx, "p1", "k", `{"a":1}`); err != nil {
		t.Fatal(err)
	}
	if err := s.PutProjectSetting(ctx, "p1", "k", `{"a":2}`); err != nil {
		t.Fatal(err)
	}
	pl, err := s.ListProjectSettings(ctx, "p1")
	if err != nil || len(pl) != 1 || pl[0].ValueJSON != `{"a":2}` {
		t.Fatalf("project settings: %+v err=%v", pl, err)
	}
}

func TestLogsMetricsAndRetention(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	if _, err := nilStore.QueryLogs(ctx, LogQuery{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.LogLevelStats(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetRetention(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.PutRetention(ctx, "p", 7))
	if _, err := nilStore.MetricsSummary(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.MetricsTrend(ctx, "p", 7); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}

	s := newTestStore(t)
	if err := s.FlushLogs(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.FlushMetrics(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	s.RecordLog(LogEvent{ProjectID: "p1", Message: "hello world", RequestID: "r1"})
	s.RecordLog(LogEvent{ID: uuid.NewString(), ProjectID: "p1", Level: "error", Logger: "api", Message: "boom", OccurredAt: now})
	s.RecordLog(LogEvent{Level: "warn", Logger: "sys", Message: "global"})
	if err := s.FlushLogs(ctx); err != nil {
		t.Fatal(err)
	}

	all, err := s.QueryLogs(ctx, LogQuery{ProjectID: catalog.ReservedSystemProjectID, Limit: 0})
	if err != nil || len(all) != 3 {
		t.Fatalf("admin logs n=%d err=%v", len(all), err)
	}
	errs, err := s.QueryLogs(ctx, LogQuery{ProjectID: "p1", Level: "error", Limit: 10})
	if err != nil || len(errs) != 1 || errs[0].Level != "error" {
		t.Fatalf("level filter: %+v err=%v", errs, err)
	}
	q, err := s.QueryLogs(ctx, LogQuery{ProjectID: "p1", Q: "hello", From: now.Add(-time.Hour), To: now.Add(time.Hour), Limit: 600})
	if err != nil || len(q) != 1 {
		t.Fatalf("text+time: %+v err=%v", q, err)
	}
	stats, err := s.LogLevelStats(ctx, "p1")
	if err != nil || len(stats) == 0 {
		t.Fatalf("stats: %+v err=%v", stats, err)
	}

	ret, err := s.GetRetention(ctx, "p1")
	if err != nil || ret.KeepDays != 14 || ret.Scope != "global" {
		t.Fatalf("default retention: %+v err=%v", ret, err)
	}
	if err := s.PutRetention(ctx, "p1", 0); err != nil {
		t.Fatal(err)
	}
	ret, err = s.GetRetention(ctx, "p1")
	if err != nil || ret.KeepDays != 14 || ret.Scope != "project" {
		t.Fatalf("put default days: %+v err=%v", ret, err)
	}
	if err := s.PutRetention(ctx, "p1", 30); err != nil {
		t.Fatal(err)
	}
	ret, err = s.GetRetention(ctx, "p1")
	if err != nil || ret.KeepDays != 30 {
		t.Fatalf("updated retention: %+v", ret)
	}

	s.RecordMetric(MetricSample{ProjectID: "p1", Name: "http_requests", Value: 10})
	s.RecordMetric(MetricSample{ID: uuid.NewString(), ProjectID: "p1", Name: "http_errors", Value: 2, OccurredAt: now})
	s.RecordMetric(MetricSample{ProjectID: "p1", Name: "http_latency_ms", Value: 15})
	s.RecordMetric(MetricSample{ProjectID: "p2", Name: "http_requests", Value: 3})
	if err := s.FlushMetrics(ctx); err != nil {
		t.Fatal(err)
	}
	sum, err := s.MetricsSummary(ctx, "p1")
	if err != nil || sum.TotalRequests != 10 || sum.ErrorRate != 0.2 || sum.AvgLatencyMS == 0 {
		t.Fatalf("project summary: %+v err=%v", sum, err)
	}
	adminSum, err := s.MetricsSummary(ctx, catalog.ReservedSystemProjectID)
	if err != nil || adminSum.TotalRequests < 13 {
		t.Fatalf("admin summary: %+v err=%v", adminSum, err)
	}
	trend, err := s.MetricsTrend(ctx, "p1", 0)
	if err != nil {
		t.Fatalf("trend: %v", err)
	}
	_ = trend
	adminTrend, err := s.MetricsTrend(ctx, catalog.ReservedSystemProjectID, 91)
	if err != nil {
		t.Fatalf("admin trend: %v", err)
	}
	_ = adminTrend

	// auto-flush path (>=64)
	for i := 0; i < 64; i++ {
		s.RecordLog(LogEvent{ProjectID: "p1", Message: "bulk"})
	}
	for i := 0; i < 64; i++ {
		s.RecordMetric(MetricSample{ProjectID: "p1", Name: "http_requests", Value: 1})
	}
}

func TestLLMSessionsAndSettings(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	if _, err := nilStore.CreateLLMSession(ctx, LLMSession{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.AppendLLMMessage(ctx, LLMMessage{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetLLMSettings(ctx, "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	assertUnavailable(t, nilStore.PutLLMSettings(ctx, LLMSettings{}))

	s := newTestStore(t)
	sess, err := s.CreateLLMSession(ctx, LLMSession{ProjectID: "p1"})
	if err != nil || sess.Title != "New chat" || sess.ID == "" {
		t.Fatalf("create: %+v err=%v", sess, err)
	}
	fixedID := uuid.NewString()
	if _, err := s.CreateLLMSession(ctx, LLMSession{ID: fixedID, ProjectID: "p1", Title: "named"}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_llm_sessions WHERE project_id = ?`, "p1").Scan(&n); err != nil || n != 2 {
		t.Fatalf("sessions: n=%d err=%v", n, err)
	}

	long := "abcdefghijklmnopqrstuvwxyz0123456789XXXXX" // >40
	if _, err := s.AppendLLMMessage(ctx, LLMMessage{SessionID: sess.ID, ProjectID: "p1", Role: "user", Content: long}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendLLMMessage(ctx, LLMMessage{ID: uuid.NewString(), SessionID: sess.ID, ProjectID: "p1", Role: "assistant", Content: "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_llm_messages WHERE session_id = ?`, sess.ID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("msgs: n=%d err=%v", n, err)
	}
	var title string
	if err := s.db.QueryRowContext(ctx, `SELECT title FROM sys_llm_sessions WHERE id = ?`, sess.ID).Scan(&title); err != nil || title == "New chat" || len(title) > 40 {
		t.Fatalf("title should truncate: %q err=%v", title, err)
	}

	st, err := s.GetLLMSettings(ctx, "p1")
	if err != nil || st.Temperature != 0.7 || st.MaxTokens != 1024 {
		t.Fatalf("default settings: %+v err=%v", st, err)
	}
	if err := s.PutLLMSettings(ctx, LLMSettings{ProjectID: "p1", DefaultProvider: "openai", DefaultModel: "m", Temperature: 0.1, MaxTokens: 8}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutLLMSettings(ctx, LLMSettings{ProjectID: "p1", DefaultProvider: "openai", DefaultModel: "m2", Temperature: 0.2, MaxTokens: 16}); err != nil {
		t.Fatal(err)
	}
	st, err = s.GetLLMSettings(ctx, "p1")
	if err != nil || st.DefaultModel != "m2" || st.MaxTokens != 16 {
		t.Fatalf("updated settings: %+v err=%v", st, err)
	}
}

func TestS3IndexRefreshAndCRUD(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	assertUnavailable(t, nilStore.UpsertS3Object(ctx, "p", "k", 1, "", "", time.Time{}))
	assertUnavailable(t, nilStore.SoftDeleteS3Object(ctx, "p", "k"))
	if _, err := nilStore.ListS3Objects(ctx, "p", "", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := nilStore.GetS3Object(ctx, "p", "k"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, _, err := nilStore.RefreshS3Index(ctx, "p", nil); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}

	s := newTestStore(t)
	pid := "proj-s3"
	if err := s.UpsertS3Object(ctx, pid, "keep.txt", 1, "e1", "text/plain", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertS3Object(ctx, pid, "keep.txt", 2, "e2", "text/plain", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertS3Object(ctx, pid, "gone.txt", 3, "", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertS3Object(ctx, pid, "pref/a.txt", 4, "", "", time.Now()); err != nil {
		t.Fatal(err)
	}

	pref, err := s.ListS3Objects(ctx, pid, "pref/", 0)
	if err != nil || len(pref) != 1 || pref[0].Key != "pref/a.txt" {
		t.Fatalf("prefix list: %+v err=%v", pref, err)
	}
	got, err := s.GetS3Object(ctx, pid, "keep.txt")
	if err != nil || got.Size != 2 || got.ETag != "e2" {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if _, err := s.GetS3Object(ctx, pid, "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing: %v", err)
	}
	if err := s.SoftDeleteS3Object(ctx, pid, "pref/a.txt"); err != nil {
		t.Fatal(err)
	}

	listed := []objectstore.FileObject{
		{Key: pid + "/keep.txt", Size: 9, LastModified: time.Now()},
		{Key: "new.txt", Size: 5, LastModified: time.Now()},
	}
	ins, rem, err := s.RefreshS3Index(ctx, pid, listed)
	if err != nil {
		t.Fatal(err)
	}
	if ins != 1 || rem != 1 {
		t.Fatalf("refresh ins=%d rem=%d want 1/1", ins, rem)
	}
	if _, err := s.GetS3Object(ctx, pid, "gone.txt"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("gone should be removed")
	}
	keep, err := s.GetS3Object(ctx, pid, "keep.txt")
	if err != nil || keep.Size != 9 {
		t.Fatalf("updated keep: %+v err=%v", keep, err)
	}
	if _, err := s.GetS3Object(ctx, pid, "new.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestSeedGlobalRetentionAndFlushTicker(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	s.SetDefaultLogKeepDays(21)
	if err := s.SeedGlobalRetention(ctx, 21); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedGlobalRetention(ctx, 99); err != nil {
		t.Fatal(err)
	}
	ret, err := s.GetRetention(ctx, "p1")
	if err != nil || ret.KeepDays != 21 || ret.Scope != "global" {
		t.Fatalf("seeded retention: %+v err=%v", ret, err)
	}

	s.DB().SetMaxOpenConns(1)
	s.StartPeriodicFlush(20*time.Millisecond, 20*time.Millisecond)
	s.RecordLog(LogEvent{ProjectID: "p1", Message: "ticker"})
	s.RecordMetric(MetricSample{ProjectID: "p1", Name: "n", Value: 1})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		logs, err := s.QueryLogs(ctx, LogQuery{ProjectID: "p1", Q: "ticker", Limit: 10})
		if err == nil && len(logs) == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for log flush ticker")
}
