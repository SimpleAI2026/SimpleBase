// agent_schedules.go — sys_agent_schedules / sys_agent_schedule_runs 的只读访问。
//
// Agent 定时执行已合并到 /cron-jobs（planv6.0），旧表只保留历史归档数据的读取，
// 不再提供写入接口。

package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	pagelimit "github.com/linkxzhou/SimpleBase/internal/utils/limit"
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

const (
	// agentScheduleColumns 是 sys_agent_schedules 未归档行的读取列清单，
	// 与 scanAgentSchedule 的扫描顺序一一对应（planv6.0 §7 A3）。
	agentScheduleColumns = `id, project_id, agent_id, thread_id, prompt, cron_expr, enabled, last_run_at, next_run_at, created_by, created_at, updated_at`
	// agentScheduleRunColumns 是 sys_agent_schedule_runs 的读取列清单，与 scanAgentScheduleRun 的顺序一一对应。
	agentScheduleRunColumns = `id, schedule_id, project_id, agent_id, thread_id, run_id, trigger, status, error, started_at, finished_at, created_at`
)

// GetAgentSchedule 按 ID 取未归档 schedule。
func (s *Store) GetAgentSchedule(ctx context.Context, projectID, id string) (AgentSchedule, error) {
	if s == nil || s.db == nil {
		return AgentSchedule{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+agentScheduleColumns+`
		 FROM sys_agent_schedules WHERE id = ? AND project_id = ? AND archived_at IS NULL`,
		id, projectID)
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
		`SELECT `+agentScheduleColumns+`
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

// ListAgentScheduleRuns 按 schedule 列出触发记录（新→旧），最多 limit 条。
func (s *Store) ListAgentScheduleRuns(ctx context.Context, projectID, scheduleID string, limit int) ([]AgentScheduleRun, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	limit = pagelimit.Normalize(limit, 20, 100)
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+agentScheduleRunColumns+`
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
