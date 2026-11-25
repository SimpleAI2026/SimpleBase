// 文件说明：agent_collab_test.go — internal/systemdb 的单元测试（TestAgentThreadCollabRoundTrip、TestClaimAgentTurnCAS 等）
//
// 用法
//
//	go test ./internal/systemdb -run 'TestAgentThreadCollabRoundTrip|TestClaimAgentTurnCAS|TestCreateAgentTurnIdempotency|TestUpdateAgentTurnStatusCAS'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app 等 8 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/agent_collab_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_approvals.go, agent_approvals_test.go, agent_collab.go, agent_collab_cron.go, agent_events.go 等 54 个
//
// 主要符号
//
//	collabCtx() context.Context
//	newCollabThread(t *testing.T, s *Store, projectID, tagID, kind string) string
//	TestAgentThreadCollabRoundTrip(t *testing.T)
//	TestClaimAgentTurnCAS(t *testing.T)
//	TestCreateAgentTurnIdempotency(t *testing.T)
//	TestUpdateAgentTurnStatusCAS(t *testing.T)
//	TestAgentTaskLifecycle(t *testing.T)
//	TestAgentTurnEventsSeq(t *testing.T)
//	... 另有 8 个顶层符号

// planv6.7 v2 协作群：agent_collab.go / agent_collab_cron.go 的仓储测试。
// 覆盖：建会话协作元数据、活动轮次 CAS 抢占与释放、轮次幂等键、
// 状态 CAS、任务创建/更新/待办统计、事件序号、消息身份幂等、
// 启动收敛查询、群定时绑定 upsert 与触发去重。
package systemdb

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

func collabCtx() context.Context { return context.Background() }

// newCollabThread 建一条会话并写协作元数据，返回 threadID。
func newCollabThread(t *testing.T, s *Store, projectID, tagID, kind string) string {
	t.Helper()
	th, err := s.CreateAgentThreadWithWorkspace(collabCtx(), AgentThread{ProjectID: projectID, Title: "t"},
		AgentThreadWorkspace{TagID: tagID, DispatchMode: DispatchCollaborative, Source: ThreadSourceWorkspace})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if _, err := s.CreateAgentThreadCollab(collabCtx(), AgentThreadCollab{
		ProjectID: projectID, ThreadID: th.ID, TagID: tagID, ThreadKind: kind,
	}); err != nil {
		t.Fatalf("create collab row: %v", err)
	}
	return th.ID
}

func TestAgentThreadCollabRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := collabCtx()
	threadID := newCollabThread(t, s, "p1", "tag-1", ThreadKindAlert)

	got, err := s.GetAgentThreadCollab(ctx, "p1", threadID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TagID != "tag-1" || got.ThreadKind != ThreadKindAlert || got.ProtocolVersion != 2 || got.Version != 1 {
		t.Fatalf("collab row: %+v", got)
	}
	if got.ActiveTurnID != "" {
		t.Fatalf("new thread should have no active turn: %+v", got)
	}
	// 旧会话无行 → 零值 + nil
	legacy, err := s.GetAgentThreadCollab(ctx, "p1", "no-such-thread")
	if err != nil || legacy.ThreadID != "" {
		t.Fatalf("legacy collab: %v %+v", err, legacy)
	}
	// 批量读取
	batch, err := s.ListAgentThreadCollabs(ctx, "p1", []string{threadID, "no-such-thread"})
	if err != nil || len(batch) != 1 || batch[threadID].ThreadKind != ThreadKindAlert {
		t.Fatalf("batch: %v %+v", err, batch)
	}
	// 项目隔离
	if other, _ := s.GetAgentThreadCollab(ctx, "p2", threadID); other.ThreadID != "" {
		t.Fatalf("project isolation broken: %+v", other)
	}
}

func TestClaimAgentTurnCAS(t *testing.T) {
	s := newTestStore(t)
	ctx := collabCtx()
	threadID := newCollabThread(t, s, "p1", "", ThreadKindChat)

	if err := s.ClaimAgentTurn(ctx, "p1", threadID, "turn-a"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	// 他人抢占 → ErrTurnBusy
	if err := s.ClaimAgentTurn(ctx, "p1", threadID, "turn-b"); !errors.Is(err, ErrTurnBusy) {
		t.Fatalf("second claim should be busy, got %v", err)
	}
	// 自己重复 claim 幂等
	if err := s.ClaimAgentTurn(ctx, "p1", threadID, "turn-a"); err != nil {
		t.Fatalf("re-claim own turn: %v", err)
	}
	// 释放后他人可抢占
	if err := s.ReleaseAgentTurn(ctx, "p1", threadID, "turn-a"); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimAgentTurn(ctx, "p1", threadID, "turn-b"); err != nil {
		t.Fatalf("claim after release: %v", err)
	}
	// 非持有者释放无效
	if err := s.ReleaseAgentTurn(ctx, "p1", threadID, "turn-a"); err != nil {
		t.Fatal(err)
	}
	row, _ := s.GetAgentThreadCollab(ctx, "p1", threadID)
	if row.ActiveTurnID != "turn-b" {
		t.Fatalf("non-owner release must not clear: %+v", row)
	}
	// 没有协作行的旧会话：不可抢占
	if err := s.ClaimAgentTurn(ctx, "p1", "no-such-thread", "turn-x"); !errors.Is(err, ErrTurnBusy) {
		t.Fatalf("legacy thread claim: %v", err)
	}
}

func TestCreateAgentTurnIdempotency(t *testing.T) {
	s := newTestStore(t)
	ctx := collabCtx()
	threadID := newCollabThread(t, s, "p1", "tag-1", ThreadKindChat)

	first, reused, err := s.CreateAgentTurn(ctx, AgentTurn{
		ProjectID: "p1", ThreadID: threadID, TagID: "tag-1",
		InitiatorKind: TurnInitiatorUser, InitiatorID: "u1",
		IdempotencyKey: "key-1", Status: TurnStatusPlanning,
	})
	if err != nil || reused {
		t.Fatalf("create turn: %v reused=%v", err, reused)
	}
	if first.ID == "" || first.Version != 1 || first.RouteJSON != "{}" || first.ConfigSnapshotJS != "{}" {
		t.Fatalf("turn defaults: %+v", first)
	}
	// 同幂等键 → 返回既有轮次（不新建）
	second, reused, err := s.CreateAgentTurn(ctx, AgentTurn{
		ProjectID: "p1", ThreadID: threadID, IdempotencyKey: "key-1",
	})
	if err != nil || !reused || second.ID != first.ID {
		t.Fatalf("idempotent create: %v reused=%v %+v", err, reused, second)
	}
	// 不同幂等键 → 新轮次
	third, reused, err := s.CreateAgentTurn(ctx, AgentTurn{
		ProjectID: "p1", ThreadID: threadID, IdempotencyKey: "key-2",
	})
	if err != nil || reused || third.ID == first.ID {
		t.Fatalf("distinct key: %v reused=%v", err, reused)
	}
	// 空幂等键不参与去重
	a, _, _ := s.CreateAgentTurn(ctx, AgentTurn{ProjectID: "p1", ThreadID: threadID})
	b, _, _ := s.CreateAgentTurn(ctx, AgentTurn{ProjectID: "p1", ThreadID: threadID})
	if a.ID == b.ID {
		t.Fatal("empty idempotency key must not dedupe")
	}
	// 读回
	got, err := s.GetAgentTurn(ctx, "p1", first.ID)
	if err != nil || got.ThreadID != threadID {
		t.Fatalf("get turn: %v %+v", err, got)
	}
	if _, err := s.GetAgentTurn(ctx, "p2", first.ID); !errors.Is(err, ErrTurnNotFound) {
		t.Fatalf("cross project turn: %v", err)
	}
	list, err := s.ListAgentTurns(ctx, "p1", threadID, 20)
	if err != nil || len(list) < 3 {
		t.Fatalf("list turns: %v %d", err, len(list))
	}
}

func TestUpdateAgentTurnStatusCAS(t *testing.T) {
	s := newTestStore(t)
	ctx := collabCtx()
	threadID := newCollabThread(t, s, "p1", "", ThreadKindChat)
	turn, _, err := s.CreateAgentTurn(ctx, AgentTurn{ProjectID: "p1", ThreadID: threadID, Status: TurnStatusPlanning})
	if err != nil {
		t.Fatal(err)
	}
	// 正常推进（带 route 与错误码）
	if err := s.UpdateAgentTurnStatus(ctx, "p1", turn.ID, TurnStatusRunning, `{"tasks":1}`, "", turn.Version); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := s.GetAgentTurn(ctx, "p1", turn.ID)
	if got.Status != TurnStatusRunning || got.RouteJSON != `{"tasks":1}` || got.Version != turn.Version+1 {
		t.Fatalf("after update: %+v", got)
	}
	// 用旧版本 → 冲突
	if err := s.UpdateAgentTurnStatus(ctx, "p1", turn.ID, TurnStatusCompleted, "", "", turn.Version); !errors.Is(err, ErrTurnConflict) {
		t.Fatalf("stale version: %v", err)
	}
	// 不传 route/errCode 时不覆盖已有值
	if err := s.UpdateAgentTurnStatus(ctx, "p1", turn.ID, TurnStatusFailed, "", "routing_failed", 0); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetAgentTurn(ctx, "p1", turn.ID)
	if got.RouteJSON != `{"tasks":1}` || got.ErrorCode != "routing_failed" {
		t.Fatalf("keep route: %+v", got)
	}
	// 清空错误码
	if err := s.ClearAgentTurnError(ctx, "p1", turn.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetAgentTurn(ctx, "p1", turn.ID)
	if got.ErrorCode != "" {
		t.Fatalf("clear error: %+v", got)
	}
	// 不存在的轮次
	if err := s.UpdateAgentTurnStatus(ctx, "p1", "missing", TurnStatusCompleted, "", "", 0); !errors.Is(err, ErrTurnNotFound) {
		t.Fatalf("missing turn: %v", err)
	}
}

func TestAgentTaskLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := collabCtx()
	threadID := newCollabThread(t, s, "p1", "", ThreadKindChat)
	turn, _, _ := s.CreateAgentTurn(ctx, AgentTurn{ProjectID: "p1", ThreadID: threadID})

	agentTask, err := s.CreateAgentTask(ctx, AgentTask{
		ProjectID: "p1", TurnID: turn.ID, ThreadID: threadID, Seq: 1,
		TargetKind: TaskTargetAgent, TargetID: "agent-1", Instruction: "查订单",
		DependsJSON: MarshalAgentTaskDepends(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if agentTask.Status != TaskStatusPending || agentTask.Version != 1 || agentTask.DependsJSON != "[]" {
		t.Fatalf("agent task defaults: %+v", agentTask)
	}
	humanTask, err := s.CreateAgentTask(ctx, AgentTask{
		ProjectID: "p1", TurnID: turn.ID, ThreadID: threadID, Seq: 2,
		TargetKind: TaskTargetHuman, TargetID: "u1", AssigneeID: "u1",
		Instruction: "确认异常", Status: TaskStatusWaitingHuman,
		DependsJSON: MarshalAgentTaskDepends([]string{agentTask.ID}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if deps := UnmarshalAgentTaskDepends(humanTask.DependsJSON); len(deps) != 1 || deps[0] != agentTask.ID {
		t.Fatalf("depends: %+v", deps)
	}
	if UnmarshalAgentTaskDepends("not-json") != nil {
		t.Fatal("bad depends should be nil")
	}

	tasks, err := s.ListAgentTasks(ctx, "p1", turn.ID)
	if err != nil || len(tasks) != 2 || tasks[0].Seq != 1 || tasks[1].Seq != 2 {
		t.Fatalf("list tasks: %v %+v", err, tasks)
	}

	// 待办列表与计数（只算 waiting_human 的人工任务）
	pend, err := s.ListHumanAgentTasks(ctx, "p1", "u1", TaskStatusWaitingHuman, 20)
	if err != nil || len(pend) != 1 || pend[0].ID != humanTask.ID {
		t.Fatalf("human pending: %v %+v", err, pend)
	}
	if n, err := s.CountPendingHumanTasks(ctx, "p1", "u1"); err != nil || n != 1 {
		t.Fatalf("count pending: %v %d", err, n)
	}
	if n, _ := s.CountPendingHumanTasks(ctx, "p1", "u2"); n != 0 {
		t.Fatalf("other assignee: %d", n)
	}

	// 回复：版本 CAS + 状态收敛
	agentTask.Status = TaskStatusCompleted
	agentTask.RunID = "run-1"
	updated, err := s.UpdateAgentTask(ctx, agentTask, agentTask.Version)
	if err != nil || updated.Status != TaskStatusCompleted || updated.RunID != "run-1" || updated.Version != 2 {
		t.Fatalf("update task: %v %+v", err, updated)
	}
	// 旧版本 → 冲突
	if _, err := s.UpdateAgentTask(ctx, agentTask, agentTask.Version); !errors.Is(err, ErrTurnConflict) {
		t.Fatalf("stale task version: %v", err)
	}
	// 不存在的任务
	if _, err := s.GetAgentTask(ctx, "p1", "missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("missing task: %v", err)
	}
	// 运行历史
	if err := s.AppendAgentTaskRun(ctx, AgentTaskRun{
		ProjectID: "p1", TaskID: agentTask.ID, Attempt: 1, RunID: "run-1", Status: "completed",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAgentTurnEventsSeq(t *testing.T) {
	s := newTestStore(t)
	ctx := collabCtx()
	threadID := newCollabThread(t, s, "p1", "", ThreadKindChat)
	turn, _, _ := s.CreateAgentTurn(ctx, AgentTurn{ProjectID: "p1", ThreadID: threadID})

	if seq, err := s.LastAgentTurnSeq(ctx, "p1", turn.ID); err != nil || seq != 0 {
		t.Fatalf("empty seq: %v %d", err, seq)
	}
	for i := int64(1); i <= 3; i++ {
		if _, err := s.AppendAgentTurnEvent(ctx, AgentTurnEvent{
			ProjectID: "p1", TurnID: turn.ID, Seq: i, Kind: "task", EntityID: "task-1",
			PayloadJSON: `{"status":"running"}`,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if seq, _ := s.LastAgentTurnSeq(ctx, "p1", turn.ID); seq != 3 {
		t.Fatalf("last seq: %d", seq)
	}
	// after 过滤（快照 + 续传）
	events, err := s.ListAgentTurnEvents(ctx, "p1", turn.ID, 1, 10)
	if err != nil || len(events) != 2 || events[0].Seq != 2 || events[1].Seq != 3 {
		t.Fatalf("events after=1: %v %+v", err, events)
	}
	// 空 payload 兜底
	ev, err := s.AppendAgentTurnEvent(ctx, AgentTurnEvent{ProjectID: "p1", TurnID: turn.ID, Seq: 4, Kind: "turn"})
	if err != nil || ev.PayloadJSON != "{}" {
		t.Fatalf("default payload: %v %+v", err, ev)
	}
	// 项目隔离
	if list, _ := s.ListAgentTurnEvents(ctx, "p2", turn.ID, 0, 10); len(list) != 0 {
		t.Fatalf("cross project events: %d", len(list))
	}
}

func TestAgentMessageMetaIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := collabCtx()
	meta := AgentMessageMeta{
		ProjectID: "p1", MessageID: "m-1", TurnID: "turn-1", TaskID: "task-1",
		SpeakerKind: TaskTargetAgent, SpeakerID: "agent-1", Kind: MessageKindAgent,
	}
	if err := s.AppendAgentMessageMeta(ctx, meta); err != nil {
		t.Fatal(err)
	}
	// 重复写入幂等（不报错、不产生第二行）
	if err := s.AppendAgentMessageMeta(ctx, meta); err != nil {
		t.Fatal(err)
	}
	batch, err := s.ListAgentMessageMeta(ctx, "p1", []string{"m-1", "m-missing"})
	if err != nil || len(batch) != 1 || batch["m-1"].TaskID != "task-1" {
		t.Fatalf("batch meta: %v %+v", err, batch)
	}
	byTurn, err := s.ListAgentMessageMetaByTurn(ctx, "p1", "turn-1")
	if err != nil || len(byTurn) != 1 {
		t.Fatalf("by turn: %v %+v", err, byTurn)
	}
	// 空列表短路
	if empty, err := s.ListAgentMessageMeta(ctx, "p1", nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty ids: %v %+v", err, empty)
	}
}

func TestListRunningAgentTurnsForConvergence(t *testing.T) {
	s := newTestStore(t)
	ctx := collabCtx()
	threadID := newCollabThread(t, s, "p1", "", ThreadKindChat)
	running, _, _ := s.CreateAgentTurn(ctx, AgentTurn{ProjectID: "p1", ThreadID: threadID, Status: TurnStatusRunning})
	done, _, _ := s.CreateAgentTurn(ctx, AgentTurn{ProjectID: "p1", ThreadID: threadID, Status: TurnStatusCompleted})
	// 新轮次不会被「旧的 running」查询命中
	list, err := s.ListRunningAgentTurns(ctx, time.Now().UTC().Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list {
		if item.ID == done.ID {
			t.Fatal("completed turn must not be listed")
		}
	}
	// cutoff 在未来 → 命中 running
	list, err = s.ListRunningAgentTurns(ctx, time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range list {
		if item.ID == running.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("running turn not listed: %+v", list)
	}
}

func TestAppendAgentTurnEventAtomic(t *testing.T) {
	s := newTestStore(t)
	// 隔离 fixture 使用 SQLite :memory:；限制单连接避免并发拿到独立内存库。
	s.db.SetMaxOpenConns(1)
	ctx := collabCtx()
	threadID := newCollabThread(t, s, "p1", "", ThreadKindChat)
	turn, _, _ := s.CreateAgentTurn(ctx, AgentTurn{ProjectID: "p1", ThreadID: threadID})
	const total = 16
	seqs := make(chan int64, total)
	errs := make(chan error, total)
	var wg sync.WaitGroup
	for range total {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event, err := s.AppendAgentTurnEventAtomic(ctx, AgentTurnEvent{ProjectID: "p1", TurnID: turn.ID, Kind: "progress"})
			if err != nil {
				errs <- err
				return
			}
			seqs <- event.Seq
		}()
	}
	wg.Wait()
	close(errs)
	close(seqs)
	for err := range errs {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for seq := range seqs {
		if seen[seq] {
			t.Fatalf("duplicate seq %d", seq)
		}
		seen[seq] = true
	}
	if len(seen) != total || !seen[1] || !seen[total] {
		t.Fatalf("atomic seqs = %v", seen)
	}
}

// 回归：轮次事件 seq 的分配必须在真实系统库（DuckLake 表）上可用。
// SQLite 夹具支持 INSERT … RETURNING，DuckLake 会报
// 「RETURNING clause not yet supported for insertion into DuckLake table」，
// 线上表现为 POST /agent-threads/:id/turns 500；上面的 SQLite 用例拦不住。
func TestAppendAgentTurnEventAtomicOnDuckLake(t *testing.T) {
	dir := t.TempDir()
	f := &ducklake.Factory{
		CacheDir: filepath.Join(dir, "system", "dbs"),
		Options:  ducklake.DefaultOptions(),
		Syncer:   ducklake.NewLocalSyncer(),
	}
	s, err := Bootstrap(context.Background(), BootstrapInput{
		LocatorDir: filepath.Join(dir, "system"),
		Name:       DefaultName,
		Factory:    f,
		Keys:       objectstore.KeyBuilder{RootPrefix: "simplebase"},
	})
	if err != nil {
		t.Fatalf("bootstrap (needs CGO + ducklake extension): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := collabCtx()
	threadID := newCollabThread(t, s, "p1", "", ThreadKindChat)
	turn, _, _ := s.CreateAgentTurn(ctx, AgentTurn{ProjectID: "p1", ThreadID: threadID})
	const total = 4
	seqs := make(chan int64, total)
	errs := make(chan error, total)
	var wg sync.WaitGroup
	for range total {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event, err := s.AppendAgentTurnEventAtomic(ctx, AgentTurnEvent{ProjectID: "p1", TurnID: turn.ID, Kind: "progress"})
			if err != nil {
				errs <- err
				return
			}
			seqs <- event.Seq
		}()
	}
	wg.Wait()
	close(errs)
	close(seqs)
	for err := range errs {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for seq := range seqs {
		if seen[seq] {
			t.Fatalf("duplicate seq %d", seq)
		}
		seen[seq] = true
	}
	if len(seen) != total || !seen[1] || !seen[total] {
		t.Fatalf("ducklake seqs = %v", seen)
	}
	events, err := s.ListAgentTurnEvents(ctx, "p1", turn.ID, 0, 10)
	if err != nil || len(events) != total {
		t.Fatalf("events=%+v err=%v", events, err)
	}
}

func TestReplaceAgentTasks(t *testing.T) {
	s := newTestStore(t)
	ctx := collabCtx()
	threadID := newCollabThread(t, s, "p1", "", ThreadKindChat)
	turn, _, _ := s.CreateAgentTurn(ctx, AgentTurn{ProjectID: "p1", ThreadID: threadID})
	completed, err := s.CreateAgentTask(ctx, AgentTask{ProjectID: "p1", TurnID: turn.ID, ThreadID: threadID, Seq: 1, Status: TaskStatusCompleted, RunID: "run-1", Attempt: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAgentTask(ctx, AgentTask{ID: "old", ProjectID: "p1", TurnID: turn.ID, ThreadID: threadID, Seq: 2}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceAgentTasks(ctx, "p1", turn.ID, []AgentTask{
		completed,
		{ID: "new", ProjectID: "p1", TurnID: turn.ID, ThreadID: threadID, Seq: 2, Instruction: "replacement"},
	}); err != nil {
		t.Fatal(err)
	}
	tasks, err := s.ListAgentTasks(ctx, "p1", turn.ID)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("replaced tasks: %v %+v", err, tasks)
	}
	if tasks[0].ID != completed.ID || tasks[0].RunID != "run-1" || tasks[0].Attempt != 2 || tasks[1].ID != "new" {
		t.Fatalf("replacement did not preserve completed task: %+v", tasks)
	}
}

func TestAgentGroupCronBindingAndFireDedup(t *testing.T) {
	s := newTestStore(t)
	ctx := collabCtx()

	// 无绑定 → 零值（普通 agent 定时任务）
	none, err := s.GetAgentGroupCron(ctx, "p1", "job-1")
	if err != nil || none.JobID != "" {
		t.Fatalf("no binding: %v %+v", err, none)
	}

	created, err := s.UpsertAgentGroupCron(ctx, AgentGroupCron{
		ProjectID: "p1", JobID: "job-1", TagID: "tag-1", Prompt: "看告警",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ThreadPolicy != GroupCronReuseThread || created.ThreadKind != ThreadKindScheduled {
		t.Fatalf("defaults: %+v", created)
	}

	// 更新策略与负责人
	updated, err := s.UpsertAgentGroupCron(ctx, AgentGroupCron{
		ProjectID: "p1", JobID: "job-1", TagID: "tag-1",
		ThreadPolicy: GroupCronNewThreadPerFire, ThreadKind: ThreadKindAlert,
		OwnerUserID: "u1", Prompt: "看告警 v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ThreadPolicy != GroupCronNewThreadPerFire || updated.OwnerUserID != "u1" || updated.Prompt != "看告警 v2" {
		t.Fatalf("updated: %+v", updated)
	}

	list, err := s.ListAgentGroupCrons(ctx, "p1")
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}

	// 触发去重：首次 true，重复 false
	ok, err := s.ClaimCronFire(ctx, "p1", "job-1", "2026-11-21T09:00")
	if err != nil || !ok {
		t.Fatalf("first fire: %v %v", err, ok)
	}
	ok, err = s.ClaimCronFire(ctx, "p1", "job-1", "2026-11-21T09:00")
	if err != nil || ok {
		t.Fatalf("duplicate fire must be skipped: %v %v", err, ok)
	}
	ok, err = s.ClaimCronFire(ctx, "p1", "job-1", "2026-11-21T10:00")
	if err != nil || !ok {
		t.Fatalf("next fire: %v %v", err, ok)
	}
	if err := s.CompleteCronFire(ctx, "p1", "job-1", "2026-11-21T09:00", "turn-1", "thread-1"); err != nil {
		t.Fatal(err)
	}

	// 群归档 → 绑定暂停
	if err := s.ArchiveAgentGroupCronsByTag(ctx, "p1", "tag-1"); err != nil {
		t.Fatal(err)
	}
	if after, _ := s.GetAgentGroupCron(ctx, "p1", "job-1"); after.JobID != "" {
		t.Fatalf("archived binding still active: %+v", after)
	}
	// 单条归档幂等
	if err := s.ArchiveAgentGroupCron(ctx, "p1", "job-1"); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinAgentTagHelpers(t *testing.T) {
	for _, key := range []string{AgentTagGeneral, AgentTagAlerts, AgentTagStats, AgentTagScheduled} {
		if !IsBuiltinAgentTag(key) || !IsReservedAgentTagID(key) {
			t.Fatalf("%s should be builtin/reserved", key)
		}
	}
	if IsBuiltinAgentTag("custom") || IsReservedAgentTagID("custom") {
		t.Fatal("custom must not be reserved")
	}
	// 预置群：scheduled 也允许写覆盖行
	s := newTestStore(t)
	row, err := s.UpsertBuiltinAgentTag(collabCtx(), "p1", AgentTagScheduled, "定时", AgentTagConfig{
		AgentIDs: []string{"a1"}, RoutingPrompt: "按告警分组",
	}, 0)
	if err != nil || row.ID != AgentTagScheduled || row.Version != 1 {
		t.Fatalf("upsert scheduled: %v %+v", err, row)
	}
	// 非预置 key 拒绝
	if _, err := s.UpsertBuiltinAgentTag(collabCtx(), "p1", "custom", "x", AgentTagConfig{}, 0); !errors.Is(err, ErrAgentTagNotFound) {
		t.Fatalf("custom upsert: %v", err)
	}
	// 预置群不可归档
	for _, key := range []string{AgentTagGeneral, AgentTagAlerts, AgentTagStats, AgentTagScheduled} {
		if err := s.ArchiveAgentTag(collabCtx(), "p1", key); !errors.Is(err, ErrAgentTagBuiltin) {
			t.Fatalf("archive %s: %v", key, err)
		}
	}
}

func TestPageAgentThreadsV2GeneralScopeAndKind(t *testing.T) {
	s := newTestStore(t)
	ctx := collabCtx()

	// #通用 是真实标签分区，不再按 created_by 隐式筛选。
	login, err := s.CreateAgentThreadWithWorkspace(ctx, AgentThread{ProjectID: "p1", Title: "login"},
		AgentThreadWorkspace{TagID: AgentTagGeneral, DispatchMode: DispatchCollaborative, Source: ThreadSourceWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAgentThreadCollab(ctx, AgentThreadCollab{
		ProjectID: "p1", ThreadID: login.ID, ThreadKind: ThreadKindChat,
	}); err != nil {
		t.Fatal(err)
	}
	apiKey, err := s.CreateAgentThreadWithWorkspace(ctx, AgentThread{ProjectID: "p1", Title: "key", CreatedBy: "k1"},
		AgentThreadWorkspace{TagID: AgentTagGeneral, DispatchMode: DispatchCollaborative, Source: ThreadSourceWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAgentThreadCollab(ctx, AgentThreadCollab{
		ProjectID: "p1", ThreadID: apiKey.ID, ThreadKind: ThreadKindChat,
	}); err != nil {
		t.Fatal(err)
	}
	alert, err := s.CreateAgentThreadWithWorkspace(ctx, AgentThread{ProjectID: "p1", Title: "alert"},
		AgentThreadWorkspace{TagID: AgentTagGeneral, DispatchMode: DispatchCollaborative, Source: ThreadSourceWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAgentThreadCollab(ctx, AgentThreadCollab{
		ProjectID: "p1", ThreadID: alert.ID, ThreadKind: ThreadKindAlert,
	}); err != nil {
		t.Fatal(err)
	}

	general, _, err := s.PageAgentThreadsFiltered(ctx, "p1", AgentThreadFilter{TagID: AgentTagGeneral}, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, th := range general {
		ids[th.ID] = true
	}
	if len(general) != 3 || !ids[apiKey.ID] || !ids[login.ID] || !ids[alert.ID] {
		t.Fatalf("general tag scope: %+v", ids)
	}

	// 按类型过滤
	onlyAlert, _, err := s.PageAgentThreadsFiltered(ctx, "p1", AgentThreadFilter{ThreadKind: ThreadKindAlert}, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyAlert) != 1 || onlyAlert[0].ID != alert.ID {
		t.Fatalf("kind filter: %+v", onlyAlert)
	}
	// 旧会话（无协作行）按 chat 处理
	plain, err := s.CreateAgentThreadWithWorkspace(ctx, AgentThread{ProjectID: "p1", Title: "plain"}, AgentThreadWorkspace{})
	if err != nil {
		t.Fatal(err)
	}
	kindChat, _, err := s.PageAgentThreadsFiltered(ctx, "p1", AgentThreadFilter{ThreadKind: ThreadKindChat}, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, th := range kindChat {
		if th.ID == plain.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("legacy thread without collab row should count as chat")
	}
}
