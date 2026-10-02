package ducklake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

func testRemote() RemoteStorage {
	return RemoteStorage{Enabled: true, Region: "r", Bucket: "b", RootPrefix: "simplebase", Environment: "e"}
}

func testMeta() catalog.Database {
	return catalog.Database{ID: "33333333-3333-3333-3333-333333333333", TenantID: "11111111-1111-1111-1111-111111111111"}
}

func TestManifestWriteReadLatest(t *testing.T) {
	store := objectstore.NewMemoryBlobStore()
	remote := testRemote()
	meta := testMeta()
	ctx := context.Background()

	if m, err := ReadLatestManifest(ctx, store, remote, meta.TenantID, meta.ID, 0); err != nil || m != nil {
		t.Fatalf("empty lake: got %v, %v", m, err)
	}

	for seq := int64(1); seq <= 3; seq++ {
		key, err := remote.keyBuilder().DuckLakeSnapshotKey(meta.TenantID, meta.ID, seq*10, seq, "")
		if err != nil {
			t.Fatal(err)
		}
		m := NewManifest(seq, seq*10, key, "duckdb", 77, 0, "")
		if err := WriteManifest(ctx, store, remote, meta.TenantID, meta.ID, m); err != nil {
			t.Fatal(err)
		}
	}

	latest, err := ReadLatestManifest(ctx, store, remote, meta.TenantID, meta.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.Seq != 3 || latest.SnapshotID != 30 {
		t.Fatalf("latest = %+v", latest)
	}

	// 带缓存的探测：从 seq 2 起步，应仍收敛到 3。
	latest2, err := ReadLatestManifest(ctx, store, remote, meta.TenantID, meta.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if latest2 == nil || latest2.Seq != 3 {
		t.Fatalf("latest2 = %+v", latest2)
	}
}

func TestManifestRejectsKeyPayloadSeqMismatch(t *testing.T) {
	ctx := context.Background()
	store := objectstore.NewMemoryBlobStore()
	remote := testRemote()
	meta := testMeta()
	key, err := remote.keyBuilder().DuckLakeManifestKey(meta.TenantID, meta.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutBytes(ctx, key, []byte(`{"seq":3,"snapshot_id":10,"snapshot_key":"snapshot"}`), "application/json"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLatestManifest(ctx, store, remote, meta.TenantID, meta.ID, 0); !IsManifestChainBroken(err) {
		t.Fatalf("mismatched manifest should fail closed: %v", err)
	}
}

func TestManifestConflictIsSplitBrain(t *testing.T) {
	store := objectstore.NewMemoryBlobStore()
	remote := testRemote()
	meta := testMeta()
	ctx := context.Background()

	m := NewManifest(1, 100, "snap-key", "duckdb", 5, 0, "")
	if err := WriteManifest(ctx, store, remote, meta.TenantID, meta.ID, m); err != nil {
		t.Fatal(err)
	}
	// 第二个 writer 写同一 seq：必须报冲突（ErrPreconditionFailed）。
	err := WriteManifest(ctx, store, remote, meta.TenantID, meta.ID, m)
	if err == nil || !errors.Is(err, objectstore.ErrPreconditionFailed) {
		t.Fatalf("want ErrPreconditionFailed, got %v", err)
	}
}

func TestManifestSeqFromKey(t *testing.T) {
	if got := manifestSeqFromKey("simplebase/e/db/tenant/3333.../catalog/manifest/00000000000000000042.json"); got != 42 {
		t.Fatalf("got %d", got)
	}
	if got := manifestSeqFromKey("garbage"); got != 0 {
		t.Fatalf("got %d", got)
	}
}

func TestSnapshotKeyStructure(t *testing.T) {
	kb := objectstore.KeyBuilder{RootPrefix: "sb", Environment: "e"}
	k1, err := kb.DuckLakeSnapshotKey("11111111-1111-1111-1111-111111111111", "33333333-3333-3333-3333-333333333333", 7, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	k2, _ := kb.DuckLakeSnapshotKey("11111111-1111-1111-1111-111111111111", "33333333-3333-3333-3333-333333333333", 7, 200, "")
	if k1 == k2 {
		t.Fatal("different epochs must yield different keys")
	}
	if _, err := kb.DuckLakeSnapshotKey("11111111-1111-1111-1111-111111111111", "33333333-3333-3333-3333-333333333333", 0, 1, ""); err == nil {
		t.Fatal("snapshot 0 rejected")
	}
}

func TestLocalStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dbID := "33333333-3333-3333-3333-333333333333"

	if _, ok := ReadLocalState(dir, dbID); ok {
		t.Fatal("no state yet")
	}
	if err := SaveLocalState(dir, dbID, LocalState{SnapshotID: 42, SyncedSnapshotID: 42, SyncedSeq: 4}); err != nil {
		t.Fatal(err)
	}
	st, ok := ReadLocalState(dir, dbID)
	if !ok || st.SnapshotID != 42 || st.SyncedSeq != 4 {
		t.Fatalf("st = %+v, ok=%v", st, ok)
	}

	// 损坏文件：读取视为缺失（ok=false），写入可覆盖。
	layout := layoutFor(dir, dbID)
	if err := writeFileBytes(localStatePath(layout), []byte("{bad json")); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadLocalState(dir, dbID); ok {
		t.Fatal("corrupted state should read as missing")
	}
	if err := SaveLocalState(dir, dbID, LocalState{SnapshotID: 50}); err != nil {
		t.Fatal(err)
	}
	st2, ok := ReadLocalState(dir, dbID)
	if !ok || st2.SnapshotID != 50 {
		t.Fatalf("st2 = %+v", st2)
	}
}

func TestEnsureLocalCatalogLocalAheadNotOverwritten(t *testing.T) {
	remote := testRemote()
	meta := testMeta()
	dir := t.TempDir()
	store := objectstore.NewMemoryBlobStore()
	ctx := context.Background()

	// 远端 manifest：snapshot 10。
	key, _ := remote.keyBuilder().DuckLakeSnapshotKey(meta.TenantID, meta.ID, 10, 1, "")
	if err := WriteManifest(ctx, store, remote, meta.TenantID, meta.ID, NewManifest(1, 10, key, "duckdb", 1, 0, "")); err != nil {
		t.Fatal(err)
	}

	// 本地 catalog 存在且水位 20（领先远端）。
	layout := layoutFor(dir, meta.ID)
	if err := layout.ensure(); err != nil {
		t.Fatal(err)
	}
	if err := writeFileBytes(layout.CatalogFile, []byte("local-newer-catalog")); err != nil {
		t.Fatal(err)
	}
	if err := SaveLocalState(dir, meta.ID, LocalState{SnapshotID: 20, SyncedSnapshotID: 20, SyncedSeq: 2}); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureLocalCatalog(ctx, store, remote, dir, meta, ""); !IsLocalAhead(err) {
		t.Fatalf("want IsLocalAhead, got %v", err)
	}
	// 本地文件必须未被覆盖。
	body, err := readFileBytes(layout.CatalogFile)
	if err != nil || string(body) != "local-newer-catalog" {
		t.Fatalf("local catalog overwritten: %q, %v", body, err)
	}
}

func TestEnsureLocalCatalogDownloadsLatestManifest(t *testing.T) {
	remote := testRemote()
	meta := testMeta()
	dir := t.TempDir()
	store := objectstore.NewMemoryBlobStore()
	ctx := context.Background()

	// 两个快照版本；manifest 指向 seq2/snapshot20。
	for seq, snap := range map[int64]int64{1: 10, 2: 20} {
		key, _ := remote.keyBuilder().DuckLakeSnapshotKey(meta.TenantID, meta.ID, snap, 1, "")
		body := []byte{byte('0' + snap/10)}
		if err := store.PutBytes(ctx, key, body, "application/octet-stream"); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		if err := WriteManifest(ctx, store, remote, meta.TenantID, meta.ID, NewManifest(seq, snap, key, "duckdb", 1, int64(len(body)), hex.EncodeToString(sum[:]))); err != nil {
			t.Fatal(err)
		}
	}

	downloaded, err := EnsureLocalCatalog(ctx, store, remote, dir, meta, "")
	if err != nil || !downloaded {
		t.Fatalf("downloaded=%v err=%v", downloaded, err)
	}
	layout := layoutFor(dir, meta.ID)
	body, err := readFileBytes(layout.CatalogFile)
	if err != nil || len(body) != 1 || body[0] != '2' {
		t.Fatalf("want latest snapshot body, got %q %v", body, err)
	}
	st, ok := ReadLocalState(dir, meta.ID)
	if !ok || st.SnapshotID != 20 || st.SyncedSeq != 2 {
		t.Fatalf("local state = %+v ok=%v", st, ok)
	}

	// 再次调用：已最新，不重复下载（downloaded=false）。
	if again, err := EnsureLocalCatalog(ctx, store, remote, dir, meta, ""); err != nil || again {
		t.Fatalf("again=%v err=%v", again, err)
	}
}

func writeFileBytes(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}
