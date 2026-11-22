// agent_approvals.go — sys_agent_approvals 的读写（破坏性工具调用的审批队列）。
//
// 与 sys_agent_runs 的分工：run 记录「执行了什么」，审批记录「等谁点头」。
// 审批必须落库而不是只活在内存 confirmHub 里，原因有三：
//  1. 列表页要跨请求查询「当前有哪些待办」；
//  2. 进程重启后 pending 项要能被识别并收敛为 expired，而不是永远悬空；
//  3. 超时收敛需要一条可审计的终态记录。
//
// 状态机：pending → approved | denied | expired | canceled，终态不可回退。

package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	pagelimit "github.com/linkxzhou/SimpleBase/godatabase/limit"
)

const (
	// AgentApprovalPending 表示等待人工确认。
	AgentApprovalPending = "pending"
	// AgentApprovalApproved 表示已放行。
	AgentApprovalApproved = "approved"
	// AgentApprovalDenied 表示被拒绝。
	AgentApprovalDenied = "denied"
	// AgentApprovalExpired 表示超过确认窗口未处理。
	AgentApprovalExpired = "expired"
	// AgentApprovalCanceled 表示 run 被取消或进程重启导致作废。
	AgentApprovalCanceled = "canceled"

	// agentApprovalColumns 是 sys_agent_approvals 的读取列清单，
	// 与 scanAgentApproval 的扫描顺序一一对应。
	agentApprovalColumns = `id, project_id, run_id, call_id, thread_id, agent_id, tool_name, argv_json, message, status, initiator_id, resolved_by, created_at, expires_at, resolved_at`
)

// AgentApproval 是 sys_agent_approvals 一行。
type AgentApproval struct {
	ID        string
	ProjectID string
	RunID     string
	CallID    string
	ThreadID  string
	AgentID   string
	ToolName  string
	// ArgvJSON 是脱敏后的参数摘要（JSON 数组），禁止写入原始密钥。
	ArgvJSON string
	Message  string
	Status   string
	// InitiatorID 是发起人标识，确认时与确认人比对（planv6.0 D5）。
	InitiatorID string
	ResolvedBy  string
	CreatedAt   time.Time
	ExpiresAt   time.Time // 零值表示不设超时
	ResolvedAt  time.Time // 零值表示尚未收敛
}

// CreateAgentApproval 写入一条 pending 审批。
// 同一 (project_id, run_id, call_id) 已有 pending 时返回既有行，保证幂等。
func (s *Store) CreateAgentApproval(ctx context.Context, a AgentApproval) (AgentApproval, error) {
	if s == nil || s.db == nil {
		return AgentApproval{}, ErrUnavailable
	}
	if existing, err := s.GetPendingAgentApproval(ctx, a.ProjectID, a.RunID, a.CallID); err == nil {
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return AgentApproval{}, err
	}
	now := time.Now().UTC()
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.Status == "" {
		a.Status = AgentApprovalPending
	}
	if a.ArgvJSON == "" {
		a.ArgvJSON = "[]"
	}
	a.CreatedAt = now
	var expires any
	if !a.ExpiresAt.IsZero() {
		expires = a.ExpiresAt.UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_approvals(id, project_id, run_id, call_id, thread_id, agent_id, tool_name, argv_json, message, status, initiator_id, resolved_by, created_at, expires_at, resolved_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, NULL)`,
		a.ID, a.ProjectID, a.RunID, a.CallID, a.ThreadID, a.AgentID, a.ToolName, a.ArgvJSON, a.Message, a.Status, a.InitiatorID, now, expires)
	if err != nil {
		return AgentApproval{}, err
	}
	s.notifyWrite(ctx)
	return a, nil
}

// GetAgentApproval 按 ID 取审批（必须属于 project）。
func (s *Store) GetAgentApproval(ctx context.Context, projectID, id string) (AgentApproval, error) {
	if s == nil || s.db == nil {
		return AgentApproval{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+agentApprovalColumns+` FROM sys_agent_approvals WHERE id = ? AND project_id = ?`,
		id, projectID)
	a, err := scanAgentApproval(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentApproval{}, sql.ErrNoRows
	}
	return a, err
}

// GetPendingAgentApproval 按 (run, call) 取 pending 审批。
func (s *Store) GetPendingAgentApproval(ctx context.Context, projectID, runID, callID string) (AgentApproval, error) {
	if s == nil || s.db == nil {
		return AgentApproval{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+agentApprovalColumns+`
		 FROM sys_agent_approvals WHERE project_id = ? AND run_id = ? AND call_id = ? AND status = ?
		 ORDER BY created_at DESC LIMIT 1`,
		projectID, runID, callID, AgentApprovalPending)
	a, err := scanAgentApproval(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentApproval{}, sql.ErrNoRows
	}
	return a, err
}

// ListAgentApprovals 列出项目内审批，status 为空表示不过滤（新→旧）。
func (s *Store) ListAgentApprovals(ctx context.Context, projectID, status string, limit int) ([]AgentApproval, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	limit = pagelimit.Normalize(limit, 50, 200)
	query := `SELECT ` + agentApprovalColumns + ` FROM sys_agent_approvals WHERE project_id = ?`
	args := []any{projectID}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, int64(limit))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentApproval, 0)
	for rows.Next() {
		a, err := scanAgentApproval(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CountPendingAgentApprovals 返回项目内 pending 数量（侧栏角标用）。
func (s *Store) CountPendingAgentApprovals(ctx context.Context, projectID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_agent_approvals WHERE project_id = ? AND status = ?`,
		projectID, AgentApprovalPending).Scan(&n)
	return int(n), err
}

// ResolveAgentApproval 把 pending 审批收敛到终态。
// 条件更新保证单向迁移：已收敛的行不会被二次改写，返回 sql.ErrNoRows。
func (s *Store) ResolveAgentApproval(ctx context.Context, projectID, id, status, resolvedBy string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	switch status {
	case AgentApprovalApproved, AgentApprovalDenied, AgentApprovalExpired, AgentApprovalCanceled:
	default:
		return errors.New("systemdb: invalid agent approval status")
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_approvals SET status=?, resolved_by=?, resolved_at=?
		 WHERE id=? AND project_id=? AND status=?`,
		status, resolvedBy, now, id, projectID, AgentApprovalPending)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return nil
}

// ExpireAgentApprovals 把已过确认窗口的 pending 收敛为 expired，返回收敛条数。
// 由调用方按需触发（列表查询前 / 定时清扫），不引入独立后台 goroutine。
func (s *Store) ExpireAgentApprovals(ctx context.Context, projectID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_approvals SET status=?, resolved_at=?
		 WHERE project_id=? AND status=? AND expires_at IS NOT NULL AND expires_at < ?`,
		AgentApprovalExpired, now, projectID, AgentApprovalPending, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		s.notifyWrite(ctx)
	}
	return int(n), nil
}

// CancelRunApprovals 在 run 结束时作废其残留 pending 审批。
func (s *Store) CancelRunApprovals(ctx context.Context, projectID, runID string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_approvals SET status=?, resolved_at=?
		 WHERE project_id=? AND run_id=? AND status=?`,
		AgentApprovalCanceled, now, projectID, runID, AgentApprovalPending)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		s.notifyWrite(ctx)
	}
	return nil
}

func scanAgentApproval(sc rowScanner) (AgentApproval, error) {
	var a AgentApproval
	var expires, resolved sql.NullTime
	if err := sc.Scan(&a.ID, &a.ProjectID, &a.RunID, &a.CallID, &a.ThreadID, &a.AgentID,
		&a.ToolName, &a.ArgvJSON, &a.Message, &a.Status, &a.InitiatorID, &a.ResolvedBy,
		&a.CreatedAt, &expires, &resolved); err != nil {
		return AgentApproval{}, err
	}
	if expires.Valid {
		a.ExpiresAt = expires.Time
	}
	if resolved.Valid {
		a.ResolvedAt = resolved.Time
	}
	return a, nil
}
