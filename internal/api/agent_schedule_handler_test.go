package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func scheduleEcho(store *systemdb.Store) *echo.Echo {
	e := echo.New()
	h := &agentScheduleHandler{store: store}
	e.GET("/agent-schedules", h.ListSchedules)
	e.POST("/agent-schedules", h.CreateSchedule)
	e.GET("/agent-schedules/:scheduleID", h.GetSchedule)
	e.PATCH("/agent-schedules/:scheduleID", h.PatchSchedule)
	e.DELETE("/agent-schedules/:scheduleID", h.DeleteSchedule)
	e.GET("/agent-schedules/:scheduleID/runs", h.ListScheduleRuns)
	e.POST("/agent-schedules/:scheduleID/run", h.TriggerScheduleRun)
	return e
}

func cloudAgentFixture() systemdb.CloudAgent {
	return systemdb.CloudAgent{
		ProjectID:   catalog.DevProjectID,
		Name:        "Database",
		Module:      "database",
		Description: "test",
		ToolIDs:     []string{"list_databases"},
	}
}

func scheduleFixture(agentID string) systemdb.AgentSchedule {
	return systemdb.AgentSchedule{
		ProjectID: catalog.DevProjectID,
		AgentID:   agentID,
		ThreadID:  "t",
		Prompt:    "p",
		CronExpr:  "*/15 * * * *",
		Enabled:   true,
		NextRunAt: time.Now().UTC().Add(time.Hour),
	}
}

func TestAgentScheduleCRUD(t *testing.T) {
	store := openAgentStore(t)
	e := scheduleEcho(store)

	// 准备 agent 与线程。
	agent, err := store.CreateCloudAgent(context.Background(), cloudAgentFixture())
	if err != nil {
		t.Fatal(err)
	}

	// 空 prompt → 400。
	rec := agentReq(e, http.MethodPost, "/agent-schedules", map[string]any{
		"agent_id": agent.ID, "prompt": "  ", "cron_expr": "*/15 * * * *",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty prompt status=%d body=%s", rec.Code, rec.Body.String())
	}
	// 非法 cron → 400。
	rec = agentReq(e, http.MethodPost, "/agent-schedules", map[string]any{
		"agent_id": agent.ID, "prompt": "p", "cron_expr": "bad",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad cron status=%d body=%s", rec.Code, rec.Body.String())
	}
	// agent 不存在 → 400。
	rec = agentReq(e, http.MethodPost, "/agent-schedules", map[string]any{
		"agent_id": "nope", "prompt": "p", "cron_expr": "0 8 * * *",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing agent status=%d body=%s", rec.Code, rec.Body.String())
	}
	// 正常创建 → 201，附带 next_run_at 与自动创建的 thread。
	rec = agentReq(e, http.MethodPost, "/agent-schedules", map[string]any{
		"agent_id": agent.ID, "prompt": "check orders", "cron_expr": "0 8 * * *",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID       string  `json:"id"`
		ThreadID string  `json:"thread_id"`
		NextRun  *string `json:"next_run_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.ThreadID == "" || created.NextRun == nil {
		t.Fatalf("created = %+v", created)
	}

	// 重复创建 → 409。
	rec = agentReq(e, http.MethodPost, "/agent-schedules", map[string]any{
		"agent_id": agent.ID, "prompt": "again", "cron_expr": "0 8 * * *",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 列表含 1 条，带 agent_name。
	rec = agentReq(e, http.MethodGet, "/agent-schedules", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d", rec.Code)
	}
	var listed struct {
		Schedules []agentScheduleDTO `json:"schedules"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Schedules) != 1 || listed.Schedules[0].AgentName != "Database" {
		t.Fatalf("listed = %+v", listed)
	}

	// PATCH 更新 prompt + cron。
	rec = agentReq(e, http.MethodPatch, "/agent-schedules/"+created.ID, map[string]any{
		"prompt": "new prompt", "cron_expr": "*/30 * * * *",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", rec.Code, rec.Body.String())
	}
	var patched agentScheduleDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &patched)
	if patched.Prompt != "new prompt" || patched.CronExpr != "*/30 * * * *" {
		t.Fatalf("patched = %+v", patched)
	}

	// 停用后 next_run_at 清空。
	rec = agentReq(e, http.MethodPatch, "/agent-schedules/"+created.ID, map[string]any{
		"enabled": false,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", rec.Code, rec.Body.String())
	}
	var disabled agentScheduleDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &disabled)
	if disabled.Enabled || disabled.NextRunAt != nil {
		t.Fatalf("disabled = %+v", disabled)
	}

	// runs 列表 200。
	rec = agentReq(e, http.MethodGet, "/agent-schedules/"+created.ID+"/runs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("runs status=%d", rec.Code)
	}

	// 不存在的 schedule → 404。
	rec = agentReq(e, http.MethodGet, "/agent-schedules/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", rec.Code)
	}

	// 删除 → 204；再删 → 404。
	rec = agentReq(e, http.MethodDelete, "/agent-schedules/"+created.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = agentReq(e, http.MethodDelete, "/agent-schedules/"+created.ID, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("redelete status=%d", rec.Code)
	}
	// 软删后可重新创建。
	rec = agentReq(e, http.MethodPost, "/agent-schedules", map[string]any{
		"agent_id": agent.ID, "prompt": "again", "cron_expr": "0 8 * * *",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("recreate status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestScheduleTriggerWithoutScheduler(t *testing.T) {
	store := openAgentStore(t)
	e := scheduleEcho(store) // scheduler 为 nil

	agent, err := store.CreateCloudAgent(context.Background(), cloudAgentFixture())
	if err != nil {
		t.Fatal(err)
	}
	sc, err := store.CreateAgentSchedule(context.Background(), scheduleFixture(agent.ID))
	if err != nil {
		t.Fatal(err)
	}
	rec := agentReq(e, http.MethodPost, "/agent-schedules/"+sc.ID+"/run", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("trigger status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestScheduleStoreClaimSemantics(t *testing.T) {
	store := openAgentStore(t)
	ctx := context.Background()
	agent, err := store.CreateCloudAgent(ctx, cloudAgentFixture())
	if err != nil {
		t.Fatal(err)
	}
	sc, err := store.CreateAgentSchedule(ctx, scheduleFixture(agent.ID))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	ok, err := store.ClaimAgentSchedule(ctx, sc.ID, sc.NextRunAt, now.Add(15*time.Minute), now)
	if err != nil || !ok {
		t.Fatalf("first claim ok=%v err=%v", ok, err)
	}
	// 用旧值再 claim → false。
	ok, err = store.ClaimAgentSchedule(ctx, sc.ID, sc.NextRunAt, now.Add(30*time.Minute), now)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("second claim with stale next_run_at should fail")
	}
	// GetByAgent 唯一。
	got, err := store.GetAgentScheduleByAgent(ctx, sc.ProjectID, agent.ID)
	if err != nil || got.ID != sc.ID {
		t.Fatalf("GetByAgent = %+v err=%v", got, err)
	}
	// due 列表：把 next_run_at 拨到过去后出现。
	due, err := store.ListDueAgentSchedules(ctx, now.Add(2*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 {
		t.Fatalf("due = %d, want 1", len(due))
	}
}

func TestScheduleTriggerAcceptsWhenSchedulerSet(t *testing.T) {
	store := openAgentStore(t)
	ctx := context.Background()
	agent, err := store.CreateCloudAgent(ctx, cloudAgentFixture())
	if err != nil {
		t.Fatal(err)
	}
	sc, err := store.CreateAgentSchedule(ctx, scheduleFixture(agent.ID))
	if err != nil {
		t.Fatal(err)
	}
	// 用真实 store + 假 runner 构造 scheduler（runner 不应被调用：trigger 是异步的）。
	scheduler := newTestScheduler(store)
	e := echo.New()
	h := &agentScheduleHandler{store: store, scheduler: scheduler}
	e.POST("/agent-schedules/:scheduleID/run", h.TriggerScheduleRun)

	req := httptest.NewRequest(http.MethodPost, "/agent-schedules/"+sc.ID+"/run", nil)
	req = withProjectPrincipal(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("trigger status=%d body=%s", rec.Code, rec.Body.String())
	}
	scheduler.Stop()
}

// noopRunner 满足 ScheduleRunner，记录调用。
type noopRunner struct{}

func (noopRunner) StartRun(_ context.Context, _ cloudagent.RunRequest, _ func(cloudagent.Event)) (cloudagent.RunResult, error) {
	return cloudagent.RunResult{Content: "ok", ToolCallsJSON: "[]"}, nil
}

func newTestScheduler(store *systemdb.Store) *cloudagent.Scheduler {
	return cloudagent.NewScheduler(store, noopRunner{}, nil)
}

func withProjectPrincipal(r *http.Request) *http.Request {
	ctx := WithProject(r.Context(), ProjectContext{ID: catalog.DevProjectID, TenantID: catalog.ReservedTenantID})
	return r.WithContext(WithPrincipal(ctx, auth.Principal{APIKeyID: "k1", TenantID: catalog.ReservedTenantID}))
}
