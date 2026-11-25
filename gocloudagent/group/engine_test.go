// 文件说明：engine_test.go — gocloudagent/group 的单元测试（TestEngineSubmitCompletesAndReleasesClaim、TestEngineCancelDoesNotBecomeFailure 等）
//
// 用法
//
//	go test ./gocloudagent/group -run 'TestEngineSubmitCompletesAndReleasesClaim|TestEngineCancelDoesNotBecomeFailure|TestEngineHumanReplyContinuesDependentTask|TestEngineFailureBlocksWholeDependencyChain'
//	同包调用：testMembers → shutdown_test.go；testStore → broker_test.go, shutdown_test.go；newTestStore() → broker_test.go, shutdown_test.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, internal/agentrun, internal/api
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/group/engine_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] broker.go, broker_test.go, engine.go, fence.go, ports.go 等 10 个
//	[关键引用] testMembers ← shutdown_test.go；testStore ← broker_test.go, shutdown_test.go；newTestStore ← broker_test.go, shutdown_test.go
//
// 主要符号
//
//	type runnerFunc func(context.Context, TaskInput, func(Event)) (TaskResult, error)
//	(f runnerFunc) RunTask(ctx context.Context, in TaskInput, emit func(Event)) (TaskResult, error)
//	type testMembers struct{…}
//	(m testMembers) Group(context.Context, string) (Group, error)
//	(m testMembers) Candidates(context.Context, string) ([]plan.Candidate, error)
//	(m testMembers) History(context.Context, string, int) ([]plan.Turn, error)
//	type testStore struct{…}
//	newTestStore() *testStore
//	... 另有 34 个顶层符号

package group

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

type runnerFunc func(context.Context, TaskInput, func(Event)) (TaskResult, error)

func (f runnerFunc) RunTask(ctx context.Context, in TaskInput, emit func(Event)) (TaskResult, error) {
	return f(ctx, in, emit)
}

type testMembers struct {
	group Group
}

func (m testMembers) Group(context.Context, string) (Group, error) { return m.group, nil }
func (m testMembers) Candidates(context.Context, string) ([]plan.Candidate, error) {
	out := make([]plan.Candidate, 0, len(m.group.MemberIDs))
	for _, id := range m.group.MemberIDs {
		out = append(out, plan.Candidate{ID: id, Name: id})
	}
	return out, nil
}
func (m testMembers) History(context.Context, string, int) ([]plan.Turn, error) {
	return nil, nil
}

type testStore struct {
	mu       sync.Mutex
	nextTurn int
	turns    map[string]Turn
	tasks    map[string][]Task
	events   map[string][]Event
	claimed  map[string]string
	releases int
}

func newTestStore() *testStore {
	return &testStore{
		turns: map[string]Turn{}, tasks: map[string][]Task{},
		events: map[string][]Event{}, claimed: map[string]string{},
	}
}

func (s *testStore) CreateTurn(ctx context.Context, in Turn) (Turn, bool, error) {
	if err := ctx.Err(); err != nil {
		return Turn{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.IdempotencyKey != "" {
		for _, turn := range s.turns {
			if turn.ThreadID == in.ThreadID && turn.IdempotencyKey == in.IdempotencyKey {
				return turn, true, nil
			}
		}
	}
	s.nextTurn++
	in.ID = fmt.Sprintf("turn-%d", s.nextTurn)
	if in.Status == "" {
		in.Status = TurnPlanning
	}
	s.turns[in.ID] = in
	return in, false, nil
}

func (s *testStore) TransitionTurn(ctx context.Context, id, from, to, code string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	turn, ok := s.turns[id]
	if !ok || turn.Status != from {
		return ErrIllegalTransition
	}
	turn.Status = to
	turn.Version++
	s.turns[id] = turn
	return nil
}

func (s *testStore) SaveDecision(ctx context.Context, id string, d plan.Decision, confirm string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	turn, ok := s.turns[id]
	if !ok {
		return fmt.Errorf("turn not found")
	}
	turn.Decision = d
	turn.ConfirmState = confirm
	s.turns[id] = turn
	return nil
}

func (s *testStore) SaveReplans(ctx context.Context, id string, replans int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	turn, ok := s.turns[id]
	if !ok {
		return fmt.Errorf("turn not found")
	}
	turn.Replans = replans
	s.turns[id] = turn
	return nil
}

func (s *testStore) LoadTurn(ctx context.Context, id string) (Turn, bool, error) {
	if err := ctx.Err(); err != nil {
		return Turn{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	turn, ok := s.turns[id]
	return turn, ok, nil
}

func (s *testStore) CreateTasks(ctx context.Context, turnID string, tasks []Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[turnID] = append([]Task(nil), tasks...)
	return nil
}

func (s *testStore) TransitionTask(ctx context.Context, id, from, to, result, code string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for turnID, tasks := range s.tasks {
		for i := range tasks {
			if tasks[i].ID != id {
				continue
			}
			if tasks[i].Status != from {
				return ErrIllegalTransition
			}
			tasks[i].Status = to
			tasks[i].Result = result
			tasks[i].ErrorCode = code
			tasks[i].Version++
			s.tasks[turnID] = tasks
			return nil
		}
	}
	return fmt.Errorf("task not found")
}

func (s *testStore) SetTaskRunID(ctx context.Context, id, runID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for turnID, tasks := range s.tasks {
		for i := range tasks {
			if tasks[i].ID == id {
				tasks[i].RunID = runID
				s.tasks[turnID] = tasks
				return nil
			}
		}
	}
	return fmt.Errorf("task not found")
}

func (s *testStore) LoadTasks(ctx context.Context, turnID string) ([]Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Task(nil), s.tasks[turnID]...), nil
}

func (s *testStore) AppendEvent(ctx context.Context, turnID string, e Event) (Event, error) {
	if err := ctx.Err(); err != nil {
		return Event{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e.Seq = len(s.events[turnID]) + 1
	e.TurnID = turnID
	s.events[turnID] = append(s.events[turnID], e)
	return e, nil
}

func (s *testStore) ListEvents(ctx context.Context, turnID string, after int) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Event
	for _, e := range s.events[turnID] {
		if e.Seq > after {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *testStore) Claim(ctx context.Context, threadID, turnID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if other := s.claimed[threadID]; other != "" && other != turnID {
		return ErrTurnBusy
	}
	s.claimed[threadID] = turnID
	return nil
}

func (s *testStore) Release(ctx context.Context, threadID, turnID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed[threadID] == turnID {
		delete(s.claimed, threadID)
		s.releases++
	}
	return nil
}

func (s *testStore) ListUnfinishedTurns(ctx context.Context) ([]Turn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Turn
	for _, turn := range s.turns {
		if !TerminalTurn(turn.Status) {
			out = append(out, turn)
		}
	}
	return out, nil
}

func (s *testStore) snapshotTurn(id string) Turn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turns[id]
}

func (s *testStore) snapshotTasks(id string) []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Task(nil), s.tasks[id]...)
}

func (s *testStore) releaseCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.releases
}

func newTestEngine(t *testing.T, store Store, members Group, runner Runner) *Engine {
	t.Helper()
	engine, err := NewEngine(nil, runner, store, testMembers{group: members})
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func waitTurnStatus(t *testing.T, store *testStore, turnID, status string) Turn {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		turn := store.snapshotTurn(turnID)
		if turn.Status == status {
			return turn
		}
		time.Sleep(5 * time.Millisecond)
	}
	turn := store.snapshotTurn(turnID)
	t.Fatalf("turn status = %q, want %q; tasks = %+v", turn.Status, status, store.snapshotTasks(turnID))
	return Turn{}
}

func TestEngineSubmitCompletesAndReleasesClaim(t *testing.T) {
	store := newTestStore()
	runner := runnerFunc(func(_ context.Context, in TaskInput, _ func(Event)) (TaskResult, error) {
		in.OnRunID("run-1")
		return TaskResult{Content: "done", RunID: "run-1"}, nil
	})
	engine := newTestEngine(t, store, Group{ID: "g", MemberIDs: []string{"a"}}, runner)
	turn, err := engine.Submit(context.Background(), TurnInput{ThreadID: "thread", GroupID: "g", UserText: "work"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Cancel(context.Background(), turn.ID) })

	waitTurnStatus(t, store, turn.ID, TurnCompleted)
	if got := store.releaseCount(); got != 1 {
		t.Fatalf("Store.Release calls = %d, want 1", got)
	}
	tasks := store.snapshotTasks(turn.ID)
	if len(tasks) != 1 || tasks[0].RunID != "run-1" {
		t.Fatalf("task run ID not persisted: %+v", tasks)
	}
}

func TestEngineCancelDoesNotBecomeFailure(t *testing.T) {
	store := newTestStore()
	started := make(chan struct{})
	runner := runnerFunc(func(ctx context.Context, _ TaskInput, _ func(Event)) (TaskResult, error) {
		close(started)
		<-ctx.Done()
		return TaskResult{Canceled: true}, nil
	})
	engine := newTestEngine(t, store, Group{ID: "g", MemberIDs: []string{"a"}}, runner)
	turn, err := engine.Submit(context.Background(), TurnInput{ThreadID: "thread", GroupID: "g", UserText: "work"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}
	got, err := engine.Cancel(context.Background(), turn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != TurnCanceled {
		t.Fatalf("turn status = %q, want canceled", got.Status)
	}
	tasks := store.snapshotTasks(turn.ID)
	if len(tasks) != 1 || tasks[0].Status != TaskInterrupted {
		t.Fatalf("tasks after cancel = %+v, want running task interrupted", tasks)
	}
	if got := store.releaseCount(); got != 1 {
		t.Fatalf("Store.Release calls = %d, want 1", got)
	}
}

func TestEngineHumanReplyContinuesDependentTask(t *testing.T) {
	store := newTestStore()
	store.turns["turn-1"] = Turn{ID: "turn-1", ThreadID: "thread", GroupID: "g", Status: TurnRunning}
	store.tasks["turn-1"] = []Task{
		{ID: "turn-1-h", TurnID: "turn-1", Ref: "h", TargetKind: plan.TargetHuman, TargetID: "user", Message: "review", Status: TaskPending},
		{ID: "turn-1-a", TurnID: "turn-1", Ref: "a", TargetKind: plan.TargetAgent, TargetID: "a", Message: "continue", Status: TaskPending, DependsOn: []string{"h"}, ContextFrom: []string{"h"}},
	}
	inputs := make(chan TaskInput, 1)
	runner := runnerFunc(func(_ context.Context, in TaskInput, _ func(Event)) (TaskResult, error) {
		inputs <- in
		return TaskResult{Content: "finished"}, nil
	})
	engine := newTestEngine(t, store, Group{ID: "g", MemberIDs: []string{"a"}, HumanID: "user", HumanName: "User"}, runner)
	t.Cleanup(func() { _, _ = engine.Cancel(context.Background(), "turn-1") })
	engine.ensureExecuting(context.Background(), store.snapshotTurn("turn-1"))
	waitTurnStatus(t, store, "turn-1", TurnWaitingHuman)
	if _, err := engine.ReplyHuman(context.Background(), "turn-1-h", Reply{Text: "approved context"}); err != nil {
		t.Fatal(err)
	}
	waitTurnStatus(t, store, "turn-1", TurnCompleted)
	select {
	case in := <-inputs:
		if in.Context == "" || in.Context != "[来自 @user 的结果]\napproved context" {
			t.Fatalf("dependent task context = %q", in.Context)
		}
	default:
		t.Fatal("dependent agent task did not run")
	}
}

func TestEngineFailureBlocksWholeDependencyChain(t *testing.T) {
	store := newTestStore()
	store.turns["turn-1"] = Turn{ID: "turn-1", ThreadID: "thread", GroupID: "g", Status: TurnRunning, Replans: 1}
	store.tasks["turn-1"] = []Task{
		{ID: "turn-1-a", TurnID: "turn-1", Ref: "a", TargetKind: plan.TargetAgent, TargetID: "a", Status: TaskPending},
		{ID: "turn-1-b", TurnID: "turn-1", Ref: "b", TargetKind: plan.TargetAgent, TargetID: "a", Status: TaskPending, DependsOn: []string{"a"}},
		{ID: "turn-1-c", TurnID: "turn-1", Ref: "c", TargetKind: plan.TargetAgent, TargetID: "a", Status: TaskPending, DependsOn: []string{"b"}},
	}
	runner := runnerFunc(func(_ context.Context, _ TaskInput, _ func(Event)) (TaskResult, error) {
		return TaskResult{Failed: true, Content: "boom", ErrorCode: "run_failed"}, nil
	})
	engine := newTestEngine(t, store, Group{ID: "g", MemberIDs: []string{"a"}, Policy: Policy{MaxReplans: 1}}, runner)
	t.Cleanup(func() { _, _ = engine.Cancel(context.Background(), "turn-1") })
	engine.ensureExecuting(context.Background(), store.snapshotTurn("turn-1"))
	waitTurnStatus(t, store, "turn-1", TurnFailed)
	tasks := store.snapshotTasks("turn-1")
	if tasks[1].Status != TaskBlocked || tasks[2].Status != TaskBlocked {
		t.Fatalf("downstream statuses = %q, %q; want blocked, blocked", tasks[1].Status, tasks[2].Status)
	}
}

type scriptedChat struct {
	mu        sync.Mutex
	decisions []plan.Decision
	calls     int
}

func (c *scriptedChat) Chat(_ context.Context, _ string, _ agentcore.ChatRequest) (agentcore.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	index := c.calls
	c.calls++
	if index >= len(c.decisions) {
		index = len(c.decisions) - 1
	}
	raw, err := json.Marshal(c.decisions[index])
	if err != nil {
		return agentcore.ChatResponse{}, err
	}
	return agentcore.ChatResponse{ToolCalls: []agentcore.ChatToolCall{{Name: plan.ToolPlan, Arguments: string(raw)}}}, nil
}

func (c *scriptedChat) Stream(context.Context, string, agentcore.ChatRequest) (agentcore.TokenStream, error) {
	return nil, fmt.Errorf("stream not implemented")
}

func newScriptedPlanner(decisions ...plan.Decision) *plan.Planner {
	return &plan.Planner{LLM: &scriptedChat{decisions: decisions}}
}

func agentDecision(risk string) plan.Decision {
	return plan.Decision{
		Action: plan.ActionCreate, Goal: "goal", Complexity: plan.ComplexitySimple,
		Dispatches: []plan.Dispatch{{Ref: "d1", TargetKind: plan.TargetAgent, TargetID: "a", Message: "work", Risk: risk}},
	}
}

func TestEngineRetryTaskRevivesFailedTurn(t *testing.T) {
	store := newTestStore()
	store.turns["turn-1"] = Turn{ID: "turn-1", ThreadID: "thread", GroupID: "g", Status: TurnFailed, Decision: agentDecision("")}
	store.tasks["turn-1"] = []Task{{ID: "turn-1-d1", TurnID: "turn-1", Ref: "d1", TargetKind: plan.TargetAgent, TargetID: "a", Status: TaskFailed}}
	engine := newTestEngine(t, store, Group{ID: "g", MemberIDs: []string{"a"}}, runnerFunc(func(_ context.Context, _ TaskInput, _ func(Event)) (TaskResult, error) {
		return TaskResult{Content: "retried"}, nil
	}))
	t.Cleanup(func() { _, _ = engine.Cancel(context.Background(), "turn-1") })
	if _, err := engine.RetryTask(context.Background(), "turn-1-d1"); err != nil {
		t.Fatal(err)
	}
	waitTurnStatus(t, store, "turn-1", TurnCompleted)
	if got := store.snapshotTasks("turn-1")[0].Result; got != "retried" {
		t.Fatalf("retry result = %q", got)
	}
}

func TestEngineConvergeAndResume(t *testing.T) {
	store := newTestStore()
	store.turns["turn-1"] = Turn{ID: "turn-1", ThreadID: "thread", GroupID: "g", Status: TurnRunning, Decision: agentDecision("")}
	store.tasks["turn-1"] = []Task{{ID: "turn-1-d1", TurnID: "turn-1", Ref: "d1", TargetKind: plan.TargetAgent, TargetID: "a", Status: TaskPending}}
	engine := newTestEngine(t, store, Group{ID: "g", MemberIDs: []string{"a"}}, runnerFunc(func(_ context.Context, _ TaskInput, _ func(Event)) (TaskResult, error) {
		return TaskResult{Content: "resumed"}, nil
	}))
	t.Cleanup(func() { _, _ = engine.Cancel(context.Background(), "turn-1") })
	if err := engine.Converge(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitTurnStatus(t, store, "turn-1", TurnInterrupted)
	if got := store.snapshotTasks("turn-1")[0].Status; got != TaskInterrupted {
		t.Fatalf("converged task status = %q, want interrupted", got)
	}
	if _, err := engine.Resume(context.Background(), "turn-1"); err != nil {
		t.Fatal(err)
	}
	waitTurnStatus(t, store, "turn-1", TurnCompleted)
	if got := store.snapshotTasks("turn-1")[0].Result; got != "resumed" {
		t.Fatalf("resume result = %q", got)
	}
}

func TestEngineReplanPersistsBudget(t *testing.T) {
	store := newTestStore()
	store.turns["turn-1"] = Turn{ID: "turn-1", ThreadID: "thread", GroupID: "g", Status: TurnRunning, Decision: agentDecision("")}
	store.tasks["turn-1"] = []Task{{ID: "turn-1-d1", TurnID: "turn-1", Ref: "d1", TargetKind: plan.TargetAgent, TargetID: "a", Status: TaskPending}}
	var mu sync.Mutex
	runs := 0
	runner := runnerFunc(func(_ context.Context, _ TaskInput, _ func(Event)) (TaskResult, error) {
		mu.Lock()
		defer mu.Unlock()
		runs++
		if runs == 1 {
			return TaskResult{Content: "partial", NeedReplan: true}, nil
		}
		return TaskResult{Content: "done"}, nil
	})
	engine, err := NewEngine(newScriptedPlanner(agentDecision("")), runner, store, testMembers{group: Group{ID: "g", MemberIDs: []string{"a", "b"}, Policy: Policy{MaxReplans: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Cancel(context.Background(), "turn-1") })
	engine.ensureExecuting(context.Background(), store.snapshotTurn("turn-1"))
	waitTurnStatus(t, store, "turn-1", TurnCompleted)
	if got := store.snapshotTurn("turn-1").Replans; got != 1 {
		t.Fatalf("persisted replans = %d, want 1", got)
	}
}

func TestEngineReplanDisabledDoesNotCallPlannerAgain(t *testing.T) {
	store := newTestStore()
	store.turns["turn-1"] = Turn{ID: "turn-1", ThreadID: "thread", GroupID: "g", Status: TurnRunning, Decision: agentDecision("")}
	store.tasks["turn-1"] = []Task{{ID: "turn-1-d1", TurnID: "turn-1", Ref: "d1", TargetKind: plan.TargetAgent, TargetID: "a", Status: TaskPending}}
	runs := 0
	runner := runnerFunc(func(_ context.Context, _ TaskInput, _ func(Event)) (TaskResult, error) {
		runs++
		if runs > 1 {
			return TaskResult{Content: "unexpected second run"}, nil
		}
		return TaskResult{Content: "partial", NeedReplan: true}, nil
	})
	planner := newScriptedPlanner(agentDecision(""))
	engine, err := NewEngine(planner, runner, store, testMembers{group: Group{ID: "g", MemberIDs: []string{"a", "b"}, Policy: Policy{MaxReplans: -1}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Cancel(context.Background(), "turn-1") })
	engine.ensureExecuting(context.Background(), store.snapshotTurn("turn-1"))
	waitTurnStatus(t, store, "turn-1", TurnFailed)
	if runs != 1 {
		t.Fatalf("runner calls = %d, want 1", runs)
	}
	if got := planner.LLM.(*scriptedChat).calls; got != 0 {
		t.Fatalf("planner calls = %d, want 0", got)
	}
}

func TestEngineDestructiveReplanWaitsForConfirmation(t *testing.T) {
	store := newTestStore()
	store.turns["turn-1"] = Turn{ID: "turn-1", ThreadID: "thread", GroupID: "g", Status: TurnRunning, Decision: agentDecision("")}
	store.tasks["turn-1"] = []Task{{ID: "turn-1-d1", TurnID: "turn-1", Ref: "d1", TargetKind: plan.TargetAgent, TargetID: "a", Status: TaskPending}}
	runs := 0
	runner := runnerFunc(func(_ context.Context, _ TaskInput, _ func(Event)) (TaskResult, error) {
		runs++
		return TaskResult{Content: "partial", NeedReplan: true}, nil
	})
	engine, err := NewEngine(newScriptedPlanner(agentDecision(plan.RiskDestructive)), runner, store, testMembers{group: Group{ID: "g", MemberIDs: []string{"a", "b"}, Policy: Policy{MaxReplans: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Cancel(context.Background(), "turn-1") })
	engine.ensureExecuting(context.Background(), store.snapshotTurn("turn-1"))
	waitTurnStatus(t, store, "turn-1", TurnAwaitingPlanConfirm)
	if runs != 1 {
		t.Fatalf("runner calls before confirmation = %d, want 1", runs)
	}
}

func TestEngineRunHeadlessReturnsAtHumanTask(t *testing.T) {
	store := newTestStore()
	decision := plan.Decision{
		Action: plan.ActionCreate, Goal: "goal", Complexity: plan.ComplexitySimple,
		Dispatches: []plan.Dispatch{{Ref: "h", TargetKind: plan.TargetHuman, TargetID: "user", Message: "review"}},
	}
	planner := newScriptedPlanner(decision)
	engine, err := NewEngine(planner, runnerFunc(func(_ context.Context, _ TaskInput, _ func(Event)) (TaskResult, error) {
		return TaskResult{}, fmt.Errorf("human task must not use runner")
	}), store, testMembers{group: Group{ID: "g", MemberIDs: []string{"a", "b"}, HumanID: "user"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	result, err := engine.RunHeadless(ctx, TurnInput{ThreadID: "thread", GroupID: "g", UserText: "review"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != TurnWaitingHuman {
		t.Fatalf("headless status = %q, want waiting_human", result.Status)
	}
}
