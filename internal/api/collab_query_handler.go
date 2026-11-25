// 文件说明：collab_query_handler.go — internal/api 中 cloudAgentHandler.ListThreadTurns、cloudAgentHandler.GetTurn 等符号的实现
//
// 用法
//
//	(h *cloudAgentHandler) ListThreadTurns(c echo.Context) error  // ListThreadTurns 返回会话的轮次列表（GET /agent-threads/:threadID/turns）
//	(h *cloudAgentHandler) GetTurn(c echo.Context) error  // GetTurn 返回单个轮次快照（GET /agent-turns/:turnID）
//	(h *cloudAgentHandler) TurnEvents(c echo.Context) error  // TurnEvents 是轮次事件的 SSE：先按 after 回放事件表，再等待新事件或结束
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/collab_query_handler.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	type turnDTO struct{…}
//	type taskDTO struct{…}
//	toTaskDTO(t systemdb.AgentTask) taskDTO
//	(h *cloudAgentHandler) buildTurnDTO(ctx context.Context, projectID string, turn systemdb.AgentTurn) (turnDTO, ...
//	(h *cloudAgentHandler) ListThreadTurns(c echo.Context) error
//	(h *cloudAgentHandler) GetTurn(c echo.Context) error
//	(h *cloudAgentHandler) TurnEvents(c echo.Context) error
//	terminalTurn(status string) bool

//
// 主要符号
//
//	type turnDTO struct {…}
//	type taskDTO struct {…}
//	func toTaskDTO(t systemdb.AgentTask) taskDTO
//	func (h *cloudAgentHandler) buildTurnDTO(ctx context.Context, projectID string, turn systemdb.AgentTurn) (turn
//	func (h *cloudAgentHandler) replayTurnSnapshot(c echo.Context, projectID string, turn systemdb.AgentTurn) erro
//	func (h *cloudAgentHandler) respondTurnSnapshot(c echo.Context, projectID string, tc turnContext) error
//	func (h *cloudAgentHandler) ListThreadTurns(c echo.Context) error
//	func (h *cloudAgentHandler) GetTurn(c echo.Context) error
//	... 另有 2 个顶层符号

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// turnDTO 是轮次的响应形态（含任务与进度）。
type turnDTO struct {
	ID        string          `json:"id"`
	ThreadID  string          `json:"thread_id"`
	TagID     string          `json:"tag_id,omitempty"`
	Status    string          `json:"status"`
	ErrorCode string          `json:"error_code,omitempty"`
	RouteJSON string          `json:"route,omitempty"`
	Plan      json.RawMessage `json:"plan,omitempty"`
	Version   int64           `json:"version"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Tasks     []taskDTO       `json:"tasks"`
	// LastEventSeq 是事件表里的最大序号，供客户端续传。
	LastEventSeq int64 `json:"last_event_seq"`
}

// taskDTO 是任务的响应形态。
type taskDTO struct {
	ID             string    `json:"id"`
	TurnID         string    `json:"turn_id"`
	Seq            int64     `json:"seq"`
	Ref            string    `json:"ref,omitempty"`
	TargetKind     string    `json:"target_kind"`
	TargetID       string    `json:"target_id"`
	Instruction    string    `json:"instruction"`
	Status         string    `json:"status"`
	Attempt        int64     `json:"attempt"`
	RunID          string    `json:"run_id,omitempty"`
	AssigneeID     string    `json:"assignee_id,omitempty"`
	ReplyText      string    `json:"reply_text,omitempty"`
	ReplyOutcome   string    `json:"reply_outcome,omitempty"`
	RepliedBy      string    `json:"replied_by,omitempty"`
	DependsOn      []string  `json:"depends_on"`
	ContextFrom    []string  `json:"context_from,omitempty"`
	StepID         string    `json:"step_id,omitempty"`
	Risk           string    `json:"risk,omitempty"`
	ExpectedOutput string    `json:"expected_output,omitempty"`
	ErrorCode      string    `json:"error_code,omitempty"`
	Result         string    `json:"result,omitempty"`
	Version        int64     `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
}

func toTaskDTO(t systemdb.AgentTask) taskDTO {
	return taskDTO{
		ID: t.ID, TurnID: t.TurnID, Seq: t.Seq, TargetKind: t.TargetKind, TargetID: t.TargetID,
		Instruction: t.Instruction, Status: t.Status, Attempt: t.Attempt, RunID: t.RunID,
		AssigneeID: t.AssigneeID, ReplyText: t.ReplyText, ReplyOutcome: t.ReplyOutcome,
		RepliedBy: t.RepliedBy, DependsOn: systemdb.UnmarshalAgentTaskDepends(t.DependsJSON),
		ErrorCode: t.ErrorCode, Version: t.Version, CreatedAt: t.CreatedAt,
	}
}

// buildTurnDTO 组装轮次快照（含任务与事件序号）。
func (h *cloudAgentHandler) buildTurnDTO(ctx context.Context, projectID string, turn systemdb.AgentTurn) (turnDTO, error) {
	tasks, err := h.store.ListAgentTasks(ctx, projectID, turn.ID)
	if err != nil {
		return turnDTO{}, err
	}
	dto := turnDTO{
		ID: turn.ID, ThreadID: turn.ThreadID, TagID: turn.TagID, Status: turn.Status,
		ErrorCode: turn.ErrorCode, RouteJSON: turn.RouteJSON, Version: turn.Version,
		CreatedAt: turn.CreatedAt, UpdatedAt: turn.UpdatedAt,
		Tasks: make([]taskDTO, 0, len(tasks)),
	}
	groupTasks, groupErr := h.svc.LoadTurnTasks(ctx, projectID, turn.ID)
	byID := make(map[string]group.Task, len(groupTasks))
	for _, task := range groupTasks {
		byID[task.ID] = task
	}
	for _, t := range tasks {
		item := toTaskDTO(t)
		if groupErr == nil {
			if current, ok := byID[t.ID]; ok {
				item.Ref = current.Ref
				item.Result = current.Result
				item.DependsOn = current.DependsOn
				item.ContextFrom = current.ContextFrom
				item.StepID = current.StepID
				item.Risk = current.Risk
				item.ExpectedOutput = current.ExpectedOutput
			}
		}
		dto.Tasks = append(dto.Tasks, item)
	}
	if planJSON, ok := h.svc.TurnPlan(turn); ok {
		dto.Plan = planJSON
	}
	if seq, err := h.store.LastAgentTurnSeq(ctx, projectID, turn.ID); err == nil {
		dto.LastEventSeq = seq
	}
	return dto, nil
}

// ListThreadTurns 返回会话的轮次列表（GET /agent-threads/:threadID/turns）。
func (h *cloudAgentHandler) ListThreadTurns(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	threadID := c.Param("threadID")
	if _, err := h.store.GetAgentThread(c.Request().Context(), pc.ID, threadID); errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	limit := 20
	list, err := h.store.ListAgentTurns(c.Request().Context(), pc.ID, threadID, limit)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]turnDTO, 0, len(list))
	for _, turn := range list {
		dto, err := h.buildTurnDTO(c.Request().Context(), pc.ID, turn)
		if err != nil {
			return WriteError(c, err)
		}
		out = append(out, dto)
	}
	return c.JSON(http.StatusOK, map[string]any{"turns": out})
}

// GetTurn 返回单个轮次快照（GET /agent-turns/:turnID）。
func (h *cloudAgentHandler) GetTurn(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	turn, err := h.store.GetAgentTurn(c.Request().Context(), pc.ID, c.Param("turnID"))
	if errors.Is(err, systemdb.ErrTurnNotFound) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "turn not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	dto, err := h.buildTurnDTO(c.Request().Context(), pc.ID, turn)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, dto)
}

// TurnEvents 是轮次事件的 SSE：先按 after 回放事件表，再等待新事件或结束。
func (h *cloudAgentHandler) TurnEvents(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	turn, err := h.store.GetAgentTurn(c.Request().Context(), pc.ID, c.Param("turnID"))
	if errors.Is(err, systemdb.ErrTurnNotFound) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "turn not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	after, _ := strconv.ParseInt(c.QueryParam("after"), 10, 64)
	return h.streamGroupTurn(c, pc.ID, turn.ID, int(after))
}

// terminalTurn 报告轮次是否已经结束（无需保持 SSE）。
func terminalTurn(status string) bool {
	switch status {
	case systemdb.TurnStatusCompleted, systemdb.TurnStatusFailed,
		systemdb.TurnStatusCanceled, systemdb.TurnStatusInterrupted, group.TurnRejected:
		return true
	}
	return false
}
