package ducklake

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

func TestFactoryOpenBindsSyncerAndLogger(t *testing.T) {
	dir := t.TempDir()
	id := uuid.NewString()
	opts := DefaultOptions()
	opts.ExtensionDir = filepath.Join(dir, "extensions")
	cs := NewCatalogSyncer(objectstore.NewMemoryBlobStore(), RemoteStorage{}, dir, opts.CatalogSync, nil, nil)
	f := &Factory{
		CacheDir: dir,
		Options:  opts,
		Syncer:   cs,
		Logger:   observability.NewLogger("debug", "json", io.Discard),
	}
	meta := catalog.Database{ID: id, TenantID: "11111111-1111-1111-1111-111111111111"}
	db, err := f.Open(context.Background(), meta, database.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cs.mu.Lock()
	_, bound := cs.bound[id]
	cs.mu.Unlock()
	if !bound {
		t.Fatal("expected bind")
	}
}

func TestSyncOnceNilStoreAndPruneBadTenant(t *testing.T) {
	db, _ := openTestLake(t)
	id := uuid.NewString()
	meta := catalog.Database{ID: id, TenantID: "11111111-1111-1111-1111-111111111111"}
	cs := NewCatalogSyncer(nil, RemoteStorage{Enabled: true, Bucket: "b", Region: "r"}, t.TempDir(), CatalogSyncOptions{}, nil, nil)
	cs.Bind(id, db, meta, DefaultLakeAlias)
	if err := cs.Sync(context.Background(), id); err == nil {
		t.Fatal("nil store")
	}

	cs2 := NewCatalogSyncer(objectstore.NewMemoryBlobStore(), RemoteStorage{Enabled: true, Bucket: "b", Region: "r"}, t.TempDir(), CatalogSyncOptions{KeepVersions: 1}, nil, nil)
	if err := cs2.pruneVersions(context.Background(), catalog.Database{ID: "bad", TenantID: "bad"}, 20); err == nil {
		t.Fatal("bad ids")
	}
}

func TestEnsureLocalCatalogDownloadError(t *testing.T) {
	remote := RemoteStorage{Enabled: true, Region: "r", Bucket: "b", RootPrefix: "simplebase", Environment: "e"}
	meta := catalog.Database{ID: uuid.NewString(), TenantID: "11111111-1111-1111-1111-111111111111"}
	key, _ := remote.keyBuilder().DuckLakeCatalogKey(meta.TenantID, meta.ID)
	store := &headOKDownloadFail{key: key}
	_, err := EnsureLocalCatalog(context.Background(), store, remote, t.TempDir(), meta)
	if err == nil {
		t.Fatal("download")
	}
}

type headOKDownloadFail struct{ key string }

func (h headOKDownloadFail) Head(ctx context.Context, key string) (objectstore.ObjectInfo, error) {
	return objectstore.ObjectInfo{Key: key, Size: 1}, nil
}
func (h headOKDownloadFail) PutBytes(ctx context.Context, key string, data []byte, ct string) error {
	return nil
}
func (h headOKDownloadFail) GetBytes(ctx context.Context, key string) ([]byte, objectstore.ObjectInfo, error) {
	return nil, objectstore.ObjectInfo{}, errStr("no")
}
func (h headOKDownloadFail) DownloadFile(ctx context.Context, key, dest string) (objectstore.ObjectInfo, error) {
	return objectstore.ObjectInfo{}, errStr("download fail")
}
func (h headOKDownloadFail) Delete(ctx context.Context, key string) error { return nil }
