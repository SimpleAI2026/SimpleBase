// 文件说明：gate.go — godatabase/lease 中 Gate、NewGate 等符号的实现
//
// 用法
//
//	NewGate() *Gate
//	(g *Gate) Do(key string, fn func() error) error  // Do 执行 fn：同 key 并发调用只有一个真正执行，其余等待并复用其结果
//	(g *Gate) TryDo(key string, fn func() error) error  // TryDo 与 Do 类似，但同 key 已有在途调用时**不等待**，直接返回 ErrAcquiring
//	同包调用：Gate → lease.go；Do() → lease.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/lease/gate.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/internal/singleflight
//	[同包] lease.go
//	[关键引用] Gate ← lease.go
//
// 主要符号
//
//	type Gate struct {…}
//	NewGate() *Gate
//	(g *Gate) Do(key string, fn func() error) error
//	(g *Gate) TryDo(key string, fn func() error) error
//	(g *Gate) Clear(key string)
//	... 另有 1 个顶层符号

package lease

import (
	"errors"

	"github.com/linkxzhou/SimpleBase/godatabase/internal/singleflight"
)

// Gate 按 key 串行化首次租约获取（perf §1.5）。
//
// 背景：同一进程内并发首写同一 DB 时，每个请求都会独立调用
// Manager.Acquire，互相把对方当作「他人持租」触发 TTL+Grace 的
// 观察等待，最终其中一个请求得到 ErrLeaseHeld（45s + 500）。
// Gate 保证同一 dbID 同时只有一个 Acquire 在执行，后来的请求
// 复用第一个的结果（成功后由 ValidFor/Has 快速路径返回）。
//
// 单飞机制本身在 internal/utils/singleflight；这里只负责把 ErrInFlight
// 翻译成租约领域的 ErrAcquiring。
type Gate struct {
	g singleflight.Group[struct{}]
}

// NewGate 构造写门。
func NewGate() *Gate {
	return &Gate{}
}

// Do 执行 fn：同 key 并发调用只有一个真正执行，其余等待并复用其结果。
// fn 内部的二次检查（ValidFor/Has）保证成功获取后后续调用走缓存路径。
// 第一个调用成功时，等待者返回 nil，调用方需自行复查快速路径（ValidFor）。
func (g *Gate) Do(key string, fn func() error) error {
	if g == nil {
		return fn()
	}
	_, err := g.g.Do(key, wrap(fn))
	return err
}

// TryDo 与 Do 类似，但同 key 已有在途调用时**不等待**，直接返回 ErrAcquiring。
// 写路径的首租获取耗时可长达 TTL+Grace（观察式判活），排队等待会放大尾延迟；
// 首个请求完成获取后，后续请求经 ValidFor 快速路径放行。
func (g *Gate) TryDo(key string, fn func() error) error {
	if g == nil {
		return fn()
	}
	_, err := g.g.TryDo(key, wrap(fn))
	if errors.Is(err, singleflight.ErrInFlight) {
		return ErrAcquiring
	}
	return err
}

// Clear 失租回调后清除记录：允许后续请求重新竞争（Gate 本身无状态残留，
// 这里仅为语义明确与潜在观测点）。
func (g *Gate) Clear(key string) {
	if g == nil {
		return
	}
	g.g.Forget(key)
}

func wrap(fn func() error) func() (struct{}, error) {
	return func() (struct{}, error) { return struct{}{}, fn() }
}
