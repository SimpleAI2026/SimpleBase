// 文件说明：callgate.go — 确认流程的串行闸门（callGate）

package agentbridge

import (
	"sync"
	"time"
)

// callGate 按 call_id 串行化「tool_call 事件已发出」与「工具开始执行」两个时点。
//
// 登记项同时记录所属 run：dropRun 只回收该 run 的登记，不能碰其他并发 run
// 正在 wait 的 channel（否则 release 会为同一 call_id 新建 channel，等待者
// 手里的旧 channel 永远收不到信号，只能等满超时）。
type callGate struct {
	mu    sync.Mutex
	slots map[gateKey]chan struct{}
}

type gateKey struct {
	runID  string
	callID string
}

func (g *callGate) slot(k gateKey) chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.slots == nil {
		g.slots = map[gateKey]chan struct{}{}
	}
	ch, ok := g.slots[k]
	if !ok {
		ch = make(chan struct{})
		g.slots[k] = ch
	}
	return ch
}

// wait 阻塞到该 call_id 的 tool_call 收束或超时（planv6.0 §3.4）。
// 无论 release 还是超时，结束后删除登记，防止残留累积。
func (g *callGate) wait(runID, callID string, d time.Duration) {
	if g == nil || callID == "" {
		return
	}
	k := gateKey{runID, callID}
	ch := g.slot(k)
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

func (g *callGate) release(runID, callID string) {
	if g == nil || callID == "" {
		return
	}
	ch := g.slot(gateKey{runID, callID})
	select {
	case <-ch:
	default:
		close(ch)
	}
}

// dropRun 在 run 结束时清理该 run 残留的登记（planv6.0 §3.4）。
// 超时与 release 路径的删除已由 wait 的 defer 保证；这里兜底回收「已 release
// 但从未 wait」的项（例如 tool_call 发出后工具被取消）。只删本 run 的项。
func (g *callGate) dropRun(runID string) {
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

func (r *RuntimeProvider) ensureGate() *callGate {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gate == nil {
		r.gate = &callGate{}
	}
	return r.gate
}
