// 文件说明：run_events_test.go — internal/agentrun 的单元测试（TestRunEventHub_ReplayMissingRunReportsGap、TestRunEventHub_ReplayAfterDropReportsGap 等）
//
// 用法
//
//	go test ./internal/agentrun -run 'TestRunEventHub_ReplayMissingRunReportsGap|TestRunEventHub_ReplayAfterDropReportsGap|TestRunEventHub_ReplayBoundaries|TestConfirmHub_PendingReleased'
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/run_events_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] confirm.go, cron.go, errors.go, group_engine.go, group_runner.go 等 17 个
//
// 主要符号
//
//	TestRunEventHub_ReplayMissingRunReportsGap(t *testing.T)
//	TestRunEventHub_ReplayAfterDropReportsGap(t *testing.T)
//	TestRunEventHub_ReplayBoundaries(t *testing.T)
//	TestConfirmHub_PendingReleased(t *testing.T)
//	TestConfirmHub_RejectedResolveKeepsPending(t *testing.T)
//	TestConfirmHub_DropRunClearsPending(t *testing.T)
//	(h *ConfirmHub) pendingCount() int
//	TestRunEventHubGapAndConfirmTimeout(t *testing.T)

package agentrun

import (
	"context"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
)

// planv6.0 v2 §2.4：run_events 的 after 边界与 gap 语义回归。
// 修复前 b == nil（缓存已 drop / 被总量淘汰）返回 gap=false，客户端拿到 200 空流。

func TestRunEventHub_ReplayMissingRunReportsGap(t *testing.T) {
	h := newRunEventHub()
	// 从未注册过的 run：缓存起点不可达，必须走 stream_gap 而不是空数组。
	frames, gap, live, sub := h.replay("missing", 0)
	if !gap || live || sub != nil || frames != nil {
		t.Fatalf("gap=%v live=%v frames=%v sub=%v", gap, live, frames, sub)
	}
}

func TestRunEventHub_ReplayAfterDropReportsGap(t *testing.T) {
	h := newRunEventHub()
	h.append("run", cloudagent.Event{Type: "run", RunID: "run"})
	h.append("run", cloudagent.Event{Type: "end", RunID: "run"})
	// 模拟 2 分钟后的 drop（测试直接删除，等价于 AfterFunc 触发）。
	h.mu.Lock()
	delete(h.runs, "run")
	h.mu.Unlock()
	if _, gap, _, _ := h.replay("run", 1); !gap {
		t.Fatal("dropped run must report gap")
	}
}

func TestRunEventHub_ReplayBoundaries(t *testing.T) {
	h := newRunEventHub()
	h.append("run", cloudagent.Event{Type: "run", RunID: "run"})   // id=1
	h.append("run", cloudagent.Event{Type: "token", RunID: "run"}) // id=2
	h.append("run", cloudagent.Event{Type: "end", RunID: "run"})   // id=3
	cases := []struct {
		after int64
		want  int
	}{
		{0, 3}, // 全量
		{1, 2},
		{2, 1},
		{3, 0},
	}
	for _, c := range cases {
		frames, gap, live, _ := h.replay("run", c.after)
		if gap || live || len(frames) != c.want {
			t.Fatalf("after=%d frames=%d gap=%v live=%v", c.after, len(frames), gap, live)
		}
	}
}

// BUG-B4：确认项必须回收，不能每次破坏性确认都留下永久条目。
func TestConfirmHub_PendingReleased(t *testing.T) {
	h := NewConfirmHub()
	// 超时路径。
	ok, err := h.Wait(context.Background(), "run-1", "call-1", "u1", 20*time.Millisecond)
	if ok || err != nil {
		t.Fatalf("timeout wait approve=%v err=%v", ok, err)
	}
	if n := h.pendingCount(); n != 0 {
		t.Fatalf("pending=%d after timeout, want 0", n)
	}
	// 取消路径。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Wait(ctx, "run-1", "call-2", "u1", time.Second); err == nil {
		t.Fatal("expected ctx error")
	}
	if n := h.pendingCount(); n != 0 {
		t.Fatalf("pending=%d after cancel, want 0", n)
	}
	// 放行路径。
	go func() {
		time.Sleep(10 * time.Millisecond)
		h.Resolve("run-2", "call-3", true, "u1")
	}()
	if ok, err := h.Wait(context.Background(), "run-2", "call-3", "u1", time.Second); !ok || err != nil {
		t.Fatalf("approve=%v err=%v", ok, err)
	}
	if n := h.pendingCount(); n != 0 {
		t.Fatalf("pending=%d after resolve, want 0", n)
	}
}

// 非发起人的确认尝试不得删除 pending，否则发起人随后确认会拿到 404。
func TestConfirmHub_RejectedResolveKeepsPending(t *testing.T) {
	h := NewConfirmHub()
	go func() {
		time.Sleep(20 * time.Millisecond)
		// 非发起人：拒绝，且必须保留条目。
		if h.Resolve("run-3", "call-4", true, "intruder") {
			t.Error("intruder must not resolve")
		}
		if n := h.pendingCount(); n != 1 {
			t.Errorf("pending=%d after rejected resolve, want 1", n)
		}
		// 发起人：放行。
		if !h.Resolve("run-3", "call-4", true, "u1") {
			t.Error("initiator must resolve")
		}
	}()
	if ok, err := h.Wait(context.Background(), "run-3", "call-4", "u1", time.Second); !ok || err != nil {
		t.Fatalf("approve=%v err=%v", ok, err)
	}
}

// DropRun 清理该 run 的全部残留确认项。
func TestConfirmHub_DropRunClearsPending(t *testing.T) {
	h := NewConfirmHub()
	h.Prepare("run-4", "call-5", "u1")
	h.Prepare("run-4", "call-6", "u1")
	h.Prepare("run-5", "call-7", "u1")
	h.DropRun("run-4")
	if n := h.pendingCount(); n != 1 {
		t.Fatalf("pending=%d after DropRun, want 1 (other run kept)", n)
	}
}

// pendingCount 返回当前登记数（测试与诊断用）。
func (h *ConfirmHub) pendingCount() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.pending)
}

func TestRunEventHubGapAndConfirmTimeout(t *testing.T) {
	hub := newRunEventHub()
	// 容量 2000。丢掉两条之后，客户端只看到 id=1 时中间的 id=2 已经不在环里。
	for i := 0; i < 2002; i++ {
		hub.append("run", cloudagent.Event{Type: "token", Content: "x"})
	}
	if _, gap, _, _ := hub.replay("run", 1); !gap {
		t.Fatal("expected stream gap")
	}
	confirms := NewConfirmHub()
	ok, err := confirms.Wait(context.Background(), "run", "call", "u1", 30*time.Millisecond)
	if ok || err != nil {
		t.Fatalf("timeout approve=%v err=%v", ok, err)
	}
}
