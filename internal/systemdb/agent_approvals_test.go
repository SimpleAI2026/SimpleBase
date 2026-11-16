package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestAgentApproval_CreateIsIdempotentPerCall(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	first, err := store.CreateAgentApproval(ctx, AgentApproval{
		ProjectID: "p1", RunID: "r1", CallID: "c1", ToolName: "simplebase",
		ArgvJSON: `["database","delete"]`, Message: "将执行破坏性命令", InitiatorID: "u1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != AgentApprovalPending {
		t.Fatalf("status=%q want pending", first.Status)
	}

	// 同一 (run, call) 重复登记必须返回既有行，不能产生第二条 pending。
	second, err := store.CreateAgentApproval(ctx, AgentApproval{
		ProjectID: "p1", RunID: "r1", CallID: "c1", ToolName: "simplebase",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate approval created: %s vs %s", second.ID, first.ID)
	}
	n, err := store.CountPendingAgentApprovals(ctx, "p1")
	if err != nil || n != 1 {
		t.Fatalf("pending=%d err=%v want 1", n, err)
	}
}

func TestAgentApproval_ResolveIsOneWay(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	a, err := store.CreateAgentApproval(ctx, AgentApproval{ProjectID: "p1", RunID: "r1", CallID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveAgentApproval(ctx, "p1", a.ID, AgentApprovalApproved, "u1"); err != nil {
		t.Fatal(err)
	}
	// 终态不可回退：再次收敛必须报 ErrNoRows，且状态保持 approved。
	if err := store.ResolveAgentApproval(ctx, "p1", a.ID, AgentApprovalDenied, "u2"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second resolve err=%v want ErrNoRows", err)
	}
	got, err := store.GetAgentApproval(ctx, "p1", a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != AgentApprovalApproved || got.ResolvedBy != "u1" {
		t.Fatalf("status=%q by=%q want approved/u1", got.Status, got.ResolvedBy)
	}
	if got.ResolvedAt.IsZero() {
		t.Fatal("resolved_at not set")
	}
}

func TestAgentApproval_ExpireAndCancelRun(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// 已过窗口的 pending 应被收敛为 expired。
	expired, err := store.CreateAgentApproval(ctx, AgentApproval{
		ProjectID: "p1", RunID: "r1", CallID: "c1",
		ExpiresAt: time.Now().UTC().Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	// 未到窗口的保持 pending。
	live, err := store.CreateAgentApproval(ctx, AgentApproval{
		ProjectID: "p1", RunID: "r2", CallID: "c2",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := store.ExpireAgentApprovals(ctx, "p1")
	if err != nil || n != 1 {
		t.Fatalf("expired=%d err=%v want 1", n, err)
	}
	got, err := store.GetAgentApproval(ctx, "p1", expired.ID)
	if err != nil || got.Status != AgentApprovalExpired {
		t.Fatalf("expired status=%q err=%v", got.Status, err)
	}
	got, err = store.GetAgentApproval(ctx, "p1", live.ID)
	if err != nil || got.Status != AgentApprovalPending {
		t.Fatalf("live status=%q err=%v", got.Status, err)
	}

	// run 结束时作废其残留 pending。
	if err := store.CancelRunApprovals(ctx, "p1", "r2"); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetAgentApproval(ctx, "p1", live.ID)
	if err != nil || got.Status != AgentApprovalCanceled {
		t.Fatalf("canceled status=%q err=%v", got.Status, err)
	}
}

func TestAgentApproval_ListFiltersByStatus(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	a, err := store.CreateAgentApproval(ctx, AgentApproval{ProjectID: "p1", RunID: "r1", CallID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAgentApproval(ctx, AgentApproval{ProjectID: "p1", RunID: "r2", CallID: "c2"}); err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveAgentApproval(ctx, "p1", a.ID, AgentApprovalDenied, "u1"); err != nil {
		t.Fatal(err)
	}

	all, err := store.ListAgentApprovals(ctx, "p1", "", 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("all=%d err=%v want 2", len(all), err)
	}
	pending, err := store.ListAgentApprovals(ctx, "p1", AgentApprovalPending, 0)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%d err=%v want 1", len(pending), err)
	}
	// 项目隔离：其他项目看不到。
	other, err := store.ListAgentApprovals(ctx, "p2", "", 0)
	if err != nil || len(other) != 0 {
		t.Fatalf("other project=%d err=%v want 0", len(other), err)
	}
}
