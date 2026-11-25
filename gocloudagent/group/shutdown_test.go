// 文件说明：shutdown_test.go — gocloudagent/group 的单元测试（TestEngineBeginShutdownRejectsSubmit、TestEngineBeginShutdownCancelsRunningTurnsAndWaitsForCleanup 等）
//
// 用法
//
//	go test ./gocloudagent/group -run 'TestEngineBeginShutdownRejectsSubmit|TestEngineBeginShutdownCancelsRunningTurnsAndWaitsForCleanup|TestEngineWaitIdleIsImmediateWithoutActiveTurns|TestEngineWaitIdleTimesOutWhileTurnKeepsRunning'
//
// 调用架构
//
//	[上游] gocloudagent/cli, internal/agentrun, internal/api
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/group/shutdown_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] broker.go, broker_test.go, engine.go, engine_test.go, fence.go 等 10 个
//
// 主要符号
//
//	type countingStore struct{…}
//	(s *countingStore) count()
//	(s *countingStore) TransitionTurn(ctx context.Context, id, from, to, code string) error
//	(s *countingStore) TransitionTask(ctx context.Context, id, from, to, result, code string) error
//	(s *countingStore) Release(ctx context.Context, threadID, turnID string) error
//	(s *countingStore) AppendEvent(ctx context.Context, turnID string, e Event) (Event, error)
//	(s *countingStore) SaveDecision(ctx context.Context, id string, d plan.Decision, confirm string) error
//	blockingRunner(started chan<- struct{}) Runner
//	... 另有 8 个顶层符号

package group

// 停机语义（Engine.BeginShutdown / WaitIdle）与 RunHeadless 取消后等待收尾的用例。

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// countingStore 在 testStore 之上统计「武装之后」的写入次数，用来证明 WaitIdle 返回后不再有落库。
type countingStore struct {
	*testStore
	armed  atomic.Bool
	writes atomic.Int64
}

func (s *countingStore) count() {
	if s.armed.Load() {
		s.writes.Add(1)
	}
}

func (s *countingStore) TransitionTurn(ctx context.Context, id, from, to, code string) error {
	s.count()
	return s.testStore.TransitionTurn(ctx, id, from, to, code)
}

func (s *countingStore) TransitionTask(ctx context.Context, id, from, to, result, code string) error {
	s.count()
	return s.testStore.TransitionTask(ctx, id, from, to, result, code)
}

func (s *countingStore) Release(ctx context.Context, threadID, turnID string) error {
	s.count()
	return s.testStore.Release(ctx, threadID, turnID)
}

func (s *countingStore) AppendEvent(ctx context.Context, turnID string, e Event) (Event, error) {
	s.count()
	return s.testStore.AppendEvent(ctx, turnID, e)
}

func (s *countingStore) SaveDecision(ctx context.Context, id string, d plan.Decision, confirm string) error {
	s.count()
	return s.testStore.SaveDecision(ctx, id, d, confirm)
}

// blockingRunner 启动后阻塞到 ctx 取消，用来制造「有执行循环的在途轮次」。
func blockingRunner(started chan<- struct{}) Runner {
	return runnerFunc(func(ctx context.Context, _ TaskInput, _ func(Event)) (TaskResult, error) {
		started <- struct{}{}
		<-ctx.Done()
		return TaskResult{Canceled: true}, nil
	})
}

func idleCtx(t *testing.T, d time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), d)
}

func TestEngineBeginShutdownRejectsSubmit(t *testing.T) {
	engine := newTestEngine(t, newTestStore(), Group{ID: "g", MemberIDs: []string{"a"}}, runnerFunc(func(context.Context, TaskInput, func(Event)) (TaskResult, error) {
		return TaskResult{}, errors.New("must not run")
	}))
	engine.BeginShutdown()
	engine.BeginShutdown() // 幂等
	if _, err := engine.Submit(context.Background(), TurnInput{ThreadID: "thread", GroupID: "g", UserText: "work"}); !errors.Is(err, ErrEngineClosed) {
		t.Fatalf("Submit after shutdown = %v, want ErrEngineClosed", err)
	}
	if got := len(engine.ActiveTurns()); got != 0 {
		t.Fatalf("active turns = %d", got)
	}
}

func TestEngineBeginShutdownCancelsRunningTurnsAndWaitsForCleanup(t *testing.T) {
	store := &countingStore{testStore: newTestStore()}
	started := make(chan struct{}, 2)
	engine := newTestEngine(t, store, Group{ID: "g", MemberIDs: []string{"a"}}, blockingRunner(started))
	first, err := engine.Submit(context.Background(), TurnInput{ThreadID: "t1", GroupID: "g", UserText: "a"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.Submit(context.Background(), TurnInput{ThreadID: "t2", GroupID: "g", UserText: "b"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	<-started
	if got := len(engine.ActiveTurns()); got != 2 {
		t.Fatalf("active turns = %d, want 2", got)
	}

	engine.BeginShutdown()
	ctx, cancel := idleCtx(t, 2*time.Second)
	defer cancel()
	if err := engine.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle = %v", err)
	}
	for _, id := range []string{first.ID, second.ID} {
		if turn := store.snapshotTurn(id); turn.Status != TurnCanceled {
			t.Fatalf("turn %s status = %q, want canceled", id, turn.Status)
		}
		tasks := store.snapshotTasks(id)
		if len(tasks) != 1 || tasks[0].Status != TaskInterrupted {
			t.Fatalf("turn %s tasks after shutdown = %+v, want running task interrupted", id, tasks)
		}
	}
	if got := store.releaseCount(); got != 2 {
		t.Fatalf("Store.Release calls = %d, want 2", got)
	}
	if got := len(engine.ActiveTurns()); got != 0 {
		t.Fatalf("active turns after WaitIdle = %d", got)
	}
	// WaitIdle 返回之后不再有任何写入：宿主此时才可以关闭存储。
	store.armed.Store(true)
	time.Sleep(30 * time.Millisecond)
	if w := store.writes.Load(); w != 0 {
		t.Fatalf("writes after WaitIdle = %d, want 0", w)
	}
}

func TestEngineWaitIdleIsImmediateWithoutActiveTurns(t *testing.T) {
	engine := newTestEngine(t, newTestStore(), Group{ID: "g", MemberIDs: []string{"a"}}, runnerFunc(func(context.Context, TaskInput, func(Event)) (TaskResult, error) {
		return TaskResult{}, nil
	}))
	ctx, cancel := idleCtx(t, 50*time.Millisecond)
	defer cancel()
	if err := engine.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle = %v", err)
	}
}

// 未停机时 WaitIdle 不会误报空闲：仍在跑的轮次让等待超时。
func TestEngineWaitIdleTimesOutWhileTurnKeepsRunning(t *testing.T) {
	started := make(chan struct{}, 1)
	engine := newTestEngine(t, newTestStore(), Group{ID: "g", MemberIDs: []string{"a"}}, blockingRunner(started))
	if _, err := engine.Submit(context.Background(), TurnInput{ThreadID: "thread", GroupID: "g", UserText: "work"}); err != nil {
		t.Fatal(err)
	}
	<-started
	ctx, cancel := idleCtx(t, 50*time.Millisecond)
	defer cancel()
	if err := engine.WaitIdle(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitIdle = %v, want DeadlineExceeded", err)
	}
	engine.BeginShutdown()
	drain, cancelDrain := idleCtx(t, 2*time.Second)
	defer cancelDrain()
	if err := engine.WaitIdle(drain); err != nil {
		t.Fatalf("WaitIdle after shutdown = %v", err)
	}
}

// 停在 awaiting_plan_confirm 的轮次没有执行循环：停机不改它的状态，WaitIdle 立即返回，确认动作被拒且不改状态。
func TestEngineBeginShutdownLeavesParkedTurnsAlone(t *testing.T) {
	store := newTestStore()
	engine, err := NewEngine(newScriptedPlanner(agentDecision(plan.RiskDestructive)), runnerFunc(func(context.Context, TaskInput, func(Event)) (TaskResult, error) {
		return TaskResult{}, errors.New("must not run")
	}), store, testMembers{group: Group{ID: "g", MemberIDs: []string{"a", "b"}, Policy: Policy{MaxReplans: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := engine.Submit(context.Background(), TurnInput{ThreadID: "thread", GroupID: "g", UserText: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if turn.Status != TurnAwaitingPlanConfirm {
		t.Fatalf("turn status = %q, want awaiting_plan_confirm", turn.Status)
	}
	engine.BeginShutdown()
	ctx, cancel := idleCtx(t, 200*time.Millisecond)
	defer cancel()
	if err := engine.WaitIdle(ctx); err != nil {
		t.Fatalf("WaitIdle = %v", err)
	}
	if got := store.snapshotTurn(turn.ID).Status; got != TurnAwaitingPlanConfirm {
		t.Fatalf("parked turn status = %q, must be untouched by shutdown", got)
	}
	if _, err := engine.ConfirmPlan(context.Background(), turn.ID, Approval{Approve: true}); !errors.Is(err, ErrEngineClosed) {
		t.Fatalf("ConfirmPlan after shutdown = %v, want ErrEngineClosed", err)
	}
	if got := store.snapshotTurn(turn.ID).Status; got != TurnAwaitingPlanConfirm {
		t.Fatalf("parked turn status after rejected confirm = %q", got)
	}
}

// 停机后显式复活动作（RetryTask / Resume）一律被拒，不会重新拉起执行循环。
func TestEngineBeginShutdownRejectsReviveActions(t *testing.T) {
	store := newTestStore()
	store.turns["turn-1"] = Turn{ID: "turn-1", ThreadID: "thread", GroupID: "g", Status: TurnFailed, Decision: agentDecision("")}
	store.tasks["turn-1"] = []Task{{ID: "turn-1-d1", TurnID: "turn-1", Ref: "d1", TargetKind: plan.TargetAgent, TargetID: "a", Status: TaskFailed}}
	engine := newTestEngine(t, store, Group{ID: "g", MemberIDs: []string{"a"}}, runnerFunc(func(context.Context, TaskInput, func(Event)) (TaskResult, error) {
		return TaskResult{}, errors.New("must not run")
	}))
	engine.BeginShutdown()
	if _, err := engine.RetryTask(context.Background(), "turn-1-d1"); !errors.Is(err, ErrEngineClosed) {
		t.Fatalf("RetryTask = %v, want ErrEngineClosed", err)
	}
	if _, err := engine.Resume(context.Background(), "turn-1"); !errors.Is(err, ErrEngineClosed) {
		t.Fatalf("Resume = %v, want ErrEngineClosed", err)
	}
	if got := store.snapshotTurn("turn-1").Status; got != TurnFailed {
		t.Fatalf("turn status = %q, must stay failed", got)
	}
}

// RunHeadless 在 ctx 取消时等执行循环收尾后再返回：调用方拿到错误即可安全关闭资源。
func TestEngineRunHeadlessWaitsForCleanupAfterCancel(t *testing.T) {
	store := newTestStore()
	started := make(chan struct{}, 1)
	engine := newTestEngine(t, store, Group{ID: "g", MemberIDs: []string{"a"}}, blockingRunner(started))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-started
		cancel()
	}()
	result, err := engine.RunHeadless(ctx, TurnInput{ThreadID: "thread", GroupID: "g", UserText: "work"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunHeadless = %v, want context.Canceled", err)
	}
	if result.TurnID == "" || result.Status != TurnCanceled {
		t.Fatalf("result = %+v", result)
	}
	if got := store.snapshotTurn(result.TurnID).Status; got != TurnCanceled {
		t.Fatalf("turn status on return = %q, want canceled (cleanup must finish first)", got)
	}
	if got := len(engine.ActiveTurns()); got != 0 {
		t.Fatalf("active turns on return = %d", got)
	}
	if got := store.releaseCount(); got != 1 {
		t.Fatalf("Store.Release calls = %d, want 1", got)
	}
}
