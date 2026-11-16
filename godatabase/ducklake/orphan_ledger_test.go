package ducklake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"testing"
)

func TestOrphanLedgerRecord(t *testing.T) {
	store := blob.NewMemory()
	kb := blob.KeyBuilder{RootPrefix: "sb"}
	led := &OrphanLedger{Store: store, Keys: kb}
	ctx := context.Background()

	if err := led.Record(ctx, "prj12345", "33333333-3333-3333-3333-333333333333", "data/foo.parquet", "delete_orphaned_files"); err != nil {
		t.Fatal(err)
	}
	// key 必须在按日目录下。
	if err := led.Record(ctx, "prj12345", "33333333-3333-3333-3333-333333333333", "data/bar.parquet", "expire_snapshots"); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceGate(t *testing.T) {
	// 未配置（nil）：一切通过。
	var gate *MaintenanceGate
	if err := gate.Check("db"); err != nil {
		t.Fatal(err)
	}

	// 持租 + 追平：通过。
	g := &MaintenanceGate{
		LeaseValid: func(dbID string) bool { return true },
		SyncLag:    func(dbID string) int64 { return 0 },
	}
	if err := g.Check("db"); err != nil {
		t.Fatal(err)
	}

	// 失租：拦截，reason=no_lease。
	g2 := &MaintenanceGate{LeaseValid: func(string) bool { return false }}
	err := g2.Check("db")
	if !errors.Is(err, ErrGateBlocked) {
		t.Fatalf("want ErrGateBlocked, got %v", err)
	}
	if r := BlockedReason(err); r != "no_lease" {
		t.Fatalf("reason = %q", r)
	}

	// 滞后：拦截，reason=sync_lag。
	g3 := &MaintenanceGate{SyncLag: func(string) int64 { return 3 }}
	err = g3.Check("db")
	if !errors.Is(err, ErrGateBlocked) {
		t.Fatalf("want ErrGateBlocked, got %v", err)
	}
	if r := BlockedReason(err); r != "sync_lag" {
		t.Fatalf("reason = %q", r)
	}
	if r := BlockedReason(nil); r != "" {
		t.Fatalf("nil reason = %q", r)
	}
}

// §4.7 接线回归：pruneVersions 删除快照前必须先写孤儿账本；
// 失租（CloseNoFlush）后门禁拦截，不再有任何删除与记账。
func TestPruneRecordsOrphansAndGateBlocksLostLease(t *testing.T) {
	store := blob.NewMemory()
	remote := testRemote()
	meta := testMeta()
	ctx := context.Background()

	cs := NewCatalogSyncer(store, remote, t.TempDir(), CatalogSyncOptions{KeepVersions: 2}, "", nil, nil)
	snapKeys := writeChain(t, store, remote, meta.ProjectID, meta.ID, 4)

	if err := cs.pruneVersions(ctx, meta, 400, 4); err != nil {
		t.Fatal(err)
	}
	// seq 1..2 到期且未被保留窗口（[3,4]）引用 → 已删；每次删除前各有一条 pending 账目。
	keys, _, err := store.List(ctx, "simplebase/prj12345/databases/"+meta.ID+"/orphans/", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("orphan ledger entries = %d, want 2", len(keys))
	}
	for _, k := range keys {
		body, _, err := store.GetBytes(ctx, k)
		if err != nil {
			t.Fatal(err)
		}
		var rec OrphanRecord
		if err := json.Unmarshal(body, &rec); err != nil {
			t.Fatal(err)
		}
		if rec.Status != "pending" || rec.Reason != "expire_snapshots" || rec.DatabaseID != meta.ID || rec.TenantID != meta.ProjectID {
			t.Fatalf("unexpected record: %+v", rec)
		}
		if rec.FileKey != snapKeys[0] && rec.FileKey != snapKeys[1] {
			t.Fatalf("record references unexpected key: %s", rec.FileKey)
		}
	}

	// 失租后：写一条新链制造新的到期区间，门禁必须拦截删除。
	// 复用独立 store 避免与已写链的 manifest seq 冲突。
	store2 := blob.NewMemory()
	cs2 := NewCatalogSyncer(store2, remote, t.TempDir(), CatalogSyncOptions{KeepVersions: 2}, "", nil, nil)
	snapKeys2 := writeChain(t, store2, remote, meta.ProjectID, meta.ID, 4)
	// 先正常清一轮（seq 1..2 到期），验证记账后推进。
	if err := cs2.pruneVersions(ctx, meta, 400, 4); err != nil {
		t.Fatal(err)
	}
	cs2.CloseNoFlush(meta.ID)
	writeChainAt(t, store2, remote, meta.ProjectID, meta.ID, 5, 4) // seq 5..6
	if err := cs2.pruneVersions(ctx, meta, 600, 6); err != nil {
		t.Fatal(err)
	}
	if !cs2.HasLostLease(meta.ID) {
		t.Fatal("lost lease flag expected")
	}
	// 门禁拦截：到期区间 [3,4] 的快照仍在，账本条目数不变。
	for _, key := range snapKeys2[2:4] {
		if _, err := store2.Head(ctx, key); err != nil {
			t.Fatalf("snapshot must survive while gate blocks: %v", err)
		}
	}
	after, _, err := store2.List(ctx, "simplebase/prj12345/databases/"+meta.ID+"/orphans/", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 2 {
		t.Fatalf("orphan ledger entries changed while gate blocked: %d", len(after))
	}
}

// writeChainAt 从 startSeq 起写入连续 manifest 序列（供失租场景追加新链）。
func writeChainAt(t *testing.T, store blob.Store, remote RemoteStorage, tenant, db string, startSeq, n int64) {
	t.Helper()
	ctx := context.Background()
	for seq := startSeq; seq < startSeq+n; seq++ {
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
	}
}
