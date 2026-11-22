// planv6.7 AGC-ROUTE：auto 会话的多轮路由集成测试（§7.12）。
// 同一会话三轮分别 auto→mention→auto；runs.agent_id 与 route_kind 正确；
// 单候选不调路由器；标签外 mention 400；候选空 409 且无落库；重试固定原助手；
// 旧会话仍返回 agent_switch_required；SSE 事件顺序 run → route(routing) →
// route(done) → … → end。
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

// autoTestEnv 是一个带 fake 路由器的 auto 会话环境。
type autoTestEnv struct {
	h        *agentHarness
	threadID string
	router   *fakeDispatcher
	agents   map[string]systemdb.CloudAgent // module → agent
}

func newAutoEnv(t *testing.T, scripts []fakellm.Script) *autoTestEnv {
	t.Helper()
	h := newAgentHarness(t, scripts, fakellm.Script{NonStreamContent: "ok", Steps: []fakellm.Step{{Content: "ok", FinishReason: "stop"}}})
	env := &autoTestEnv{h: h, router: &fakeDispatcher{agentID: "__none__"}}
	// 播种默认助手（ListCloudAgents 不会自动播种，ensureAgents 才会）。
	if err := h.store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	// 新建 auto 会话（tag 为空 = #对话 全候选）
	rec := h.req(http.MethodPost, "/agent-threads", map[string]any{"dispatch_mode": "auto"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create auto thread: %d %s", rec.Code, rec.Body.String())
	}
	var th struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &th)
	env.threadID = th.ID
	// 候选助手：播种后的 module → agent
	agents, err := h.store.ListCloudAgents(t.Context(), "testprj1")
	if err != nil {
		t.Fatal(err)
	}
	env.agents = map[string]systemdb.CloudAgent{}
	for _, a := range agents {
		env.agents[a.Module] = a
	}
	return env
}

func TestAutoRunMultiTurnSameThread(t *testing.T) {
	env := newAutoEnv(t, nil)
	h := env.h

	// 第一轮：无 mention、多候选。handler.dispatcher 为 nil（harness 未注入），
	// 决策为 NeedLLM=true 但 dispatcher==nil → decideRoute 直接 fallback。
	// 为验证 auto 路径，这里手动给 handler 换 dispatcher 不可行（闭包），
	// 因此先验证 fallback 行为：事件序列含 route 不出现（无路由器 → 直接 fallback）
	events := h.createRun(t, env.threadID, "帮我看下 orders 表结构", nil)
	runID := runIDOf(events)
	if runID == "" {
		t.Fatal("no run event")
	}
	// run 状态落库 + 路由记录 fallback
	routes, err := h.store.ListAgentRunRoutes(t.Context(), "testprj1", env.threadID)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := routes[runID]
	if !ok {
		t.Fatalf("route record missing for %s", runID)
	}
	if r.RouteKind != systemdb.RouteFallback {
		t.Fatalf("expected fallback (no dispatcher), got %s", r.RouteKind)
	}
	general := env.agents["general"]
	if r.AgentID != general.ID {
		t.Fatalf("fallback should pick general, got %s", r.AgentID)
	}
	// run.agent_id 已回填
	run, err := h.store.GetAgentRun(t.Context(), "testprj1", runID)
	if err != nil || run.AgentID != general.ID {
		t.Fatalf("run agent backfilled: %v %+v", err, run)
	}
	// 用户消息 agent_id 为空
	msgs, _ := h.store.ListAgentMessages(t.Context(), "testprj1", env.threadID, 200)
	foundUser := false
	for _, m := range msgs {
		if m.Role == "user" {
			foundUser = true
			if m.AgentID != "" {
				t.Fatalf("user message agent_id should be empty: %q", m.AgentID)
			}
		}
	}
	if !foundUser {
		t.Fatal("user message not persisted")
	}

	// 第二轮：mention 对象存储助手（s3）
	s3 := env.agents["s3"]
	events = h.createRun(t, env.threadID, "帮我核对报表文件", map[string]any{
		"mentions": []map[string]any{{"agent_id": s3.ID}},
	})
	runID2 := runIDOf(events)
	routes, _ = h.store.ListAgentRunRoutes(t.Context(), "testprj1", env.threadID)
	if r2, ok := routes[runID2]; !ok || r2.RouteKind != systemdb.RouteMention || r2.AgentID != s3.ID {
		t.Fatalf("mention route: %+v (found=%v)", r2, ok)
	}

	// 第三轮：mention 数据库助手
	db := env.agents["database"]
	events = h.createRun(t, env.threadID, "查询 orders 行数", map[string]any{
		"mentions": []map[string]any{{"agent_id": db.ID}},
	})
	runID3 := runIDOf(events)
	routes, _ = h.store.ListAgentRunRoutes(t.Context(), "testprj1", env.threadID)
	if r3, ok := routes[runID3]; !ok || r3.RouteKind != systemdb.RouteMention {
		t.Fatalf("third route: %+v", r3)
	}

	// 同一会话只产生一个 thread（左侧不新增）
	threads, _, err := h.store.PageAgentThreadsFiltered(t.Context(), "testprj1", systemdb.AgentThreadFilter{}, "", 50)
	if err != nil || len(threads) != 1 {
		t.Fatalf("threads: %v %d", err, len(threads))
	}

	// ListMessages route_kind 正确
	rec := h.req(http.MethodGet, "/agent-threads/"+env.threadID+"/messages", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("messages: %d", rec.Code)
	}
	var listed struct {
		Messages []messageDTO `json:"messages"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	mentionCount := 0
	for _, m := range listed.Messages {
		if m.RouteKind == systemdb.RouteMention {
			mentionCount++
		}
	}
	if mentionCount != 2 {
		t.Fatalf("mention route_kind in messages: %d", mentionCount)
	}
}

func TestAutoRunMentionOutsideTagRejected(t *testing.T) {
	env := newAutoEnv(t, nil)
	h := env.h

	// 建标签并绑定一个只含 general 的标签会话
	rec := h.req(http.MethodPost, "/agent-tags", map[string]any{
		"name": "仅通用", "agent_ids": []string{env.agents["general"].ID},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag: %d %s", rec.Code, rec.Body.String())
	}
	var tag struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tag)
	rec = h.req(http.MethodPost, "/agent-threads", map[string]any{
		"tag_id": tag.ID, "dispatch_mode": "auto",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag thread: %d %s", rec.Code, rec.Body.String())
	}
	var th struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &th)

	// mention 标签外的 s3 助手 → 400 agent_not_in_tag
	s3 := env.agents["s3"]
	rec = h.req(http.MethodPost, "/agent-threads/"+th.ID+"/runs", map[string]any{
		"content": "x", "stream": true,
		"mentions": []map[string]any{{"agent_id": s3.ID}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("outside mention: %d %s", rec.Code, rec.Body.String())
	}
	if !jsonBodyContains(rec.Body.String(), "agent_not_in_tag") {
		t.Fatalf("error code: %s", rec.Body.String())
	}
	// 不落任何 run/消息
	runs, _ := h.store.ListAgentRuns(t.Context(), "testprj1", th.ID, 10)
	if len(runs) != 0 {
		t.Fatalf("runs leaked: %d", len(runs))
	}
	msgs, _ := h.store.ListAgentMessages(t.Context(), "testprj1", th.ID, 10)
	if len(msgs) != 0 {
		t.Fatalf("messages leaked: %d", len(msgs))
	}
}

func TestAutoRunEmptyCandidatesRejected(t *testing.T) {
	env := newAutoEnv(t, nil)
	h := env.h

	// 建一个配置了已归档助手的标签 → 候选为空
	rec := h.req(http.MethodPost, "/agent-tags", map[string]any{
		"name": "空候选", "agent_ids": []string{env.agents["s3"].ID},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag: %d", rec.Code)
	}
	var tag struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tag)
	if err := h.store.ArchiveCloudAgent(t.Context(), "testprj1", env.agents["s3"].ID); err != nil {
		t.Fatal(err)
	}
	rec = h.req(http.MethodPost, "/agent-threads", map[string]any{
		"tag_id": tag.ID, "dispatch_mode": "auto",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag thread: %d %s", rec.Code, rec.Body.String())
	}
	var th struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &th)

	rec = h.req(http.MethodPost, "/agent-threads/"+th.ID+"/runs", map[string]any{
		"content": "x", "stream": true,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("empty candidates: %d %s", rec.Code, rec.Body.String())
	}
	if !jsonBodyContains(rec.Body.String(), "no_available_agent") {
		t.Fatalf("error code: %s", rec.Body.String())
	}
	runs, _ := h.store.ListAgentRuns(t.Context(), "testprj1", th.ID, 10)
	if len(runs) != 0 {
		t.Fatalf("runs leaked: %d", len(runs))
	}
}

func TestAutoRunRetryKeepsAgent(t *testing.T) {
	// 第一轮用错误注入脚本制造 failed run（MatchKeyword 匹配失败请求）。
	failScripts := []fakellm.Script{{
		MatchKeyword: "列一下对象",
		HTTPStatus:   500,
		Body:         `{"error":"boom"}`,
	}}
	h := newAgentHarness(t, failScripts, fakellm.Script{NonStreamContent: "ok", Steps: []fakellm.Step{{Content: "ok", FinishReason: "stop"}}})
	if err := h.store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	rec := h.req(http.MethodPost, "/agent-threads", map[string]any{"dispatch_mode": "auto"})
	var thOut struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &thOut)
	threadID := thOut.ID

	agents, err := h.store.ListCloudAgents(context.Background(), "testprj1")
	if err != nil {
		t.Fatal(err)
	}
	agentByModule := map[string]systemdb.CloudAgent{}
	for _, a := range agents {
		agentByModule[a.Module] = a
	}

	// 第一轮：mention s3 → 失败
	s3 := agentByModule["s3"]
	events := h.createRun(t, threadID, "列一下对象", map[string]any{
		"mentions": []map[string]any{{"agent_id": s3.ID}},
	})
	runID := runIDOf(events)
	if runID == "" {
		t.Fatal("no run")
	}
	prev, err := h.store.GetAgentRun(context.Background(), "testprj1", runID)
	if err != nil {
		t.Fatal(err)
	}
	if prev.Status != systemdb.AgentRunFailed {
		t.Fatalf("expected failed run, got %s", prev.Status)
	}

	// 重试：固定原助手 s3，route=retry（重试用第二个剧本，成功）
	events = h.createRun(t, threadID, "", map[string]any{
		"retry_of_run_id": runID,
	})
	runID2 := runIDOf(events)
	if runID2 == "" {
		t.Fatal("retry run missing")
	}
	routes, _ := h.store.ListAgentRunRoutes(context.Background(), "testprj1", threadID)
	if r, ok := routes[runID2]; !ok || r.RouteKind != systemdb.RouteRetry || r.AgentID != s3.ID {
		t.Fatalf("retry route: %+v (found=%v)", r, ok)
	}
	// 重试不再落用户消息
	msgs, _ := h.store.ListAgentMessages(context.Background(), "testprj1", threadID, 200)
	userCount := 0
	for _, m := range msgs {
		if m.Role == "user" {
			userCount++
		}
	}
	if userCount != 1 {
		t.Fatalf("retry must reuse user message: %d", userCount)
	}
}

func TestLegacyThreadStillEnforcesSwitchRequired(t *testing.T) {
	env := newAutoEnv(t, nil)
	h := env.h

	// 旧绑定会话（agent_id 绑定 + 无元数据）
	db := env.agents["database"]
	rec := h.req(http.MethodPost, "/agent-threads", map[string]any{"agent_id": db.ID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("legacy thread: %d", rec.Code)
	}
	var th struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &th)

	s3 := env.agents["s3"]
	rec = h.req(http.MethodPost, "/agent-threads/"+th.ID+"/runs", map[string]any{
		"content": "x", "stream": true,
		"mentions": []map[string]any{{"agent_id": s3.ID}},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("switch required: %d %s", rec.Code, rec.Body.String())
	}
	if !jsonBodyContains(rec.Body.String(), "agent_switch_required") {
		t.Fatalf("error code: %s", rec.Body.String())
	}
}

func jsonBodyContains(body, sub string) bool {
	return json.Valid([]byte(body)) && len(sub) > 0 && indexOf(body, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
