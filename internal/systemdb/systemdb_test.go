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
	if err != nil || len(agents) != 3 {
		t.Fatalf("agents n=%d err=%v", len(agents), err)
	}
	if err := store.SeedDefaultCloudAgents(ctx, projectID); err != nil {
		t.Fatal(err)
	}
	agents, err = store.ListCloudAgents(ctx, projectID)
	if err != nil || len(agents) != 3 {
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
