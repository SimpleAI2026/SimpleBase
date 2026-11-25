// 文件说明：registry.go — 进程内活动轮次登记（planv6.8 §13.8）
//
// 职责：
//   - 轮次级 cancel：Cancel 通过登记的 context 取消执行循环（不是只改库，
//     修复 CB-BE-02/03 的取消路径）；
//   - 会话单轮占用：同线程同时只有一个活动轮次（进程内部分；跨进程占用由
//     Store.Claim 承担）；
//   - 后台执行：执行 goroutine 持有脱离 HTTP 请求的 context（WithoutCancel 语义）；
//   - humanCh：人工回复唤醒等待中的执行循环；
//   - 停机：close 拒绝新登记并取消全部活动轮次（Engine.BeginShutdown），
//     idle 通道在轮次表清空（全部执行循环收尾完毕）时关闭（Engine.WaitIdle）。

package group

import (
	"context"
	"errors"
	"sync"
)

// activeTurn 是一个正在执行的轮次。
type activeTurn struct {
	cancel   context.CancelFunc
	done     chan struct{} // 执行循环结束时关闭
	humanCh  chan struct{} // 人工回复 / 外部推进的唤醒信号
	threadID string
}

// ErrTurnBusy 表示会话已有活动轮次（HTTP 层映射 409 turn_busy）。
var ErrTurnBusy = errors.New("group: thread already has an active turn")

// ErrEngineClosed 表示引擎已进入停机：不再接受新轮次，也不再用动作重启执行循环。
var ErrEngineClosed = errors.New("group: engine is shutting down")

// registry 登记进程内活动轮次。
type registry struct {
	mu       sync.Mutex
	turns    map[string]*activeTurn // turnID → active
	occupied map[string]string      // threadID → turnID（会话占用，进程内部分）
	closed   bool                   // 停机后拒绝新登记
	idle     chan struct{}          // 轮次表非空期间存在，清空时关闭
}

func newRegistry() *registry {
	return &registry{turns: map[string]*activeTurn{}, occupied: map[string]string{}}
}

// register 登记一个活动轮次；threadID 已被其他轮次占用时返回 ErrTurnBusy。
func (r *registry) register(turnID, threadID string, cancel context.CancelFunc) (*activeTurn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, ErrEngineClosed
	}
	if other, busy := r.occupied[threadID]; busy && other != turnID {
		return nil, ErrTurnBusy
	}
	// 同轮次重复登记（RetryTask/Resume 在活动时）：复用现有登记。
	if at, ok := r.turns[turnID]; ok {
		return at, nil
	}
	if len(r.turns) == 0 {
		r.idle = make(chan struct{})
	}
	at := &activeTurn{cancel: cancel, done: make(chan struct{}), humanCh: make(chan struct{}, 1), threadID: threadID}
	r.turns[turnID] = at
	r.occupied[threadID] = turnID
	return at, nil
}

// cancelTurn 取消一个轮次；未登记（已结束/别的进程）返回 false。
func (r *registry) cancelTurn(turnID string) bool {
	r.mu.Lock()
	at, ok := r.turns[turnID]
	r.mu.Unlock()
	if !ok {
		return false
	}
	at.cancel()
	return true
}

// get 返回登记的轮次。
func (r *registry) get(turnID string) (*activeTurn, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	at, ok := r.turns[turnID]
	return at, ok
}

// release 撤销登记（执行循环结束时调用；幂等）。
func (r *registry) release(turnID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if at, ok := r.turns[turnID]; ok {
		close(at.done)
		delete(r.turns, turnID)
		if r.occupied[at.threadID] == turnID {
			delete(r.occupied, at.threadID)
		}
	}
	if len(r.turns) == 0 && r.idle != nil {
		close(r.idle)
		r.idle = nil
	}
}

// closed 报告引擎是否已进入停机。
func (r *registry) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

// close 进入停机（幂等）：拒绝新登记并取消全部活动轮次；不等待，收尾由各执行循环自行完成。
// 没有执行循环的轮次（waiting_human / awaiting_plan_confirm）不在登记表里，不受影响。
func (r *registry) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	for _, at := range r.turns {
		at.cancel()
	}
}

// idleCh 返回「轮次表清空时关闭」的通道；表已空时返回 nil。
func (r *registry) idleCh() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.turns) == 0 {
		return nil
	}
	return r.idle
}

// activeCount 返回活动轮次数（诊断 / 测试）。
func (r *registry) activeCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.turns)
}

// active 返回全部活动轮次 ID（诊断用）。
func (r *registry) active() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.turns))
	for id := range r.turns {
		out = append(out, id)
	}
	return out
}
