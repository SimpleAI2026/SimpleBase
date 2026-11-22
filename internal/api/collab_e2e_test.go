// planv6.7 v2：协作群端到端测试（真实链路 router → 协调器 → Runtime → fakellm）。
// 覆盖：能力探测、多助手 @ 串行执行（都真的跑了）、轮次快照/事件、
// 人工待办闭环（回复续跑 / 无法处理）、会话占用互斥、幂等重放、
// 群归档拒绝发送、成员增删、告警输入入口、启动收敛。
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

// newCollabEnv 建一个协作群 + 一条群会话，返回环境。
//
// dispatcher 固定：需要模型路由时返回空 plan（让用例显式 @，避免依赖真实模型）。
func newCollabEnv(t *testing.T) (*agentHarness, *fakeDispatcher, string, map[string]systemdb.CloudAgent) {
	t.Helper()
	h := newAgentHarness(t, nil, fakellm.Script{
		NonStreamContent: "ok",
		Steps:            []fakellm.Step{{Content: "ok", FinishReason: "stop"}},
	})
	if err := h.store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	agents, err := h.store.ListCloudAgents(context.Background(), "testprj1")
	if err != nil {
		t.Fatal(err)
	}
	byModule := map[string]systemdb.CloudAgent{}
	for _, a := range agents {
		byModule[a.Module] = a
	}
	// 群成员：数据库 + 对象存储，覆盖「一轮两个助手」。
	rec := h.req(http.MethodPost, "/agent-tags", map[string]any{
		"name":              "订单核对",
		"agent_ids":         []string{byModule["database"].ID, byModule["s3"].ID},
		"routing_prompt":    "先查数据库，再核对对象存储文件。",
		"fallback_agent_id": byModule["database"].ID,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag: %d %s", rec.Code, rec.Body.String())
	}
	var tag struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tag)
	// 群会话（协作模式）。
	rec = h.req(http.MethodPost, "/agent-threads", map[string]any{
		"title": "核对订单", "tag_id": tag.ID, "dispatch_mode": "collaborative",
		"thread_kind": "chat",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create thread: %d %s", rec.Code, rec.Body.String())
	}
	var th struct {
		ID           string `json:"id"`
		DispatchMode string `json:"dispatch_mode"`
		ThreadKind   string `json:"thread_kind"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &th)
	if th.DispatchMode != "collaborative" || th.ThreadKind != "chat" {
		t.Fatalf("thread dto: %+v", th)
	}
	dispatcher := &fakeDispatcher{}
	h.handler.dispatcher = dispatcher
	return h, dispatcher, th.ID, byModule
}

// createTurn 以登录态用户发起一轮，返回 SSE 事件。
func (h *agentHarness) createTurn(t *testing.T, threadID, content string, mentions []map[string]any, userID string) []sseEvent {
	t.Helper()
	body := map[string]any{"content": content, "stream": true}
	if mentions != nil {
		body["mentions"] = mentions
	}
	rec := h.reqAs(http.MethodPost, "/agent-threads/"+threadID+"/turns", body, userPrincipal(userID))
	if rec.Code != http.StatusOK {
		t.Fatalf("create turn: %d %s", rec.Code, rec.Body.String())
	}
	events, err := parseSSE(rec.Body.String())
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func TestWorkspaceCapabilities(t *testing.T) {
	h := newAgentHarness(t, nil, fakellm.Script{NonStreamContent: "ok"})
	rec := h.req(http.MethodGet, "/agent-workspace-capabilities", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("capabilities: %d %s", rec.Code, rec.Body.String())
	}
	var dto workspaceCapabilitiesDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.ProtocolVersion != 2 || !dto.Collaboration {
		t.Fatalf("capabilities: %+v", dto)
	}
	if dto.MaxTasks <= 0 || dto.MaxDepth <= 0 || len(dto.ThreadKinds) != 4 {
		t.Fatalf("limits: %+v", dto)
	}
	if len(dto.MissingTables) != 0 {
		t.Fatalf("schema should be ready in tests: %+v", dto.MissingTables)
	}
}

func TestCollabMultiAgentTurnExecutesAll(t *testing.T) {
	h, _, threadID, agents := newCollabEnv(t)
	db, s3 := agents["database"], agents["s3"]

	events := h.createTurn(t, threadID, "核对订单和文件", []map[string]any{
		{"kind": "agent", "id": db.ID}, {"kind": "agent", "id": s3.ID},
	}, "u1")
	runID := runIDOf(events)
	if runID == "" {
		t.Fatal("no run event")
	}
	// 路由卡：两步（都是助手任务）。
	routes := eventsOf(events, "route")
	if len(routes) == 0 || len(routes[0].Steps) != 2 {
		t.Fatalf("route card: %+v", routes)
	}
	// 两个任务都必须真的执行（不允许「多选只跑一个」）。
	tasks, err := listTurnTasks(t, h, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("tasks: %d", len(tasks))
	}
	for _, task := range tasks {
		if task.Status != systemdb.TaskStatusCompleted {
			t.Fatalf("task not completed: %+v", task)
		}
		if task.RunID == "" {
			t.Fatalf("task has no run: %+v", task)
		}
	}
	// 两位助手各自产生了 run 与 assistant 消息。
	runs, _ := h.store.ListAgentRuns(context.Background(), "testprj1", threadID, 10)
	if len(runs) != 2 {
		t.Fatalf("runs: %d", len(runs))
	}
	msgs, _ := h.store.ListAgentMessages(context.Background(), "testprj1", threadID, 50)
	assistantAgents := map[string]bool{}
	for _, m := range msgs {
		if m.Role == "assistant" {
			assistantAgents[m.AgentID] = true
		}
	}
	if !assistantAgents[db.ID] || !assistantAgents[s3.ID] {
		t.Fatalf("assistant messages missing per agent: %+v", assistantAgents)
	}
	// 轮次终态 completed，且占用已释放。
	turn, err := lastTurn(t, h, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if turn.Status != systemdb.TurnStatusCompleted {
		t.Fatalf("turn status: %+v", turn)
	}
	collab, _ := h.store.GetAgentThreadCollab(context.Background(), "testprj1", threadID)
	if collab.ActiveTurnID != "" {
		t.Fatalf("active turn not released: %+v", collab)
	}
	// 事件表有 route + task + turn 记录。
	evs, err := h.store.ListAgentTurnEvents(context.Background(), "testprj1", turn.ID, 0, 100)
	if err != nil || len(evs) == 0 {
		t.Fatalf("turn events: %v %d", err, len(evs))
	}
	if evs[0].Seq != 1 {
		t.Fatalf("event seq should start at 1: %+v", evs[0])
	}
}

// listTurnTasks 返回会话最新轮次的任务。
func listTurnTasks(t *testing.T, h *agentHarness, threadID string) ([]systemdb.AgentTask, error) {
	t.Helper()
	turn, err := lastTurn(t, h, threadID)
	if err != nil {
		return nil, err
	}
	return h.store.ListAgentTasks(context.Background(), "testprj1", turn.ID)
}

// lastTurn 返回会话最新轮次。
func lastTurn(t *testing.T, h *agentHarness, threadID string) (systemdb.AgentTurn, error) {
	t.Helper()
	list, err := h.store.ListAgentTurns(context.Background(), "testprj1", threadID, 20)
	if err != nil {
		return systemdb.AgentTurn{}, err
	}
	if len(list) == 0 {
		return systemdb.AgentTurn{}, systemdb.ErrTurnNotFound
	}
	return list[0], nil
}

func TestCollabHumanTaskReplyContinuesTurn(t *testing.T) {
	h, _, threadID, _ := newCollabEnv(t)

	// @人：登录态用户才有「人」的身份。
	events := h.createTurn(t, threadID, "帮我确认这笔异常", []map[string]any{
		{"kind": "human", "id": "me"},
	}, "u1")
	if len(eventsOf(events, "human")) == 0 {
		t.Fatalf("human event missing: %+v", events)
	}
	turn, err := lastTurn(t, h, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if turn.Status != systemdb.TurnStatusWaitingHuman {
		t.Fatalf("turn should wait for human: %+v", turn)
	}
	// 等人不占会话占用（释放后可再发）。
	collab, _ := h.store.GetAgentThreadCollab(context.Background(), "testprj1", threadID)
	if collab.ActiveTurnID != "" {
		t.Fatalf("waiting turn must release the thread: %+v", collab)
	}
	tasks, _ := h.store.ListAgentTasks(context.Background(), "testprj1", turn.ID)
	if len(tasks) != 1 || tasks[0].TargetKind != systemdb.TaskTargetHuman || tasks[0].AssigneeID != "u1" {
		t.Fatalf("human task: %+v", tasks)
	}
	// 待办列表与角标。
	rec := h.reqAs(http.MethodGet, "/agent-human-tasks?assignee=me&status=pending", nil, userPrincipal("u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("human tasks: %d %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Tasks   []humanTaskDTO `json:"tasks"`
		Pending int            `json:"pending"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Tasks) != 1 || listed.Pending != 1 {
		t.Fatalf("pending tasks: %+v", listed)
	}
	// 别人不能处理（非接收人且非管理员）。
	rec = h.reqAs(http.MethodPost, "/agent-tasks/"+tasks[0].ID+"/replies",
		map[string]any{"outcome": "continue", "text": "我来处理"}, userPrincipal("u2"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("other user reply: %d %s", rec.Code, rec.Body.String())
	}
	// 接收人回复并继续 → 任务完成、轮次结束。
	rec = h.reqAs(http.MethodPost, "/agent-tasks/"+tasks[0].ID+"/replies",
		map[string]any{"outcome": "continue", "text": "已确认为正常波动"}, userPrincipal("u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("reply: %d %s", rec.Code, rec.Body.String())
	}
	tasks, _ = h.store.ListAgentTasks(context.Background(), "testprj1", turn.ID)
	if len(tasks) != 1 || tasks[0].Status != systemdb.TaskStatusCompleted || tasks[0].RepliedBy != "u1" {
		t.Fatalf("task after reply: %+v", tasks)
	}
	turn, _ = lastTurn(t, h, threadID)
	if turn.Status != systemdb.TurnStatusCompleted {
		t.Fatalf("turn after reply: %+v", turn)
	}
	// 回复进入会话历史（人工回复消息）。
	msgs, _ := h.store.ListAgentMessages(context.Background(), "testprj1", threadID, 50)
	found := false
	for _, m := range msgs {
		if m.Role == "user" && m.Content == "[人工回复] 已确认为正常波动" {
			found = true
		}
	}
	if !found {
		t.Fatal("human reply should be appended to the thread history")
	}
}

func TestCollabHumanUnableMarksTurnFailed(t *testing.T) {
	h, _, threadID, _ := newCollabEnv(t)
	h.createTurn(t, threadID, "需要你确认", []map[string]any{{"kind": "human", "id": "me"}}, "u1")
	turn, _ := lastTurn(t, h, threadID)
	tasks, _ := h.store.ListAgentTasks(context.Background(), "testprj1", turn.ID)
	rec := h.reqAs(http.MethodPost, "/agent-tasks/"+tasks[0].ID+"/replies",
		map[string]any{"outcome": "unable"}, userPrincipal("u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("unable reply: %d %s", rec.Code, rec.Body.String())
	}
	tasks, _ = h.store.ListAgentTasks(context.Background(), "testprj1", turn.ID)
	if tasks[0].Status != systemdb.TaskStatusFailed || tasks[0].ErrorCode != "human_unable" {
		t.Fatalf("unable task: %+v", tasks[0])
	}
	turn, _ = lastTurn(t, h, threadID)
	if turn.Status != systemdb.TurnStatusFailed {
		t.Fatalf("turn should fail: %+v", turn)
	}
}

func TestCollabHumanMentionFromAPIKeyRejected(t *testing.T) {
	h, _, threadID, _ := newCollabEnv(t)
	// API Key 没有「人」的身份：@人 必须 409 human_assignee_required。
	rec := h.reqAs(http.MethodPost, "/agent-threads/"+threadID+"/turns", map[string]any{
		"content": "确认一下", "stream": true,
		"mentions": []map[string]any{{"kind": "human", "id": "me"}},
	}, auth_APIKey())
	if rec.Code != http.StatusConflict {
		t.Fatalf("api key human mention: %d %s", rec.Code, rec.Body.String())
	}
	if !jsonBodyContains(rec.Body.String(), "human_assignee_required") {
		t.Fatalf("error code: %s", rec.Body.String())
	}
}

func TestCollabTurnIdempotency(t *testing.T) {
	h, _, threadID, agents := newCollabEnv(t)
	db := agents["database"]
	body := map[string]any{
		"content": "看看订单", "stream": false, "idempotency_key": "k-1",
		"mentions": []map[string]any{{"kind": "agent", "id": db.ID}},
	}
	rec := h.reqAs(http.MethodPost, "/agent-threads/"+threadID+"/turns", body, userPrincipal("u1"))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("first turn: %d %s", rec.Code, rec.Body.String())
	}
	var first turnDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &first)
	runsBefore, _ := h.store.ListAgentRuns(context.Background(), "testprj1", threadID, 20)

	// 同幂等键重放：返回同一轮次，且不重复建 run。
	rec = h.reqAs(http.MethodPost, "/agent-threads/"+threadID+"/turns", body, userPrincipal("u1"))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("replay: %d %s", rec.Code, rec.Body.String())
	}
	var replay turnDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &replay)
	if replay.ID != first.ID {
		t.Fatalf("replay should return the same turn: %s vs %s", replay.ID, first.ID)
	}
	runsAfter, _ := h.store.ListAgentRuns(context.Background(), "testprj1", threadID, 20)
	if len(runsAfter) != len(runsBefore) {
		t.Fatalf("replay must not create runs: %d -> %d", len(runsBefore), len(runsAfter))
	}
}

func TestCollabTurnBusyGuard(t *testing.T) {
	h, _, threadID, _ := newCollabEnv(t)
	// 直接占用会话，模拟另一轮正在跑。
	active, _, err := h.store.CreateAgentTurn(context.Background(), systemdb.AgentTurn{
		ProjectID: "testprj1", ThreadID: threadID, Status: systemdb.TurnStatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.ClaimAgentTurn(context.Background(), "testprj1", threadID, active.ID); err != nil {
		t.Fatal(err)
	}
	rec := h.reqAs(http.MethodPost, "/agent-threads/"+threadID+"/turns", map[string]any{
		"content": "再发一条", "stream": true,
	}, userPrincipal("u1"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("busy turn: %d %s", rec.Code, rec.Body.String())
	}
	if !jsonBodyContains(rec.Body.String(), "turn_busy") {
		t.Fatalf("error code: %s", rec.Body.String())
	}
}

func TestCollabArchivedGroupRejectsSend(t *testing.T) {
	h, _, threadID, _ := newCollabEnv(t)
	// 找到会话所属群并归档。
	ws, _ := h.store.GetAgentThreadWorkspace(context.Background(), "testprj1", threadID)
	rec := h.req(http.MethodDelete, "/agent-tags/"+ws.TagID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("archive tag: %d %s", rec.Code, rec.Body.String())
	}
	rec = h.reqAs(http.MethodPost, "/agent-threads/"+threadID+"/turns", map[string]any{
		"content": "还能发吗", "stream": true,
	}, userPrincipal("u1"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("archived group: %d %s", rec.Code, rec.Body.String())
	}
	if !jsonBodyContains(rec.Body.String(), "group_archived") {
		t.Fatalf("error code: %s", rec.Body.String())
	}
	// 历史仍可读（会话未被归档）。
	if rec := h.req(http.MethodGet, "/agent-threads/"+threadID, nil); rec.Code != http.StatusOK {
		t.Fatalf("thread should stay readable: %d", rec.Code)
	}
}

func TestGroupMembersAddAndRemove(t *testing.T) {
	h, _, _, agents := newCollabEnv(t)
	general := agents["general"]

	rec := h.req(http.MethodPost, "/agent-tags", map[string]any{"name": "成员群", "agent_ids": []string{}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag: %d %s", rec.Code, rec.Body.String())
	}
	var tag agentTagDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &tag)

	// 加入已有助手。
	rec = h.req(http.MethodPost, "/agent-tags/"+tag.ID+"/members", map[string]any{
		"agent_ids": []string{general.ID}, "expected_version": tag.Version,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("add member: %d %s", rec.Code, rec.Body.String())
	}
	var updated agentTagDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if len(updated.AgentIDs) != 1 || updated.AgentIDs[0] != general.ID {
		t.Fatalf("members: %+v", updated)
	}
	// 新建助手并加入（组合事务）。
	rec = h.req(http.MethodPost, "/agent-tags/"+tag.ID+"/members", map[string]any{
		"new_agent": map[string]any{"name": "新助手", "module": "database"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("new agent member: %d %s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if len(updated.AgentIDs) != 2 {
		t.Fatalf("members after new agent: %+v", updated)
	}
	// 移出成员（不删除助手）。
	rec = h.req(http.MethodDelete, "/agent-tags/"+tag.ID+"/members/"+general.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove member: %d %s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if len(updated.AgentIDs) != 1 || updated.AgentIDs[0] == general.ID {
		t.Fatalf("members after remove: %+v", updated)
	}
	if _, err := h.store.GetCloudAgent(context.Background(), "testprj1", general.ID); err != nil {
		t.Fatalf("removing a member must not archive the agent: %v", err)
	}
	// 移出非成员 → 404。
	rec = h.req(http.MethodDelete, "/agent-tags/"+tag.ID+"/members/"+general.ID, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("remove non member: %d", rec.Code)
	}
}

func TestGeneralGroupMembersAddAndRemove(t *testing.T) {
	h, _, _, agents := newCollabEnv(t)
	database := agents["database"]

	rec := h.req(http.MethodDelete, "/agent-tags/general/members/"+database.ID+"?expected_version=0", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove general member: %d %s", rec.Code, rec.Body.String())
	}
	var updated agentTagDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Version != 1 || slices.Contains(updated.AgentIDs, database.ID) {
		t.Fatalf("general members after remove: %+v", updated)
	}

	rec = h.req(http.MethodGet, "/agent-tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list general members: %d", rec.Code)
	}
	var listed struct {
		Tags []agentTagDTO `json:"tags"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Tags) == 0 || slices.Contains(listed.Tags[0].AgentIDs, database.ID) {
		t.Fatalf("general override must be used by list: %+v", listed.Tags)
	}

	rec = h.req(http.MethodPost, "/agent-tags/general/members", map[string]any{
		"agent_ids": []string{database.ID}, "expected_version": updated.Version,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("add general member: %d %s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if !slices.Contains(updated.AgentIDs, database.ID) {
		t.Fatalf("general members after add: %+v", updated)
	}
}

func TestGroupEventEntryCreatesThread(t *testing.T) {
	h := newAgentHarness(t, nil, fakellm.Script{NonStreamContent: "ok"})
	if err := h.store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	agents, _ := h.store.ListCloudAgents(context.Background(), "testprj1")
	general := ""
	for _, a := range agents {
		if a.BuiltinKey == "general" {
			general = a.ID
		}
	}
	rec := h.req(http.MethodPost, "/agent-tags", map[string]any{"name": "告警入口", "agent_ids": []string{general}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag: %d %s", rec.Code, rec.Body.String())
	}
	var tag agentTagDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &tag)

	req := map[string]any{
		"external_id": "alarm-001", "title": "CPU 告警",
		"prompt": "CPU 使用率持续 90% 以上，请分析", "thread_kind": "alert",
	}
	rec = h.req(http.MethodPost, "/agent-tags/"+tag.ID+"/events", req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("group event: %d %s", rec.Code, rec.Body.String())
	}
	var turn turnDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &turn)
	// 事件会话类型是 alert，且没有人工任务（机器入口）。
	thread, err := h.store.GetAgentThread(context.Background(), "testprj1", turn.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if thread.Title != "CPU 告警" {
		t.Fatalf("thread title: %+v", thread)
	}
	collab, _ := h.store.GetAgentThreadCollab(context.Background(), "testprj1", turn.ThreadID)
	if collab.ThreadKind != systemdb.ThreadKindAlert {
		t.Fatalf("thread kind: %+v", collab)
	}
	tasks, _ := h.store.ListAgentTasks(context.Background(), "testprj1", turn.ID)
	for _, task := range tasks {
		if task.TargetKind == systemdb.TaskTargetHuman {
			t.Fatalf("machine entry must not create human tasks: %+v", task)
		}
	}
}

func TestTurnEventsReplay(t *testing.T) {
	h, _, threadID, agents := newCollabEnv(t)
	db := agents["database"]
	h.createTurn(t, threadID, "查一下", []map[string]any{{"kind": "agent", "id": db.ID}}, "u1")
	turn, _ := lastTurn(t, h, threadID)
	events, err := h.store.ListAgentTurnEvents(context.Background(), "testprj1", turn.ID, 0, 100)
	if err != nil || len(events) == 0 {
		t.Fatalf("events: %v %d", err, len(events))
	}
	last := events[len(events)-1].Seq
	rec := h.req(http.MethodGet, "/agent-turns/"+turn.ID+"/events?after="+strconv.FormatInt(last, 10), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("events replay: %d %s", rec.Code, rec.Body.String())
	}
	// 终态轮次不再回放事件，只有 end 帧（SSE 文本，不是单个 JSON）。
	if !strings.Contains(rec.Body.String(), `"type":"end"`) {
		t.Fatalf("terminal replay should end the stream: %s", rec.Body.String())
	}
}

func TestCollabTurnResumeAfterInterrupt(t *testing.T) {
	h, _, threadID, agents := newCollabEnv(t)
	db := agents["database"]
	h.createTurn(t, threadID, "查看数据", []map[string]any{{"kind": "agent", "id": db.ID}}, "u1")
	turn, _ := lastTurn(t, h, threadID)
	// 模拟进程重启收敛：轮次与任务都被标 interrupted。
	if err := h.store.UpdateAgentTurnStatus(context.Background(), "testprj1", turn.ID,
		systemdb.TurnStatusInterrupted, "", "interrupted", 0); err != nil {
		t.Fatal(err)
	}
	tasks, _ := h.store.ListAgentTasks(context.Background(), "testprj1", turn.ID)
	for i := range tasks {
		tasks[i].Status = systemdb.TaskStatusInterrupted
		if _, err := h.store.UpdateAgentTask(context.Background(), tasks[i], 0); err != nil {
			t.Fatal(err)
		}
	}
	// resume 后任务重新执行并完成。
	rec := h.reqAs(http.MethodPost, "/agent-turns/"+turn.ID+"/actions",
		map[string]any{"action": "resume"}, userPrincipal("u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("resume: %d %s", rec.Code, rec.Body.String())
	}
	tasks, _ = h.store.ListAgentTasks(context.Background(), "testprj1", turn.ID)
	if len(tasks) != 1 || tasks[0].Status != systemdb.TaskStatusCompleted {
		t.Fatalf("task after resume: %+v", tasks)
	}
}

func TestCollabTurnCancelStopsTask(t *testing.T) {
	h, _, threadID, agents := newCollabEnv(t)
	db := agents["database"]
	h.createTurn(t, threadID, "查看数据", []map[string]any{{"kind": "agent", "id": db.ID}}, "u1")
	turn, _ := lastTurn(t, h, threadID)
	rec := h.reqAs(http.MethodPost, "/agent-turns/"+turn.ID+"/actions",
		map[string]any{"action": "cancel"}, userPrincipal("u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
	}
	turn, _ = lastTurn(t, h, threadID)
	if turn.Status != systemdb.TurnStatusCanceled {
		t.Fatalf("turn after cancel: %+v", turn)
	}
}

// auth_APIKey 返回 API Key 主体（用于「不能担任人」的用例）。
func auth_APIKey() auth.Principal {
	return auth.Principal{APIKeyID: "k1", TenantID: catalog.ReservedTenantID}
}
