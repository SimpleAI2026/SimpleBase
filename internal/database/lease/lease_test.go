package lease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

func newTestManager(t *testing.T, cfg Config) (*Manager, objectstore.BlobStore) {
	t.Helper()
	store := objectstore.NewMemoryBlobStore()
	keys := objectstore.KeyBuilder{RootPrefix: "sb", Environment: "env"}
	if cfg.TTL == 0 {
		cfg.TTL = 50 * time.Millisecond
	}
	if cfg.Grace == 0 {
		cfg.Grace = 20 * time.Millisecond
	}
	cfg.Enabled = true
	return NewManager(store, keys, cfg), store
}

// testLogger 收集 Printf 输出（perf §1.5 同 PID 重启提示用）。
type testLogger struct {
	mu     sync.Mutex
	lines  []string
}

func (l *testLogger) Printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *testLogger) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

func TestAcquireFirstWriter(t *testing.T) {
	m, _ := newTestManager(t, Config{})
	ctx := context.Background()
	l, err := m.Acquire(ctx, "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "inst-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if l.Epoch != 1 {
		t.Fatalf("epoch = %d, want 1", l.Epoch)
	}
	if !l.Held() {
		t.Fatal("should be held")
	}
	if !m.ValidFor("22222222-2222-2222-2222-222222222222") {
		t.Fatal("ValidFor should be true")
	}
	l.Stop()
	m.StopAll()
}

func TestAcquireSecondInstanceBlockedWhileHeld(t *testing.T) {
	m, _ := newTestManager(t, Config{TTL: 30 * time.Second, Grace: 100 * time.Millisecond})
	ctx := context.Background()
	if _, err := m.Acquire(ctx, "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "inst-a", nil); err != nil {
		t.Fatal(err)
	}
	_, err := m.Acquire(ctx, "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "inst-b", nil)
	if !IsHeld(err) {
		t.Fatalf("want ErrLeaseHeld, got %v", err)
	}
	m.StopAll()
}

func TestTakeoverAfterStopRenew(t *testing.T) {
	// 实例 A 停止续约（进程死亡模拟）：B 在 ttl+grace 观察后应可接管。
	store := objectstore.NewMemoryBlobStore()
	keys := objectstore.KeyBuilder{RootPrefix: "sb", Environment: "env"}
	cfg := Config{Enabled: true, TTL: 50 * time.Millisecond, Grace: 50 * time.Millisecond}
	m := NewManager(store, keys, cfg)
	ctx := context.Background()
	la, err := m.Acquire(ctx, "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "inst-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	la.Stop() // 停止续约；对象留在 store
	m.StopAll()

	m2 := NewManager(store, keys, cfg)
	lb, err := m2.Acquire(ctx, "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "inst-b", nil)
	if err != nil {
		t.Fatalf("takeover should succeed after ttl+grace: %v", err)
	}
	if lb.Epoch != 2 {
		t.Fatalf("epoch = %d, want 2", lb.Epoch)
	}
	m2.StopAll()
}

func TestOnLostCalledWhenPreempted(t *testing.T) {
	// A 持租；外部伪造更高 epoch（模拟抢占）；A 的续约应失败并回调 onLost。
	store := objectstore.NewMemoryBlobStore()
	keys := objectstore.KeyBuilder{RootPrefix: "sb", Environment: "env"}
	cfg := Config{Enabled: true, TTL: 40 * time.Millisecond, RenewInterval: 20 * time.Millisecond}
	m := NewManager(store, keys, cfg)
	ctx := context.Background()

	lost := make(chan string, 1)
	if _, err := m.Acquire(ctx, "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "inst-a", func(dbID string) {
		lost <- dbID
	}); err != nil {
		t.Fatal(err)
	}

	// 抢占：直接写 epoch 2（外部 writer）。
	pl := Payload{OwnerID: "inst-b/1", InstanceID: "inst-b", TTLMS: 40000, Epoch: 2}
	body, err := json.Marshal(pl)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := keys.DuckLakeLeaseKey("11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", 2)
	if _, err := store.PutIfAbsent(ctx, key, body, "application/json"); err != nil {
		t.Fatal(err)
	}

	select {
	case dbID := <-lost:
		if dbID != "22222222-2222-2222-2222-222222222222" {
			t.Fatalf("onLost dbID = %s", dbID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onLost not called after preemption")
	}
	m.StopAll()
}

func TestEpochForAndHas(t *testing.T) {
	m, _ := newTestManager(t, Config{})
	ctx := context.Background()
	if m.Has("33333333-3333-3333-3333-333333333333") {
		t.Fatal("no lease yet")
	}
	if m.EpochFor("33333333-3333-3333-3333-333333333333") != 0 {
		t.Fatal("epoch should be 0")
	}
	l, err := m.Acquire(ctx, "11111111-1111-1111-1111-111111111111", "33333333-3333-3333-3333-333333333333", "inst-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Has("33333333-3333-3333-3333-333333333333") || m.EpochFor("33333333-3333-3333-3333-333333333333") != l.Epoch {
		t.Fatal("Has/EpochFor mismatch")
	}
	m.StopAll()
}

func TestDisabledManager(t *testing.T) {
	m := NewManager(nil, objectstore.KeyBuilder{}, Config{})
	if _, err := m.Acquire(context.Background(), "t", "d", "i", nil); !errors.Is(err, ErrLeaseHeld) && err == nil {
		t.Fatal("disabled manager must reject Acquire")
	}
	if m.ValidFor("d") {
		t.Fatal("disabled manager never valid")
	}
}

// TestRestartSamePIDLogged 验证发现旧持租者与本进程同 instance/pid（即重启
// 场景）时输出明确提示日志（perf §1.5 方案 2）。
func TestRestartSamePIDLogged(t *testing.T) {
	m, store := newTestManager(t, Config{TTL: 50 * time.Millisecond, Grace: 20 * time.Millisecond})
	tl := &testLogger{}
	m.SetLogger(tl)

	// 手动放置一个「本进程旧 bootID」的租约对象，模拟崩溃前残留。
	pl := Payload{
		OwnerID:    fmt.Sprintf("inst-a/%d/old-boot", m.pid),
		InstanceID: "inst-a",
		PID:        m.pid,
		BootID:     "old-boot",
		Epoch:      1,
	}
	keys := objectstore.KeyBuilder{RootPrefix: "sb", Environment: "env"}
	body, _ := json.Marshal(pl)
	leaseKey, err := keys.DuckLakeLeaseKey("11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutIfAbsent(context.Background(), leaseKey, body, "application/json"); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Acquire(context.Background(), "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "inst-a", nil); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range tl.all() {
		if strings.Contains(line, "previous holder has same instance/pid") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected restart hint log, got %v", tl.all())
	}
	m.StopAll()
}
