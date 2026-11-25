// 文件说明：lease_write_gate_test.go — internal/app 的单元测试（TestLeaseWriteGateAcquireHoldAndLost 等）
//
// 用法
//
//	go test ./internal/app -run 'TestLeaseWriteGateAcquireHoldAndLost'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/lease_write_gate_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_shutdown_test.go, app.go, app_alerts.go, app_lease.go, app_test.go 等 41 个
//
// 主要符号
//
//	type preemptedBlobStore struct{…}
//	(s *preemptedBlobStore) PutIfAbsent(ctx context.Context, key string, data []byte, contentType string) (blob.Ob...
//	TestLeaseWriteGateAcquireHoldAndLost(t *testing.T)

package app

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"github.com/linkxzhou/SimpleBase/godatabase/lease"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// preemptedBlobStore 在 preempt 打开后让续约写（lease/{epoch+1}）返回条件写冲突，
// 模拟租约被其他实例抢占；首次取租不受影响。
type preemptedBlobStore struct {
	objectstore.BlobStore
	preempt atomic.Bool
}

func (s *preemptedBlobStore) PutIfAbsent(ctx context.Context, key string, data []byte, contentType string) (blob.ObjectInfo, error) {
	if s.preempt.Load() {
		return blob.ObjectInfo{}, blob.ErrPreconditionFailed
	}
	return s.BlobStore.PutIfAbsent(ctx, key, data, contentType)
}

// TestLeaseWriteGateAcquireHoldAndLost 固定写门的三段语义：
// 首次写惰性取租 → 持租期间直接放行 → 续约失败（被抢占）后 onLost 触发、租约记录仍在但拒绝新写。
func TestLeaseWriteGateAcquireHoldAndLost(t *testing.T) {
	store := &preemptedBlobStore{BlobStore: objectstore.NewMemoryBlobStore()}
	keys := objectstore.KeyBuilder{RootPrefix: "lease-gate-test"}
	lm := lease.NewManager(store, keys.DB(), lease.Config{
		Enabled:       true,
		TTL:           300 * time.Millisecond,
		RenewInterval: 20 * time.Millisecond,
		Grace:         10 * time.Millisecond,
	})
	defer lm.StopAll()

	lost := make(chan string, 1)
	a := &App{leaseGate: lease.NewGate()}
	a.onLostLease = func(scope string) { lost <- scope }
	cfg := config.Config{}
	cfg.Instance.ID = "inst-1"
	gate := a.newLeaseWriteGate(context.Background(), cfg, lm)
	db := godatabase.Spec{ProjectID: "proj0001", ID: "0b7fa1a2-5f0e-4c3a-9d21-6f2d3c4e5a61"}
	scope := lm.ScopeKey(db.ProjectID, db.ID)

	if err := gate(db); err != nil {
		t.Fatalf("first write should acquire the lease: %v", err)
	}
	if !lm.ValidFor(scope) || !lm.Has(scope) {
		t.Fatal("expected the lease to be held after the first write")
	}
	if err := gate(db); err != nil {
		t.Fatalf("write while holding the lease should pass: %v", err)
	}

	store.preempt.Store(true)
	select {
	case got := <-lost:
		if got != scope {
			t.Fatalf("onLost scope = %q, want %q", got, scope)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("onLost was not called after the renewal was preempted")
	}
	if lm.ValidFor(scope) || !lm.Has(scope) {
		t.Fatal("expected a lost lease that still has its record")
	}
	err := gate(db)
	if err == nil || !strings.Contains(err.Error(), "write lease lost for database "+db.ID) {
		t.Fatalf("write after losing the lease should be rejected with the lost-lease error, got %v", err)
	}
}
