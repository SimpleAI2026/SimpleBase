// 文件说明：callgate.go — 工具调用时序闸门（ToolCallGate）
//
// 增量协议下，模型的 tool_call 参数是分片到达的：工具节点可能在 Runtime 登记完
// 这一轮调用之前就开始执行。宿主工具用 Wait 等待「该 call_id 已被 Runtime 登记」，
// Runtime 经 RunPreparation.OnToolCall 调 Release 放行。
//
// 时序保证：Runtime 先发出 tool_call 事件，再经 OnToolCall 调 Release，因此工具被放行时
// 订阅方已经看到对应的 tool_call，其后的 confirmation_required / tool_start 不会早于它。

package cloudagent

import (
	"sync"
	"time"
)

// ToolCallGate 按 (runID, callID) 串行化「tool_call 已登记」与「工具开始执行」两个时点。
// 零值可用，并发安全。
//
// 登记项同时记录所属 run：DropRun 只回收该 run 的登记，不能碰其他并发 run
// 正在 Wait 的 channel（否则 Release 会为同一 call_id 新建 channel，等待者
// 手里的旧 channel 永远收不到信号，只能等满超时）。
type ToolCallGate struct {
	mu    sync.Mutex
	slots map[gateKey]*gateSlot
}

type gateKey struct {
	runID  string
	callID string
}

type gateSlot struct {
	ch       chan struct{}
	released bool
}

// slot 返回（必要时创建）key 对应的登记项。调用方必须持有 g.mu。
func (g *ToolCallGate) slot(k gateKey) *gateSlot {
	if g.slots == nil {
		g.slots = map[gateKey]*gateSlot{}
	}
	s, ok := g.slots[k]
	if !ok {
		s = &gateSlot{ch: make(chan struct{})}
		g.slots[k] = s
	}
	return s
}

// Wait 阻塞到该 call_id 被 Release 或超时。无论哪种结束，都会删除登记，防止残留累积。
// g 为 nil 或 callID 为空时立即返回。
func (g *ToolCallGate) Wait(runID, callID string, d time.Duration) {
	if g == nil || callID == "" {
		return
	}
	k := gateKey{runID, callID}
	g.mu.Lock()
	ch := g.slot(k).ch
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.slots, k)
		g.mu.Unlock()
	}()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
	}
}

// Release 放行等待该 call_id 的工具；先于 Wait 调用也有效（登记项会保留到 Wait 或 DropRun）。
// 重复、并发的 Release 是安全的。
func (g *ToolCallGate) Release(runID, callID string) {
	if g == nil || callID == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.slot(gateKey{runID, callID})
	if !s.released {
		s.released = true
		close(s.ch)
	}
}

// DropRun 在 run 结束时回收该 run 残留的登记：超时与放行路径的删除已由 Wait 保证，
// 这里兜底回收「已 Release 但从未 Wait」的项（例如 tool_call 发出后工具被取消）。
// 只删本 run 的项。
func (g *ToolCallGate) DropRun(runID string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for k := range g.slots {
		if k.runID == runID {
			delete(g.slots, k)
		}
	}
}

// size 返回当前登记数（仅测试）。
func (g *ToolCallGate) size() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.slots)
}
