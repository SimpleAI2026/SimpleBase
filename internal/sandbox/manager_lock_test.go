package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
)

// holdLock 在后台占住 name 的 VM 锁，直到返回的 release 被调用。
func holdLock(t *testing.T, m *Manager, name string) (release func()) {
	t.Helper()
	entered, done := make(chan struct{}), make(chan struct{})
	go func() {
		_ = m.withLock(context.Background(), name, func() error {
			close(entered)
			<-done
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("lock holder never entered")
	}
	return func() { close(done) }
}

func TestManagerDefaultLockWaitIsFiveSeconds(t *testing.T) {
	m := NewManager(config.SandboxConfig{}, nil, nil)
	if m.lockWait != 5*time.Second {
		t.Fatalf("lockWait = %v, want 5s", m.lockWait)
	}
}

func TestManagerLockWaitTimeoutReturnsErrBusy(t *testing.T) {
	cfg := config.SandboxConfig{Enabled: true, Workdir: "/workspace", ExecTimeout: time.Second,
		ExecTimeoutMax: time.Minute, MaxFileBytes: 16}
	m := NewManager(cfg, nil, NewFakeDriver())
	m.lockWait = 50 * time.Millisecond
	release := holdLock(t, m, "sbx-1")
	defer release()

	ctx := context.Background()
	start := time.Now()
	if _, err := m.Exec(ctx, "p", "sbx-1", ExecInput{Command: "echo hi"}); !errors.Is(err, ErrBusy) {
		t.Fatalf("exec while locked: %v", err)
	}
	if waited := time.Since(start); waited < 50*time.Millisecond || waited > 2*time.Second {
		t.Fatalf("waited %v, want about lockWait", waited)
	}
	if err := m.WriteFile(ctx, "p", "sbx-1", "/workspace/a", []byte("x")); !errors.Is(err, ErrBusy) {
		t.Fatalf("write while locked: %v", err)
	}
	if _, err := m.Stop(ctx, "p", "sbx-1"); !errors.Is(err, ErrBusy) {
		t.Fatalf("stop while locked: %v", err)
	}
	// 其它沙盒不受影响：不同 VM 锁互不阻塞。
	if err := m.withLock(ctx, "sbx-2", func() error { return nil }); err != nil {
		t.Fatalf("other sandbox blocked: %v", err)
	}
}

func TestManagerLockReleasedAfterBusy(t *testing.T) {
	m := NewManager(config.SandboxConfig{}, nil, nil)
	m.lockWait = 20 * time.Millisecond
	release := holdLock(t, m, "x")
	if err := m.withLock(context.Background(), "x", func() error { return nil }); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected busy: %v", err)
	}
	release()
	deadline := time.Now().Add(time.Second)
	for {
		err := m.withLock(context.Background(), "x", func() error { return nil })
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("lock never released: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	// lockWait 非法值回退默认，不能变成立即失败。
	m.lockWait = 0
	if err := m.withLock(context.Background(), "x", func() error { return nil }); err != nil {
		t.Fatalf("zero lockWait: %v", err)
	}
}
