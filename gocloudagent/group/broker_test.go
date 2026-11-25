// 文件说明：broker_test.go — gocloudagent/group 的单元测试（TestBrokerReplayAndLiveEventsStayOrdered、TestBrokerSlowSubscriberReceivesSingleSequencedEnd 等）
//
// 用法
//
//	go test ./gocloudagent/group -run 'TestBrokerReplayAndLiveEventsStayOrdered|TestBrokerSlowSubscriberReceivesSingleSequencedEnd'
//
// 调用架构
//
//	[上游] gocloudagent/cli, internal/agentrun, internal/api
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/group/broker_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] broker.go, engine.go, engine_test.go, fence.go, ports.go 等 10 个
//
// 主要符号
//
//	type listGateStore struct{…}
//	(s *listGateStore) ListEvents(ctx context.Context, turnID string, after int) ([]Event, error)
//	TestBrokerReplayAndLiveEventsStayOrdered(t *testing.T)
//	TestBrokerSlowSubscriberReceivesSingleSequencedEnd(t *testing.T)

package group

import (
	"context"
	"testing"
	"time"
)

type listGateStore struct {
	*testStore
	entered chan struct{}
	proceed chan struct{}
}

func (s *listGateStore) ListEvents(ctx context.Context, turnID string, after int) ([]Event, error) {
	close(s.entered)
	select {
	case <-s.proceed:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return s.testStore.ListEvents(ctx, turnID, after)
}

func TestBrokerReplayAndLiveEventsStayOrdered(t *testing.T) {
	base := newTestStore()
	base.turns["turn"] = Turn{ID: "turn", Status: TurnRunning}
	_, _ = base.AppendEvent(context.Background(), "turn", Event{Type: EventTask})
	store := &listGateStore{testStore: base, entered: make(chan struct{}), proceed: make(chan struct{})}
	b := newBroker(store, "turn")

	type subscribeResult struct {
		sub *subscription
		err error
	}
	subscribed := make(chan subscribeResult, 1)
	go func() {
		sub, err := b.subscribe(context.Background(), "turn", 0)
		subscribed <- subscribeResult{sub: sub, err: err}
	}()
	<-store.entered
	published := make(chan struct{})
	go func() {
		_, _ = b.publish(context.Background(), "turn", Event{Type: EventProgress})
		close(published)
	}()
	select {
	case <-published:
	case <-time.After(50 * time.Millisecond):
	}
	close(store.proceed)
	result := <-subscribed
	if result.err != nil {
		t.Fatal(result.err)
	}
	<-published
	defer result.sub.close()
	first, ok := result.sub.next()
	if !ok || first.Seq != 1 {
		t.Fatalf("first event = %+v, %v; want seq 1", first, ok)
	}
	second, ok := result.sub.next()
	if !ok || second.Seq != 2 {
		t.Fatalf("second event = %+v, %v; want seq 2", second, ok)
	}
}

func TestBrokerSlowSubscriberReceivesSingleSequencedEnd(t *testing.T) {
	store := newTestStore()
	store.turns["turn"] = Turn{ID: "turn", Status: TurnRunning}
	b := newBroker(store, "turn")
	sub, err := b.subscribe(context.Background(), "turn", 0)
	if err != nil {
		t.Fatal(err)
	}
	const total = 100
	for range total {
		if _, err := b.publish(context.Background(), "turn", Event{Type: EventProgress}); err != nil {
			t.Fatal(err)
		}
	}
	b.finish(EndDone)

	got := 0
	ends := 0
	for {
		event, ok := sub.next()
		if !ok {
			break
		}
		if event.Type == EventEnd {
			ends++
			if event.TurnID != "turn" || event.Seq != total+1 {
				t.Fatalf("end event = %+v, want turn_id=turn seq=%d", event, total+1)
			}
			continue
		}
		got++
	}
	if got != total || ends != 1 {
		t.Fatalf("received events=%d ends=%d, want %d and 1", got, ends, total)
	}

	store.mu.Lock()
	turn := store.turns["turn"]
	turn.Status = TurnCompleted
	store.turns["turn"] = turn
	store.mu.Unlock()
	replayed, err := b.subscribe(context.Background(), "turn", total)
	if err != nil {
		t.Fatal(err)
	}
	event, ok := replayed.next()
	if !ok || event.Type != EventEnd || event.Seq != total+1 {
		t.Fatalf("replayed end = %+v, %v", event, ok)
	}
	if _, ok := replayed.next(); ok {
		t.Fatal("subscription should close after replayed end")
	}
}
