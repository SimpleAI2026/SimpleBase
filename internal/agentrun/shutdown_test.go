// 文件说明：shutdown_test.go — internal/agentrun 的单元测试（TestServiceShutdownCancelsStreamingRunAndWaits、TestServiceSynchronousRunFollowsRequestCancel 等）
//
// 用法
//
//	go test ./internal/agentrun -run 'TestServiceShutdownCancelsStreamingRunAndWaits|TestServiceSynchronousRunFollowsRequestCancel|TestServiceShutdownCancelsRunningTurnAndWaits|TestServiceWaitIsImmediateWithoutWork'
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/shutdown_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] confirm.go, cron.go, errors.go, group_engine.go, group_runner.go 等 17 个
//
// 主要符号
//
//	type blockingChat struct{…}
//	(c *blockingChat) Chat(ctx context.Context, _ string, _ agentcore.ChatRequest) (agentcore.ChatResponse, error)
//	(c *blockingChat) Stream(ctx context.Context, _ string, _ agentcore.ChatRequest) (agentcore.TokenStream, error...
//	(c *blockingChat) block(ctx context.Context) error
//	newBlockingRuntime(chat *blockingChat) *cloudagent.Runtime
//	waitStarted(t *testing.T, chat *blockingChat)
//	seedProject(t *testing.T, store *systemdb.Store) string
//	runStatus(t *testing.T, store *systemdb.Store, projectID, runID string) systemdb.AgentRun
//	... 另有 5 个顶层符号

package agentrun

// shutdown_test.go：D6 的应用服务级用例——BeginShutdown 取消在途 run / 轮次、Wait 等收尾落库、
// 停机后拒绝新建；以及 D6-b（非流式 run 随请求取消，流式不随）。

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// blockingChat 是「启动后阻塞到 ctx 取消」的模型客户端；planFirst 非 nil 时第一次 Chat 返回该决策（栅栏用）。
type blockingChat struct {
	planFirst *plan.Decision
	planCalls atomic.Int32
	started   chan struct{}
}

func (c *blockingChat) Chat(ctx context.Context, _ string, _ agentcore.ChatRequest) (agentcore.ChatResponse, error) {
	if c.planFirst != nil && c.planCalls.Add(1) == 1 {
		raw, _ := json.Marshal(*c.planFirst)
		return agentcore.ChatResponse{ToolCalls: []agentcore.ChatToolCall{{Name: plan.ToolPlan, Arguments: string(raw)}}}, nil
	}
	return agentcore.ChatResponse{}, c.block(ctx)
}

func (c *blockingChat) Stream(ctx context.Context, _ string, _ agentcore.ChatRequest) (agentcore.TokenStream, error) {
	return nil, c.block(ctx)
}

// block 通知「已进入模型调用」后等待 ctx 取消。
func (c *blockingChat) block(ctx context.Context) error {
	c.started <- struct{}{}
	<-ctx.Done()
	return ctx.Err()
}

func newBlockingRuntime(chat *blockingChat) *cloudagent.Runtime {
	return &cloudagent.Runtime{LLM: chat, RunTimeout: time.Minute, ToolProtocol: "native"}
}

func waitStarted(t *testing.T, chat *blockingChat) {
	t.Helper()
	select {
	case <-chat.started:
	case <-time.After(5 * time.Second):
		t.Fatal("model was not called")
	}
}

func seedProject(t *testing.T, store *systemdb.Store) string {
	t.Helper()
	ctx := context.Background()
	if err := store.SeedDefaultCloudAgents(ctx, "testprj1"); err != nil {
		t.Fatal(err)
	}
	agents, err := store.ListCloudAgents(ctx, "testprj1")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range agents {
		if a.BuiltinKey == "general" {
			return a.ID
		}
	}
	t.Fatal("general agent not seeded")
	return ""
}

func runStatus(t *testing.T, store *systemdb.Store, projectID, runID string) systemdb.AgentRun {
	t.Helper()
	row, err := store.GetAgentRun(context.Background(), projectID, runID)
	if err != nil {
		t.Fatalf("run %s: %v", runID, err)
	}
	return row
}

// 流式（extra=nil）直接 run：BeginShutdown 取消、Wait 等到收尾落库、之后拒绝新建。
func TestServiceShutdownCancelsStreamingRunAndWaits(t *testing.T) {
	store := openAgentStore(t)
	seedProject(t, store)
	thread, err := store.CreateAgentThread(context.Background(), systemdb.AgentThread{ProjectID: "testprj1", Title: "title"})
	if err != nil {
		t.Fatal(err)
	}
	chat := &blockingChat{started: make(chan struct{}, 8)}
	s := NewServiceWithStore(Config{}, store, newBlockingRuntime(chat), nil)
	d, err := s.StartDirectRun(context.Background(), DirectRunInput{ProjectID: "testprj1", Thread: thread, Principal: auth.Principal{UserID: "u1"}, Content: "work"})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan RunOutcome, 1)
	go func() { outcomes <- s.ExecuteRun(nil, d, nil) }()
	waitStarted(t, chat)

	s.BeginShutdown()
	s.BeginShutdown() // 幂等
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Wait(ctx); err != nil {
		t.Fatalf("Wait = %v", err)
	}
	out := <-outcomes
	if !errors.Is(out.RunErr, context.Canceled) || out.PersistErr != nil {
		t.Fatalf("outcome = %+v", out)
	}
	if row := runStatus(t, store, "testprj1", d.Run.ID); row.Status != systemdb.AgentRunCanceled || row.FinishedAt.IsZero() {
		t.Fatalf("run after shutdown = %+v, want canceled with finished_at", row)
	}
	if !s.Capabilities().Stopping {
		t.Fatal("capabilities must report stopping")
	}
	if _, err := s.StartDirectRun(context.Background(), DirectRunInput{ProjectID: "testprj1", Thread: thread, Content: "again"}); !errors.Is(err, ErrStopping) {
		t.Fatalf("StartDirectRun after shutdown = %v, want ErrStopping", err)
	}
	if _, err := s.SubmitTurn(context.Background(), TurnInput{ProjectID: "testprj1", Thread: thread, Content: "again"}); !errors.Is(err, ErrStopping) {
		t.Fatalf("SubmitTurn after shutdown = %v, want ErrStopping", err)
	}
	if _, err := s.RunGroupCron(context.Background(), "testprj1", "job", agentbridge.GroupCronBinding{}); !errors.Is(err, ErrStopping) {
		t.Fatalf("RunGroupCron after shutdown = %v, want ErrStopping", err)
	}
}

// D6-b：非流式 run 跟随请求取消（extra 的取消即 run 取消），服务本身继续工作。
func TestServiceSynchronousRunFollowsRequestCancel(t *testing.T) {
	store := openAgentStore(t)
	seedProject(t, store)
	thread, err := store.CreateAgentThread(context.Background(), systemdb.AgentThread{ProjectID: "testprj1", Title: "title"})
	if err != nil {
		t.Fatal(err)
	}
	chat := &blockingChat{started: make(chan struct{}, 8)}
	s := NewServiceWithStore(Config{}, store, newBlockingRuntime(chat), nil)
	t.Cleanup(func() { _ = s.Close(context.Background()) })
	d, err := s.StartDirectRun(context.Background(), DirectRunInput{ProjectID: "testprj1", Thread: thread, Principal: auth.Principal{UserID: "u1"}, Content: "work"})
	if err != nil {
		t.Fatal(err)
	}
	reqCtx, cancel := context.WithCancel(context.Background())
	outcomes := make(chan RunOutcome, 1)
	go func() { outcomes <- s.ExecuteRun(reqCtx, d, nil) }()
	waitStarted(t, chat)
	cancel()
	out := <-outcomes
	if !errors.Is(out.RunErr, context.Canceled) || out.PersistErr != nil {
		t.Fatalf("outcome = %+v", out)
	}
	if row := runStatus(t, store, "testprj1", d.Run.ID); row.Status != systemdb.AgentRunCanceled {
		t.Fatalf("run after request cancel = %s, want canceled", row.Status)
	}
	// 服务未停机：仍可新建（回表的读走已取消的请求 ctx，结果由 RunOutcome.ReadErr 表达，不影响 run 落库）。
	if _, err := s.StartDirectRun(context.Background(), DirectRunInput{ProjectID: "testprj1", Thread: thread, Principal: auth.Principal{UserID: "u1"}, Content: "again"}); err != nil {
		t.Fatalf("service must keep accepting runs after a request cancel: %v", err)
	}
}

// 有执行循环的群轮次：BeginShutdown 取消（轮次 canceled、running 任务 interrupted），Wait 等到收尾。
func TestServiceShutdownCancelsRunningTurnAndWaits(t *testing.T) {
	store := openAgentStore(t)
	general := seedProject(t, store)
	ctx := context.Background()
	decision := plan.Decision{Action: plan.ActionCreate, Goal: "goal", Complexity: plan.ComplexitySimple,
		Dispatches: []plan.Dispatch{{Ref: "d1", TargetKind: plan.TargetAgent, TargetID: general, Message: "work"}}}
	chat := &blockingChat{started: make(chan struct{}, 8), planFirst: &decision}
	s := NewServiceWithStore(Config{}, store, newBlockingRuntime(chat), nil)
	th, err := store.CreateAgentThreadWithWorkspace(ctx, systemdb.AgentThread{ProjectID: "testprj1", Title: "group"},
		systemdb.AgentThreadWorkspace{TagID: systemdb.AgentTagGeneral, DispatchMode: systemdb.DispatchCollaborative, Source: systemdb.ThreadSourceWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAgentThreadCollab(ctx, systemdb.AgentThreadCollab{ProjectID: "testprj1", ThreadID: th.ID, TagID: systemdb.AgentTagGeneral}); err != nil {
		t.Fatal(err)
	}
	turnID, err := s.SubmitTurn(ctx, TurnInput{ProjectID: "testprj1", Thread: th, Principal: auth.Principal{UserID: "u1"}, Content: "work", IdempotencyKey: "shutdown-1"})
	if err != nil {
		t.Fatal(err)
	}
	// 栅栏由 blockingChat 的第一次 Chat 直接给出决策（不阻塞）；任务 run 的模型调用才阻塞，等它进入。
	waitStarted(t, chat)

	s.BeginShutdown()
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Wait(waitCtx); err != nil {
		t.Fatalf("Wait = %v", err)
	}
	turn, err := store.GetAgentTurn(ctx, "testprj1", turnID)
	if err != nil || turn.Status != group.TurnCanceled {
		t.Fatalf("turn after shutdown = %+v err=%v, want canceled", turn, err)
	}
	tasks, err := store.ListAgentTasks(ctx, "testprj1", turnID)
	if err != nil || len(tasks) != 1 || tasks[0].Status != group.TaskInterrupted {
		t.Fatalf("tasks after shutdown = %+v err=%v, want running task interrupted", tasks, err)
	}
	if s.ActiveTurns()["testprj1"] != 0 {
		t.Fatalf("engine still has active turns: %v", s.ActiveTurns())
	}
	if _, err := s.SubmitTurn(ctx, TurnInput{ProjectID: "testprj1", Thread: th, Principal: auth.Principal{UserID: "u1"}, Content: "again", IdempotencyKey: "shutdown-2"}); !errors.Is(err, ErrStopping) {
		t.Fatalf("SubmitTurn after shutdown = %v, want ErrStopping", err)
	}
}

// 没有在途工作时 Wait 立即返回；多次 Close 幂等。
func TestServiceWaitIsImmediateWithoutWork(t *testing.T) {
	s := NewServiceWithStore(Config{}, openAgentStore(t), nil, nil)
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		if err := s.Close(ctx); err != nil {
			t.Fatalf("Close #%d = %v", i, err)
		}
		cancel()
	}
}

// 并发：多个订阅者 / 多次 BeginShutdown 不竞态（-race 下运行）。
func TestServiceBeginShutdownConcurrent(t *testing.T) {
	s := NewServiceWithStore(Config{}, openAgentStore(t), nil, nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.BeginShutdown()
			_ = s.Capabilities()
			_ = s.ActiveTurns()
		}()
	}
	wg.Wait()
	if _, err := s.groupEngineFor("testprj1"); !errors.Is(err, ErrStopping) {
		t.Fatalf("groupEngineFor after shutdown = %v", err)
	}
}
