package ducklake

import (
	"context"
	"errors"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

func TestOrphanLedgerRecord(t *testing.T) {
	store := objectstore.NewMemoryBlobStore()
	kb := objectstore.KeyBuilder{RootPrefix: "sb", Environment: "e"}
	led := &OrphanLedger{Store: store, Keys: kb}
	ctx := context.Background()

	if err := led.Record(ctx, "11111111-1111-1111-1111-111111111111", "33333333-3333-3333-3333-333333333333", "data/foo.parquet", "delete_orphaned_files"); err != nil {
		t.Fatal(err)
	}
	// key 必须在按日目录下。
	if err := led.Record(ctx, "11111111-1111-1111-1111-111111111111", "33333333-3333-3333-3333-333333333333", "data/bar.parquet", "expire_snapshots"); err != nil {
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
