// 文件说明：agent_runs.go — 云助手运行记录（AgentRun）的持久化
//
// 用法
//
//	AgentRun、CreateAgentRun、GetAgentRun（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] godatabase/bench, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/agent_runs.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/ducklake, godatabase/limit, internal/utils/strutil
//	[同包] async_flush.go, bootstrap.go, cron_jobs.go, function_triggers.go 等 22 个
//
// 主要符号
//
//	type AgentRun struct {…}
//	func (s *Store) CreateAgentRun(ctx context.Context, r AgentRun) (AgentRun, error)
//	func (s *Store) GetAgentRun(ctx context.Context, projectID, id string) (AgentRun, error)
//	func (s *Store) ListAgentRuns(ctx context.Context, projectID, threadID string, limit int) ([]AgentRun, error)
//	func (s *Store) ListActiveAgentRuns(ctx context.Context, projectID string) ([]AgentRun, error)
//	func (s *Store) UpdateAgentRunMetrics(ctx context.Context, projectID, id string, r AgentRun) error
//	func (s *Store) UpdateAgentRunStatus(ctx context.Context, projectID, id, status, errMsg string, started, finis

package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	pagelimit "github.com/linkxzhou/SimpleBase/godatabase/limit"
)

// AgentRun 是 sys_agent_runs 一行。
type AgentRun struct {
	ID               string
	ThreadID         string
	ProjectID        string
	AgentID          string
	Status           string
	Error            string
	StartedAt        time.Time
	FinishedAt       time.Time
	CreatedAt        time.Time
	DurationMS       int64
	PromptTokens     int
	CompletionTokens int
	ReasoningTokens  int
	ToolCalls        int
	ErrorCode        string
	RetryOfRunID     string
	// InitiatorID 是发起人标识，取 auth.Principal.Actor()（user:<id> / key:<id>）；
	// 升级前的历史行保持旧格式或空串。
	InitiatorID string
}

// CreateAgentRun 写入 queued/running 记录。
func (s *Store) CreateAgentRun(ctx context.Context, r AgentRun) (AgentRun, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return AgentRun{}, connErr
	}

	now := time.Now().UTC()
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	if r.Status == "" {
		r.Status = AgentRunQueued
	}
	r.CreatedAt = now
	var started any
	if !r.StartedAt.IsZero() {
		started = r.StartedAt.UTC()
	}
	_, err := sqlDB.ExecContext(ctx,
		`INSERT INTO sys_agent_runs(id, thread_id, project_id, agent_id, status, error, started_at, finished_at, created_at, initiator_id)
		 VALUES(?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
		r.ID, r.ThreadID, r.ProjectID, r.AgentID, r.Status, r.Error, started, now, r.InitiatorID)
	if err != nil {
		return AgentRun{}, err
	}
	s.notifyWrite(ctx)
	return r, nil
}

// GetAgentRun 按 ID 取 run（必须属于 project）。
func (s *Store) GetAgentRun(ctx context.Context, projectID, id string) (AgentRun, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return AgentRun{}, connErr
	}

	var r AgentRun
	var started, finished sql.NullTime
	err := sqlDB.QueryRowContext(ctx,
		`SELECT `+agentRunColumns+`
		 FROM sys_agent_runs WHERE id = ? AND project_id = ?`,
		id, projectID).Scan(&r.ID, &r.ThreadID, &r.ProjectID, &r.AgentID, &r.Status, &r.Error, &started, &finished, &r.CreatedAt, &r.DurationMS, &r.PromptTokens, &r.CompletionTokens, &r.ReasoningTokens, &r.ToolCalls, &r.ErrorCode, &r.InitiatorID)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentRun{}, sql.ErrNoRows
	}
	if err != nil {
		return AgentRun{}, err
	}
	if started.Valid {
		r.StartedAt = started.Time
	}
	if finished.Valid {
		r.FinishedAt = finished.Time
	}
	return r, nil
}

// ListAgentRuns 列出项目内某个会话最近的运行记录。
func (s *Store) ListAgentRuns(ctx context.Context, projectID, threadID string, limit int) ([]AgentRun, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return nil, connErr
	}

	limit = pagelimit.Normalize(limit, 20, 100)
	rows, err := sqlDB.QueryContext(ctx, `SELECT `+agentRunColumns+` FROM sys_agent_runs WHERE project_id=? AND thread_id=? ORDER BY created_at DESC, id DESC LIMIT ?`, projectID, threadID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentRun, 0)
	for rows.Next() {
		var r AgentRun
		var start, finish sql.NullTime
		if err := rows.Scan(&r.ID, &r.ThreadID, &r.ProjectID, &r.AgentID, &r.Status, &r.Error, &start, &finish, &r.CreatedAt, &r.DurationMS, &r.PromptTokens, &r.CompletionTokens, &r.ReasoningTokens, &r.ToolCalls, &r.ErrorCode, &r.InitiatorID); err != nil {
			return nil, err
		}
		if start.Valid {
			r.StartedAt = start.Time
		}
		if finish.Valid {
			r.FinishedAt = finish.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListActiveAgentRuns 列出项目内状态仍为 running/queued 的 run（含定时任务触发的）。
// 进程重启后可能残留未收尾的 running 行，调用方需再与运行时内存态核对。
func (s *Store) ListActiveAgentRuns(ctx context.Context, projectID string) ([]AgentRun, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return nil, connErr
	}

	rows, err := sqlDB.QueryContext(ctx, `SELECT `+agentRunColumns+` FROM sys_agent_runs WHERE project_id=? AND status IN (?, ?) ORDER BY created_at DESC, id DESC LIMIT 200`, projectID, AgentRunRunning, AgentRunQueued)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentRun, 0)
	for rows.Next() {
		var r AgentRun
		var start, finish sql.NullTime
		if err := rows.Scan(&r.ID, &r.ThreadID, &r.ProjectID, &r.AgentID, &r.Status, &r.Error, &start, &finish, &r.CreatedAt, &r.DurationMS, &r.PromptTokens, &r.CompletionTokens, &r.ReasoningTokens, &r.ToolCalls, &r.ErrorCode, &r.InitiatorID); err != nil {
			return nil, err
		}
		if start.Valid {
			r.StartedAt = start.Time
		}
		if finish.Valid {
			r.FinishedAt = finish.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateAgentRunMetrics 写入运行指标与安全错误码，不存上游原始错误。
func (s *Store) UpdateAgentRunMetrics(ctx context.Context, projectID, id string, r AgentRun) error {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return connErr
	}

	_, err := sqlDB.ExecContext(ctx, `UPDATE sys_agent_runs SET duration_ms=?, prompt_tokens=?, completion_tokens=?, reasoning_tokens=?, tool_calls=?, error_code=? WHERE project_id=? AND id=?`, r.DurationMS, r.PromptTokens, r.CompletionTokens, r.ReasoningTokens, r.ToolCalls, r.ErrorCode, projectID, id)
	if err == nil {
		s.notifyWrite(ctx)
	}
	return err
}

// UpdateAgentRunStatus 更新 run 状态与错误。
// LIFE-04：状态只允许单向迁移——已处于终态（completed/failed/canceled）的
// run 不再接受任何状态写入（同状态重复写入幂等跳过），避免 CancelRun 与
// 应用服务的收尾（agentrun.finishRun）交错时最终状态取决于谁后写。
func (s *Store) UpdateAgentRunStatus(ctx context.Context, projectID, id, status, errMsg string, started, finished *time.Time) error {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return connErr
	}

	var startAny, finAny any
	if started != nil && !started.IsZero() {
		startAny = started.UTC()
	}
	if finished != nil && !finished.IsZero() {
		finAny = finished.UTC()
	}
	res, err := sqlDB.ExecContext(ctx,
		`UPDATE sys_agent_runs SET status=?, error=?,
		 started_at = CASE WHEN ? IS NOT NULL THEN ? ELSE started_at END,
		 finished_at = CASE WHEN ? IS NOT NULL THEN ? ELSE finished_at END
		 WHERE id=? AND project_id=? AND (status NOT IN ('completed','failed','canceled') OR status=?)`,
		status, errMsg, startAny, startAny, finAny, finAny, id, projectID, status)
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	_, _ = res.RowsAffected() // 条件不命中（终态保持）不算错误：单向迁移语义
	return nil
}
