// 文件说明：plans.go — 云助手计划模式（planv6.7 cloud-agent-plan-mode）：计划与阶段状态
//
// 用法
//
//	(s *Store) CreateAgentPlan(ctx, plan, stages) (AgentPlan, error)   // 幂等：同线程同幂等键只建一条
//	(s *Store) GetAgentPlan(ctx, projectID, id) (AgentPlan, error)
//	(s *Store) ListAgentPlans(ctx, projectID, threadID, limit) ([]AgentPlan, error)
//	(s *Store) ListAgentPlanStages(ctx, planID) ([]AgentPlanStage, error)
//	(s *Store) UpdateAgentPlanStage(ctx, stage) error
//	(s *Store) UpdateAgentPlanStatus(ctx, projectID, id, status, currentStage, expectedVersion) error
//	(s *Store) InterruptRunningPlans(ctx) (int, error)                 // 启动时收敛：running → interrupted
//
// 计划是服务端驱动的六阶段编排（规划 → 数据库 → KV → 云函数 → 定时任务 → 前端站点）。
// 单写实例：version 只做乐观并发，不承担跨节点选主。

package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// 计划状态。
const (
	AgentPlanQueued      = "queued"
	AgentPlanRunning     = "running"
	AgentPlanPaused      = "paused"
	AgentPlanCompleted   = "completed"
	AgentPlanCanceled    = "canceled"
	AgentPlanInterrupted = "interrupted"
)

// 阶段状态。
const (
	AgentPlanStagePending = "pending"
	AgentPlanStageRunning = "running"
	AgentPlanStageDone    = "done"
	AgentPlanStageSkipped = "skipped"
	AgentPlanStageFailed  = "failed"
	AgentPlanStageBlocked = "blocked"
)

// AgentPlan 是 sys_agent_plans 一行。
type AgentPlan struct {
	ID             string
	ProjectID      string
	ThreadID       string
	AgentID        string
	Goal           string
	Status         string
	CurrentStage   string
	Version        int64
	IdempotencyKey string
	CreatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// AgentPlanStage 是 sys_agent_plan_stages 一行。
type AgentPlanStage struct {
	PlanID     string
	Stage      string
	Status     string
	Attempt    int
	RunID      string
	Goal       string
	SkipReason string
	Detail     string
	UpdatedAt  time.Time
}

// ErrAgentPlanVersion 表示计划版本已变化（并发推进被拒绝）。
var ErrAgentPlanVersion = errors.New("systemdb: agent plan version conflict")

// CreateAgentPlan 写入计划与六个阶段。同线程同幂等键已存在时返回既有计划。
func (s *Store) CreateAgentPlan(ctx context.Context, plan AgentPlan, stages []AgentPlanStage) (AgentPlan, error) {
	if s == nil || s.db == nil {
		return AgentPlan{}, ErrUnavailable
	}
	if plan.ID == "" {
		plan.ID = uuid.NewString()
	}
	if plan.IdempotencyKey != "" {
		existing, err := s.agentPlanByIdempotency(ctx, plan.ThreadID, plan.IdempotencyKey)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return AgentPlan{}, err
		}
	}
	now := time.Now().UTC()
	plan.CreatedAt, plan.UpdatedAt = now, now
	if plan.Status == "" {
		plan.Status = AgentPlanQueued
	}
	if plan.Version == 0 {
		plan.Version = 1
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_plans(id, project_id, thread_id, agent_id, goal, status, current_stage, version, idempotency_key, created_by, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		plan.ID, plan.ProjectID, plan.ThreadID, plan.AgentID, plan.Goal, plan.Status, plan.CurrentStage,
		plan.Version, plan.IdempotencyKey, plan.CreatedBy, now, now); err != nil {
		return AgentPlan{}, err
	}
	for _, stage := range stages {
		status := stage.Status
		if status == "" {
			status = AgentPlanStagePending
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO sys_agent_plan_stages(plan_id, stage, status, attempt, run_id, goal, skip_reason, detail, updated_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			plan.ID, stage.Stage, status, stage.Attempt, stage.RunID, stage.Goal, stage.SkipReason, stage.Detail, now); err != nil {
			return AgentPlan{}, err
		}
	}
	s.notifyWrite(ctx)
	return plan, nil
}

// FindAgentPlanByKey 按线程与幂等键查计划；不存在返回 sql.ErrNoRows。
func (s *Store) FindAgentPlanByKey(ctx context.Context, threadID, key string) (AgentPlan, error) {
	if s == nil || s.db == nil {
		return AgentPlan{}, ErrUnavailable
	}
	return s.agentPlanByIdempotency(ctx, threadID, key)
}

func (s *Store) agentPlanByIdempotency(ctx context.Context, threadID, key string) (AgentPlan, error) {
	var p AgentPlan
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, thread_id, agent_id, goal, status, current_stage, version, idempotency_key, created_by, created_at, updated_at
		 FROM sys_agent_plans WHERE thread_id = ? AND idempotency_key = ? LIMIT 1`,
		threadID, key).Scan(&p.ID, &p.ProjectID, &p.ThreadID, &p.AgentID, &p.Goal, &p.Status, &p.CurrentStage,
		&p.Version, &p.IdempotencyKey, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// GetAgentPlan 按 ID 取计划。
func (s *Store) GetAgentPlan(ctx context.Context, projectID, id string) (AgentPlan, error) {
	if s == nil || s.db == nil {
		return AgentPlan{}, ErrUnavailable
	}
	var p AgentPlan
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, thread_id, agent_id, goal, status, current_stage, version, idempotency_key, created_by, created_at, updated_at
		 FROM sys_agent_plans WHERE id = ? AND project_id = ?`,
		id, projectID).Scan(&p.ID, &p.ProjectID, &p.ThreadID, &p.AgentID, &p.Goal, &p.Status, &p.CurrentStage,
		&p.Version, &p.IdempotencyKey, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// ListAgentPlans 列出线程的计划（新的在前）。
func (s *Store) ListAgentPlans(ctx context.Context, projectID, threadID string, limit int) ([]AgentPlan, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, thread_id, agent_id, goal, status, current_stage, version, idempotency_key, created_by, created_at, updated_at
		 FROM sys_agent_plans WHERE project_id = ? AND thread_id = ? ORDER BY created_at DESC LIMIT ?`,
		projectID, threadID, int64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentPlan, 0)
	for rows.Next() {
		var p AgentPlan
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.ThreadID, &p.AgentID, &p.Goal, &p.Status, &p.CurrentStage,
			&p.Version, &p.IdempotencyKey, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListAgentPlanStages 按固定顺序返回计划的六个阶段。
func (s *Store) ListAgentPlanStages(ctx context.Context, planID string) ([]AgentPlanStage, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT plan_id, stage, status, attempt, run_id, goal, skip_reason, detail, updated_at
		 FROM sys_agent_plan_stages WHERE plan_id = ? ORDER BY CASE stage
		 WHEN 'plan' THEN 1 WHEN 'database' THEN 2 WHEN 'kv' THEN 3
		 WHEN 'functions' THEN 4 WHEN 'cron' THEN 5 WHEN 'site' THEN 6 ELSE 7 END`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentPlanStage, 0, 6)
	for rows.Next() {
		var st AgentPlanStage
		if err := rows.Scan(&st.PlanID, &st.Stage, &st.Status, &st.Attempt, &st.RunID, &st.Goal, &st.SkipReason, &st.Detail, &st.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// UpdateAgentPlanStage 覆盖单个阶段的状态与尝试信息。
func (s *Store) UpdateAgentPlanStage(ctx context.Context, stage AgentPlanStage) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_plan_stages SET status = ?, attempt = ?, run_id = ?, detail = ?, goal = ?, skip_reason = ?, updated_at = ?
		 WHERE plan_id = ? AND stage = ?`,
		stage.Status, stage.Attempt, stage.RunID, stage.Detail, stage.Goal, stage.SkipReason, time.Now().UTC(), stage.PlanID, stage.Stage)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return nil
}

// UpdateAgentPlanStatus 以版本 CAS 推进计划状态；版本不符返回 ErrAgentPlanVersion。
func (s *Store) UpdateAgentPlanStatus(ctx context.Context, projectID, id, status, currentStage string, expectedVersion int64) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_plans SET status = ?, current_stage = ?, version = version + 1, updated_at = ?
		 WHERE id = ? AND project_id = ? AND version = ?`,
		status, currentStage, time.Now().UTC(), id, projectID, expectedVersion)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAgentPlanVersion
	}
	s.notifyWrite(ctx)
	return nil
}

// InterruptRunningPlans 把进程重启前仍在运行的阶段与计划标记为中断。
// 内存中的 run 已不存在，不能自动重跑（避免重复副作用），由用户重试该阶段。
func (s *Store) InterruptRunningPlans(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_plan_stages SET status = ?, detail = ?, updated_at = ? WHERE status = ?`,
		AgentPlanStageFailed, "interrupted", now, AgentPlanStageRunning); err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_plans SET status = ?, version = version + 1, updated_at = ? WHERE status IN (?, ?)`,
		AgentPlanInterrupted, now, AgentPlanRunning, AgentPlanQueued)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		s.notifyWrite(ctx)
	}
	return int(n), nil
}
