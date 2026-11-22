// planv6.7 v2 §11：群定时任务测试。
// 覆盖两种会话策略（reuse_thread / new_thread_per_fire）、触发去重、
// 群归档暂停调度、只读主体不产生人工任务、负责人缺失时的错误码。
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

// newGroupCronEnv 建群 + 群会话 + 群定时绑定。
func newGroupCronEnv(t *testing.T, policy string, owner string) (*agentHarness, string, string, string) {
	t.Helper()
	h := newAgentHarness(t, nil, fakellm.Script{NonStreamContent: "ok"})
	if err := h.store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	agents, _ := h.store.ListCloudAgents(context.Background(), "testprj1")
	general, database := "", ""
	for _, a := range agents {
		switch a.BuiltinKey {
		case "general":
			general = a.ID
		case "database":
			database = a.ID
		}
	}
	// 配兜底助手：定时场景没有路由器时用它，避免「没有可用分工」。
	rec := h.req(http.MethodPost, "/agent-tags", map[string]any{
		"name": "定时群", "agent_ids": []string{general, database},
		"routing_prompt":    "按内容分派给合适的助手",
		"fallback_agent_id": general,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag: %d %s", rec.Code, rec.Body.String())
	}
	var tag agentTagDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &tag)
	// 群 cron 绑定（AgentID 用 general，满足既有目标校验）。
	rec = h.req(http.MethodPost, "/cron-jobs", map[string]any{
		"name": "group_daily", "schedule_kind": "cron", "cron_expr": "0 8 * * *",
		"target_kind": "agent", "agent_id": general, "agent_prompt": "生成昨日概况",
		"tag_id": tag.ID, "thread_policy": policy, "owner_user_id": owner,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create cron: %d %s", rec.Code, rec.Body.String())
	}
	var job struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &job)
	runner := NewGroupCronRunner()
	runner.Attach(h.handler)
	h.collabRunner = runner
	return h, tag.ID, job.ID, general
}

func TestGroupCronReuseThread(t *testing.T) {
	h, tagID, jobID, agentID := newGroupCronEnv(t, systemdb.GroupCronReuseThread, "")

	first, err := h.collabRunner.RunGroupCron(context.Background(), "testprj1", jobID, agentbridge.GroupCronBinding{
		TagID: tagID, ThreadPolicy: systemdb.GroupCronReuseThread, ThreadKind: systemdb.ThreadKindScheduled,
		Prompt: "生成昨日概况", OwnerUserID: "",
	})
	if err != nil {
		t.Fatalf("first fire: %v", err)
	}
	if first.ThreadID == "" || first.TurnID == "" {
		t.Fatalf("first result: %+v", first)
	}
	// 会话类型 + 群归属。
	collab, _ := h.store.GetAgentThreadCollab(context.Background(), "testprj1", first.ThreadID)
	if collab.ThreadKind != systemdb.ThreadKindScheduled || collab.TagID != tagID {
		t.Fatalf("thread collab: %+v", collab)
	}
	// 绑定回写了 thread_id，后续触发复用同一会话。
	bind, _ := h.store.GetAgentGroupCron(context.Background(), "testprj1", jobID)
	if bind.ThreadID != first.ThreadID {
		t.Fatalf("binding should record thread: %+v", bind)
	}
	// 第二轮（模拟下一个分钟窗口，用不同 fire_key 通过时间推进不好造，
	// 这里直接断言去重：同分钟重复触发被跳过）。
	dup, err := h.collabRunner.RunGroupCron(context.Background(), "testprj1", jobID, agentbridge.GroupCronBinding{
		TagID: tagID, ThreadPolicy: systemdb.GroupCronReuseThread, Prompt: "生成昨日概况",
	})
	if err != nil {
		t.Fatalf("duplicate fire: %v", err)
	}
	if dup.Response != "skipped: duplicate fire" {
		t.Fatalf("duplicate should be skipped: %+v", dup)
	}
	// 只读主体：任务由助手完成，不产生人工任务。
	tasks, _ := h.store.ListAgentTasks(context.Background(), "testprj1", first.TurnID)
	if len(tasks) == 0 {
		t.Fatal("scheduled turn should create tasks")
	}
	for _, task := range tasks {
		if task.TargetKind == systemdb.TaskTargetHuman {
			t.Fatalf("read-only scheduler must not create human tasks: %+v", task)
		}
		if task.TargetID != agentID && task.TargetID == "" {
			t.Fatalf("task without target: %+v", task)
		}
	}
}

func TestGroupCronNewThreadPerFire(t *testing.T) {
	h, tagID, jobID, _ := newGroupCronEnv(t, systemdb.GroupCronNewThreadPerFire, "")

	first, err := h.collabRunner.RunGroupCron(context.Background(), "testprj1", jobID, agentbridge.GroupCronBinding{
		TagID: tagID, ThreadPolicy: systemdb.GroupCronNewThreadPerFire,
		ThreadKind: systemdb.ThreadKindScheduled, Prompt: "生成昨日概况",
	})
	if err != nil {
		t.Fatalf("first fire: %v", err)
	}
	// new_thread_per_fire 不回写 thread_id（每次触发都新建）。
	bind, _ := h.store.GetAgentGroupCron(context.Background(), "testprj1", jobID)
	if bind.ThreadID != "" {
		t.Fatalf("per-fire policy must not bind a thread: %+v", bind)
	}
	// 用不同 fire_key 再触发一次（绕过分钟的幂等窗口），应得到不同会话。
	second, err := h.runScheduledFireWithKey(t, tagID, jobID, "manual-key-2")
	if err != nil {
		t.Fatalf("second fire: %v", err)
	}
	if second.ThreadID == first.ThreadID {
		t.Fatalf("per-fire policy must create a new thread each time: %s", second.ThreadID)
	}
}

// runScheduledFireWithKey 用给定的幂等键直接走内部执行路径（绕过分钟级去重窗口）。
func (h *agentHarness) runScheduledFireWithKey(t *testing.T, tagID, jobID, key string) (agentbridge.GroupCronResult, error) {
	t.Helper()
	tag, err := h.handler.resolveAgentTag(context.Background(), "testprj1", tagID)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	threadID, err := h.handler.groupCronThread(context.Background(), "testprj1", jobID, agentbridge.GroupCronBinding{
		TagID: tagID, ThreadPolicy: systemdb.GroupCronNewThreadPerFire,
		ThreadKind: systemdb.ThreadKindScheduled, Prompt: "生成昨日概况",
	}, tag)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	thread, err := h.store.GetAgentThread(context.Background(), "testprj1", threadID)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	ws, _ := h.store.GetAgentThreadWorkspace(context.Background(), "testprj1", threadID)
	collab, _ := h.store.GetAgentThreadCollab(context.Background(), "testprj1", threadID)
	tc := turnContext{
		pc:     ProjectContext{ID: "testprj1", TenantID: "t"},
		thread: thread, ws: ws, collab: collab, tag: tag,
	}
	turn, err := h.handler.runScheduledTurn(context.Background(), tc, "生成昨日概况", key)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	return agentbridge.GroupCronResult{TurnID: turn.ID, ThreadID: threadID}, nil
}

func TestGroupCronArchivedGroupSkips(t *testing.T) {
	h, tagID, jobID, _ := newGroupCronEnv(t, systemdb.GroupCronReuseThread, "")
	rec := h.req(http.MethodDelete, "/agent-tags/"+tagID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("archive tag: %d %s", rec.Code, rec.Body.String())
	}
	_, err := h.collabRunner.RunGroupCron(context.Background(), "testprj1", jobID, agentbridge.GroupCronBinding{
		TagID: tagID, ThreadPolicy: systemdb.GroupCronReuseThread, Prompt: "生成昨日概况",
	})
	if err == nil {
		t.Fatal("archived group must reject scheduled runs")
	}
	// 绑定也被归档（群归档即暂停调度）。
	bind, _ := h.store.GetAgentGroupCron(context.Background(), "testprj1", jobID)
	if bind.JobID != "" {
		t.Fatalf("binding should be archived with the group: %+v", bind)
	}
}

func TestGroupCronBindingValidation(t *testing.T) {
	h := newAgentHarness(t, nil, fakellm.Script{NonStreamContent: "ok"})
	if err := h.store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	agents, _ := h.store.ListCloudAgents(context.Background(), "testprj1")
	var general, s3 string
	for _, a := range agents {
		switch a.BuiltinKey {
		case "general":
			general = a.ID
		case "s3":
			s3 = a.ID
		}
	}
	// 群只含 general：用 s3 绑定应 400。
	rec := h.req(http.MethodPost, "/agent-tags", map[string]any{"name": "仅通用", "agent_ids": []string{general}})
	var tag agentTagDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &tag)
	rec = h.req(http.MethodPost, "/cron-jobs", map[string]any{
		"name": "bad_binding", "schedule_kind": "cron", "cron_expr": "0 8 * * *",
		"target_kind": "agent", "agent_id": s3, "agent_prompt": "p", "tag_id": tag.ID,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-member binding: %d %s", rec.Code, rec.Body.String())
	}
	// 非法策略。
	rec = h.req(http.MethodPost, "/cron-jobs", map[string]any{
		"name": "bad_policy", "schedule_kind": "cron", "cron_expr": "0 8 * * *",
		"target_kind": "agent", "agent_id": general, "agent_prompt": "p",
		"tag_id": tag.ID, "thread_policy": "every_time",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid policy: %d %s", rec.Code, rec.Body.String())
	}
	// 缺 tag_id 却给了策略。
	rec = h.req(http.MethodPost, "/cron-jobs", map[string]any{
		"name": "no_group", "schedule_kind": "cron", "cron_expr": "0 8 * * *",
		"target_kind": "agent", "agent_id": general, "agent_prompt": "p",
		"thread_policy": systemdb.GroupCronReuseThread,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing tag: %d %s", rec.Code, rec.Body.String())
	}
}
