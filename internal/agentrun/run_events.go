// 文件说明：run_events.go — run 事件的进程内事件环（断线续传）与实时订阅
//
// 用法
//
//	(s *Service) AppendEvent(runID, ev) int64                       // 追加一帧并广播给订阅者，返回帧 ID
//	(s *Service) ReplayRunEvents(runID, after)                      // 回放 id>after 的帧；live 时附带实时订阅
//	(sub *RunSubscription) Chan() / Close()                         // 实时段：select 读帧；断开时归还订阅
//	runEventHub（包内使用，由 Service 持有）
//
// 调用架构
//
//	[上游] internal/api（run 流的 SSE 写出与 ReplayEvents）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/run_events.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/cloudagent（事件类型）
//	[同包] service.go, run.go
//
// 主要符号
//
//	type RunFrame struct {…}
//	type RunSubscription struct {…}
//	type runEventHub struct {…}
//	(s *Service) AppendEvent(runID string, ev cloudagent.Event) int64
//	(s *Service) ReplayRunEvents(runID string, after int64) (...)
//	... 另有 16 个顶层符号
//
// 归属（internal-ddd-hexagonal-implementation-plan §3.1）：事件环是 run 的长期状态，归应用服务所有；
// HTTP 只是订阅者并把帧编码成 SSE。帧格式、ID 分配、2 分钟 drop / 总量淘汰 / 空闲 TTL 兜底原样保留。

package agentrun

import (
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
)

const runEventCap = 2000

// RunFrame 是 run 事件环里的一帧。ID 由 hub 单调分配：跳号即代表丢帧，续传方必须让客户端刷新消息。
type RunFrame struct {
	ID    int64
	Event cloudagent.Event
}

type runBuf struct {
	frames []RunFrame
	next   int64
	subs   []chan RunFrame
	done   bool
	ended  time.Time
	lastAt time.Time // LIFE-05：最近一次 append 时间，用于无 end 时的兜底清理
}

// runEventHub 保存 run 的 SSE 事件环，供断线续传。
type runEventHub struct {
	mu        sync.Mutex
	runs      map[string]*runBuf
	maxRuns   int           // LIFE-05：缓存 run 总量上限，0 用默认值
	idleTTL   time.Duration // LIFE-05：无 end 事件的空闲清理阈值，0 用默认值
	lastSweep time.Time     // LIFE-05：惰性 sweep 节流
}

// LIFE-05 兜底参数：未收到 end 的 run（中途 panic / 提前返回）超过空闲
// TTL 后由 sweep 清理；总量超过 maxRuns 时淘汰最旧条目。
const (
	runEventIdleTTL    = 10 * time.Minute
	runEventDefaultCap = 512
)

func newRunEventHub() *runEventHub {
	return &runEventHub{runs: map[string]*runBuf{}, maxRuns: runEventDefaultCap, idleTTL: runEventIdleTTL}
}

func (h *runEventHub) append(runID string, ev cloudagent.Event) int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.buf(runID)
	b.next++
	frame := RunFrame{ID: b.next, Event: ev}
	b.frames = append(b.frames, frame)
	b.lastAt = time.Now()
	if len(b.frames) > runEventCap {
		b.frames = b.frames[len(b.frames)-runEventCap:]
	}
	for _, sub := range b.subs {
		select {
		case sub <- frame:
		default:
		}
	}
	if ev.Type == "end" {
		b.done = true
		b.ended = time.Now()
		time.AfterFunc(2*time.Minute, func() { h.drop(runID) })
	}
	h.sweepLocked() // LIFE-05：惰性兜底清理（带节流）
	h.evictLocked()
	return frame.ID
}

func (h *runEventHub) buf(runID string) *runBuf {
	b := h.runs[runID]
	if b == nil {
		b = &runBuf{}
		h.runs[runID] = b
	}
	return b
}

// evictLocked 需要已持有 h.mu：总量超过上限时淘汰最旧的条目（LIFE-05）。
func (h *runEventHub) evictLocked() {
	cap := h.maxRuns
	if cap <= 0 {
		cap = runEventDefaultCap
	}
	for len(h.runs) > cap {
		oldestID := ""
		var oldest time.Time
		for id, b := range h.runs {
			at := b.lastAt
			if b.done {
				at = b.ended
			}
			if oldestID == "" || at.Before(oldest) {
				oldestID, oldest = id, at
			}
		}
		if oldestID == "" {
			return
		}
		delete(h.runs, oldestID)
	}
}

// sweepLocked 需要已持有 h.mu：惰性清理入口，由 append 周期性触发，
// 不引入独立后台 goroutine（router 可能被测试反复创建，避免泄漏）。
func (h *runEventHub) sweepLocked() {
	ttl := h.idleTTL
	if ttl <= 0 {
		ttl = runEventIdleTTL
	}
	now := time.Now()
	if now.Sub(h.lastSweep) < ttl/2 {
		return
	}
	h.lastSweep = now
	for id, b := range h.runs {
		if !b.done && now.Sub(b.lastAt) > ttl {
			delete(h.runs, id)
		}
	}
}

func (h *runEventHub) drop(runID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.runs[runID]
	if b != nil && b.done && time.Since(b.ended) >= 2*time.Minute {
		delete(h.runs, runID)
	}
}

// replay 返回 id>after 的帧。gap 表示 after 已经早于环的起点。
//
// BUG-B1（planv6.0 v2 §2.2）：条目因 2 分钟 drop / 总量淘汰 / 空闲 TTL 被
// 抹掉时，缓存起点已不可达 —— 与 after 早于环起点等价，必须返回 gap=true，
// 否则客户端拿到 200 空流，只会看到「流断了」而非「请刷新消息」。
func (h *runEventHub) replay(runID string, after int64) (frames []RunFrame, gap, live bool, sub chan RunFrame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.runs[runID]
	if b == nil {
		return nil, true, false, nil
	}
	if len(b.frames) > 0 && after > 0 && after < b.frames[0].ID-1 {
		return nil, true, false, nil
	}
	for _, f := range b.frames {
		if f.ID > after {
			frames = append(frames, f)
		}
	}
	if b.done {
		return frames, false, false, nil
	}
	sub = make(chan RunFrame, 16)
	b.subs = append(b.subs, sub)
	return frames, false, true, sub
}

func (h *runEventHub) unsubscribe(runID string, sub chan RunFrame) {
	if sub == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.runs[runID]
	if b == nil {
		return
	}
	next := b.subs[:0]
	for _, s := range b.subs {
		if s != sub {
			next = append(next, s)
		}
	}
	b.subs = next
}

// RunSubscription 是一个 run 的实时事件订阅（ReplayRunEvents 的 live 段）。
type RunSubscription struct {
	hub   *runEventHub
	runID string
	ch    chan RunFrame
}

// Chan 返回实时帧通道；调用方在 select 中与请求 ctx 一起等待。
func (s *RunSubscription) Chan() <-chan RunFrame { return s.ch }

// Close 归还订阅（断开只关订阅，不影响 run）。
func (s *RunSubscription) Close() {
	if s != nil {
		s.hub.unsubscribe(s.runID, s.ch)
	}
}

// AppendEvent 追加一帧并广播，返回帧 ID。事件 ID 分配、事件环写入必须在调用方的输出锁内完成。
func (s *Service) AppendEvent(runID string, ev cloudagent.Event) int64 {
	return s.events.append(runID, ev)
}

// ReplayRunEvents 返回 id>after 的帧。gap 表示 after 已早于环的起点（或条目已不存在）；
// live 为 true 时附带实时订阅，由调用方在断开时 Close。
func (s *Service) ReplayRunEvents(runID string, after int64) (frames []RunFrame, gap, live bool, sub *RunSubscription) {
	frames, gap, live, ch := s.events.replay(runID, after)
	if !live || ch == nil {
		return frames, gap, false, nil
	}
	return frames, gap, true, &RunSubscription{hub: s.events, runID: runID, ch: ch}
}
