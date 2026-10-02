package lease

import (
	"sync"
)

// Gate 按 key 串行化首次租约获取（perf §1.5）。
//
// 背景：同一进程内并发首写同一 DB 时，每个请求都会独立调用
// Manager.Acquire，互相把对方当作「他人持租」触发 TTL+Grace 的
// 观察等待，最终其中一个请求得到 ErrLeaseHeld（45s + 500）。
// Gate 保证同一 dbID 同时只有一个 Acquire 在执行，后来的请求
// 复用第一个的结果（成功后由 ValidFor/Has 快速路径返回）。
type Gate struct {
	mu       sync.Mutex
	inflight map[string]*gateCall
}

type gateCall struct {
	done chan struct{}
	err  error
}

// NewGate 构造写门。
func NewGate() *Gate {
	return &Gate{inflight: map[string]*gateCall{}}
}

// Do 执行 fn：同 key 并发调用只有一个真正执行，其余等待并复用其结果。
// fn 内部的二次检查（ValidFor/Has）保证成功获取后后续调用走缓存路径。
// 若首个调用仍在执行，等待者返回 ErrAcquiring（调用方可立即回 503+Retry-After，
// 不再挂等首次获取完成的整个 TTL+Grace 窗口）。
func (g *Gate) Do(key string, fn func() error) error {
	if g == nil {
		return fn()
	}
	g.mu.Lock()
	if g.inflight == nil {
		g.inflight = map[string]*gateCall{}
	}
	if call, ok := g.inflight[key]; ok {
		g.mu.Unlock()
		<-call.done
		if call.err != nil {
			return call.err
		}
		// 第一个调用成功：调用方需自行复查快速路径（ValidFor）。
		return nil
	}
	call := &gateCall{done: make(chan struct{})}
	g.inflight[key] = call
	g.mu.Unlock()

	call.err = fn()
	close(call.done)

	g.mu.Lock()
	delete(g.inflight, key)
	g.mu.Unlock()
	return call.err
}

// TryDo 与 Do 类似，但同 key 已有在途调用时**不等待**，直接返回 ErrAcquiring。
// 写路径的首租获取耗时可长达 TTL+Grace（观察式判活），排队等待会放大尾延迟；
// 首个请求完成获取后，后续请求经 ValidFor 快速路径放行。
func (g *Gate) TryDo(key string, fn func() error) error {
	if g == nil {
		return fn()
	}
	g.mu.Lock()
	if g.inflight == nil {
		g.inflight = map[string]*gateCall{}
	}
	if _, ok := g.inflight[key]; ok {
		g.mu.Unlock()
		return ErrAcquiring
	}
	call := &gateCall{done: make(chan struct{})}
	g.inflight[key] = call
	g.mu.Unlock()

	call.err = fn()
	close(call.done)

	g.mu.Lock()
	delete(g.inflight, key)
	g.mu.Unlock()
	return call.err
}

// Clear 失租回调后清除记录：允许后续请求重新竞争（Gate 本身无状态残留，
// 这里仅为语义明确与潜在观测点）。
func (g *Gate) Clear(key string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	delete(g.inflight, key)
	g.mu.Unlock()
}
