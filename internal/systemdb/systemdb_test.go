package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/godatabase/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
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
	if err := EnsureSystemSchema(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStoreForTest(db)
}

func TestEnsureSystemSchemaIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := EnsureSystemSchema(ctx, db); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := EnsureSystemSchema(ctx, db); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	var n int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name LIKE 'sys\_%' ESCAPE '\'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if int(n) != SystemTableCount() {
		t.Fatalf("tables=%d want %d", n, SystemTableCount())
	}
	var legacy int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'sys_migration_versions'`).Scan(&legacy); err != nil || legacy != 0 {
		t.Fatalf("version table must not exist: n=%d err=%v", legacy, err)
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
	if err := EnsureSystemSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := catalog.NewSQLRepository(db)
	tenantID := catalog.ReservedTenantID
	projectID := "testprj1"
	if err := repo.CreateTenant(ctx, catalog.Tenant{ID: tenantID, Name: "t", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateProject(ctx, catalog.Project{ID: projectID, TenantID: tenantID, Name: "商城后台", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	authSvc := auth.NewService(auth.NewSQLAPIKeyRepository(db), "secret")
	const rawKey = "sb_test_key_12345"
	if err := auth.CreateAPIKey(ctx, db, "00000000-0000-0000-0000-000000000003", projectID, authSvc.HashKey(rawKey), []auth.Permission{auth.ProjectAdmin}, time.Now()); err != nil {
		t.Fatal(err)
	}
	rec, err := authSvc.Authenticate(ctx, rawKey)
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
		Keys:       objectstore.KeyBuilder{RootPrefix: "simplebase"},
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

// pauseProbe 记录周期同步的暂停/恢复，嵌入 LocalSyncer 以免 MarkDirty 空指针。
type pauseProbe struct {
	*ducklake.LocalSyncer
	mu      sync.Mutex
	paused  []string
	resumed []string
}

func (p *pauseProbe) PausePeriodic(id string) {
	p.mu.Lock()
	p.paused = append(p.paused, id)
	p.mu.Unlock()
}

func (p *pauseProbe) ResumePeriodic(id string) {
	p.mu.Lock()
	p.resumed = append(p.resumed, id)
	p.mu.Unlock()
}

func TestBootstrapPausesSyncUntilSeed(t *testing.T) {
	dir := t.TempDir()
	probe := &pauseProbe{LocalSyncer: ducklake.NewLocalSyncer()}
	f := &ducklake.Factory{
		CacheDir: filepath.Join(dir, "system", "dbs"),
		Options:  ducklake.DefaultOptions(),
		Syncer:   probe,
	}
	store, err := Bootstrap(context.Background(), BootstrapInput{
		LocatorDir: filepath.Join(dir, "system"),
		Name:       DefaultName,
		Factory:    f,
		Keys:       objectstore.KeyBuilder{RootPrefix: "simplebase"},
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	probe.mu.Lock()
	paused := append([]string(nil), probe.paused...)
	resumed := append([]string(nil), probe.resumed...)
	probe.mu.Unlock()
	if len(paused) != 1 || paused[0] != store.Meta().ID {
		t.Fatalf("pause = %v, want [%s]", paused, store.Meta().ID)
	}
	if len(resumed) != 0 {
		t.Fatalf("resumed before seed: %v", resumed)
	}
	if err := Seed(context.Background(), SeedInput{Store: store}); err != nil {
		t.Fatal(err)
	}
	probe.mu.Lock()
	resumed = append([]string(nil), probe.resumed...)
	probe.mu.Unlock()
	if len(resumed) != 1 || resumed[0] != store.Meta().ID {
		t.Fatalf("resume = %v, want [%s]", resumed, store.Meta().ID)
	}
	// seed 保持幂等，第二次不再把周期同步提前到写入过程中。
	if err := Seed(context.Background(), SeedInput{Store: store}); err != nil {
		t.Fatal(err)
	}
}

func TestIsAdminProjectAndStoreLifecycle(t *testing.T) {
	if !IsAdminProject(catalog.ReservedSystemProjectID) {
		t.Fatal("admin project")
	}
	if IsAdminProject("testprj1") || IsAdminProject("") {
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

	if err := EnsureSystemSchema(context.Background(), raw); err != nil {
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

func TestEnsureSystemSchemaNilAndCount(t *testing.T) {
	if err := EnsureSystemSchema(context.Background(), nil); err == nil || !errors.Is(err, catalog.ErrMigrationFailed) {
		t.Fatalf("nil db: %v", err)
	}
	if SystemTableCount() < 20 {
		t.Fatalf("system table count %d", SystemTableCount())
	}
}

// 每张表只能定义一次，且都以 sys_ 开头、建表语句与 name 一致。
func TestSystemTablesAreUniqueAndWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, tb := range systemTables {
		if !strings.HasPrefix(tb.name, "sys_") || seen[tb.name] {
			t.Fatalf("bad or duplicate table %q", tb.name)
		}
		seen[tb.name] = true
		if !strings.Contains(tb.ddl, "CREATE TABLE IF NOT EXISTS "+tb.name+" (") {
			t.Fatalf("ddl of %s does not match its name", tb.name)
		}
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
	keys := objectstore.KeyBuilder{RootPrefix: "simplebase"}
	cat := catalog.NewService(s.CatalogRepo(), keys, nil, objectstore.DuckLakeStorage{}, nil)
	// Seed 不再创建演示项目或明文 Key：重复执行只保证保留租户与系统项目存在。
	if err := Seed(ctx, SeedInput{Store: s, Auth: authSvc, Catalog: cat}); err != nil {
		t.Fatal(err)
	}
	if err := Seed(ctx, SeedInput{Store: s, Auth: authSvc, Catalog: cat}); err != nil {
		t.Fatal(err)
	}
	projects, err := s.CatalogRepo().ListProjectsByTenant(ctx, catalog.ReservedTenantID)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].ID != catalog.ReservedSystemProjectID {
		t.Fatalf("seed must create exactly the system project, got %+v", projects)
	}
	// 项目级 KV catalog 与内置助手由项目创建流程负责（不再由 Seed 代建）。
	// 先按正常流程建项目，再验证 KV 与助手的幂等种子。
	const kvPrj = "kvprj001"
	if err := s.CatalogRepo().CreateProject(ctx, catalog.Project{
		ID: kvPrj, TenantID: catalog.ReservedTenantID, Name: "kv-test", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	kvDB, err := cat.CreateKVDatabase(ctx, catalog.ReservedTenantID, kvPrj)
	if err != nil {
		t.Fatalf("create kv db: %v", err)
	}
	if kvDB.Kind != catalog.DatabaseKindKV || kvDB.Name != catalog.KVDatabaseName {
		t.Fatalf("kv db: %+v", kvDB)
	}
	if err := s.SeedDefaultCloudAgents(ctx, kvPrj); err != nil {
		t.Fatalf("seed agents: %v", err)
	}
	if err := s.SeedDefaultCloudAgents(ctx, kvPrj); err != nil {
		t.Fatalf("seed agents (idempotent): %v", err)
	}
	n, err := s.CountCloudAgents(ctx, kvPrj)
	if err != nil || n != 4 {
		t.Fatalf("seeded agents n=%d err=%v", n, err)
	}
	dbs, _, err := s.CatalogRepo().ListDatabasesByKind(ctx, kvPrj, catalog.DatabaseKindKV, catalog.Page{Limit: 10})
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
	exported, err := s.ExportLogs(ctx, LogQuery{ProjectID: "p1", From: now.Add(-time.Hour), To: now.Add(time.Hour)})
	if err != nil || len(exported) != 3 {
		t.Fatalf("export logs n=%d err=%v", len(exported), err)
	}
	globalFound := false
	for _, event := range exported {
		globalFound = globalFound || event.ProjectID == ""
	}
	if !globalFound {
		t.Fatal("export omitted global log")
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
	if err != nil || st.Temperature != 0.7 || st.MaxTokens != 1024 || st.APIKey != "" || st.BaseURL != "" {
		t.Fatalf("default settings: %+v err=%v", st, err)
	}
	if err := s.PutLLMSettings(ctx, LLMSettings{ProjectID: "p1", DefaultProvider: "openai", DefaultModel: "m", APIKey: "sk-live-1234567890", BaseURL: "http://127.0.0.1:9/v1", Temperature: 0.1, MaxTokens: 8}); err != nil {
		t.Fatal(err)
	}
	// 连接信息落库；mask 只留首尾。
	st, err = s.GetLLMSettings(ctx, "p1")
	if err != nil || st.APIKey != "sk-live-1234567890" || st.BaseURL != "http://127.0.0.1:9/v1" {
		t.Fatalf("connection fields: %+v err=%v", st, err)
	}
	if MaskSecretValue(st.APIKey) != "sk-...7890" {
		t.Fatalf("mask: %q", MaskSecretValue(st.APIKey))
	}
	// api_key 留空表示沿用既有值（前端只拿到脱敏串，无法回传原文）。
	if err := s.PutLLMSettings(ctx, LLMSettings{ProjectID: "p1", DefaultProvider: "openai", DefaultModel: "m2", BaseURL: "http://127.0.0.1:8/v1", Temperature: 0.2, MaxTokens: 16}); err != nil {
		t.Fatal(err)
	}
	st, err = s.GetLLMSettings(ctx, "p1")
	if err != nil || st.DefaultModel != "m2" || st.MaxTokens != 16 || st.APIKey != "sk-live-1234567890" || st.BaseURL != "http://127.0.0.1:8/v1" {
		t.Fatalf("updated settings: %+v err=%v", st, err)
	}
}

func TestMaskSecretValueShort(t *testing.T) {
	if MaskSecretValue("") != "" {
		t.Fatal("empty should stay empty")
	}
	if MaskSecretValue("short") != "••••••••" {
		t.Fatalf("short value should be fully masked, got %s", MaskSecretValue("short"))
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
