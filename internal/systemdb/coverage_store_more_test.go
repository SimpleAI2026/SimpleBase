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
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	_ "github.com/uglyer/go-sqlite3"
)

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
	if err != nil || n != 3 {
		t.Fatalf("seeded agents n=%d err=%v", n, err)
	}
	dbs, _, err := s.CatalogRepo().ListDatabases(ctx, catalog.DevProjectID, catalog.Page{Limit: 10})
	if err != nil || len(dbs) != 1 || dbs[0].Name != "default" {
		t.Fatalf("seeded default db: %+v err=%v", dbs, err)
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
