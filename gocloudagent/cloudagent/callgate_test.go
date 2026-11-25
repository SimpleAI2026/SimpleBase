// 文件说明：callgate_test.go — gocloudagent/cloudagent 的单元测试（TestToolCallGateWaitCleanupOnTimeout、TestToolCallGateWaitCleanupOnRelease 等）
//
// 用法
//
//	go test ./gocloudagent/cloudagent -run 'TestToolCallGateWaitCleanupOnTimeout|TestToolCallGateWaitCleanupOnRelease|TestToolCallGateReleaseBeforeWait|TestToolCallGateDropRun'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/callgate_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] callgate.go, confirmation.go, confirmation_test.go, errors.go, errors_test.go 等 32 个
//
// 主要符号
//
//	TestToolCallGateWaitCleanupOnTimeout(t *testing.T)
//	TestToolCallGateWaitCleanupOnRelease(t *testing.T)
//	TestToolCallGateReleaseBeforeWait(t *testing.T)
//	TestToolCallGateDropRun(t *testing.T)
//	TestToolCallGateDropRunDoesNotStrandOtherRunWaiter(t *testing.T)
//	TestToolCallGateKeysByRun(t *testing.T)
//	TestToolCallGateConcurrentReleaseIsSafe(t *testing.T)
//	TestToolCallGateNilAndEmptyCallIDAreNoOps(t *testing.T)

package cloudagent

import (
	"sync"
	"testing"
	"time"
)

// gate 在 Wait 超时后必须自清理（结束 delete，防残留累积）。
func TestToolCallGateWaitCleanupOnTimeout(t *testing.T) {
	g := &ToolCallGate{}
	g.Wait("r1", "c1", 10*time.Millisecond)
	if n := g.size(); n != 0 {
		t.Fatalf("Wait 超时后登记残留 %d 条", n)
	}
}

func TestToolCallGateWaitCleanupOnRelease(t *testing.T) {
	g := &ToolCallGate{}
	go func() {
		time.Sleep(5 * time.Millisecond)
		g.Release("r1", "c2")
	}()
	g.Wait("r1", "c2", time.Second)
	if n := g.size(); n != 0 {
		t.Fatalf("Wait release 后登记残留 %d 条", n)
	}
}

// Release 先于 Wait：Wait 立即返回，不等超时。
func TestToolCallGateReleaseBeforeWait(t *testing.T) {
	g := &ToolCallGate{}
	g.Release("r", "c")
	start := time.Now()
	g.Wait("r", "c", 2*time.Second)
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("release-before-wait 阻塞了 %v", d)
	}
}

// DropRun 只清本 run 的登记，其他 run 的登记必须保留。
func TestToolCallGateDropRun(t *testing.T) {
	g := &ToolCallGate{}
	g.Release("run-1", "c3")
	g.Release("run-2", "c4")
	g.DropRun("run-1")
	if n := g.size(); n != 1 {
		t.Fatalf("DropRun(run-1) 后应只剩 run-2 的 1 条登记，实际 %d", n)
	}
	g.DropRun("run-2")
	if n := g.size(); n != 0 {
		t.Fatalf("DropRun 后仍有 %d 条登记", n)
	}
}

// run B 收尾的 DropRun 不能让 run A 正在 Wait 的 call 错过 Release。
func TestToolCallGateDropRunDoesNotStrandOtherRunWaiter(t *testing.T) {
	g := &ToolCallGate{}
	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		g.Wait("runA", "callA", 2*time.Second)
		done <- time.Since(start)
	}()
	time.Sleep(50 * time.Millisecond)
	g.DropRun("runB")
	g.Release("runA", "callA")
	select {
	case d := <-done:
		if d > 500*time.Millisecond {
			t.Fatalf("run A 的等待被 run B 的 DropRun 拖到 %v", d)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Wait 未返回")
	}
}

// 不同 run 的相同 callID 互不串扰。
func TestToolCallGateKeysByRun(t *testing.T) {
	g := &ToolCallGate{}
	g.Release("runA", "same")
	start := time.Now()
	g.Wait("runB", "same", 80*time.Millisecond)
	if d := time.Since(start); d < 60*time.Millisecond {
		t.Fatalf("runB 不应被 runA 的 Release 放行（等待仅 %v）", d)
	}
}

// BUG-03：同 key 并发 Release 不得 double close（-race 下也不能 panic）。
func TestToolCallGateConcurrentReleaseIsSafe(t *testing.T) {
	g := &ToolCallGate{}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			g.Release("r", "c")
		}()
	}
	close(start)
	wg.Wait()
	g.Wait("r", "c", time.Second)
}

func TestToolCallGateNilAndEmptyCallIDAreNoOps(t *testing.T) {
	var nilGate *ToolCallGate
	nilGate.Wait("r", "c", time.Second)
	nilGate.Release("r", "c")
	nilGate.DropRun("r")
	g := &ToolCallGate{}
	g.Wait("r", "", time.Second)
	g.Release("r", "")
	if n := g.size(); n != 0 {
		t.Fatalf("空 callID 不应登记，实际 %d", n)
	}
}
