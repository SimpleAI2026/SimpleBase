package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm/fake"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	_ "github.com/uglyer/go-sqlite3"
)

func newSandboxManagerTest(t *testing.T, limit int) (*sandbox.Manager, *fake.Driver, *systemdb.Store) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := systemdb.NewStoreForTest(db)
	driver := fake.New()
	cfg := config.SandboxConfig{Enabled: true, Backend: "fake", Image: "python:3.12-slim", CPUs: 1,
		MemoryMiB: 256, Network: "none", Workdir: "/workspace", IdleTimeout: time.Minute,
		MaxDuration: 30 * time.Minute, ExecTimeout: time.Second, ExecTimeoutMax: time.Minute,
		MaxOutputBytes: 4, MaxFileBytes: 8, MaxPerProject: limit,
		Terminal: config.SandboxTerminalConfig{Enabled: true, MaxSessionsPerSandbox: 2, IdleTimeout: time.Minute}}
	return sandbox.NewManager(cfg, sandboxStoreAdapter{s: store}, driver), driver, store
}

func TestSandboxManagerLifecycleAndLimits(t *testing.T) {
	m, d, store := newSandboxManagerTest(t, 2)
	ctx := context.Background()
	first, err := m.Create(ctx, "p", sandbox.CreateInput{Name: "demo"}, "u")
	if err != nil || first.Status != "pending" || d.VMCount() != 0 {
		t.Fatalf("lazy create: %+v, %v, vms=%d", first, err, d.VMCount())
	}
	if _, err = m.Create(ctx, "p", sandbox.CreateInput{Name: "demo"}, "u"); !errors.Is(err, sandbox.ErrNameConflict) {
		t.Fatalf("name conflict: %v", err)
	}
	if _, err = m.Create(ctx, "p", sandbox.CreateInput{Name: "second"}, "u"); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Create(ctx, "p", sandbox.CreateInput{Name: "third"}, "u"); !errors.Is(err, sandbox.ErrLimitExceeded) {
		t.Fatalf("limit: %v", err)
	}
	if _, err = m.Get(ctx, "other", first.ID, false); !errors.Is(err, sandbox.ErrNotFound) {
		t.Fatalf("cross project: %v", err)
	}
	result, err := m.Exec(ctx, "p", first.ID, sandbox.ExecInput{Command: "echo abcdef"})
	if err != nil || result.Stdout != "abcd" || !result.StdoutTruncated {
		t.Fatalf("truncation: %+v, %v", result, err)
	}
	if err = m.WriteFile(ctx, "p", first.ID, "/workspace/data", []byte("content")); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ReadFile(ctx, "p", first.ID, "/workspace/../etc/passwd"); !errors.Is(err, sandbox.ErrInvalidPath) {
		t.Fatalf("path: %v", err)
	}
	if _, err = m.Stop(ctx, "p", first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Start(ctx, "p", first.ID); err != nil {
		t.Fatal(err)
	}
	if file, err := m.ReadFile(ctx, "p", first.ID, "/workspace/data"); err != nil || string(file.Content) != "content" {
		t.Fatalf("restart: %+v %v", file, err)
	}
	if err = m.Delete(ctx, "p", first.ID); err != nil {
		t.Fatal(err)
	}
	if d.VMCount() != 0 {
		t.Fatalf("VM leaked: %d", d.VMCount())
	}
	if _, err = m.Get(ctx, "p", first.ID, false); !errors.Is(err, sandbox.ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}
	if count, err := store.CountSandboxes(ctx, "p"); err != nil || count != 1 {
		t.Fatalf("count: %d %v", count, err)
	}
}

func TestSandboxManagerThreadCompatibility(t *testing.T) {
	m, _, store := newSandboxManagerTest(t, 3)
	ctx := context.Background()
	thread := "550e8400-e29b-41d4-a716-446655440000"
	agent, err := m.EnsureForThread(ctx, "p", thread)
	if err != nil || !strings.HasPrefix(agent.CloudName, "sbx-") {
		t.Fatalf("thread: %+v %v", agent, err)
	}
	// agent 来源的沙盒对 HTTP 一侧是只读的。
	if _, err = m.Exec(ctx, "p", agent.ID, sandbox.ExecInput{Command: "echo no"}); !errors.Is(err, sandbox.ErrBusy) {
		t.Fatalf("agent HTTP exec: %v", err)
	}
	if _, err = m.StartProc(ctx, "p", agent.ID, sandbox.StartSpec{Kind: sandbox.ProcTerminal}); !errors.Is(err, sandbox.ErrBusy) {
		t.Fatalf("agent sandboxes must refuse terminals too: %v", err)
	}
	if _, err = m.ExecForThread(ctx, "p", thread, sandbox.ExecInput{Command: "echo yes"}); err != nil {
		t.Fatal(err)
	}
	if err = m.ReleaseThread(ctx, "p", thread); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetSandboxByThread(ctx, "p", thread); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("thread released: %v", err)
	}
}

func TestSandboxManagerReaper(t *testing.T) {
	m, d, store := newSandboxManagerTest(t, 5)
	ctx := context.Background()
	expired, err := m.Create(ctx, "p", sandbox.CreateInput{Name: "expired", Start: true}, "u")
	if err != nil {
		t.Fatal(err)
	}
	row, err := store.GetSandbox(ctx, "p", expired.ID)
	if err != nil {
		t.Fatal(err)
	}
	row.ExpiresAt = time.Now().Add(-time.Hour)
	if err = store.UpdateSandbox(ctx, row); err != nil {
		t.Fatal(err)
	}
	stale, err := m.Create(ctx, "p", sandbox.CreateInput{Name: "stale-run", Source: "run", Start: true}, "u")
	if err != nil {
		t.Fatal(err)
	}
	// 只把一次性任务创建时间置为过去；常驻沙盒仍应保留、标记为 expired。
	if _, err = store.DB().ExecContext(ctx, `UPDATE sys_sandboxes SET created_at=? WHERE id=?`, time.Now().UTC().Add(-24*time.Hour), stale.ID); err != nil {
		t.Fatal(err)
	}
	check, err := store.GetSandbox(ctx, "p", stale.ID)
	if err != nil {
		t.Fatal(err)
	}
	if check.Source != "run" {
		t.Fatalf("unexpected source: %s", check.Source)
	}
	rows, err := store.ListStaleRunSandboxes(ctx, time.Now().Add(-5*time.Minute), 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("stale selection: %+v %v", rows, err)
	}
	if err = m.ReapOnce(ctx); err != nil {
		t.Fatal(err)
	}
	updated, err := m.Get(ctx, "p", expired.ID, false)
	if err != nil || updated.Status != "expired" {
		t.Fatalf("expire: %+v %v", updated, err)
	}
	if _, err = m.Get(ctx, "p", stale.ID, false); !errors.Is(err, sandbox.ErrNotFound) {
		t.Fatalf("stale run not deleted: %v", err)
	}
	if d.VMCount() != 1 {
		t.Fatalf("expected only persistent VM, got %d", d.VMCount())
	}
}

// transientRemoveDriver 模拟 Cloud 首次删除失败，再次调用成功。
type transientRemoveDriver struct {
	*fake.Driver
	fail bool
}

func (d *transientRemoveDriver) Remove(ctx context.Context, name string) error {
	if d.fail {
		d.fail = false
		return errors.New("temporary Cloud outage")
	}
	return d.Driver.Remove(ctx, name)
}

func TestSandboxManagerRetryFailedCloudRemoval(t *testing.T) {
	_, base, store := newSandboxManagerTest(t, 2)
	cfg := config.SandboxConfig{Enabled: true, Backend: "fake", Image: "python:3.12-slim", CPUs: 1,
		MemoryMiB: 256, Network: "none", Workdir: "/workspace", IdleTimeout: time.Minute,
		MaxDuration: 30 * time.Minute, ExecTimeout: time.Second, ExecTimeoutMax: time.Minute,
		MaxFileBytes: 8, MaxOutputBytes: 4, MaxPerProject: 2}
	driver := &transientRemoveDriver{Driver: base, fail: true}
	m := sandbox.NewManager(cfg, sandboxStoreAdapter{s: store}, driver)
	ctx := context.Background()
	r, err := m.Create(ctx, "p", sandbox.CreateInput{Name: "retry", Start: true}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Delete(ctx, "p", r.ID); !errors.Is(err, vm.ErrBackend) {
		t.Fatalf("first delete: %v", err)
	}
	failed, err := store.ListFailedSandboxRemovals(ctx, 10)
	if err != nil || len(failed) != 1 || failed[0].ID != r.ID {
		t.Fatalf("failed list: %+v %v", failed, err)
	}
	if err = m.ReapOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Get(ctx, "p", r.ID, false); !errors.Is(err, sandbox.ErrNotFound) {
		t.Fatalf("retry failed: %v", err)
	}
	if base.VMCount() != 0 {
		t.Fatalf("VM leaked: %d", base.VMCount())
	}
}

// blockingExecDriver 用于断言同一沙盒的操作按 VM 串行。
type blockingExecDriver struct {
	*fake.Driver
	entered chan struct{}
	release chan struct{}
}

func (d *blockingExecDriver) Exec(ctx context.Context, target vm.Target, req vm.ExecSpec) (vm.ExecResult, error) {
	close(d.entered)
	select {
	case <-d.release:
		return d.Driver.Exec(ctx, target, req)
	case <-ctx.Done():
		return vm.ExecResult{}, ctx.Err()
	}
}

func TestSandboxManagerSerializesSameVMAndCancelsWaiter(t *testing.T) {
	_, base, store := newSandboxManagerTest(t, 2)
	cfg := config.SandboxConfig{Enabled: true, Backend: "fake", Image: "python:3.12-slim", CPUs: 1, MemoryMiB: 256,
		Network: "none", Workdir: "/workspace", IdleTimeout: time.Minute, MaxDuration: 30 * time.Minute,
		ExecTimeout: time.Second, ExecTimeoutMax: time.Minute, MaxPerProject: 2, MaxFileBytes: 8, MaxOutputBytes: 4}
	driver := &blockingExecDriver{Driver: base, entered: make(chan struct{}), release: make(chan struct{})}
	m := sandbox.NewManager(cfg, sandboxStoreAdapter{s: store}, driver)
	ctx := context.Background()
	r, err := m.Create(ctx, "p", sandbox.CreateInput{Name: "locked"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { _, err := m.Exec(ctx, "p", r.ID, sandbox.ExecInput{Command: "echo a"}); finished <- err }()
	select {
	case <-driver.entered:
	case <-time.After(time.Second):
		t.Fatal("exec never acquired VM lock")
	}
	waitCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err := m.WriteFile(waitCtx, "p", r.ID, "/workspace/x", []byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting op must cancel: %v", err)
	}
	close(driver.release)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("exec not released")
	}
	if err := m.WriteFile(ctx, "p", r.ID, "/workspace/x", []byte("x")); err != nil {
		t.Fatalf("lock not released: %v", err)
	}
}
