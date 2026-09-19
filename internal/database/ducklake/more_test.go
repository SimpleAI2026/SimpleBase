package ducklake

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

func TestBeforeCloseBindingAndAfterWriteError(t *testing.T) {
	db, _ := openTestLake(t)
	cs := NewCatalogSyncer(objectstore.NewMemoryBlobStore(), RemoteStorage{}, t.TempDir(), CatalogSyncOptions{Mode: "debounce"}, nil, nil)
	id := uuid.NewString()
	meta := catalog.Database{ID: id, TenantID: "11111111-1111-1111-1111-111111111111"}
	cs.Bind(id, db, meta, DefaultLakeAlias)
	f := &Factory{Syncer: cs, Options: DefaultOptions()}
	if err := f.BeforeClose(context.Background(), id, db); err != nil {
		t.Fatal(err)
	}
	closed, err := openTestLake(t)
	_ = closed.Close()
	if err := f.AfterWrite(context.Background(), meta, closed); err == nil {
		t.Fatal("after write on closed")
	}
	_ = err
}

func TestInspectClosedDBAndListFiles(t *testing.T) {
	db, _ := openTestLake(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE files (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := ListTableFiles(ctx, db, DefaultLakeAlias, "files"); err != nil {
		t.Log(err)
	}
	_ = db.Close()
	if _, err := CurrentSnapshot(ctx, db, DefaultLakeAlias); err == nil {
		t.Fatal("current")
	}
	if _, err := ListSnapshots(ctx, db, DefaultLakeAlias); err == nil {
		t.Fatal("snaps")
	}
	if _, err := ListSettings(ctx, db, DefaultLakeAlias); err == nil {
		t.Fatal("settings")
	}
	if _, err := ListTableFiles(ctx, db, DefaultLakeAlias, "files"); err == nil {
		t.Fatal("files")
	}
	if _, err := AssertDuckDBVersion(ctx, db); err == nil {
		t.Fatal("version")
	}
	if err := AssertExtensionsLoaded(ctx, db); err == nil {
		t.Fatal("ext")
	}
	if err := validateRemoteDataPath(ctx, db, DefaultLakeAlias, "x"); err == nil {
		t.Fatal("validate")
	}
}

func TestOpenCacheDirIsFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notdir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &Factory{CacheDir: file, Options: DefaultOptions()}
	_, err := f.Open(context.Background(), catalog.Database{ID: uuid.NewString()}, database.ReadWrite)
	if err == nil {
		t.Fatal("expected mkdir fail")
	}
}

func TestCatalogSyncerCloseFlushesBound(t *testing.T) {
	db, _ := openTestLake(t)
	id := uuid.NewString()
	meta := catalog.Database{ID: id, TenantID: "11111111-1111-1111-1111-111111111111"}
	cs := NewCatalogSyncer(objectstore.NewMemoryBlobStore(), RemoteStorage{}, t.TempDir(), CatalogSyncOptions{}, nil, nil)
	cs.Bind(id, db, meta, DefaultLakeAlias)
	if _, err := db.ExecContext(context.Background(), `CREATE TABLE c (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	snap, err := CurrentSnapshot(context.Background(), db, DefaultLakeAlias)
	if err != nil {
		t.Fatal(err)
	}
	cs.MarkDirty(id, snap)
	_ = cs.Close(context.Background())
}

func TestEnsureLocalCatalogHeadError(t *testing.T) {
	store := &errHeadStore{err: errorsNew("head fail")}
	remote := RemoteStorage{Enabled: true, Region: "r", Bucket: "b", RootPrefix: "simplebase", Environment: "e"}
	meta := catalog.Database{ID: uuid.NewString(), TenantID: "11111111-1111-1111-1111-111111111111"}
	_, err := EnsureLocalCatalog(context.Background(), store, remote, t.TempDir(), meta)
	if err == nil {
		t.Fatal("head error")
	}
}

type errHeadStore struct{ err error }

func (e errHeadStore) Head(ctx context.Context, key string) (objectstore.ObjectInfo, error) {
	return objectstore.ObjectInfo{}, e.err
}
func (e errHeadStore) PutBytes(ctx context.Context, key string, data []byte, ct string) error {
	return e.err
}
func (e errHeadStore) GetBytes(ctx context.Context, key string) ([]byte, objectstore.ObjectInfo, error) {
	return nil, objectstore.ObjectInfo{}, e.err
}
func (e errHeadStore) DownloadFile(ctx context.Context, key, dest string) (objectstore.ObjectInfo, error) {
	return objectstore.ObjectInfo{}, e.err
}
func (e errHeadStore) Delete(ctx context.Context, key string) error { return e.err }

func errorsNew(s string) error { return errStr(s) }

type errStr string

func (e errStr) Error() string { return string(e) }

func TestFactoryDurabilityNilCatalogSyncer(t *testing.T) {
	f := &Factory{Remote: RemoteStorage{Enabled: true}, Syncer: (*CatalogSyncer)(nil)}
	if f.DurabilityFor("x") != DurabilityCommittedLocal {
		t.Fatal(f.DurabilityFor("x"))
	}
}

func TestSummarizeSQLShort(t *testing.T) {
	if summarizeSQL("  SELECT 1  ") != "SELECT 1" {
		t.Fatal(summarizeSQL("  SELECT 1  "))
	}
	if !strings.Contains(summarizeSQL("CREATE SECRET foo KEY_ID x"), "SECRET") {
		t.Fatal("secret")
	}
}
