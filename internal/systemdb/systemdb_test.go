package systemdb

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

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
