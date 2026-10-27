package ducklake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/godatabase"
)

func TestCatalogSyncerDebounceAndUpload(t *testing.T) {
	dir := t.TempDir()
	id := uuid.NewString()
	tenant := "prj12345" // 存储路径按项目 ID 组织
	blobs := blob.NewMemory()
	remote := RemoteStorage{
		Enabled:    true,
		Region:     "us-east-1",
		Bucket:     "test-bucket",
		RootPrefix: "simplebase",
	}
	opts := DefaultOptions()
	opts.ExtensionDir = filepath.Join(dir, "extensions")
	opts.CatalogSync.Debounce = 50 * time.Millisecond
	opts.CatalogSync.Mode = "debounce"

	cs := NewCatalogSyncer(blobs, remote, dir, opts.CatalogSync, "", nil, nil)
	f := &Factory{
		CacheDir: dir,
		Options:  opts,
		Syncer:   cs,
		Remote:   RemoteStorage{}, // keep DATA_PATH local for unit test; syncer still uploads catalog
		Blobs:    blobs,
	}
	// Force remote keys to work while DATA_PATH stays local: syncer.Remote enabled, factory remote off.
	meta := godatabase.Spec{ID: id, TenantID: tenant, ProjectID: "prj12345"}
	db, err := f.Open(context.Background(), meta, godatabase.ReadWrite)
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
	if got := cs.LastSynced(id); got < snap {
		t.Fatalf("LastSynced=%d want at least %d", got, snap)
	}
	// 不可变快照对象必须存在（snapshots/{snap}-{epoch}.ducklake），manifest 带引擎与校验信息。
	latest, err := ReadLatestManifest(ctx, blobs, remote, tenant, id, 0)
	if err != nil || latest == nil {
		t.Fatalf("manifest not written: %v %v", latest, err)
	}
	if latest.CatalogEngine != EngineDuckDB || latest.SHA256 == "" || latest.Size <= 0 {
		t.Fatalf("manifest missing engine/checksum: %+v", latest)
	}
	if !strings.HasSuffix(latest.SnapshotKey, ".ducklake") {
		t.Fatalf("snapshot key suffix: %s", latest.SnapshotKey)
	}
	if _, err := blobs.Head(ctx, latest.SnapshotKey); err != nil {
		t.Fatalf("snapshot object not uploaded: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := cs.Flush(ctx, id); err != nil {
			t.Fatalf("idle flush: %v", err)
		}
	}
	idle, err := ReadLatestManifest(ctx, blobs, remote, tenant, id, 0)
	if err != nil || idle == nil || idle.Seq != latest.Seq {
		t.Fatalf("idle flush advanced manifest: %v %v", idle, err)
	}
	// 兼容镜像 catalog/catalog.sqlite 已删除：远端 catalog/ 下不应再出现 .sqlite 对象。
	keys, _, err := blobs.List(ctx, "simplebase/prj12345/", "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if strings.HasSuffix(k, ".sqlite") {
			t.Fatalf("legacy sqlite object written: %s", k)
		}
	}
}

type failCatalogPutStore struct {
	blob.Store
	failSnapshot bool
	failManifest bool
}

func (b *failCatalogPutStore) PutIfAbsent(ctx context.Context, key string, data []byte, ct string) (blob.ObjectInfo, error) {
	if (b.failSnapshot && strings.Contains(key, "/snapshots/")) ||
		(b.failManifest && strings.Contains(key, "/manifest/")) {
		return blob.ObjectInfo{}, context.DeadlineExceeded
	}
	return b.Store.PutIfAbsent(ctx, key, data, ct)
}

func TestCatalogSyncFailureKeepsUnsyncedAgeAndManifest(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	meta := godatabase.Spec{ID: uuid.NewString(), TenantID: "11111111-1111-1111-1111-111111111111", ProjectID: "prj12345"}
	remote := RemoteStorage{Enabled: true, Region: "us-east-1", Bucket: "test", RootPrefix: "simplebase"}
	for _, failManifest := range []bool{false, true} {
		t.Run(map[bool]string{false: "snapshot", true: "manifest"}[failManifest], func(t *testing.T) {
			store := &failCatalogPutStore{Store: blob.NewMemory(), failSnapshot: !failManifest, failManifest: failManifest}
			opts := DefaultOptions()
			opts.ExtensionDir = filepath.Join(dir, uuid.NewString(), "extensions")
			opts.CatalogSync.Interval = time.Hour
			cache := filepath.Join(dir, uuid.NewString())
			cs := NewCatalogSyncer(store, remote, cache, opts.CatalogSync, EngineDuckDB, nil, nil)
			f := &Factory{CacheDir: cache, Options: opts, Syncer: cs}
			db, err := f.Open(ctx, meta, godatabase.ReadWrite)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			cs.Bind(meta.ID, db, meta, DefaultLakeAlias)
			if _, err := db.ExecContext(ctx, `CREATE TABLE t (x INTEGER)`); err != nil {
				t.Fatal(err)
			}
			snap, err := CurrentSnapshot(ctx, db, DefaultLakeAlias)
			if err != nil {
				t.Fatal(err)
			}
			cs.MarkDirty(meta.ID, snap)
			if age := cs.OldestUnsyncedAge(meta.ID); age < 0 {
				t.Fatalf("unsynced age=%s", age)
			}
			if err := cs.Sync(ctx, meta.ID); err == nil {
				t.Fatal("expected injected upload failure")
			}
			if cs.LastSynced(meta.ID) >= snap || cs.SyncLag(meta.ID) == 0 {
				t.Fatal("failed sync advanced watermark")
			}
			latest, err := ReadLatestManifest(ctx, store, remote, meta.ProjectID, meta.ID, 0)
			if err != nil || latest != nil {
				t.Fatalf("failed sync published manifest: %+v %v", latest, err)
			}
			store.failSnapshot, store.failManifest = false, false
			if err := cs.Sync(ctx, meta.ID); err != nil {
				t.Fatalf("recover sync: %v", err)
			}
			if age := cs.OldestUnsyncedAge(meta.ID); age != 0 {
				t.Fatalf("synced age=%s", age)
			}
			latest, err = ReadLatestManifest(ctx, store, remote, meta.ProjectID, meta.ID, 0)
			if err != nil || latest == nil || latest.SnapshotID < snap {
				t.Fatalf("recovery manifest: %+v %v", latest, err)
			}
		})
	}
}

type delayedSnapshotStore struct {
	blob.Store
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *delayedSnapshotStore) PutIfAbsent(ctx context.Context, key string, data []byte, ct string) (blob.ObjectInfo, error) {
	if strings.Contains(key, "/snapshots/") {
		b.once.Do(func() {
			close(b.started)
			select {
			case <-b.release:
			case <-ctx.Done():
			}
		})
	}
	return b.Store.PutIfAbsent(ctx, key, data, ct)
}

func TestCatalogSyncerInflightWriteEventuallySynced(t *testing.T) {
	dir := t.TempDir()
	meta := godatabase.Spec{ID: uuid.NewString(), TenantID: "11111111-1111-1111-1111-111111111111", ProjectID: "prj12345"}
	remote := RemoteStorage{Enabled: true, Region: "us-east-1", Bucket: "b", RootPrefix: "simplebase"}
	store := &delayedSnapshotStore{Store: blob.NewMemory(), started: make(chan struct{}), release: make(chan struct{})}
	opts := DefaultOptions()
	opts.ExtensionDir = filepath.Join(dir, "extensions")
	opts.CatalogSync.Debounce = 20 * time.Millisecond
	cs := NewCatalogSyncer(store, remote, dir, opts.CatalogSync, "", nil, nil)
	f := &Factory{CacheDir: dir, Options: opts, Syncer: cs}
	db, err := f.Open(context.Background(), meta, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cs.Bind(meta.ID, db, meta, DefaultLakeAlias)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE t (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	first, err := CurrentSnapshot(ctx, db, DefaultLakeAlias)
	if err != nil {
		t.Fatal(err)
	}
	cs.MarkDirty(meta.ID, first)
	select {
	case <-store.started:
	case <-time.After(30 * time.Second):
		t.Fatal("first snapshot upload not started")
	}
	defer func() {
		select {
		case <-store.release:
		default:
			close(store.release)
		}
	}()
	if _, err := db.ExecContext(ctx, `INSERT INTO t VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	second, err := CurrentSnapshot(ctx, db, DefaultLakeAlias)
	if err != nil {
		t.Fatal(err)
	}
	if second <= first {
		t.Fatalf("snapshots %d -> %d", first, second)
	}
	cs.MarkDirty(meta.ID, second)
	time.Sleep(3 * opts.CatalogSync.Debounce)
	close(store.release)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		latest, err := ReadLatestManifest(ctx, store, remote, meta.ProjectID, meta.ID, 0)
		if err == nil && latest != nil && latest.SnapshotID >= second {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("write during inflight sync was never uploaded")
}

func TestEnsureLocalCatalogDownload(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	id := uuid.NewString()
	tenant := "prj12345" // 存储路径按项目 ID 组织
	blobs := blob.NewMemory()
	remote := RemoteStorage{Enabled: true, Region: "us-east-1", Bucket: "b", RootPrefix: "simplebase"}
	meta := godatabase.Spec{ID: id, TenantID: tenant, ProjectID: "prj12345"}

	// 旧 key catalog/catalog.sqlite 不再被读取：无 manifest 即新库。
	prefix, _ := remote.keyBuilder().DatabasePrefix(tenant, id)
	_ = blobs.PutBytes(ctx, prefix+"/catalog/catalog.sqlite", []byte("legacy"), "")
	if ok, err := EnsureLocalCatalog(ctx, blobs, remote, dir, meta, EngineDuckDB); err != nil || ok {
		t.Fatalf("legacy key must be ignored: ok=%v err=%v", ok, err)
	}

	body := []byte("catalog-body")
	sum := sha256.Sum256(body)
	key, _ := remote.keyBuilder().DuckLakeSnapshotKey(tenant, id, 5, 1, EngineDuckDB)
	_ = blobs.PutBytes(ctx, key, body, "")
	m := NewManifest(1, 5, key, EngineDuckDB, 1, int64(len(body)), hex.EncodeToString(sum[:]))
	if err := WriteManifest(ctx, blobs, remote, tenant, id, m); err != nil {
		t.Fatal(err)
	}
	layout := layoutForEngine(dir, id, EngineDuckDB)
	_ = os.MkdirAll(filepath.Dir(layout.CatalogFile), 0o755)
	_ = os.WriteFile(layout.WALFile(EngineDuckDB), []byte("stale"), 0o644)

	ok, err := EnsureLocalCatalog(ctx, blobs, remote, dir, meta, EngineDuckDB)
	if err != nil || !ok {
		t.Fatalf("downloaded=%v err=%v", ok, err)
	}
	b, err := os.ReadFile(layout.CatalogFile)
	if err != nil || string(b) != "catalog-body" {
		t.Fatalf("local catalog: %v %q", err, b)
	}
	if _, err := os.Stat(layout.WALFile(EngineDuckDB)); !os.IsNotExist(err) {
		t.Fatalf("stale wal must be removed before replace: %v", err)
	}

	// 配置为 sqlite 时拒绝下载 duckdb manifest。
	if _, err := EnsureLocalCatalog(ctx, blobs, remote, t.TempDir(), meta, EngineSQLite); !IsCatalogEngineMismatch(err) {
		t.Fatalf("want engine mismatch, got %v", err)
	}
}

func TestVerifySnapshotBodyRejectsMissingIntegrityFields(t *testing.T) {
	for _, manifest := range []Manifest{
		{Seq: 1, Size: 0, SHA256: strings.Repeat("a", 64)},
		{Seq: 1, Size: 1},
	} {
		if err := verifySnapshotBody([]byte("x"), &manifest); err == nil {
			t.Fatalf("missing integrity fields should be rejected: %+v", manifest)
		}
	}
}

func TestEnsureLocalCatalogChecksumMismatch(t *testing.T) {
	ctx := context.Background()
	id := uuid.NewString()
	tenant := "prj12345" // 存储路径按项目 ID 组织
	blobs := blob.NewMemory()
	remote := RemoteStorage{Enabled: true, Region: "us-east-1", Bucket: "b", RootPrefix: "simplebase"}
	key, _ := remote.keyBuilder().DuckLakeSnapshotKey(tenant, id, 5, 1, EngineDuckDB)
	_ = blobs.PutBytes(ctx, key, []byte("tampered"), "")
	m := NewManifest(1, 5, key, EngineDuckDB, 1, 8, strings.Repeat("0", 64))
	if err := WriteManifest(ctx, blobs, remote, tenant, id, m); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := EnsureLocalCatalog(ctx, blobs, remote, dir, godatabase.Spec{ID: id, TenantID: tenant, ProjectID: "prj12345"}, EngineDuckDB); err == nil {
		t.Fatal("checksum mismatch must fail")
	}
	if _, err := os.Stat(layoutForEngine(dir, id, EngineDuckDB).CatalogFile); !os.IsNotExist(err) {
		t.Fatal("catalog must not be replaced on checksum mismatch")
	}
}

func TestEnsureLocalCatalogLegacyManifestRejected(t *testing.T) {
	ctx := context.Background()
	id := uuid.NewString()
	tenant := "prj12345" // 存储路径按项目 ID 组织
	blobs := blob.NewMemory()
	remote := RemoteStorage{Enabled: true, Region: "us-east-1", Bucket: "b", RootPrefix: "simplebase"}
	mk, _ := remote.keyBuilder().DuckLakeManifestKey(tenant, id, 1)
	_ = blobs.PutBytes(ctx, mk, []byte(`{"seq":1,"snapshot_id":3,"snapshot_key":"x.sqlite","writer_epoch":1}`), "")
	if _, err := EnsureLocalCatalog(ctx, blobs, remote, t.TempDir(), godatabase.Spec{ID: id, TenantID: tenant, ProjectID: "prj12345"}, EngineDuckDB); !IsCatalogEngineUnknown(err) {
		t.Fatalf("legacy manifest without engine must be rejected, got %v", err)
	}
}
