package filedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	_ "github.com/duckdb/duckdb-go/v2"
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

// —— R2：Flush 期间的写入不得丢失 dirty 标记（planv6.4 §2）——

// gatedPutBlobs 第一次 PutBytes 阻塞直到放行；其余直接放行。
// 用计数而不是无缓冲通道，避免测试时序死锁。
type gatedPutBlobs struct {
	blob.Store
	gate    chan struct{} // 关闭后放行第一次上传
	firstMu sync.Mutex
	first   bool
	calls   chan string // 每次上传的 key（带缓冲）
}

func (b *gatedPutBlobs) PutBytes(ctx context.Context, key string, data []byte, contentType string) error {
	b.firstMu.Lock()
	first := b.first
	b.first = false
	b.firstMu.Unlock()
	// 先通知（带缓冲），再按需阻塞：测试侧能观察到「已进入上传」。
	select {
	case b.calls <- key:
	default:
	}
	if first && b.gate != nil {
		select {
		case <-b.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return b.Store.PutBytes(ctx, key, data, contentType)
}

func TestFileSyncerFlushKeepsDirtyForWritesDuringUpload(t *testing.T) {
	ctx := context.Background()
	blobs := &gatedPutBlobs{
		Store:  blob.NewMemory(),
		gate:   make(chan struct{}),
		first:  true,
		calls:  make(chan string, 16),
	}
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
	_ = factory.AfterWrite(ctx, meta, db) // MarkDirty -> gen=1

	// 后台 Flush：卡在第一条 PutBytes（快照上传）里。
	done := make(chan error, 1)
	go func() { done <- syncer.Flush(ctx, meta.ProjectID) }()
	// 等待 Flush 真正进入上传（第一条 key 到达）。
	select {
	case <-blobs.calls:
	case <-time.After(10 * time.Second):
		t.Fatal("flush never reached PutBytes")
	}
	// Flush 窗口内的新写入：gen=2。
	// SQLite 单连接：VACUUM 已结束（snapshot 在上传前完成），
	// 此时 INSERT 只需等数据库锁，不会等上传。
	if _, err := db.ExecContext(ctx, "INSERT INTO t VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	_ = factory.AfterWrite(ctx, meta, db)

	// 放行本轮上传。
	close(blobs.gate)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("flush did not finish")
	}
	// 关键断言：窗口内的写入必须仍被视为未同步（gen 领先 flushedGen）。
	if lag := syncer.SyncLag(meta.ProjectID); lag != 1 {
		t.Fatalf("write during flush lost: lag=%d, want 1", lag)
	}
	// 下一轮必须上传新的 seq。
	blobs.gate = nil
	if err := syncer.Flush(ctx, meta.ProjectID); err != nil {
		t.Fatal(err)
	}
	if lag := syncer.SyncLag(meta.ProjectID); lag != 0 {
		t.Fatalf("second flush must clear lag: lag=%d", lag)
	}
	if last := syncer.LastSynced(meta.ProjectID); last != 2 {
		t.Fatalf("second flush must publish seq 2, got %d", last)
	}
}

// R3 复现：DuckDB 快照「CHECKPOINT 后裸拷贝」无写互斥。
// 后台持续 INSERT 的同时循环 Flush，下载每个发布的快照并打开校验，
// 撕裂文件会在此暴露（打开失败 / 行数异常 / integrity 报错）。
// 不能复现则降级记录（见 plan §R3），修复互斥后再跑此用例作回归。
func TestFileSyncerDuckDBSnapshotConsistencyUnderConcurrentWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("skip concurrency stress in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	blobs := blob.NewMemory()
	meta := remoteMeta(t, godatabase.EngineDuckDB)
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

	// 后台持续写入（每轮 INSERT 后 MarkDirty，让 Flush 一直在有事可做）。
	stopWrites := make(chan struct{})
	var writes atomic.Int64
	writeErr := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stopWrites:
				writeErr <- nil
				return
			default:
			}
			// 批量写入：让 WAL 快速增长，提高拷贝窗口内自动 checkpoint 概率。
			batch := writes.Add(500)
			if _, err := db.ExecContext(ctx,
				"INSERT INTO t SELECT range FROM range(?, ?)", batch-500, batch); err != nil {
				writeErr <- err
				return
			}
			syncer.MarkDirty(meta.ProjectID)
		}
	}()

	// 并发循环 Flush：每一轮都会 CHECKPOINT + 拷贝主文件。
	last := int64(0)
	for i := 0; i < 60 && ctx.Err() == nil; i++ {
		syncer.MarkDirty(meta.ProjectID)
		if err := syncer.Flush(ctx, meta.ProjectID); err != nil {
			close(stopWrites)
			t.Fatalf("flush %d: %v", i, err)
		}
		published := syncer.LastSynced(meta.ProjectID)
		if published == last {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		last = published
		// 下载本轮快照并打开校验完整性。
		keys := blob.KeyBuilder{RootPrefix: "sb"}
		snapKey, err := keys.ProjectDatabaseSnapshotKey(meta.ProjectID, published, 1, ".duckdb")
		if err != nil {
			close(stopWrites)
			t.Fatal(err)
		}
		data, _, err := blobs.GetBytes(ctx, snapKey)
		if err != nil {
			close(stopWrites)
			t.Fatalf("get snapshot seq=%d: %v", published, err)
		}
		snapPath := filepath.Join(t.TempDir(), fmt.Sprintf("snap-%020d.duckdb", published))
		if err := os.WriteFile(snapPath, data, 0o644); err != nil {
			close(stopWrites)
			t.Fatal(err)
		}
		verify, err := sql.Open("duckdb", snapPath)
		if err != nil {
			close(stopWrites)
			t.Fatalf("open snapshot seq=%d: %v (torn file)", published, err)
		}
		var rows int64
		if err := verify.QueryRowContext(ctx, "SELECT count(*) FROM t").Scan(&rows); err != nil {
			_ = verify.Close()
			close(stopWrites)
			t.Fatalf("query snapshot seq=%d: %v (torn file)", published, err)
		}
		_ = verify.Close()
	}
	close(stopWrites)
	if err := <-writeErr; err != nil {
		t.Fatalf("background writes failed: %v", err)
	}

	// 结束后本地库必须完好。
	var total int64
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM t").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != writes.Load() {
		t.Fatalf("local rows = %d, writes = %d", total, writes.Load())
	}
}
