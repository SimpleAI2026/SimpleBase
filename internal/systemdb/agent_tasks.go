// 文件说明：agent_tasks.go — 协作任务（AgentTask）的持久化、依赖编解码与人类待办查询
//
// 用法
//
//	AgentTask、AgentTaskRun、CreateAgentTask（包内使用，由同包 handler / 路由装配调用）
//
// 主要符号
//
//	type AgentTask struct {…}
//	type AgentTaskRun struct {…}
//	func scanAgentTask(sc rowScanner) (AgentTask, error)
//	func (s *Store) CreateAgentTask(ctx context.Context, t AgentTask) (AgentTask, error)
//	func (s *Store) GetAgentTask(ctx context.Context, projectID, taskID string) (AgentTask, error)
//	func (s *Store) ListAgentTasks(ctx context.Context, projectID, turnID string) ([]AgentTask, error)
//	func (s *Store) ListHumanAgentTasks(ctx context.Context, projectID, assignee, status string, limit int) ([]Age
//	func (s *Store) CountPendingHumanTasks(ctx context.Context, projectID, assignee string) (int, error)
//	... 另有 4 个顶层符号

package systemdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AgentTask 是 sys_agent_tasks 一行。
type AgentTask struct {
	ID           string
	ProjectID    string
	TurnID       string
	ThreadID     string
	Seq          int64
	TargetKind   string
	TargetID     string
	Instruction  string
	DependsJSON  string
	Status       string
	Attempt      int64
	RunID        string
	AssigneeID   string
	ReplyText    string
	ReplyOutcome string
	RepliedBy    string
	ErrorCode    string
	Version      int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// AgentTaskRun 是 sys_agent_task_runs 一行：任务的一次执行尝试。
type AgentTaskRun struct {
	ProjectID string
	TaskID    string
	Attempt   int64
	RunID     string
	Status    string
	CreatedAt time.Time
}

func scanAgentTask(sc rowScanner) (AgentTask, error) {
	var t AgentTask
	if err := sc.Scan(&t.ID, &t.ProjectID, &t.TurnID, &t.ThreadID, &t.Seq, &t.TargetKind, &t.TargetID,
		&t.Instruction, &t.DependsJSON, &t.Status, &t.Attempt, &t.RunID, &t.AssigneeID, &t.ReplyText,
		&t.ReplyOutcome, &t.RepliedBy, &t.ErrorCode, &t.Version, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return AgentTask{}, err
	}
	return t, nil
}

// CreateAgentTask 写入任务；seq 由调用方按轮次内顺序分配。
func (s *Store) CreateAgentTask(ctx context.Context, t AgentTask) (AgentTask, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return AgentTask{}, connErr
	}

	now := time.Now().UTC()
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.Status == "" {
		t.Status = TaskStatusPending
	}
	if t.DependsJSON == "" {
		t.DependsJSON = "[]"
	}
	t.Version = 1
	t.CreatedAt = now
	t.UpdatedAt = now
	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO sys_agent_tasks(id, project_id, turn_id, thread_id, seq, target_kind, target_id,
			instruction, depends_json, status, attempt, run_id, assignee_id, reply_text,
			reply_outcome, replied_by, error_code, version, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, '', ?, '', '', '', '', 1, ?, ?)`,
		t.ID, t.ProjectID, t.TurnID, t.ThreadID, t.Seq, t.TargetKind, t.TargetID,
		t.Instruction, t.DependsJSON, t.Status, t.AssigneeID, now, now); err != nil {
		return AgentTask{}, err
	}
	s.notifyWrite(ctx)
	return t, nil
}

// GetAgentTask 按 ID 取任务。
func (s *Store) GetAgentTask(ctx context.Context, projectID, taskID string) (AgentTask, error) {
	t, err := queryOne(ctx, s, scanAgentTask,
		`SELECT `+agentTaskColumns+` FROM sys_agent_tasks WHERE project_id=? AND id=?`, projectID, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentTask{}, ErrTaskNotFound
	}
	return t, err
}

// ListAgentTasks 按 seq 升序返回轮次内的任务。
func (s *Store) ListAgentTasks(ctx context.Context, projectID, turnID string) ([]AgentTask, error) {
	return queryList(ctx, s, scanAgentTask,
		`SELECT `+agentTaskColumns+` FROM sys_agent_tasks
		 WHERE project_id=? AND turn_id=? ORDER BY seq, id`, projectID, turnID)
}

// ReplaceAgentTasks 在一个事务内整组替换轮次任务（replan 使用）。
// 输入中的已完成任务会连同 attempt/run/reply/version 一起保留。
func (s *Store) ReplaceAgentTasks(ctx context.Context, projectID, turnID string, tasks []AgentTask) error {
	sqlDB, err := s.conn()
	if err != nil {
		return err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sys_agent_tasks WHERE project_id=? AND turn_id=?`, projectID, turnID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for i, task := range tasks {
		if task.ID == "" {
			task.ID = uuid.NewString()
		}
		task.ProjectID = projectID
		task.TurnID = turnID
		if task.Status == "" {
			task.Status = TaskStatusPending
		}
		if task.DependsJSON == "" {
			task.DependsJSON = "[]"
		}
		if task.Version <= 0 {
			task.Version = 1
		}
		if task.CreatedAt.IsZero() {
			task.CreatedAt = now
		}
		task.UpdatedAt = now
		if task.Seq == 0 {
			task.Seq = int64(i + 1)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO sys_agent_tasks(
			id, project_id, turn_id, thread_id, seq, target_kind, target_id, instruction,
			depends_json, status, attempt, run_id, assignee_id, reply_text, reply_outcome,
			replied_by, error_code, version, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			task.ID, task.ProjectID, task.TurnID, task.ThreadID, task.Seq, task.TargetKind,
			task.TargetID, task.Instruction, task.DependsJSON, task.Status, task.Attempt,
			task.RunID, task.AssigneeID, task.ReplyText, task.ReplyOutcome, task.RepliedBy,
			task.ErrorCode, task.Version, task.CreatedAt, task.UpdatedAt); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// ListHumanAgentTasks 列出指派给某人的待办（assignee 为空表示不按人过滤）。
func (s *Store) ListHumanAgentTasks(ctx context.Context, projectID, assignee, status string, limit int) ([]AgentTask, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `SELECT ` + agentTaskColumns + ` FROM sys_agent_tasks
		WHERE project_id=? AND target_kind=?`
	args := []any{projectID, TaskTargetHuman}
	if assignee != "" {
		query += ` AND assignee_id=?`
		args = append(args, assignee)
	}
	if status != "" {
		query += ` AND status=?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	return queryList(ctx, s, scanAgentTask, query, args...)
}

// CountPendingHumanTasks 统计某人的待办数（侧栏角标）。
func (s *Store) CountPendingHumanTasks(ctx context.Context, projectID, assignee string) (int, error) {
	query := `SELECT COUNT(*) FROM sys_agent_tasks WHERE project_id=? AND target_kind=? AND status=?`
	args := []any{projectID, TaskTargetHuman, TaskStatusWaitingHuman}
	if assignee != "" {
		query += ` AND assignee_id=?`
		args = append(args, assignee)
	}
	return s.countRows(ctx, query, args...)
}

// UpdateAgentTask 以版本 CAS 更新任务；expectedVersion<=0 表示不校验版本。
// 只更新可写字段：状态、run、重试次数、回复、错误码。
func (s *Store) UpdateAgentTask(ctx context.Context, t AgentTask, expectedVersion int64) (AgentTask, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return AgentTask{}, connErr
	}

	query := `UPDATE sys_agent_tasks SET status=?, attempt=?, run_id=?, assignee_id=?, reply_text=?,
		reply_outcome=?, replied_by=?, error_code=?, version=version+1, updated_at=?
		WHERE project_id=? AND id=?`
	args := []any{t.Status, t.Attempt, t.RunID, t.AssigneeID, t.ReplyText,
		t.ReplyOutcome, t.RepliedBy, t.ErrorCode, time.Now().UTC(), t.ProjectID, t.ID}
	if expectedVersion > 0 {
		query += ` AND version=?`
		args = append(args, expectedVersion)
	}
	res, err := sqlDB.ExecContext(ctx, query, args...)
	if err != nil {
		return AgentTask{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if expectedVersion > 0 {
			return AgentTask{}, ErrTurnConflict
		}
		return AgentTask{}, ErrTaskNotFound
	}
	s.notifyWrite(ctx)
	return s.GetAgentTask(ctx, t.ProjectID, t.ID)
}

// AppendAgentTaskRun 记录任务的一次运行尝试（保留历史，不覆盖）。
func (s *Store) AppendAgentTaskRun(ctx context.Context, r AgentTaskRun) error {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return connErr
	}

	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO sys_agent_task_runs(project_id, task_id, attempt, run_id, status, created_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		r.ProjectID, r.TaskID, r.Attempt, r.RunID, r.Status, time.Now().UTC()); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// MarshalAgentTaskDepends 把依赖任务 ID 列表序列化成 depends_json。
func MarshalAgentTaskDepends(ids []string) string {
	if ids == nil {
		ids = []string{}
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// UnmarshalAgentTaskDepends 解析 depends_json；解析失败返回空列表（不阻断推进）。
func UnmarshalAgentTaskDepends(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
