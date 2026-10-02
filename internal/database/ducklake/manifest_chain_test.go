package ducklake

// manifest_chain_test.go 覆盖 v2.0 计划 §3.1 的发现协议回归：
// 断链 fail closed、锚点校验、指数+二分定位、修剪不删 manifest。

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// writeChain 写入连续 manifest 序列 [1, n]，快照对象一并存在。
func writeChain(t *testing.T, store objectstore.BlobStore, remote RemoteStorage, tenant, db string, n int64) []string {
	t.Helper()
	ctx := context.Background()
	keys := make([]string, 0, n)
	for seq := int64(1); seq <= n; seq++ {
		snapKey, err := remote.keyBuilder().DuckLakeSnapshotKey(tenant, db, seq*100, 7, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PutBytes(ctx, snapKey, []byte(fmt.Sprintf("snap-%d", seq)), "application/x-sqlite3"); err != nil {
			t.Fatal(err)
		}
		if err := WriteManifest(ctx, store, remote, tenant, db, NewManifest(seq, seq*100, snapKey, "duckdb", 7, 0, "")); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, snapKey)
	}
	return keys
}

func TestReadLatestManifestLongChain(t *testing.T) {
	store := objectstore.NewMemoryBlobStore()
	remote := testRemote()
	meta := testMeta()
	// 200 个 seq：旧实现的 64 次前探上限会错误地停在 64。
	writeChain(t, store, remote, meta.TenantID, meta.ID, 200)

	latest, err := ReadLatestManifest(context.Background(), store, remote, meta.TenantID, meta.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.Seq != 200 {
		t.Fatalf("latest = %+v, want seq 200", latest)
	}
	// 带锚点：同样收敛到 200。
	latest2, err := ReadLatestManifest(context.Background(), store, remote, meta.TenantID, meta.ID, 150)
	if err != nil {
		t.Fatal(err)
	}
	if latest2 == nil || latest2.Seq != 200 {
		t.Fatalf("anchored latest = %+v, want seq 200", latest2)
	}
}

func TestReadLatestManifestBrokenChainFailsClosed(t *testing.T) {
	store := objectstore.NewMemoryBlobStore()
	remote := testRemote()
	meta := testMeta()
	ctx := context.Background()

	// 模拟历史部署的修剪破坏：seq 1 已删，2..5 存在。
	writeChain(t, store, remote, meta.TenantID, meta.ID, 5)
	key1, _ := remote.keyBuilder().DuckLakeManifestKey(meta.TenantID, meta.ID, 1)
	if err := store.Delete(ctx, key1); err != nil {
		t.Fatal(err)
	}

	// 冷缓存发现：seq 1 缺失但更高 seq 存在 → 断链，不得当作新库。
	if _, err := ReadLatestManifest(ctx, store, remote, meta.TenantID, meta.ID, 0); !IsManifestChainBroken(err) {
		t.Fatalf("want chain broken, got %v", err)
	}
	// 锚点缺失：cached=1 已被删除 → 断链。
	if _, err := ReadLatestManifest(ctx, store, remote, meta.TenantID, meta.ID, 1); !IsManifestChainBroken(err) {
		t.Fatalf("want chain broken for missing anchor, got %v", err)
	}
	// 断链时冷启动必须 fail closed，不得回退旧镜像。
	if _, err := EnsureLocalCatalog(ctx, store, remote, t.TempDir(), meta, ""); !IsManifestChainBroken(err) {
		t.Fatalf("EnsureLocalCatalog want chain broken, got %v", err)
	}
}

func TestPruneKeepsManifestsAndReferencedSnapshots(t *testing.T) {
	store := objectstore.NewMemoryBlobStore()
	remote := testRemote()
	meta := testMeta()
	ctx := context.Background()
	dir := t.TempDir()

	cs := NewCatalogSyncer(store, remote, dir, CatalogSyncOptions{KeepVersions: 3}, "", nil, nil)
	snapKeys := writeChain(t, store, remote, meta.TenantID, meta.ID, 6)

	// 第 6 次同步后清理：保留窗口为 seq [4,6]，限速单轮处理 3 个 → [1,3]。
	if err := cs.pruneVersions(ctx, meta, 600, 6); err != nil {
		t.Fatal(err)
	}
	if err := cs.pruneVersions(ctx, meta, 600, 6); err != nil { // 第二轮推进剩余进度
		t.Fatal(err)
	}

	// manifest 索引永不删除：1..6 全部仍在。
	for seq := int64(1); seq <= 6; seq++ {
		m, err := getManifestAt(ctx, store, remote.keyBuilder(), meta.TenantID, meta.ID, seq)
		if err != nil || m == nil {
			t.Fatalf("manifest %d must survive pruning: %v", seq, err)
		}
	}
	// 过期且未被引用的快照（seq 1..3）已删除；保留窗口（4..6）仍在。
	for i, key := range snapKeys {
		_, err := store.Head(ctx, key)
		if i < 3 && err == nil {
			t.Fatalf("expired snapshot seq %d should be deleted", i+1)
		}
		if i >= 3 && err != nil {
			t.Fatalf("retained snapshot seq %d must exist: %v", i+1, err)
		}
	}
	// 进度已持久化：第三轮不再从 seq 1 重扫（直接无操作返回）。
	if err := cs.pruneVersions(ctx, meta, 600, 6); err != nil {
		t.Fatal(err)
	}
}

func TestPruneSkipsSnapshotStillReferenced(t *testing.T) {
	store := objectstore.NewMemoryBlobStore()
	remote := testRemote()
	meta := testMeta()
	ctx := context.Background()

	cs := NewCatalogSyncer(store, remote, t.TempDir(), CatalogSyncOptions{KeepVersions: 2}, "", nil, nil)
	// seq 1..4：seq 4 与过期 seq 2 指向同一快照对象（重试路径会产生这种引用）。
	shared, _ := remote.keyBuilder().DuckLakeSnapshotKey(meta.TenantID, meta.ID, 200, 7, "")
	if err := store.PutBytes(ctx, shared, []byte("shared"), "application/x-sqlite3"); err != nil {
		t.Fatal(err)
	}
	write := func(seq int64, key string) {
		if err := WriteManifest(ctx, store, remote, meta.TenantID, meta.ID, NewManifest(seq, 200, key, "duckdb", 7, 0, "")); err != nil {
			t.Fatal(err)
		}
	}
	write(1, shared)
	write(2, shared)
	write(3, shared)
	write(4, shared)
	// KeepVersions=2，currentSeq=4 → 到期 [1,3]，保留窗口 [3,4] 仍引用 shared。
	if err := cs.pruneVersions(ctx, meta, 200, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Head(ctx, shared); err != nil {
		t.Fatalf("snapshot still referenced by retained manifest must survive: %v", err)
	}
}

func TestCloseNoFlushIsPerDatabase(t *testing.T) {
	db1, l1 := openTestLake(t)
	db2, l2 := openTestLake(t)
	// id 必须与打开时一致：staging 位于 {cache}/{id}/sync-staging，
	// duckdb 原生 ATTACH 受 allowed_directories（= 该库根目录）约束。
	id1 := filepath.Base(l1.Root)
	id2 := filepath.Base(l2.Root)
	tenant := "33333333-3333-3333-3333-333333333333"
	ctx := context.Background()

	blobs := objectstore.NewMemoryBlobStore()
	remote := testRemote()
	cs := NewCatalogSyncer(blobs, remote, t.TempDir(), CatalogSyncOptions{}, "", nil, nil)
	// duckdb 引擎的 staging 走原生 ATTACH，受 allowed_directories 约束：
	// 必须绑定打开该库的 Factory 缓存目录（生产路径 Factory.Open 即如此）。
	cs.BindWithCacheDir(id1, filepath.Dir(l1.Root), db1, testMetaWith(id1, tenant), DefaultLakeAlias)
	cs.BindWithCacheDir(id2, filepath.Dir(l2.Root), db2, testMetaWith(id2, tenant), DefaultLakeAlias)

	cs.CloseNoFlush(id1) // 仅 id1 失租

	// id1：失租后 Sync 不再上传（也不报 not bound/closed）。
	if err := cs.Sync(ctx, id1); err != nil {
		t.Fatalf("lost db sync should be a no-op: %v", err)
	}
	if latest, _ := ReadLatestManifest(ctx, blobs, remote, tenant, id1, 0); latest != nil {
		t.Fatal("lost lease db must not upload manifest")
	}
	// id2：不受连坐，正常同步上传。
	if _, err := db2.ExecContext(ctx, `CREATE TABLE t2 (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	snap2, err := CurrentSnapshot(ctx, db2, DefaultLakeAlias)
	if err != nil {
		t.Fatal(err)
	}
	cs.MarkDirty(id2, snap2)
	if err := cs.Flush(ctx, id2); err != nil {
		t.Fatalf("healthy db sync: %v", err)
	}
	if latest, _ := ReadLatestManifest(ctx, blobs, remote, tenant, id2, 0); latest == nil {
		t.Fatal("healthy db manifest must be uploaded")
	}
	if !cs.HasLostLease(id1) || cs.HasLostLease(id2) {
		t.Fatal("per-db lease loss scope broken")
	}
}

func testMetaWith(id, tenant string) catalog.Database {
	return catalog.Database{ID: id, TenantID: tenant}
}
