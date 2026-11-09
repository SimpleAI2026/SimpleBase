package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// scheduleEcho 按 router.go 的兼容层形态挂载：读路由走真实 handler，写路由统一 Gone。
func scheduleEcho(store *systemdb.Store) *echo.Echo {
	e := echo.New()
	h := &agentScheduleHandler{store: store}
	e.GET("/agent-schedules", h.ListSchedules)
	e.POST("/agent-schedules", h.Gone)
	e.GET("/agent-schedules/:scheduleID", h.GetSchedule)
	e.PATCH("/agent-schedules/:scheduleID", h.Gone)
	e.DELETE("/agent-schedules/:scheduleID", h.Gone)
	e.GET("/agent-schedules/:scheduleID/runs", h.ListScheduleRuns)
	e.POST("/agent-schedules/:scheduleID/run", h.Gone)
	return e
}

func cloudAgentFixture() systemdb.CloudAgent {
	return systemdb.CloudAgent{
		ProjectID:   "testprj1",
		Name:        "Database",
		Module:      "database",
		Description: "test",
		ToolIDs:     []string{"list_databases"},
	}
}

func scheduleFixture(agentID string) systemdb.AgentSchedule {
	return systemdb.AgentSchedule{
		ProjectID: "testprj1",
		AgentID:   agentID,
		ThreadID:  "t",
		Prompt:    "p",
		CronExpr:  "*/15 * * * *",
		Enabled:   true,
		NextRunAt: time.Now().UTC().Add(time.Hour),
	}
}

// 历史归档数据仍可读：列表带 agent_name，详情/runs 可查，缺失返回 404。
func TestAgentScheduleReadOnlyCompat(t *testing.T) {
	store := openAgentStore(t)
	ctx := context.Background()
	e := scheduleEcho(store)

	agent, err := store.CreateCloudAgent(ctx, cloudAgentFixture())
	if err != nil {
		t.Fatal(err)
	}
	sc, err := store.CreateAgentSchedule(ctx, scheduleFixture(agent.ID))
	if err != nil {
		t.Fatal(err)
	}

	rec := agentReq(e, http.MethodGet, "/agent-schedules", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Schedules []agentScheduleDTO `json:"schedules"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Schedules) != 1 || listed.Schedules[0].AgentName != "Database" {
		t.Fatalf("listed = %+v", listed)
	}

	rec = agentReq(e, http.MethodGet, "/agent-schedules/"+sc.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got agentScheduleDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.ID != sc.ID || got.AgentName != "Database" || got.NextRunAt == nil {
		t.Fatalf("got = %+v", got)
	}

	rec = agentReq(e, http.MethodGet, "/agent-schedules/"+sc.ID+"/runs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("runs status=%d", rec.Code)
	}

	rec = agentReq(e, http.MethodGet, "/agent-schedules/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", rec.Code)
	}
	rec = agentReq(e, http.MethodGet, "/agent-schedules/nope/runs", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing runs status=%d", rec.Code)
	}
}

// 所有写入类旧路由统一返回 410 + agent_schedules_merged + migrated_to，且不改动归档数据。
func TestAgentScheduleWritesAreGone(t *testing.T) {
	store := openAgentStore(t)
	ctx := context.Background()
	e := scheduleEcho(store)

	agent, err := store.CreateCloudAgent(ctx, cloudAgentFixture())
	if err != nil {
		t.Fatal(err)
	}
	sc, err := store.CreateAgentSchedule(ctx, scheduleFixture(agent.ID))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/agent-schedules", map[string]any{"agent_id": agent.ID, "prompt": "p", "cron_expr": "0 8 * * *"}},
		{http.MethodPatch, "/agent-schedules/" + sc.ID, map[string]any{"prompt": "new"}},
		{http.MethodDelete, "/agent-schedules/" + sc.ID, nil},
		{http.MethodPost, "/agent-schedules/" + sc.ID + "/run", nil},
	}
	for _, tc := range cases {
		rec := agentReq(e, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusGone {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		var out struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
			MigratedTo string `json:"migrated_to"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.Error.Code != "agent_schedules_merged" || out.MigratedTo != "/cron-jobs" {
			t.Fatalf("%s %s body=%s", tc.method, tc.path, rec.Body.String())
		}
	}

	after, err := store.GetAgentSchedule(ctx, sc.ProjectID, sc.ID)
	if err != nil {
		t.Fatalf("archived schedule must stay readable: %v", err)
	}
	if after.Prompt != sc.Prompt {
		t.Fatalf("prompt mutated: %q", after.Prompt)
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
