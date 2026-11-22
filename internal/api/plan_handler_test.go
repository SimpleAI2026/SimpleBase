package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// 计划模式（planv6.7 cloud-agent-plan-mode）：创建、幂等、阶段校验与动作。

func starterAdminPrincipal() auth.Principal {
	return auth.Principal{
		APIKeyID: "k-admin", TenantID: catalog.ReservedTenantID,
		Permissions: map[auth.Permission]struct{}{auth.DatabaseAdmin: {}},
	}
}

func starterEnv(t *testing.T, skillsCLI, writable bool) (*echo.Echo, *systemdb.Store, systemdb.CloudAgent, systemdb.CloudAgent) {
	t.Helper()
	store := openAgentStore(t)
	if err := store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	var general, database systemdb.CloudAgent
	agents, err := store.ListCloudAgents(context.Background(), "testprj1")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range agents {
		switch a.BuiltinKey {
		case "general":
			general = a
		case "database":
			database = a
		}
	}
	if general.ID == "" || database.ID == "" {
		t.Fatalf("seed agents: general=%q database=%q", general.ID, database.ID)
	}
	w := writable
	h := &cloudAgentHandler{store: store, runtime: &cloudagent.Runtime{Provider: &agentbridge.RuntimeProvider{SkillsCLI: skillsCLI}}, writable: &w}
	e := echo.New()
	e.POST("/agent-threads", h.CreateThread)
	e.POST("/agent-threads/:threadID/runs", h.CreateRun)
	e.GET("/agent-threads/:threadID/plans", h.ListPlans)
	e.POST("/agent-threads/:threadID/plans", h.CreatePlan)
	e.GET("/agent-threads/:threadID/plans/:planID", h.GetPlan)
	e.POST("/agent-threads/:threadID/plans/:planID/actions", h.PlanAction)
	return e, store, general, database
}

func createStarterThread(t *testing.T, e *echo.Echo, agentID, _ string) string {
	t.Helper()
	rec := agentReqAs(e, http.MethodPost, "/agent-threads", map[string]any{"title": "s", "agent_id": agentID}, starterAdminPrincipal())
	if rec.Code != http.StatusCreated {
		t.Fatalf("thread: %d %s", rec.Code, rec.Body.String())
	}
	var th struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &th); err != nil {
		t.Fatal(err)
	}
	return th.ID
}

func errorCode(rec interface{ Bytes() []byte }) string {
	var body APIErrorBody
	_ = json.Unmarshal(rec.Bytes(), &body)
	return body.Error.Code
}

func TestPlanStagesFromBody(t *testing.T) {
	specs, err := planStagesFromBody("build a dashboard", nil)
	if err != nil || len(specs) != 6 {
		t.Fatalf("default stages: %v %v", specs, err)
	}
	for i, id := range agentbridge.PlanStageOrder() {
		if specs[i].ID != id || specs[i].Goal != "build a dashboard" {
			t.Fatalf("stage %d: %+v", i, specs[i])
		}
	}
	body := []planStageBody{
		{ID: "plan", Goal: "plan"},
		{ID: "database", Goal: "tables"},
		{ID: "kv", SkipReason: "no kv"},
		{ID: "functions", Goal: "fn"},
		{ID: "cron", SkipReason: "no cron"},
		{ID: "site", Goal: "site"},
	}
	if _, err := planStagesFromBody("x", body); err != nil {
		t.Fatalf("valid body rejected: %v", err)
	}
	bad := []planStageBody{{ID: "database", Goal: "x"}}
	if _, err := planStagesFromBody("x", bad); err == nil {
		t.Fatal("short body must be rejected")
	}
	badOrder := append([]planStageBody(nil), body...)
	badOrder[1], badOrder[2] = badOrder[2], badOrder[1]
	if _, err := planStagesFromBody("x", badOrder); err == nil {
		t.Fatal("wrong order must be rejected")
	}
}

func TestCreatePlanGuards(t *testing.T) {
	e, _, general, database := starterEnv(t, true, true)
	thread := createStarterThread(t, e, general.ID, "")
	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"missing goal", map[string]any{"agent_id": general.ID}, http.StatusBadRequest},
		{"unknown agent", map[string]any{"goal": "x", "agent_id": "nope"}, http.StatusBadRequest},
		{"agent mismatch", map[string]any{"goal": "x", "agent_id": database.ID}, http.StatusConflict},
		{"bad stages", map[string]any{"goal": "x", "agent_id": general.ID, "stages": []map[string]any{{"id": "database", "goal": "x"}}}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		rec := agentReqAs(e, http.MethodPost, "/agent-threads/"+thread+"/plans", tc.body, starterAdminPrincipal())
		if rec.Code != tc.want {
			t.Errorf("%s: got %d want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}
	// CLI 技能关闭时拒绝
	e2, _, general2, _ := starterEnv(t, false, true)
	thread2 := createStarterThread(t, e2, general2.ID, "")
	rec := agentReqAs(e2, http.MethodPost, "/agent-threads/"+thread2+"/plans", map[string]any{"goal": "x", "agent_id": general2.ID}, starterAdminPrincipal())
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("skills off: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePlanIdempotentAndSnapshot(t *testing.T) {
	e, store, general, _ := starterEnv(t, true, true)
	thread := createStarterThread(t, e, general.ID, "")
	body := map[string]any{"goal": "build", "agent_id": general.ID, "idempotency_key": "k1"}
	rec := agentReqAs(e, http.MethodPost, "/agent-threads/"+thread+"/plans", body, starterAdminPrincipal())
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var first planDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Stages) != 6 || first.Stages[0].Stage != "plan" || first.Stages[2].Stage != "kv" {
		t.Fatalf("stages: %+v", first.Stages)
	}
	// 同幂等键重复创建返回同一条计划
	rec = agentReqAs(e, http.MethodPost, "/agent-threads/"+thread+"/plans", body, starterAdminPrincipal())
	var again planDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &again)
	if again.ID != first.ID {
		t.Fatalf("idempotency: %s vs %s", again.ID, first.ID)
	}
	plans, err := store.ListAgentPlans(context.Background(), "testprj1", thread, 10)
	if err != nil || len(plans) != 1 {
		t.Fatalf("plans=%v err=%v", plans, err)
	}
	// 快照接口
	rec = agentReqAs(e, http.MethodGet, "/agent-threads/"+thread+"/plans/"+first.ID, nil, starterAdminPrincipal())
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	// 跨线程访问 404
	other := createStarterThread(t, e, general.ID, "")
	rec = agentReqAs(e, http.MethodGet, "/agent-threads/"+other+"/plans/"+first.ID, nil, starterAdminPrincipal())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross thread: %d", rec.Code)
	}
}

func TestPlanActionCancelAndSkip(t *testing.T) {
	e, store, general, _ := starterEnv(t, true, true)
	thread := createStarterThread(t, e, general.ID, "")
	plan, err := store.CreateAgentPlan(context.Background(), systemdb.AgentPlan{
		ProjectID: "testprj1", ThreadID: thread, AgentID: general.ID, Goal: "x",
		Status: systemdb.AgentPlanPaused, CreatedBy: principalIdentity(starterAdminPrincipal()),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 未知动作
	rec := agentReqAs(e, http.MethodPost, "/agent-threads/"+thread+"/plans/"+plan.ID+"/actions", map[string]any{"action": "nope"}, starterAdminPrincipal())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown action: %d %s", rec.Code, rec.Body.String())
	}
	// 取消
	rec = agentReqAs(e, http.MethodPost, "/agent-threads/"+thread+"/plans/"+plan.ID+"/actions", map[string]any{"action": "cancel"}, starterAdminPrincipal())
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
	}
	saved, err := store.GetAgentPlan(context.Background(), "testprj1", plan.ID)
	if err != nil || saved.Status != systemdb.AgentPlanCanceled {
		t.Fatalf("status=%q err=%v", saved.Status, err)
	}
	// 客户端拿着旧版本推进：返回冲突
	rec = agentReqAs(e, http.MethodPost, "/agent-threads/"+thread+"/plans/"+plan.ID+"/actions", map[string]any{"action": "cancel", "expected_version": plan.Version}, starterAdminPrincipal())
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale version: %d %s", rec.Code, rec.Body.String())
	}
}

func TestInterruptRunningPlans(t *testing.T) {
	_, store, general, _ := starterEnv(t, true, true)
	ctx := context.Background()
	plan, err := store.CreateAgentPlan(ctx, systemdb.AgentPlan{
		ProjectID: "testprj1", ThreadID: "t1", AgentID: general.ID, Goal: "x", Status: systemdb.AgentPlanRunning,
	}, []systemdb.AgentPlanStage{{Stage: "plan", Status: systemdb.AgentPlanStageRunning}})
	if err != nil {
		t.Fatal(err)
	}
	n, err := store.InterruptRunningPlans(ctx)
	if err != nil || n != 1 {
		t.Fatalf("interrupt n=%d err=%v", n, err)
	}
	saved, err := store.GetAgentPlan(ctx, "testprj1", plan.ID)
	if err != nil || saved.Status != systemdb.AgentPlanInterrupted {
		t.Fatalf("status=%q err=%v", saved.Status, err)
	}
	stages, err := store.ListAgentPlanStages(ctx, plan.ID)
	if err != nil || len(stages) != 1 || stages[0].Status != systemdb.AgentPlanStageFailed {
		t.Fatalf("stages=%+v err=%v", stages, err)
	}
}

func TestNextPlanStage(t *testing.T) {
	stages := []systemdb.AgentPlanStage{
		{Stage: "plan", Status: systemdb.AgentPlanStageDone},
		{Stage: "database", Status: systemdb.AgentPlanStageSkipped},
		{Stage: "kv", Status: systemdb.AgentPlanStagePending},
	}
	if got := nextPlanStage(stages); got == nil || got.Stage != "kv" {
		t.Fatalf("next=%+v", got)
	}
	for i := range stages {
		stages[i].Status = systemdb.AgentPlanStageDone
	}
	if got := nextPlanStage(stages); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestPlanStageMessage(t *testing.T) {
	msg := planStageMessage(agentbridge.PlanStageKV, "store counters")
	if !strings.Contains(msg, "KV") || !strings.Contains(msg, "store counters") {
		t.Fatalf("msg=%q", msg)
	}
}

type oneShotStream struct {
	content string
	done    bool
}

func (s *oneShotStream) Next() (string, bool, error) {
	if s.done {
		return "", true, io.EOF
	}
	s.done = true
	return s.content, true, nil
}

func (s *oneShotStream) Close() error { return nil }

type planStreamOnlyLLM struct {
	streamCalls int
}

func (l *planStreamOnlyLLM) Chat(context.Context, string, agentcore.ChatRequest) (agentcore.ChatResponse, error) {
	return agentcore.ChatResponse{}, errors.New("plan stage used non-streaming chat")
}

func (l *planStreamOnlyLLM) Stream(context.Context, string, agentcore.ChatRequest) (agentcore.TokenStream, error) {
	l.streamCalls++
	return &oneShotStream{content: `[
		{"id":"plan","goal":"build"},
		{"id":"database","goal":"tables"},
		{"id":"kv","skip_reason":"not needed"},
		{"id":"functions","goal":"api","depends_on":["database"]},
		{"id":"cron","skip_reason":"not needed"},
		{"id":"site","goal":"page","depends_on":["functions"]}
	]`}, nil
}

func TestRunPlanStageStreamsModelOutput(t *testing.T) {
	_, store, general, _ := starterEnv(t, true, true)
	ctx := context.Background()
	plan, err := store.CreateAgentPlan(ctx, systemdb.AgentPlan{
		ProjectID: "testprj1", ThreadID: "plan-stream-thread", AgentID: general.ID,
		Goal: "build", Status: systemdb.AgentPlanRunning,
	}, []systemdb.AgentPlanStage{{Stage: "plan", Status: systemdb.AgentPlanStagePending}})
	if err != nil {
		t.Fatal(err)
	}
	stages, err := store.ListAgentPlanStages(ctx, plan.ID)
	if err != nil || len(stages) != 1 {
		t.Fatalf("stages=%+v err=%v", stages, err)
	}
	llm := &planStreamOnlyLLM{}
	h := &cloudAgentHandler{store: store, runtime: &cloudagent.Runtime{LLM: llm, Provider: &agentbridge.RuntimeProvider{SkillsCLI: true}}}
	if ok := h.runPlanStage(ctx, "testprj1", plan.ThreadID, general.ID, plan.ID, stages[0], starterAdminPrincipal()); !ok {
		t.Fatal("planning stage failed")
	}
	if llm.streamCalls != 1 {
		t.Fatalf("stream calls=%d, want 1", llm.streamCalls)
	}
}
