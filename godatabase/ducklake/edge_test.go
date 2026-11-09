package ducklake

import (
	"context"
	"encoding/json"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/godatabase"
)

func TestFactoryOpenBindsSyncerAndLogger(t *testing.T) {
	dir := t.TempDir()
	id := uuid.NewString()
	opts := DefaultOptions()
	opts.ExtensionDir = filepath.Join(dir, "extensions")
	cs := NewCatalogSyncer(blob.NewMemory(), RemoteStorage{}, dir, opts.CatalogSync, "", nil, nil)
	f := &Factory{
		CacheDir: dir,
		Options:  opts,
		Syncer:   cs,
		Logger:   nil,
	}
	meta := godatabase.Spec{ID: id, TenantID: "11111111-1111-1111-1111-111111111111", ProjectID: "prj12345"}
	db, err := f.Open(context.Background(), meta, godatabase.ReadWrite)
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
	meta := godatabase.Spec{ID: id, TenantID: "11111111-1111-1111-1111-111111111111", ProjectID: "prj12345"}
	cs := NewCatalogSyncer(nil, RemoteStorage{Enabled: true, Bucket: "b", Region: "r"}, t.TempDir(), CatalogSyncOptions{}, "", nil, nil)
	cs.Bind(id, db, meta, DefaultLakeAlias)
	if err := cs.Sync(context.Background(), id); err == nil {
		t.Fatal("nil store")
	}

	cs2 := NewCatalogSyncer(blob.NewMemory(), RemoteStorage{Enabled: true, Bucket: "b", Region: "r"}, t.TempDir(), CatalogSyncOptions{KeepVersions: 1}, "", nil, nil)
	if err := cs2.pruneVersions(context.Background(), godatabase.Spec{ID: "bad", TenantID: "bad", ProjectID: "prj12345"}, 20, 20); err == nil {
		t.Fatal("bad ids")
	}
}

func TestEnsureLocalCatalogDownloadError(t *testing.T) {
	remote := RemoteStorage{Enabled: true, Region: "r", Bucket: "b", RootPrefix: "simplebase"}
	meta := godatabase.Spec{ID: uuid.NewString(), TenantID: "11111111-1111-1111-1111-111111111111", ProjectID: "prj12345"}
	key, _ := remote.keyBuilder().DuckLakeSnapshotKey(meta.ProjectID, meta.ID, 3, 1, EngineDuckDB)
	body, _ := json.Marshal(NewManifest(1, 3, key, EngineDuckDB, 1, 0, ""))
	store := &headOKDownloadFail{key: key, manifest: body}
	_, err := EnsureLocalCatalog(context.Background(), store, remote, t.TempDir(), meta, "")
	if err == nil {
		t.Fatal("download")
	}
}

type headOKDownloadFail struct {
	key      string
	manifest []byte // manifest seq 1 的内容；其余 GetBytes 返回 NotFound
}

func (h headOKDownloadFail) Head(ctx context.Context, key string) (blob.ObjectInfo, error) {
	return blob.ObjectInfo{Key: key, Size: 1}, nil
}
func (h headOKDownloadFail) PutBytes(ctx context.Context, key string, data []byte, ct string) error {
	return nil
}
func (h headOKDownloadFail) GetBytes(ctx context.Context, key string) ([]byte, blob.ObjectInfo, error) {
	if h.manifest != nil && strings.HasSuffix(key, "/manifest/00000000000000000001.json") {
		return h.manifest, blob.ObjectInfo{Key: key}, nil
	}
	return nil, blob.ObjectInfo{}, blob.ErrNotFound
}
func (h headOKDownloadFail) DownloadFile(ctx context.Context, key, dest string) (blob.ObjectInfo, error) {
	return blob.ObjectInfo{}, errStr("download fail")
}
func (h headOKDownloadFail) Delete(ctx context.Context, key string) error { return nil }
func (h headOKDownloadFail) List(ctx context.Context, prefix, cursor string, limit int) ([]string, string, error) {
	return nil, "", nil
}
func (h headOKDownloadFail) DeleteMany(ctx context.Context, keys []string) error { return nil }
func (h headOKDownloadFail) PutIfAbsent(ctx context.Context, key string, data []byte, ct string) (blob.ObjectInfo, error) {
	return blob.ObjectInfo{}, errStr("unexpected PutIfAbsent")
}
