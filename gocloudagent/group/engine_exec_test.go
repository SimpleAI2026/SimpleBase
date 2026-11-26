// 文件说明：engine_exec_test.go — 群聊执行循环的取消、存储失败与确认暂停交错测试
//
// 用法
//
//	go test ./group -run 'TestEngineExecute'
//
// 调用架构：fake Store / Runner → Engine.execute → registry / broker。
// 主要符号：execStore、startExecuteForTest、waitExecuteForTest。

package group

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

type execStore struct {
	*testStore
	loadTurn  func(context.Context, string) (Turn, bool, error)
	loadTasks func(context.Context, string) ([]Task, error)
}

func (s *execStore) LoadTurn(ctx context.Context, id string) (Turn, bool, error) {
	if s.loadTurn != nil {
		return s.loadTurn(ctx, id)
	}
	return s.testStore.LoadTurn(ctx, id)
}

func (s *execStore) LoadTasks(ctx context.Context, id string) ([]Task, error) {
	if s.loadTasks != nil {
		return s.loadTasks(ctx, id)
	}
	return s.testStore.LoadTasks(ctx, id)
}

func startExecuteForTest(t *testing.T, engine *Engine, ctx context.Context, turn Turn, g Group) (*activeTurn, <-chan struct{}) {
	t.Helper()
	if err := engine.Store.Claim(ctx, turn.ThreadID, turn.ID); err != nil {
		t.Fatal(err)
	}
	at, err := engine.reg.register(turn.ID, turn.ThreadID, func() {})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		engine.execute(at, ctx, turn, g)
	}()
	return at, done
}

func waitExecuteForTest(t *testing.T, at *activeTurn, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		at.cancel()
		<-done
		t.Fatal("execute did not stop")
	}
}

func TestEngineExecuteCancelBetweenLoadTasksAndRun(t *testing.T) {
	store := &execStore{testStore: newTestStore()}
	turn := Turn{ID: "turn-1", ThreadID: "thread", GroupID: "g", Status: TurnRunning}
	store.turns[turn.ID] = turn
	store.tasks[turn.ID] = []Task{{ID: "turn-1-a", TurnID: turn.ID, Ref: "a", TargetKind: plan.TargetAgent, TargetID: "a", Status: TaskPending}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.loadTasks = func(ctx context.Context, id string) ([]Task, error) {
		tasks, err := store.testStore.LoadTasks(ctx, id)
		cancel()
		return tasks, err
	}
	runs := 0
	engine := newTestEngine(t, store, Group{ID: "g", MemberIDs: []string{"a"}}, runnerFunc(func(context.Context, TaskInput, func(Event)) (TaskResult, error) {
		runs++
		return TaskResult{Content: "unexpected"}, nil
	}))
	at, done := startExecuteForTest(t, engine, ctx, turn, Group{ID: "g", MemberIDs: []string{"a"}})
	waitExecuteForTest(t, at, done)
	if runs != 0 {
		t.Fatalf("runner calls = %d, want 0", runs)
	}
	if got := store.snapshotTurn(turn.ID).Status; got != TurnCanceled {
		t.Fatalf("turn status = %q, want canceled", got)
	}
	if got := store.snapshotTasks(turn.ID)[0].Status; got != TaskCanceled {
		t.Fatalf("task status = %q, want canceled", got)
	}
	if got := store.releaseCount(); got != 1 {
		t.Fatalf("Store.Release calls = %d, want 1", got)
	}
}

func TestEngineExecuteLoadTurnFailureFinishesAndReleases(t *testing.T) {
	store := &execStore{testStore: newTestStore()}
	turn := Turn{ID: "turn-1", ThreadID: "thread", GroupID: "g", Status: TurnRunning}
	store.turns[turn.ID] = turn
	store.loadTurn = func(context.Context, string) (Turn, bool, error) {
		return Turn{}, false, errors.New("load turn failed")
	}
	runs := 0
	engine := newTestEngine(t, store, Group{ID: "g", MemberIDs: []string{"a"}}, runnerFunc(func(context.Context, TaskInput, func(Event)) (TaskResult, error) {
		runs++
		return TaskResult{}, nil
	}))
	at, done := startExecuteForTest(t, engine, context.Background(), turn, Group{ID: "g", MemberIDs: []string{"a"}})
	waitExecuteForTest(t, at, done)
	if runs != 0 {
		t.Fatalf("runner calls = %d, want 0", runs)
	}
	if got := store.snapshotTurn(turn.ID).Status; got != TurnFailed {
		t.Fatalf("turn status = %q, want failed", got)
	}
	if got := store.releaseCount(); got != 1 {
		t.Fatalf("Store.Release calls = %d, want 1", got)
	}
	if got := len(engine.ActiveTurns()); got != 0 {
		t.Fatalf("active turns = %d, want 0", got)
	}
}

func TestEngineExecutePausesAtAwaitingPlanConfirm(t *testing.T) {
	store := &execStore{testStore: newTestStore()}
	turn := Turn{ID: "turn-1", ThreadID: "thread", GroupID: "g", Status: TurnAwaitingPlanConfirm, ConfirmState: ConfirmRequired}
	store.turns[turn.ID] = turn
	runs := 0
	engine := newTestEngine(t, store, Group{ID: "g", MemberIDs: []string{"a"}}, runnerFunc(func(context.Context, TaskInput, func(Event)) (TaskResult, error) {
		runs++
		return TaskResult{}, nil
	}))
	at, done := startExecuteForTest(t, engine, context.Background(), turn, Group{ID: "g", MemberIDs: []string{"a"}})
	waitExecuteForTest(t, at, done)
	if runs != 0 {
		t.Fatalf("runner calls = %d, want 0", runs)
	}
	got := store.snapshotTurn(turn.ID)
	if got.Status != TurnAwaitingPlanConfirm || got.ConfirmState != ConfirmRequired {
		t.Fatalf("parked turn = %+v, want awaiting_plan_confirm with confirmation required", got)
	}
	if got := store.releaseCount(); got != 1 {
		t.Fatalf("Store.Release calls = %d, want 1", got)
	}
	if got := len(engine.ActiveTurns()); got != 0 {
		t.Fatalf("active turns = %d, want 0", got)
	}
}
