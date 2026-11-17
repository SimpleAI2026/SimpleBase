package sandbox

// pause_metrics_test.go：Pause / Resume / Metrics 与空闲冻结的语义。
//
//   1. 驱动支持真冻结时，pause 只改状态、不动工作区，resume 回到 running；
//   2. 驱动不支持时退回 Stop（保持旧行为），而不是报错；
//   3. paused 的沙盒不会被 Get 的驱动刷新覆盖；
//   4. 驱动不提供指标时 Metrics 返回 vm.ErrNotSupported；
//   5. 空闲冻结只挑 running 且没有活跃进程的沙盒。

import (
	"context"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm/fake"
)

// pausingDriver 在 fake 之上加一个可切换的 Pauser / Freezer。
type pausingDriver struct {
	*fake.Driver
	paused  int
	resumed int
	canFree bool
}

func (d *pausingDriver) Pause(context.Context, string) error {
	d.paused++
	return nil
}

func (d *pausingDriver) Resume(context.Context, vm.Target) error {
	d.resumed++
	return nil
}

func (d *pausingDriver) CanFreeze() bool { return d.canFree }

// meteredDriver 在 pausingDriver 之上加 Meter。
type meteredDriver struct{ *pausingDriver }

func (d *meteredDriver) Metrics(context.Context, string) (vm.Metrics, error) {
	return vm.Metrics{MemoryBytes: 4096, Pids: 2, Backend: "cgroup"}, nil
}

func pausingManager(t *testing.T, driver vm.Driver) (*Manager, *memStore, *pausingDriver) {
	t.Helper()
	m, store := procManager(t)
	m.driver = driver
	pd, _ := driver.(*pausingDriver)
	if pd == nil {
		if md, ok := driver.(*meteredDriver); ok {
			pd = md.pausingDriver
		}
	}
	return m, store, pd
}

func newRunningSandbox(t *testing.T, m *Manager) Sandbox {
	t.Helper()
	sb, err := m.Create(context.Background(), "p1", CreateInput{Name: "sb", Start: true}, "u")
	if err != nil {
		t.Fatal(err)
	}
	return sb
}

// 真冻结：pause 调驱动的 Pause，状态变 paused；resume 调 Resume，回到 running。
func TestPauseFreezesAndResumeThaws(t *testing.T) {
	m, _, d := pausingManager(t, &pausingDriver{Driver: fake.New(), canFree: true})
	ctx := context.Background()
	sb := newRunningSandbox(t, m)
	if !m.CanPause() {
		t.Fatal("CanPause must be true with a freezer-capable driver")
	}

	got, err := m.Pause(ctx, "p1", sb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "paused" || d.paused != 1 {
		t.Fatalf("status=%q paused=%d", got.Status, d.paused)
	}

	// paused 不会被 Get 的驱动刷新覆盖：工作区还在，驱动看起来仍是 running。
	if got, err = m.Get(ctx, "p1", sb.ID, true); err != nil {
		t.Fatal(err)
	}
	if got.Status != "paused" {
		t.Fatalf("Get must not clear paused, got %q", got.Status)
	}

	got, err = m.Resume(ctx, "p1", sb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || d.resumed != 1 {
		t.Fatalf("status=%q resumed=%d", got.Status, d.resumed)
	}
}

// 已有 paused 时再次 pause 报 ErrAlreadyPaused（handler 映射 409）。
func TestPauseTwiceIsConflict(t *testing.T) {
	m, _, _ := pausingManager(t, &pausingDriver{Driver: fake.New(), canFree: true})
	ctx := context.Background()
	sb := newRunningSandbox(t, m)
	if _, err := m.Pause(ctx, "p1", sb.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Pause(ctx, "p1", sb.ID); err != ErrAlreadyPaused {
		t.Fatalf("want ErrAlreadyPaused, got %v", err)
	}
}

// 驱动实现 Pauser 但当前环境冻结不了：CanPause 为假，pause 退回 Stop。
func TestPauseFallsBackToStopWhenFreezeUnavailable(t *testing.T) {
	m, _, d := pausingManager(t, &pausingDriver{Driver: fake.New()})
	ctx := context.Background()
	sb := newRunningSandbox(t, m)
	if m.CanPause() {
		t.Fatal("CanPause must be false when the driver cannot freeze")
	}
	got, err := m.Pause(ctx, "p1", sb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopped" || d.paused != 0 {
		t.Fatalf("status=%q paused=%d (must fall back to Stop)", got.Status, d.paused)
	}
}

// 驱动完全不是 Pauser：pause 同样退回 Stop。
func TestPauseFallsBackWithoutPauser(t *testing.T) {
	m, _ := procManager(t) // driver 是纯 fake
	ctx := context.Background()
	sb := newRunningSandbox(t, m)
	if m.CanPause() {
		t.Fatal("fake driver must not report pause")
	}
	got, err := m.Pause(ctx, "p1", sb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopped" {
		t.Fatalf("status = %q", got.Status)
	}
}

// 驱动没有指标：CanMetrics 为假，Metrics 返回 ErrNotSupported（handler 映射 501）。
func TestMetricsUnsupported(t *testing.T) {
	m, _, _ := pausingManager(t, &pausingDriver{Driver: fake.New(), canFree: true})
	ctx := context.Background()
	sb := newRunningSandbox(t, m)
	if m.CanMetrics() {
		t.Fatal("CanMetrics must be false without a meter")
	}
	if _, err := m.Metrics(ctx, "p1", sb.ID); err == nil {
		t.Fatal("want ErrNotSupported")
	}
}

// 驱动有指标：Metrics 透传，CanMetrics 为真。
func TestMetricsPassthrough(t *testing.T) {
	m, _, _ := pausingManager(t, &meteredDriver{&pausingDriver{Driver: fake.New(), canFree: true}})
	ctx := context.Background()
	sb := newRunningSandbox(t, m)
	if !m.CanMetrics() {
		t.Fatal("CanMetrics must be true with a meter")
	}
	got, err := m.Metrics(ctx, "p1", sb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.MemoryBytes != 4096 || got.Backend != "cgroup" {
		t.Fatalf("metrics = %+v", got)
	}
}

// 空闲冻结：running 且最后活动早于阈值 → pause；已有 paused 的跳过（不是 running）。
func TestFreezeIdleFreezesOnlyIdle(t *testing.T) {
	m, store, d := pausingManager(t, &pausingDriver{Driver: fake.New(), canFree: true})
	m.cfg.FreezeAfter = time.Minute
	ctx := context.Background()
	sb := newRunningSandbox(t, m)

	// 刚活动过：不冻结。
	store.idle = nil
	m.freezeIdle(ctx, m.now())
	if d.paused != 0 {
		t.Fatalf("recently active sandbox must not be frozen")
	}

	// 超过阈值：冻结。
	store.idle = []Sandbox{store.rows[sb.ID]}
	m.freezeIdle(ctx, m.now())
	if d.paused != 1 {
		t.Fatalf("idle sandbox must be frozen, paused=%d", d.paused)
	}
	if got := store.rows[sb.ID]; got.Status != "paused" {
		t.Fatalf("status = %q", got.Status)
	}

	// 已 paused：不再冻结第二次。
	m.freezeIdle(ctx, m.now())
	if d.paused != 1 {
		t.Fatalf("paused sandbox must not be frozen again, paused=%d", d.paused)
	}
}

// 驱动不能冻结时空闲冻结是空操作（不把沙盒 stop 掉）。
func TestFreezeIdleNoopWithoutFreeze(t *testing.T) {
	m, store, d := pausingManager(t, &pausingDriver{Driver: fake.New()})
	m.cfg.FreezeAfter = time.Minute
	sb := newRunningSandbox(t, m)
	store.idle = []Sandbox{store.rows[sb.ID]}
	m.freezeIdle(context.Background(), m.now())
	if d.paused != 0 {
		t.Fatalf("paused=%d", d.paused)
	}
}
