package ducklake

// c4_syncer_lease_test.go：planv6.0 C4-syncer 表征测试（零生产修改）。
// 既有覆盖盘点（审计结论，无需重复建）：
//   - TestCloseNoFlushIsPerDatabase：失租后 Sync 为 no-op、不连坐健康库；
//   - TestManifestConflictIsSplitBrain：CAS 冲突 = split-brain；
//   - TestWriteGateSerializesFlushAndWrites：flush/copy 与写事务互斥；
//   - TestCatalogSyncFailureKeepsUnsyncedAgeAndManifest：失败不污染回退源；
//   - TestReadLatestManifestBrokenChainFailsClosed：断链 fail closed。
// 本文件补计划点名的「关闭与并发 Flush」缺口：
//   CloseNoFlush 置位后，显式 Flush（而非周期 Sync）同样不得产生任何
//   远端写入（manifest/snapshot 均不出现）。
// 本文件只补测试；生产代码不动（C 类默认仅分析 + 表征）。

import (
	"context"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"path/filepath"
	"testing"
)

func TestC4_LostLeaseFlushDoesNotUpload(t *testing.T) {
	db, l := openTestLake(t)
	id := filepath.Base(l.Root)
	tenant := "prj12345"
	ctx := context.Background()

	blobs := blob.NewMemory()
	remote := testRemote()
	cs := NewCatalogSyncer(blobs, remote, t.TempDir(), CatalogSyncOptions{}, "", nil, nil)
	cs.BindWithCacheDir(id, filepath.Dir(l.Root), db, testMetaWith(id, tenant), DefaultLakeAlias)

	// 先产生脏数据（写入 + MarkDirty）。
	if _, err := db.ExecContext(ctx, `CREATE TABLE t (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	snap, err := CurrentSnapshot(ctx, db, DefaultLakeAlias)
	if err != nil {
		t.Fatal(err)
	}
	cs.MarkDirty(id, snap)

	// 失租后显式 Flush：不得上传 manifest 或 snapshot。
	cs.CloseNoFlush(id)
	if err := cs.Flush(ctx, id); err != nil {
		t.Fatalf("lost lease Flush must not error: %v", err)
	}
	if latest, _ := ReadLatestManifest(ctx, blobs, remote, tenant, id, 0); latest != nil {
		t.Fatal("lost lease Flush must not upload manifest")
	}
	if !cs.HasLostLease(id) {
		t.Fatal("lease loss flag missing")
	}
}
