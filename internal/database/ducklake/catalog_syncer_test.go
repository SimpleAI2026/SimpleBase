package ducklake

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

func TestCatalogSyncerDebounceAndUpload(t *testing.T) {
	dir := t.TempDir()
	id := uuid.NewString()
	tenant := "11111111-1111-1111-1111-111111111111"
	blobs := objectstore.NewMemoryBlobStore()
	remote := RemoteStorage{
		Enabled:     true,
		Region:      "us-east-1",
		Bucket:      "test-bucket",
		RootPrefix:  "simplebase",
		Environment: "test",
	}
	opts := DefaultOptions()
	opts.ExtensionDir = filepath.Join(dir, "extensions")
	opts.CatalogSync.Debounce = 50 * time.Millisecond
	opts.CatalogSync.Mode = "debounce"

	cs := NewCatalogSyncer(blobs, remote, dir, opts.CatalogSync, nil, nil)
	f := &Factory{
		CacheDir: dir,
		Options:  opts,
		Syncer:   cs,
		Remote:   RemoteStorage{}, // keep DATA_PATH local for unit test; syncer still uploads catalog
		Blobs:    blobs,
	}
	// Force remote keys to work while DATA_PATH stays local: syncer.Remote enabled, factory remote off.
	meta := catalog.Database{ID: id, TenantID: tenant, ProjectID: "pro-test"}
	db, err := f.Open(context.Background(), meta, database.ReadWrite)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cs.Bind(id, db, meta, DefaultLakeAlias)

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE t (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO t VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	snap, err := CurrentSnapshot(ctx, db, DefaultLakeAlias)
	if err != nil {
		t.Fatal(err)
	}
	cs.MarkDirty(id, snap)
	if err := cs.Flush(ctx, id); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := cs.LastSynced(id); got != snap {
		t.Fatalf("LastSynced=%d want %d", got, snap)
	}
	key, err := remote.keyBuilder().DuckLakeCatalogKey(tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Head(ctx, key); err != nil {
		t.Fatalf("catalog not uploaded: %v", err)
	}
	ver, err := remote.keyBuilder().DuckLakeCatalogVersionKey(tenant, id, snap)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Head(ctx, ver); err != nil {
		t.Fatalf("version not uploaded: %v", err)
	}
}

func TestEnsureLocalCatalogDownload(t *testing.T) {
	dir := t.TempDir()
	id := uuid.NewString()
	tenant := "11111111-1111-1111-1111-111111111111"
	blobs := objectstore.NewMemoryBlobStore()
	remote := RemoteStorage{Enabled: true, Region: "us-east-1", Bucket: "b", RootPrefix: "simplebase", Environment: "test"}
	meta := catalog.Database{ID: id, TenantID: tenant}
	key, _ := remote.keyBuilder().DuckLakeCatalogKey(tenant, id)
	if err := blobs.PutBytes(context.Background(), key, []byte("catalog-body"), ""); err != nil {
		t.Fatal(err)
	}
	ok, err := EnsureLocalCatalog(context.Background(), blobs, remote, dir, meta)
	if err != nil || !ok {
		t.Fatalf("downloaded=%v err=%v", ok, err)
	}
	layout := layoutFor(dir, id)
	b, err := os.ReadFile(layout.CatalogFile)
	if err != nil || string(b) != "catalog-body" {
		t.Fatalf("local catalog: %v %q", err, b)
	}
}

