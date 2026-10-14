package systemdb

import (
	"context"
	"testing"
	"time"
)

// LIFE-04：run 状态只允许单向迁移（终态不可被覆盖）。
// 修复前 UpdateAgentRunStatus 无条件覆盖：已完成（completed）的 run
// 可以被 CancelRun 改写成 canceled，最终状态取决于谁后写。
func TestAgentRunStatus_TerminalNotOverwritten(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	thread, err := store.CreateAgentThread(ctx, AgentThread{ProjectID: "p1", Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreateAgentRun(ctx, AgentRun{
		ThreadID: thread.ID, ProjectID: "p1", Status: AgentRunRunning, StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 场景 1：完成后试图取消 —— completed 是终态，必须保持不变。
	fin := time.Now().UTC()
	if err := store.UpdateAgentRunStatus(ctx, "p1", run.ID, AgentRunCompleted, "", nil, &fin); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAgentRunStatus(ctx, "p1", run.ID, AgentRunCanceled, "canceled", nil, &fin); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetAgentRun(ctx, "p1", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != AgentRunCompleted {
		t.Fatalf("terminal status overwritten: got %s want %s", got.Status, AgentRunCompleted)
	}

	// 场景 2：running 可以被取消（正常路径不受影响）。
	run2, err := store.CreateAgentRun(ctx, AgentRun{
		ThreadID: thread.ID, ProjectID: "p1", Status: AgentRunRunning, StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAgentRunStatus(ctx, "p1", run2.ID, AgentRunCanceled, "canceled", nil, &fin); err != nil {
		t.Fatal(err)
	}
	got2, err := store.GetAgentRun(ctx, "p1", run2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got2.Status != AgentRunCanceled {
		t.Fatalf("running→canceled rejected: got %s", got2.Status)
	}

	// 场景 3：canceled 同样是终态，不能再被 completed 覆盖
	//（finishAgentRun 与 CancelRun 交错的另一种方向）。
	if err := store.UpdateAgentRunStatus(ctx, "p1", run2.ID, AgentRunCompleted, "", nil, &fin); err != nil {
		t.Fatal(err)
	}
	got3, err := store.GetAgentRun(ctx, "p1", run2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got3.Status != AgentRunCanceled {
		t.Fatalf("canceled overwritten by completed: got %s", got3.Status)
	}

	// 场景 4：失败 → 取消同样被拒绝（失败终态）。
	run3, err := store.CreateAgentRun(ctx, AgentRun{
		ThreadID: thread.ID, ProjectID: "p1", Status: AgentRunRunning, StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAgentRunStatus(ctx, "p1", run3.ID, AgentRunFailed, "boom", nil, &fin); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAgentRunStatus(ctx, "p1", run3.ID, AgentRunCanceled, "canceled", nil, &fin); err != nil {
		t.Fatal(err)
	}
	got4, err := store.GetAgentRun(ctx, "p1", run3.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got4.Status != AgentRunFailed {
		t.Fatalf("failed overwritten by canceled: got %s", got4.Status)
	}
}

// LIFE-04 补充：非终态之间的迁移（running→queued 等管理场景）不受影响；
// 同状态重复写入幂等（例如重复 cancel 已取消的 run）。
func TestAgentRunStatus_IdempotentAndNonTerminalMoves(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	thread, err := store.CreateAgentThread(ctx, AgentThread{ProjectID: "p1", Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	fin := time.Now().UTC()

	// queued → running：允许。
	run, err := store.CreateAgentRun(ctx, AgentRun{
		ThreadID: thread.ID, ProjectID: "p1", Status: AgentRunQueued,
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC()
	if err := store.UpdateAgentRunStatus(ctx, "p1", run.ID, AgentRunRunning, "", &start, nil); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetAgentRun(ctx, "p1", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != AgentRunRunning {
		t.Fatalf("queued→running rejected: got %s", got.Status)
	}

	// 同状态幂等：completed 写两次仍是 completed 且不报错。
	if err := store.UpdateAgentRunStatus(ctx, "p1", run.ID, AgentRunCompleted, "", nil, &fin); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAgentRunStatus(ctx, "p1", run.ID, AgentRunCompleted, "", nil, &fin); err != nil {
		t.Fatal(err)
	}
	got2, err := store.GetAgentRun(ctx, "p1", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got2.Status != AgentRunCompleted {
		t.Fatalf("idempotent write changed status: got %s", got2.Status)
	}
}
