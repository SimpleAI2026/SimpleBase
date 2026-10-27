package filedb

import (
	"context"
	"errors"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
)

func remoteMeta(t *testing.T, engine string) godatabase.Spec {
	t.Helper()
	databasePath, err := godatabase.PathForProject("rmt00001", engine)
	if err != nil {
		t.Fatal(err)
	}
	return godatabase.Spec{ID: "db-remote", ProjectID: "rmt00001", DatabasePath: databasePath, Status: godatabase.StatusReady}
}

func newRemoteFactory(t *testing.T, blobs blob.Store, cacheDir string) (*Factory, *FileSyncer) {
	t.Helper()
	syncer := NewFileSyncer(blobs, blob.KeyBuilder{RootPrefix: "sb"}, cacheDir, time.Hour, 3, nil)
	return &Factory{CacheDir: cacheDir, Remote: true, Syncer: syncer}, syncer
}

func TestFileSyncerRoundTripColdStart(t *testing.T) {
	for _, engine := range []string{godatabase.EngineDuckDB, godatabase.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			ctx := context.Background()
			blobs := blob.NewMemory()
			meta := remoteMeta(t, engine)

			factory, syncer := newRemoteFactory(t, blobs, t.TempDir())
			syncer.SetWriterEpoch(meta.ProjectID, 1)
			db, err := factory.Open(ctx, meta, godatabase.ReadWrite)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, "CREATE TABLE t (id INTEGER, v VARCHAR)"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, "INSERT INTO t VALUES (1, 'hello')"); err != nil {
				t.Fatal(err)
			}
			if err := factory.AfterWrite(ctx, meta, db); err != nil {
				t.Fatal(err)
			}
			if lag := syncer.SyncLag(meta.ProjectID); lag != 1 {
				t.Fatalf("lag before flush = %d", lag)
			}
			if err := factory.BeforeClose(ctx, meta.ProjectID, db); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if last := syncer.LastSynced(meta.ProjectID); last != 1 {
				t.Fatalf("last synced = %d", last)
			}

			// 冷缓存：全新 cacheDir 必须仅凭远端 manifest 恢复完整文件。
			factory2, _ := newRemoteFactory(t, blobs, t.TempDir())
			db2, err := factory2.Open(ctx, meta, godatabase.ReadOnly)
			if err != nil {
				t.Fatal(err)
			}
			defer db2.Close()
			var v string
			if err := db2.QueryRowContext(ctx, "SELECT v FROM t WHERE id = 1").Scan(&v); err != nil {
				t.Fatal(err)
			}
			if v != "hello" {
				t.Fatalf("restored value %q", v)
			}
		})
	}
}

func TestFileSyncerRejectsChecksumMismatch(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemory()
	meta := remoteMeta(t, godatabase.EngineDuckDB)
	factory, syncer := newRemoteFactory(t, blobs, t.TempDir())
	syncer.SetWriterEpoch(meta.ProjectID, 1)
	db, err := factory.Open(ctx, meta, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	_ = factory.AfterWrite(ctx, meta, db)
	if err := factory.BeforeClose(ctx, meta.ProjectID, db); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	keys := blob.KeyBuilder{RootPrefix: "sb"}
	snapshotKey, err := keys.ProjectDatabaseSnapshotKey(meta.ProjectID, 1, 1, ".duckdb")
	if err != nil {
		t.Fatal(err)
	}
	if err := blobs.PutBytes(ctx, snapshotKey, []byte("corrupted"), ""); err != nil {
		t.Fatal(err)
	}
	factory2, _ := newRemoteFactory(t, blobs, t.TempDir())
	if _, err := factory2.Open(ctx, meta, godatabase.ReadOnly); err == nil {
		t.Fatal("expected checksum mismatch to fail open")
	}
}

func TestFileSyncerLostLeaseStopsUpload(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemory()
	meta := remoteMeta(t, godatabase.EngineSQLite)
	factory, syncer := newRemoteFactory(t, blobs, t.TempDir())
	syncer.SetWriterEpoch(meta.ProjectID, 1)
	db, err := factory.Open(ctx, meta, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	_ = factory.AfterWrite(ctx, meta, db)
	syncer.CloseNoFlush(meta.ProjectID)
	if err := syncer.Flush(ctx, meta.ProjectID); err != nil {
		t.Fatal(err)
	}
	keys, _, err := blobs.List(ctx, "sb/", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Fatalf("lost lease must not upload, got %v", keys)
	}
}

func TestFileSyncerManifestConflictFailsClosed(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemory()
	meta := remoteMeta(t, godatabase.EngineDuckDB)
	keys := blob.KeyBuilder{RootPrefix: "sb"}
	conflictKey, err := keys.ProjectDatabaseManifestKey(meta.ProjectID, 1)
	if err != nil {
		t.Fatal(err)
	}
	// 另一个 writer 抢先发布了 seq 1，但其 manifest 不可解析为有效快照时 latestManifest 会失败；
	// 这里写入合法 JSON 但 seq 不一致以外的情形由 Restore 校验，此处只模拟竞态：先建后冲突。
	factory, syncer := newRemoteFactory(t, blobs, t.TempDir())
	syncer.SetWriterEpoch(meta.ProjectID, 1)
	db, err := factory.Open(ctx, meta, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	_ = factory.AfterWrite(ctx, meta, db)
	if _, err := blobs.PutIfAbsent(ctx, conflictKey, []byte(`{}`), "application/json"); err != nil {
		t.Fatal(err)
	}
	if err := syncer.Flush(ctx, meta.ProjectID); err == nil {
		t.Fatal("expected manifest conflict")
	}
	// 冲突后必须进入失租模式：再次 Flush 不得上传。
	before, _, _ := blobs.List(ctx, "sb/", "", 100)
	_ = syncer.Flush(ctx, meta.ProjectID)
	after, _, _ := blobs.List(ctx, "sb/", "", 100)
	if len(before) != len(after) {
		t.Fatalf("split-brain writer kept uploading: %d -> %d", len(before), len(after))
	}
}

func TestFileSyncerPurgeProjectKeepsFiles(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemory()
	meta := remoteMeta(t, godatabase.EngineDuckDB)
	keys := blob.KeyBuilder{RootPrefix: "sb"}
	filesKey := "sb/" + meta.ProjectID + "/files/a.txt"
	snapshotKey, _ := keys.ProjectDatabaseSnapshotKey(meta.ProjectID, 1, 1, ".duckdb")
	manifestKey, _ := keys.ProjectDatabaseManifestKey(meta.ProjectID, 1)
	for _, k := range []string{filesKey, snapshotKey, manifestKey} {
		if err := blobs.PutBytes(ctx, k, []byte("x"), ""); err != nil {
			t.Fatal(err)
		}
	}
	cacheDir := t.TempDir()
	syncer := NewFileSyncer(blobs, keys, cacheDir, time.Hour, 3, nil)
	if err := syncer.PurgeProject(ctx, meta.ProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Head(ctx, filesKey); err != nil {
		t.Fatalf("project files must survive purge: %v", err)
	}
	for _, k := range []string{snapshotKey, manifestKey} {
		if _, err := blobs.Head(ctx, k); err == nil {
			t.Fatalf("%s should be purged", k)
		}
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "projects", meta.ProjectID)); !os.IsNotExist(err) {
		t.Fatalf("local project state should be removed: %v", err)
	}
}

type failingPutBlobs struct {
	blob.Store
	fail bool
}

func (b *failingPutBlobs) PutBytes(ctx context.Context, key string, data []byte, contentType string) error {
	if b.fail {
		return errors.New("simulated upload failure")
	}
	return b.Store.PutBytes(ctx, key, data, contentType)
}

func TestFileSyncerUploadFailureKeepsDirtyAndRetries(t *testing.T) {
	ctx := context.Background()
	blobs := &failingPutBlobs{Store: blob.NewMemory(), fail: true}
	meta := remoteMeta(t, godatabase.EngineSQLite)
	factory, syncer := newRemoteFactory(t, blobs, t.TempDir())
	syncer.SetWriterEpoch(meta.ProjectID, 1)
	db, err := factory.Open(ctx, meta, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	_ = factory.AfterWrite(ctx, meta, db)

	if err := syncer.Flush(ctx, meta.ProjectID); err == nil {
		t.Fatal("expected upload failure")
	}
	if syncer.SyncLag(meta.ProjectID) != 1 {
		t.Fatal("failed upload must leave the project dirty")
	}
	if last := syncer.LastSynced(meta.ProjectID); last != 0 {
		t.Fatalf("watermark advanced on failure: %d", last)
	}
	if keys, _, _ := blobs.List(ctx, "sb/", "", 100); len(keys) != 0 {
		t.Fatalf("no manifest may be published after failed upload, got %v", keys)
	}

	blobs.fail = false
	if err := syncer.Flush(ctx, meta.ProjectID); err != nil {
		t.Fatal(err)
	}
	if syncer.SyncLag(meta.ProjectID) != 0 || syncer.LastSynced(meta.ProjectID) != 1 {
		t.Fatalf("retry must publish seq 1, lag=%d last=%d", syncer.SyncLag(meta.ProjectID), syncer.LastSynced(meta.ProjectID))
	}
}

func TestFileSyncerLocalAheadIsNotOverwrittenByRemote(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemory()
	meta := remoteMeta(t, godatabase.EngineDuckDB)
	cacheDir := t.TempDir()

	factory, syncer := newRemoteFactory(t, blobs, cacheDir)
	syncer.SetWriterEpoch(meta.ProjectID, 1)
	db, err := factory.Open(ctx, meta, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	_ = factory.AfterWrite(ctx, meta, db)
	if err := syncer.Flush(ctx, meta.ProjectID); err != nil {
		t.Fatal(err)
	}
	// 本地继续写入但不发布：本地文件领先于远端 seq 1。
	if _, err := db.ExecContext(ctx, "INSERT INTO t VALUES (7)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CHECKPOINT"); err != nil {
		t.Fatal(err)
	}
	syncer.CloseNoFlush(meta.ProjectID)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	factory2, _ := newRemoteFactory(t, blobs, cacheDir)
	db2, err := factory2.Open(ctx, meta, godatabase.ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	var n int
	if err := db2.QueryRowContext(ctx, "SELECT count(*) FROM t WHERE id = 7").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("local-ahead row lost: the remote snapshot overwrote newer local data (n=%d)", n)
	}
}
