package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/crontab"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// agentScheduleHandler 提供 Cloud Agent 定时执行 CRUD 与手动触发。
type agentScheduleHandler struct {
	store     *systemdb.Store
	scheduler *cloudagent.Scheduler
	usage     UsageService
}

type agentScheduleDTO struct {
	ID        string     `json:"id"`
	AgentID   string     `json:"agent_id"`
	AgentName string     `json:"agent_name,omitempty"`
	ThreadID  string     `json:"thread_id"`
	Prompt    string     `json:"prompt"`
	CronExpr  string     `json:"cron_expr"`
	Enabled   bool       `json:"enabled"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
	CreatedBy string     `json:"created_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type agentScheduleRunDTO struct {
	ID         string     `json:"id"`
	ScheduleID string     `json:"schedule_id"`
	RunID      string     `json:"run_id"`
	Trigger    string     `json:"trigger"`
	Status     string     `json:"status"`
	Error      string     `json:"error,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func toAgentScheduleDTO(sc systemdb.AgentSchedule, agentName string) agentScheduleDTO {
	dto := agentScheduleDTO{
		ID: sc.ID, AgentID: sc.AgentID, AgentName: agentName, ThreadID: sc.ThreadID,
		Prompt: sc.Prompt, CronExpr: sc.CronExpr, Enabled: sc.Enabled,
		CreatedBy: sc.CreatedBy, CreatedAt: sc.CreatedAt, UpdatedAt: sc.UpdatedAt,
	}
	if !sc.LastRunAt.IsZero() {
		t := sc.LastRunAt
		dto.LastRunAt = &t
	}
	if !sc.NextRunAt.IsZero() {
		t := sc.NextRunAt
		dto.NextRunAt = &t
	}
	return dto
}

func toAgentScheduleRunDTO(r systemdb.AgentScheduleRun) agentScheduleRunDTO {
	dto := agentScheduleRunDTO{
		ID: r.ID, ScheduleID: r.ScheduleID, RunID: r.RunID,
		Trigger: r.Trigger, Status: r.Status, Error: r.Error, CreatedAt: r.CreatedAt,
	}
	if !r.StartedAt.IsZero() {
		t := r.StartedAt
		dto.StartedAt = &t
	}
	if !r.FinishedAt.IsZero() {
		t := r.FinishedAt
		dto.FinishedAt = &t
	}
	return dto
}

type upsertScheduleBody struct {
	AgentID  string `json:"agent_id"`
	Prompt   string `json:"prompt"`
	CronExpr string `json:"cron_expr"`
	Enabled  *bool  `json:"enabled"`
	ThreadID string `json:"thread_id"`
}

func (h *agentScheduleHandler) ListSchedules(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	list, err := h.store.ListAgentSchedules(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	agents, _ := h.store.ListCloudAgents(c.Request().Context(), pc.ID)
	names := map[string]string{}
	for _, a := range agents {
		names[a.ID] = a.Name
	}
	out := make([]agentScheduleDTO, 0, len(list))
	for _, sc := range list {
		out = append(out, toAgentScheduleDTO(sc, names[sc.AgentID]))
	}
	return c.JSON(http.StatusOK, map[string]any{"schedules": out})
}

func (h *agentScheduleHandler) CreateSchedule(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	var body upsertScheduleBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	if strings.TrimSpace(body.AgentID) == "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "agent_id is required"))
	}
	if strings.TrimSpace(body.Prompt) == "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "prompt is required"))
	}
	spec, err := crontab.ParseCron(body.CronExpr)
	if err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid cron expression"))
	}
	ctx := c.Request().Context()
	agent, err := h.store.GetCloudAgent(ctx, pc.ID, body.AgentID)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "agent not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	if existing, err := h.store.GetAgentScheduleByAgent(ctx, pc.ID, agent.ID); err == nil && existing.ID != "" {
		return WriteError(c, echo.NewHTTPError(http.StatusConflict, "schedule already exists"))
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, err)
	}

	threadID := strings.TrimSpace(body.ThreadID)
	if threadID != "" {
		if _, err := h.store.GetAgentThread(ctx, pc.ID, threadID); errors.Is(err, sql.ErrNoRows) {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "thread not found"))
		} else if err != nil {
			return WriteError(c, err)
		}
	} else {
		th, err := h.store.CreateAgentThread(ctx, systemdb.AgentThread{
			ProjectID: pc.ID, Title: "Scheduled: " + agent.Name, CreatedBy: scheduleCreatedBy(c),
		})
		if err != nil {
			return WriteError(c, err)
		}
		threadID = th.ID
	}

	next, err := spec.NextAfter(time.Now().UTC())
	if err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid cron expression"))
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	created, err := h.store.CreateAgentSchedule(ctx, systemdb.AgentSchedule{
		ProjectID: pc.ID, AgentID: agent.ID, ThreadID: threadID,
		Prompt: strings.TrimSpace(body.Prompt), CronExpr: body.CronExpr,
		Enabled: enabled, NextRunAt: next, CreatedBy: scheduleCreatedBy(c),
	})
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusCreated, toAgentScheduleDTO(created, agent.Name))
}

func (h *agentScheduleHandler) GetSchedule(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	sc, err := h.store.GetAgentSchedule(c.Request().Context(), pc.ID, c.Param("scheduleID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "schedule not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toAgentScheduleDTO(sc, h.agentName(c.Request().Context(), pc.ID, sc.AgentID)))
}

func (h *agentScheduleHandler) PatchSchedule(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	ctx := c.Request().Context()
	cur, err := h.store.GetAgentSchedule(ctx, pc.ID, c.Param("scheduleID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "schedule not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	var body upsertScheduleBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	next := cur
	if strings.TrimSpace(body.Prompt) != "" {
		next.Prompt = strings.TrimSpace(body.Prompt)
	}
	cronChanged := strings.TrimSpace(body.CronExpr) != "" && strings.TrimSpace(body.CronExpr) != cur.CronExpr
	if cronChanged {
		next.CronExpr = strings.TrimSpace(body.CronExpr)
	}
	if body.Enabled != nil {
		next.Enabled = *body.Enabled
	}
	if strings.TrimSpace(body.ThreadID) != "" && strings.TrimSpace(body.ThreadID) != cur.ThreadID {
		threadID := strings.TrimSpace(body.ThreadID)
		if _, err := h.store.GetAgentThread(ctx, pc.ID, threadID); errors.Is(err, sql.ErrNoRows) {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "thread not found"))
		} else if err != nil {
			return WriteError(c, err)
		}
		next.ThreadID = threadID
	}
	// cron 变更或重新启用时重算 next_run_at；停用时清空。
	switch {
	case !next.Enabled:
		next.NextRunAt = time.Time{}
	case cronChanged || (next.Enabled && cur.NextRunAt.IsZero()):
		spec, err := crontab.ParseCron(next.CronExpr)
		if err != nil {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid cron expression"))
		}
		n, err := spec.NextAfter(time.Now().UTC())
		if err != nil {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid cron expression"))
		}
		next.NextRunAt = n
	}
	updated, err := h.store.UpdateAgentSchedule(ctx, next)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toAgentScheduleDTO(updated, h.agentName(ctx, pc.ID, updated.AgentID)))
}

func (h *agentScheduleHandler) DeleteSchedule(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	if err := h.store.ArchiveAgentSchedule(c.Request().Context(), pc.ID, c.Param("scheduleID")); errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "schedule not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *agentScheduleHandler) ListScheduleRuns(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	if _, err := h.store.GetAgentSchedule(c.Request().Context(), pc.ID, c.Param("scheduleID")); errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "schedule not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	runs, err := h.store.ListAgentScheduleRuns(c.Request().Context(), pc.ID, c.Param("scheduleID"), 20)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]agentScheduleRunDTO, 0, len(runs))
	for _, r := range runs {
		out = append(out, toAgentScheduleRunDTO(r))
	}
	return c.JSON(http.StatusOK, map[string]any{"runs": out})
}

func (h *agentScheduleHandler) TriggerScheduleRun(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	sc, err := h.store.GetAgentSchedule(c.Request().Context(), pc.ID, c.Param("scheduleID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "schedule not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	if h.scheduler == nil {
		return WriteError(c, echo.NewHTTPError(http.StatusServiceUnavailable, "agent scheduler is not configured"))
	}
	if h.usage != nil {
		if err := h.usage.CheckQuota(c.Request().Context(), pc.ID, "llm"); err != nil {
			return WriteError(c, err)
		}
	}
	// 异步触发；结果通过 GET runs 轮询。
	scheduleCtx := c.Request().Context()
	go func(sc systemdb.AgentSchedule) {
		_, _ = h.scheduler.Trigger(context.WithoutCancel(scheduleCtx), sc)
	}(sc)
	return c.JSON(http.StatusAccepted, map[string]any{"status": "triggered"})
}

func (h *agentScheduleHandler) agentName(ctx context.Context, projectID, agentID string) string {
	a, err := h.store.GetCloudAgent(ctx, projectID, agentID)
	if err != nil {
		return ""
	}
	return a.Name
}

func scheduleCreatedBy(c echo.Context) string {
	if p, ok := PrincipalFromContext(c.Request().Context()); ok {
		return p.APIKeyID
	}
	return ""
}
