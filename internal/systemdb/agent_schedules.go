package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// AgentSchedule 是 sys_agent_schedules 一行（Cloud Agent 定时执行配置）。
type AgentSchedule struct {
	ID        string
	ProjectID string
	AgentID   string
	ThreadID  string
	Prompt    string
	CronExpr  string
	Enabled   bool
	LastRunAt time.Time // 零值表示从未执行
	NextRunAt time.Time // 服务端按 cron 计算；零值表示尚未排期
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AgentScheduleRun 是 sys_agent_schedule_runs 一行（一次定时/手动触发记录）。
type AgentScheduleRun struct {
	ID         string
	ScheduleID string
	ProjectID  string
	AgentID    string
	ThreadID   string
	RunID      string
	Trigger    string // scheduled / manual
	Status     string // 复用 AgentRun* 常量
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
	CreatedAt  time.Time
}

// CreateAgentSchedule 写入一条 schedule。每 agent 唯一性由调用方（handler 409）保证。
func (s *Store) CreateAgentSchedule(ctx context.Context, sc AgentSchedule) (AgentSchedule, error) {
	if s == nil || s.db == nil {
		return AgentSchedule{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if sc.ID == "" {
		sc.ID = uuid.NewString()
	}
	sc.CreatedAt = now
	sc.UpdatedAt = now
	enabled := int64(0)
	if sc.Enabled {
		enabled = 1
	}
	var lastRun any
	if !sc.LastRunAt.IsZero() {
		lastRun = sc.LastRunAt.UTC()
	}
	var nextRun any
	if !sc.NextRunAt.IsZero() {
		nextRun = sc.NextRunAt.UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_schedules(id, project_id, agent_id, thread_id, prompt, cron_expr, enabled, last_run_at, next_run_at, created_by, created_at, updated_at, archived_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		sc.ID, sc.ProjectID, sc.AgentID, sc.ThreadID, sc.Prompt, sc.CronExpr, enabled, lastRun, nextRun, sc.CreatedBy, now, now)
	if err != nil {
		return AgentSchedule{}, err
	}
	s.notifyWrite(ctx)
	return sc, nil
}

// GetAgentSchedule 按 ID 取未归档 schedule。
func (s *Store) GetAgentSchedule(ctx context.Context, projectID, id string) (AgentSchedule, error) {
	if s == nil || s.db == nil {
		return AgentSchedule{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, agent_id, thread_id, prompt, cron_expr, enabled, last_run_at, next_run_at, created_by, created_at, updated_at
		 FROM sys_agent_schedules WHERE id = ? AND project_id = ? AND archived_at IS NULL`,
		id, projectID)
	sc, err := scanAgentSchedule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentSchedule{}, sql.ErrNoRows
	}
	return sc, err
}

// GetAgentScheduleByAgent 取该 agent 的未归档 schedule；无则返回 sql.ErrNoRows。
func (s *Store) GetAgentScheduleByAgent(ctx context.Context, projectID, agentID string) (AgentSchedule, error) {
	if s == nil || s.db == nil {
		return AgentSchedule{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, agent_id, thread_id, prompt, cron_expr, enabled, last_run_at, next_run_at, created_by, created_at, updated_at
		 FROM sys_agent_schedules WHERE project_id = ? AND agent_id = ? AND archived_at IS NULL
		 ORDER BY created_at ASC LIMIT 1`, projectID, agentID)
	sc, err := scanAgentSchedule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentSchedule{}, sql.ErrNoRows
	}
	return sc, err
}

// ListAgentSchedules 列出项目全部未归档 schedule。
func (s *Store) ListAgentSchedules(ctx context.Context, projectID string) ([]AgentSchedule, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, agent_id, thread_id, prompt, cron_expr, enabled, last_run_at, next_run_at, created_by, created_at, updated_at
		 FROM sys_agent_schedules WHERE project_id = ? AND archived_at IS NULL
		 ORDER BY created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentSchedule, 0)
	for rows.Next() {
		sc, err := scanAgentSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// UpdateAgentSchedule 更新可变字段（prompt / cron / enabled / thread_id / 排期时间）。
func (s *Store) UpdateAgentSchedule(ctx context.Context, sc AgentSchedule) (AgentSchedule, error) {
	if s == nil || s.db == nil {
		return AgentSchedule{}, ErrUnavailable
	}
	now := time.Now().UTC()
	sc.UpdatedAt = now
	enabled := int64(0)
	if sc.Enabled {
		enabled = 1
	}
	var lastRun any
	if !sc.LastRunAt.IsZero() {
		lastRun = sc.LastRunAt.UTC()
	}
	var nextRun any
	if !sc.NextRunAt.IsZero() {
		nextRun = sc.NextRunAt.UTC()
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_schedules SET agent_id=?, thread_id=?, prompt=?, cron_expr=?, enabled=?, last_run_at=?, next_run_at=?, updated_at=?
		 WHERE id=? AND project_id=? AND archived_at IS NULL`,
		sc.AgentID, sc.ThreadID, sc.Prompt, sc.CronExpr, enabled, lastRun, nextRun, now, sc.ID, sc.ProjectID)
	if err != nil {
		return AgentSchedule{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return AgentSchedule{}, sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return sc, nil
}

// ArchiveAgentSchedule 软删。
func (s *Store) ArchiveAgentSchedule(ctx context.Context, projectID, id string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_schedules SET archived_at=?, updated_at=? WHERE id=? AND project_id=? AND archived_at IS NULL`,
		now, now, id, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return nil
}

// ListDueAgentSchedules 取启用且 next_run_at 到期（<= now）的 schedule，最多 limit 条。
func (s *Store) ListDueAgentSchedules(ctx context.Context, now time.Time, limit int) ([]AgentSchedule, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, agent_id, thread_id, prompt, cron_expr, enabled, last_run_at, next_run_at, created_by, created_at, updated_at
		 FROM sys_agent_schedules
		 WHERE archived_at IS NULL AND enabled = 1 AND next_run_at IS NOT NULL AND next_run_at <= ?
		 ORDER BY next_run_at ASC LIMIT ?`, now.UTC(), int64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentSchedule, 0)
	for rows.Next() {
		sc, err := scanAgentSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// ClaimAgentSchedule CAS 认领：仅当 next_run_at 仍等于 expectNext 时推进到 newNext 并写 lastRun。
// RowsAffected==0 返回 false（已被其他执行路径认领或修改）。
func (s *Store) ClaimAgentSchedule(ctx context.Context, id string, expectNext, newNext, lastRun time.Time) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrUnavailable
	}
	now := time.Now().UTC()
	var expectAny any
	if !expectNext.IsZero() {
		expectAny = expectNext.UTC()
	}
	var nextAny, lastAny any
	if !newNext.IsZero() {
		nextAny = newNext.UTC()
	}
	if !lastRun.IsZero() {
		lastAny = lastRun.UTC()
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_schedules
		 SET next_run_at = ?, last_run_at = CASE WHEN ? IS NOT NULL THEN ? ELSE last_run_at END, updated_at = ?
		 WHERE id = ? AND archived_at IS NULL AND enabled = 1 AND
		       ((? IS NULL AND next_run_at IS NULL) OR next_run_at = ?)`,
		nextAny, lastAny, lastAny, now, id, expectAny, expectAny)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, nil
	}
	s.notifyWrite(ctx)
	return true, nil
}

// InsertAgentScheduleRun 写入一次触发记录。
func (s *Store) InsertAgentScheduleRun(ctx context.Context, r AgentScheduleRun) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	var started, finished any
	if !r.StartedAt.IsZero() {
		started = r.StartedAt.UTC()
	}
	if !r.FinishedAt.IsZero() {
		finished = r.FinishedAt.UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_agent_schedule_runs(id, schedule_id, project_id, agent_id, thread_id, run_id, trigger, status, error, started_at, finished_at, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ScheduleID, r.ProjectID, r.AgentID, r.ThreadID, r.RunID, r.Trigger, r.Status, r.Error, started, finished, now)
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// UpdateAgentScheduleRunStatus 更新触发记录状态（供手动触发后轮询刷新）。
func (s *Store) UpdateAgentScheduleRunStatus(ctx context.Context, projectID, id, status, errMsg string, finished *time.Time) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	var finAny any
	if finished != nil && !finished.IsZero() {
		finAny = finished.UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE sys_agent_schedule_runs SET status=?, error=?,
		 finished_at = CASE WHEN ? IS NOT NULL THEN ? ELSE finished_at END
		 WHERE id=? AND project_id=?`,
		status, errMsg, finAny, finAny, id, projectID)
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// ListAgentScheduleRuns 按 schedule 列出触发记录（新→旧），最多 limit 条。
func (s *Store) ListAgentScheduleRuns(ctx context.Context, projectID, scheduleID string, limit int) ([]AgentScheduleRun, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, schedule_id, project_id, agent_id, thread_id, run_id, trigger, status, error, started_at, finished_at, created_at
		 FROM sys_agent_schedule_runs WHERE project_id = ? AND schedule_id = ?
		 ORDER BY created_at DESC LIMIT ?`, projectID, scheduleID, int64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentScheduleRun, 0)
	for rows.Next() {
		r, err := scanAgentScheduleRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanAgentSchedule(sc rowScanner) (AgentSchedule, error) {
	var a AgentSchedule
	var enabled int64
	var lastRun, nextRun sql.NullTime
	if err := sc.Scan(&a.ID, &a.ProjectID, &a.AgentID, &a.ThreadID, &a.Prompt, &a.CronExpr,
		&enabled, &lastRun, &nextRun, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return AgentSchedule{}, err
	}
	a.Enabled = enabled != 0
	if lastRun.Valid {
		a.LastRunAt = lastRun.Time
	}
	if nextRun.Valid {
		a.NextRunAt = nextRun.Time
	}
	return a, nil
}

func scanAgentScheduleRun(sc rowScanner) (AgentScheduleRun, error) {
	var r AgentScheduleRun
	var started, finished sql.NullTime
	if err := sc.Scan(&r.ID, &r.ScheduleID, &r.ProjectID, &r.AgentID, &r.ThreadID, &r.RunID,
		&r.Trigger, &r.Status, &r.Error, &started, &finished, &r.CreatedAt); err != nil {
		return AgentScheduleRun{}, err
	}
	if started.Valid {
		r.StartedAt = started.Time
	}
	if finished.Valid {
		r.FinishedAt = finished.Time
	}
	return r, nil
}
