// 文件说明：agent_schedule_handler.go — internal/api 的内部实现（agentScheduleHandler、agentScheduleDTO 等）
//
// 用法
//
//	type agentScheduleHandler（包内使用）
//	type agentScheduleDTO（包内使用）
//	同包调用：agentScheduleHandler → router.go；ListSchedules() → router.go；GetSchedule() → router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/agent_schedule_handler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/systemdb
//	[同包] adapter.go, agent_handler.go, api_key_policy.go, api_keys_handler.go, auth_handler.go 等 59 个
//	[关键引用] agentScheduleHandler ← router.go
//
// 主要符号
//
//	type agentScheduleHandler struct {…}
//	type agentScheduleDTO struct {…}
//	type agentScheduleRunDTO struct {…}
//	toAgentScheduleDTO(sc systemdb.AgentSchedule, agentName string) agentScheduleDTO
//	toAgentScheduleRunDTO(r systemdb.AgentScheduleRun) agentScheduleRunDTO
//	(h *agentScheduleHandler) ListSchedules(c echo.Context) error
//	... 另有 4 个顶层符号

package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// agentScheduleHandler 是迁移期只读兼容层（planv6.0 gocloudagent-cli-and-cronjob-unification-plan §2.6）。
// Agent 定时任务已统一到 /cron-jobs（target_kind=agent）；sys_agent_schedules 冻结为归档表，
// 本 handler 只提供历史数据的读取，所有写入与手动触发返回 410 Gone。
type agentScheduleHandler struct {
	store *systemdb.Store
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

// Gone 处理所有写入类旧路由：Agent 定时任务已合并到 /cron-jobs。
func (h *agentScheduleHandler) Gone(c echo.Context) error {
	return c.JSON(http.StatusGone, map[string]any{
		"error": map[string]any{
			"code":       "agent_schedules_merged",
			"message":    "agent schedules have been merged into cron jobs; use /cron-jobs with target_kind=agent",
			"request_id": RequestIDFromContext(c.Request().Context()),
		},
		"migrated_to": "/cron-jobs",
	})
}

func (h *agentScheduleHandler) agentName(ctx context.Context, projectID, agentID string) string {
	a, err := h.store.GetCloudAgent(ctx, projectID, agentID)
	if err != nil {
		return ""
	}
	return a.Name
}
