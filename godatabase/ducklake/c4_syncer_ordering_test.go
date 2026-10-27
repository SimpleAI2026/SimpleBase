package ducklake

// c4_syncer_ordering_test.go：planv6.0 C4-syncer 保序表征测试（提取前先行）。
// 状态图（审计结论，详见任务卡）：
//   S1 captureCatalog（锁内捕获）→ S2 上传快照（PutIfAbsent 幂等）
//   → S3 发布 manifest（CAS 冲突=split-brain）→ S4 锁内推进水位
//   → S5 本地状态落盘（仅警告）→ S6 指标/日志。
// 既有覆盖：快照/manifest 失败不推水位（catalog_syncer_test.go）、
//   split-brain、失租零 PUT（c4_syncer_lease_test.go）。
// 本文件补缺口：
//   1) manifest 发布成功后本地 state 与内存水位同步推进（S4/S5 保序）；
//   2) 本地 state 写失败不改变 Sync 结果（仅警告路径），且远端已发布。

import (
	"context"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"os"
	"path/filepath"
	"testing"
)

// TestC4_WatermarkAndLocalStateAdvanceAfterManifest：S3 成功后 S4/S5
// 必须都已推进——manifest、内存水位、本地 state 三者一致。
func TestC4_WatermarkAndLocalStateAdvanceAfterManifest(t *testing.T) {
	db, l := openTestLake(t)
	id := filepath.Base(l.Root)
	tenant := "prj12345"
	ctx := context.Background()

	blobs := blob.NewMemory()
	remote := testRemote()
	cache := t.TempDir()
	cs := NewCatalogSyncer(blobs, remote, cache, CatalogSyncOptions{}, "", nil, nil)
	// 沿用既有测试的绑定方式：cacheDir = lake 父目录，state 落在
	// {lake父}/{id}/catalog/local-state.json（与 cacheDirFor 一致）。
	cache = filepath.Dir(l.Root)
	cs.BindWithCacheDir(id, cache, db, testMetaWith(id, tenant), DefaultLakeAlias)

	if _, err := db.ExecContext(ctx, `CREATE TABLE t (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	snap, err := CurrentSnapshot(ctx, db, DefaultLakeAlias)
	if err != nil {
		t.Fatal(err)
	}
	cs.MarkDirty(id, snap)

	if err := cs.Sync(ctx, id); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// S4：内存水位推进。
	if got := cs.LastSynced(id); got < snap {
		t.Fatalf("LastSynced=%d, want >= %d", got, snap)
	}
	// S3：manifest 已发布且指向本次快照。
	latest, err := ReadLatestManifest(ctx, blobs, remote, tenant, id, 0)
	if err != nil || latest == nil {
		t.Fatalf("manifest not published: %v %v", latest, err)
	}
	if latest.SnapshotID < snap {
		t.Fatalf("manifest snapshot=%d, want >= %d", latest.SnapshotID, snap)
	}
	// S5：本地 state 同步推进（SyncedSnapshotID == 本次快照）。
	st, ok := ReadLocalState(cache, id)
	if !ok {
		t.Fatal("local state missing after sync")
	}
	if st.SyncedSnapshotID < snap {
		t.Fatalf("local state synced=%d, want >= %d", st.SyncedSnapshotID, snap)
	}
	if st.SyncedSeq != latest.Seq {
		t.Fatalf("local state seq=%d, manifest seq=%d", st.SyncedSeq, latest.Seq)
	}
}

// TestC4_LocalStateFailureDoesNotFailSync：S5 失败仅警告——
// {cache}/{id}/catalog 被普通文件占位使 SaveLocalState 的 MkdirAll 失败，
// Sync 仍成功、远端仍发布。
func TestC4_LocalStateFailureDoesNotFailSync(t *testing.T) {
	db, l := openTestLake(t)
	id := filepath.Base(l.Root)
	tenant := "prj12345"
	ctx := context.Background()

	blobs := blob.NewMemory()
	remote := testRemote()
	// 预占 local-state 目标路径：{lake父}/{id}/catalog/local-state.json
	// 本身是目录，SaveLocalState 的 os.Rename(tmp→目录) 恒失败（仅警告分支）。
	lakeParent := filepath.Dir(l.Root)
	if err := os.MkdirAll(filepath.Join(lakeParent, id, "catalog", "local-state.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	cs := NewCatalogSyncer(blobs, remote, t.TempDir(), CatalogSyncOptions{}, "", nil, nil)
	cs.BindWithCacheDir(id, lakeParent, db, testMetaWith(id, tenant), DefaultLakeAlias)

	if _, err := db.ExecContext(ctx, `CREATE TABLE t (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	snap, err := CurrentSnapshot(ctx, db, DefaultLakeAlias)
	if err != nil {
		t.Fatal(err)
	}
	cs.MarkDirty(id, snap)

	// S5 失败不得影响 Sync 结果（仅警告）。
	if err := cs.Sync(ctx, id); err != nil {
		t.Fatalf("sync must succeed despite local-state write failure: %v", err)
	}
	if got := cs.LastSynced(id); got < snap {
		t.Fatalf("watermark not advanced: %d", got)
	}
	latest, err := ReadLatestManifest(ctx, blobs, remote, tenant, id, 0)
	if err != nil || latest == nil {
		t.Fatalf("manifest must still publish: %v %v", latest, err)
	}
}
