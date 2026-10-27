// 文件说明：singleflight.go — 按 key 的单飞：同 key 并发调用只有一个真正执行
//
// 用法
//
//	(g *Group[T]) Do(key string, fn func() (T, error)) (T, error)  // Do 执行 fn：同 key 并发调用只有一个真正执行，其余等待并复用其 (value, error)
//	(g *Group[T]) TryDo(key string, fn func() (T, error)) (T, error)  // TryDo 与 Do 类似，但同 key 已有在途调用时**不等待**，返回 ErrInFlight
//
// 调用架构
//
//	[上游] godatabase/lease
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/internal/singleflight/singleflight.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//
// 主要符号
//
//	var ErrInFlight
//	type Group[T any] struct
//	(g *Group[T]) Do(key string, fn func() (T, error)) (T, error)
//	(g *Group[T]) TryDo(key string, fn func() (T, error)) (T, error)
//	(g *Group[T]) Forget(key string)
//	(g *Group[T]) InFlight(key string) bool
//	... 另有 4 个顶层符号

package singleflight

import (
	"errors"
	"sync"
)

// ErrInFlight 表示 TryDo 发现同 key 已有在途调用。
var ErrInFlight = errors.New("singleflight: call in flight")

// errPanicked 是等待者在 leader panic 时拿到的错误（leader 自身仍会 panic）。
var errPanicked = errors.New("singleflight: leader call panicked")

type call[T any] struct {
	done chan struct{}
	val  T
	err  error
}

// Group 是按 key 的单飞组。零值可用；nil *Group 退化为直接执行。
type Group[T any] struct {
	mu       sync.Mutex
	inflight map[string]*call[T]
}

// Do 执行 fn：同 key 并发调用只有一个真正执行，其余等待并复用其 (value, error)。
func (g *Group[T]) Do(key string, fn func() (T, error)) (T, error) {
	if g == nil {
		return fn()
	}
	g.mu.Lock()
	if g.inflight == nil {
		g.inflight = map[string]*call[T]{}
	}
	if c, ok := g.inflight[key]; ok {
		g.mu.Unlock()
		<-c.done
		return c.val, c.err
	}
	c := g.start(key)
	g.mu.Unlock()
	return g.run(key, c, fn)
}

// TryDo 与 Do 类似，但同 key 已有在途调用时**不等待**，返回 ErrInFlight。
func (g *Group[T]) TryDo(key string, fn func() (T, error)) (T, error) {
	if g == nil {
		return fn()
	}
	g.mu.Lock()
	if g.inflight == nil {
		g.inflight = map[string]*call[T]{}
	}
	if _, ok := g.inflight[key]; ok {
		g.mu.Unlock()
		var zero T
		return zero, ErrInFlight
	}
	c := g.start(key)
	g.mu.Unlock()
	return g.run(key, c, fn)
}

// Forget 丢弃 key 的在途记录：之后的调用不再等待/拒绝，而是成为新的 leader。
// 已在等待的调用仍会收到原 leader 的结果。
func (g *Group[T]) Forget(key string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	delete(g.inflight, key)
	g.mu.Unlock()
}

// InFlight 报告 key 当前是否有在途调用（主要供测试与观测）。
func (g *Group[T]) InFlight(key string) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.inflight[key]
	return ok
}

// start 登记 leader。调用方须持有 g.mu。
func (g *Group[T]) start(key string) *call[T] {
	c := &call[T]{done: make(chan struct{})}
	g.inflight[key] = c
	return c
}

// run 执行 fn 并在退出（含 panic）时唤醒等待者、清理 key。
func (g *Group[T]) run(key string, c *call[T], fn func() (T, error)) (T, error) {
	finished := false
	defer func() {
		if !finished {
			// fn panic：给等待者一个确定的错误，随后 panic 继续向上传播。
			c.err = errPanicked
		}
		close(c.done)
		g.mu.Lock()
		// Forget 之后 key 可能已属于新的 leader，只清理自己的记录。
		if g.inflight[key] == c {
			delete(g.inflight, key)
		}
		g.mu.Unlock()
	}()
	c.val, c.err = fn()
	finished = true
	return c.val, c.err
}
