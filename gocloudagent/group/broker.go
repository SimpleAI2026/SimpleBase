// 文件说明：broker.go — 轮次事件广播（planv6.8 §13.8，修复 CB-BE-04/05/09）
//
// 职责：
//   - seq 单调分配：事件先落库（Store.AppendEvent 原子分配），SSE id 即事件 seq；
//   - 多订阅者：HTTP 连接只是订阅者，断开不取消轮次；
//   - 回放与实时发布经同一把锁串行化，严格按 seq 投递且不重复；
//   - 结构事件进入订阅队列，慢消费者不丢 end；每个订阅只收一次 end。

package group

import (
	"context"
	"sync"
)

// subscription 是一个订阅者。stopped 后仍会排空已排队事件，因此 end 不会覆盖前序帧。
type subscription struct {
	mu      sync.Mutex
	cond    *sync.Cond
	queue   []Event
	stopped bool
	endSent bool
}

func newSubscription() *subscription {
	s := &subscription{}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// broker 是单个轮次的事件总线。publishMu 同时保护事件顺序与订阅登记边界。
type broker struct {
	store  Store
	turnID string

	publishMu sync.Mutex
	mu        sync.Mutex
	subs      map[*subscription]struct{}
	lastSeq   int
	finished  bool
	end       Event
}

func newBroker(store Store, turnID string) *broker {
	return &broker{store: store, turnID: turnID, subs: make(map[*subscription]struct{})}
}

// subscribe 返回一个订阅；after≥0 时先回放历史事件。回放登记与 publish 互斥，
// 因此不会出现实时 seq 较大事件先于回放事件到达的竞态。
func (b *broker) subscribe(ctx context.Context, turnID string, after int) (*subscription, error) {
	if after < 0 {
		after = 0
	}
	sub := newSubscription()
	b.publishMu.Lock()
	defer b.publishMu.Unlock()

	events, err := b.store.ListEvents(ctx, turnID, after)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		sub.push(event)
	}
	if sub.endSent {
		return sub, nil
	}

	turn, ok, err := b.store.LoadTurn(ctx, turnID)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.subs[sub] = struct{}{}
	b.mu.Unlock()
	if ok && (turn.Status == TurnWaitingHuman || turn.Status == TurnAwaitingPlanConfirm) {
		b.mu.Lock()
		delete(b.subs, sub)
		b.mu.Unlock()
		sub.stop()
		return sub, nil
	}
	if !ok || !TerminalTurn(turn.Status) {
		return sub, nil
	}

	// 终态重订阅优先复用已落库的 end，避免进程重启后重复分配终局帧。
	all, err := b.store.ListEvents(ctx, turnID, 0)
	if err != nil {
		b.unsubscribe(sub)
		return nil, err
	}
	for _, event := range all {
		if event.Seq > b.lastSeq {
			b.lastSeq = event.Seq
		}
		if event.Type == EventEnd {
			b.end = event
			b.finished = true
		}
	}
	if b.finished {
		b.mu.Lock()
		delete(b.subs, sub)
		b.mu.Unlock()
		if b.end.Seq > after {
			sub.push(b.end)
		} else {
			sub.close()
		}
		return sub, nil
	}
	b.finishLocked(ctx, endReasonOf(turn.Status))
	return sub, nil
}

func (b *broker) unsubscribe(sub *subscription) {
	b.mu.Lock()
	delete(b.subs, sub)
	b.mu.Unlock()
	sub.close()
}

// publish 落库（原子 seq）并按 seq 顺序广播给所有订阅者。
func (b *broker) publish(ctx context.Context, turnID string, e Event) (Event, error) {
	b.publishMu.Lock()
	defer b.publishMu.Unlock()
	e.TurnID = turnID
	stored, err := b.store.AppendEvent(ctx, turnID, e)
	if err != nil {
		return Event{}, err
	}
	b.mu.Lock()
	if stored.Seq > b.lastSeq {
		b.lastSeq = stored.Seq
	}
	subs := b.subscriptionsLocked()
	b.mu.Unlock()
	for _, sub := range subs {
		sub.push(stored)
	}
	return stored, nil
}

// pause 暂停当前订阅但保留队列；轮次未终局，不写 end。
func (b *broker) pause() {
	b.publishMu.Lock()
	defer b.publishMu.Unlock()
	b.mu.Lock()
	subs := b.subscriptionsLocked()
	b.subs = make(map[*subscription]struct{})
	b.mu.Unlock()
	for _, sub := range subs {
		sub.stop()
	}
}

// finish 轮次终局：end 同样落库并获得真实 seq；每个订阅只收一次。
func (b *broker) finish(reason string) {
	b.publishMu.Lock()
	defer b.publishMu.Unlock()
	b.finishLocked(context.Background(), reason)
}

func (b *broker) finishLocked(ctx context.Context, reason string) {
	if b.finished {
		return
	}
	event := Event{Type: EventEnd, TurnID: b.turnID, Data: endReasonJSON(reason)}
	stored, err := b.store.AppendEvent(ctx, b.turnID, event)
	if err == nil {
		event = stored
	} else {
		// finish 的既有私有契约无错误返回；内存订阅仍收到可排序的终局帧。
		// 存储错误由触发终局的状态迁移路径记录，后续续接会从持久化状态补 end。
		event.Seq = b.lastSeq + 1
	}
	b.finished = true
	b.end = event

	b.mu.Lock()
	if event.Seq > b.lastSeq {
		b.lastSeq = event.Seq
	}
	subs := b.subscriptionsLocked()
	b.subs = make(map[*subscription]struct{})
	b.mu.Unlock()
	for _, sub := range subs {
		sub.push(event)
	}
}

// 调用方必须持有 b.mu。
func (b *broker) subscriptionsLocked() []*subscription {
	out := make([]*subscription, 0, len(b.subs))
	for sub := range b.subs {
		out = append(out, sub)
	}
	return out
}

func (s *subscription) push(event Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.endSent {
		return
	}
	s.queue = append(s.queue, event)
	if event.Type == EventEnd {
		s.endSent = true
		s.stopped = true
	}
	s.cond.Signal()
}

func (s *subscription) stop() {
	s.mu.Lock()
	s.stopped = true
	s.cond.Broadcast()
	s.mu.Unlock()
}

func (s *subscription) close() {
	s.mu.Lock()
	s.stopped = true
	s.queue = nil
	s.cond.Broadcast()
	s.mu.Unlock()
}

// next 等待并返回下一帧；ok=false 表示订阅关闭且队列已排空。
func (s *subscription) next() (Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.queue) == 0 && !s.stopped {
		s.cond.Wait()
	}
	if len(s.queue) == 0 {
		return Event{}, false
	}
	event := s.queue[0]
	s.queue[0] = Event{}
	s.queue = s.queue[1:]
	return event, true
}

// endReasonOf 把轮次终态映射为 end 帧的 reason。
func endReasonOf(status string) string {
	switch status {
	case TurnCompleted:
		return EndDone
	case TurnFailed:
		return EndFailed
	case TurnCanceled:
		return EndCanceled
	case TurnRejected:
		return EndRejected
	default:
		return EndDone
	}
}

// endReasonJSON 构造 end 帧的 data。
func endReasonJSON(reason string) []byte {
	return []byte(`{"reason":"` + reason + `"}`)
}
