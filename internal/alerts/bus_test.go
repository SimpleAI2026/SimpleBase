// 文件说明：bus_test.go — 告警总线与磁盘水位测试（§8.3.4/§8.3.5）
package alerts

import (
	"sync"
	"testing"
	"time"
)

func TestBusEmitSubscribe(t *testing.T) {
	bus := NewBus(nil)
	var mu sync.Mutex
	var got []Signal
	cancel := bus.Subscribe(func(s Signal) {
		mu.Lock()
		got = append(got, s)
		mu.Unlock()
	})
	bus.Emit(Signal{Kind: "backup.failed", Severity: SeverityCritical, Detail: "boom"})
	cancel()
	bus.Emit(Signal{Kind: "disk.critical"})
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].Kind != "backup.failed" {
		t.Fatalf("subscriber got %+v", got)
	}
	if got[0].Severity != SeverityCritical {
		t.Fatal("severity must be preserved")
	}
	if got[0].At.IsZero() {
		t.Fatal("At must default to now")
	}
}

func TestBusNormalizeDefaults(t *testing.T) {
	bus := NewBus(nil)
	var got Signal
	done := make(chan struct{})
	bus.Subscribe(func(s Signal) { got = s; close(done) })
	bus.Emit(Signal{Kind: "x"}) // severity 空 → warning
	<-done
	if got.Severity != SeverityWarning {
		t.Fatalf("default severity = %s", got.Severity)
	}
	// 非法值同样归一化为 warning。
	bus2 := NewBus(nil)
	var got2 Signal
	done2 := make(chan struct{})
	bus2.Subscribe(func(s Signal) { got2 = s; close(done2) })
	bus2.Emit(Signal{Kind: "x", Severity: "severe!!"})
	<-done2
	if got2.Severity != SeverityWarning {
		t.Fatalf("invalid severity normalized to %s", got2.Severity)
	}
}

func TestBusRecentRing(t *testing.T) {
	bus := NewBus(nil)
	for i := 0; i < 300; i++ {
		bus.Emit(Signal{Kind: itoaTest(i)})
	}
	recent := bus.Recent(10)
	if len(recent) != 10 {
		t.Fatalf("recent len=%d", len(recent))
	}
	// 最新在前。
	if recent[0].Kind != "299" {
		t.Fatalf("newest first, got %s", recent[0].Kind)
	}
	all := bus.Recent(300)
	if len(all) != 256 {
		t.Fatalf("ring keeps 256, got %d", len(all))
	}
	if bus.Recent(0) != nil {
		t.Fatal("Recent(0) must be nil")
	}
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

func TestBusSubscriberPanicIsolated(t *testing.T) {
	bus := NewBus(nil)
	done := make(chan struct{})
	bus.Subscribe(func(s Signal) { panic("boom") })
	bus.Subscribe(func(s Signal) { _ = s; close(done) })
	bus.Emit(Signal{Kind: "ok"})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("second subscriber blocked by first one's panic")
	}
}

func TestDiskFreeRatio(t *testing.T) {
	// 用当前工作目录（测试运行时必然存在）。
	ratio, err := DiskFreeRatio(".")
	if err != nil {
		t.Skipf("statfs unavailable: %v", err)
	}
	if ratio < 0 || ratio > 1 {
		t.Fatalf("ratio out of range: %f", ratio)
	}
	if _, err := DiskFreeRatio("/nonexistent-path-xyz"); err == nil {
		t.Fatal("nonexistent path must error")
	}
}
