// 文件说明：cron_jobs.go — internal/systemdb 中 CronJobKindCron、CronJobKindInterval 等符号的实现
//
// 用法
//
//	(j CronJob) EffectiveTargetKind() string  // EffectiveTargetKind 返回规范化的目标类型：空串视同 function
//	(s *Store) CreateCronJob(ctx context.Context, j CronJob) (CronJob, error)  // CreateCronJob 写入一条任务
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/cron_jobs.go
//	         │ 依赖
//	         ▼
//	[下游] internal/utils/sqlval, internal/utils/limit
//	[同包] agent_schedules.go, agents.go, async_flush.go, bootstrap.go, function_triggers.go 等 22 个
//
// 主要符号
//
//	const CronJobKindCron
//	const CronJobKindInterval
//	const CronJobKindOnce
//	const CronJobTargetFunction
//	const CronJobTargetAgent
//	const CronJobRunRunning
//	... 另有 22 个顶层符号

package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"github.com/linkxzhou/SimpleBase/internal/utils/sqlval"
	"time"

	"github.com/google/uuid"

	pagelimit "github.com/linkxzhou/SimpleBase/internal/utils/limit"
)

// 定时任务调度模式（ui-cronjob-plan §3）。
const (
	CronJobKindCron     = "cron"
	CronJobKindInterval = "interval"
	CronJobKindOnce     = "once"
)

// 定时任务目标类型（planv6.0 gocloudagent-cli-and-cronjob-unification-plan §2.2）。
const (
	CronJobTargetFunction = "function"
	CronJobTargetAgent    = "agent"
)

// 定时任务运行状态（ui-cronjob-plan §4.2）。
const (
	CronJobRunRunning   = "running"
	CronJobRunCompleted = "completed"
	CronJobRunFailed    = "failed"
	CronJobRunCanceled  = "canceled"
)

const (
	// cronJobColumns 是 sys_cron_jobs 未归档行的读取列清单，
	// 与 scanCronJob 的扫描顺序一一对应（planv6.0 §7 A3）。
	cronJobColumns = `id, project_id, name, description, schedule_kind, cron_expr, interval_seconds, run_at,
		func_file, func_export, input_json, enabled, last_run_at, next_run_at, last_status, last_error, run_count,
		created_by, created_at, updated_at, target_kind, agent_id, agent_thread_id, agent_prompt`
	// cronJobRunColumns 是 sys_cron_job_runs 的读取列清单，与 scanCronJobRun 的顺序一一对应。
	cronJobRunColumns = `id, job_id, project_id, trigger, status, error, duration_ms, response_json, started_at, finished_at, created_at`
)

// CronJob 是 sys_cron_jobs 一行（云函数或 Agent 的定时调用任务）。
type CronJob struct {
	ID              string
	ProjectID       string
	Name            string
	Description     string
	ScheduleKind    string    // cron / interval / once
	CronExpr        string    // kind=cron 时非空
	IntervalSeconds int64     // kind=interval 时非空
	RunAt           time.Time // kind=once 时非空：一次性执行时刻
	FuncFile        string    // sys_go_funcs.name（云函数文件名）
	FuncExport      string    // 导出函数名
	InputJSON       string    // 固定入参 JSON 原文，默认 "{}"
	Enabled         bool
	LastRunAt       time.Time // 零值表示从未执行
	NextRunAt       time.Time // 服务端计算；零值表示未排期（禁用/暂停）
	LastStatus      string    // 最近一次 run 状态快照
	LastError       string    // 截断 300
	RunCount        int64
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time

	// TargetKind 为 function（默认，空串视同 function）或 agent。
	// 追加在末尾以匹配 cronJobColumns / scanCronJob 的按位顺序。
	TargetKind    string
	AgentID       string // TargetKind=agent 时的云助手 ID
	AgentThreadID string // TargetKind=agent 时的专属会话 ID（可为空，首次执行时创建）
	AgentPrompt   string // TargetKind=agent 时每次执行发送的用户消息，≤16KiB（API 层校验）
}

// EffectiveTargetKind 返回规范化的目标类型：空串视同 function。
func (j CronJob) EffectiveTargetKind() string {
	if j.TargetKind == CronJobTargetAgent {
		return CronJobTargetAgent
	}
	return CronJobTargetFunction
}

// CronJobRun 是 sys_cron_job_runs 一行（一次定时/手动执行记录）。
type CronJobRun struct {
	ID           string
	JobID        string
	ProjectID    string
	Trigger      string // scheduled / manual
	Status       string // running / completed / failed / canceled
	Error        string
	DurationMs   int64
	ResponseJSON string // 截断 4KB；空串表示无输出
	StartedAt    time.Time
	FinishedAt   time.Time
	CreatedAt    time.Time
}

// CreateCronJob 写入一条任务。同名唯一性由唯一索引保证（调用方映射 409）。
func (s *Store) CreateCronJob(ctx context.Context, j CronJob) (CronJob, error) {
	if s == nil || s.db == nil {
		return CronJob{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if j.ID == "" {
		j.ID = uuid.NewString()
	}
	j.CreatedAt = now
	j.UpdatedAt = now
	enabled := int64(0)
	if j.Enabled {
		enabled = 1
	}
	var lastRun, nextRun, runAt any
	if !j.LastRunAt.IsZero() {
		lastRun = j.LastRunAt.UTC()
	}
	if !j.NextRunAt.IsZero() {
		nextRun = j.NextRunAt.UTC()
	}
	if !j.RunAt.IsZero() {
		runAt = j.RunAt.UTC()
	}
	j.TargetKind = j.EffectiveTargetKind()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_cron_jobs(id, project_id, name, description, schedule_kind, cron_expr, interval_seconds, run_at,
			func_file, func_export, input_json, enabled, last_run_at, next_run_at, last_status, last_error, run_count,
			created_by, created_at, updated_at, archived_at, target_kind, agent_id, agent_thread_id, agent_prompt)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, ?)`,
		j.ID, j.ProjectID, j.Name, j.Description, j.ScheduleKind, sqlval.NullString(j.CronExpr), nullInt64IfZero(j.IntervalSeconds), runAt,
		j.FuncFile, j.FuncExport, j.InputJSON, enabled, lastRun, nextRun, j.LastStatus, j.LastError, j.RunCount,
		j.CreatedBy, now, now, j.TargetKind, j.AgentID, j.AgentThreadID, j.AgentPrompt)
	if err != nil {
		return CronJob{}, err
	}
	s.notifyWrite(ctx)
	return j, nil
}

// GetCronJob 按 ID 取未归档任务。
func (s *Store) GetCronJob(ctx context.Context, projectID, id string) (CronJob, error) {
	if s == nil || s.db == nil {
		return CronJob{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+cronJobColumns+`
		 FROM sys_cron_jobs WHERE id = ? AND project_id = ? AND archived_at IS NULL`, id, projectID)
	j, err := scanCronJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return CronJob{}, sql.ErrNoRows
	}
	return j, err
}

// GetCronJobByName 按名称取未归档任务；无则返回 sql.ErrNoRows。
func (s *Store) GetCronJobByName(ctx context.Context, projectID, name string) (CronJob, error) {
	if s == nil || s.db == nil {
		return CronJob{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+cronJobColumns+`
		 FROM sys_cron_jobs WHERE project_id = ? AND name = ? AND archived_at IS NULL`, projectID, name)
	j, err := scanCronJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return CronJob{}, sql.ErrNoRows
	}
	return j, err
}

// ListCronJobs 列出项目全部未归档任务（创建时间正序）。
func (s *Store) ListCronJobs(ctx context.Context, projectID string) ([]CronJob, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+cronJobColumns+`
		 FROM sys_cron_jobs WHERE project_id = ? AND archived_at IS NULL
		 ORDER BY created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CronJob, 0)
	for rows.Next() {
		j, err := scanCronJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// UpdateCronJob 更新可变字段（描述/调度/目标/入参/启用/排期与快照）。
func (s *Store) UpdateCronJob(ctx context.Context, j CronJob) (CronJob, error) {
	if s == nil || s.db == nil {
		return CronJob{}, ErrUnavailable
	}
	now := time.Now().UTC()
	j.UpdatedAt = now
	enabled := int64(0)
	if j.Enabled {
		enabled = 1
	}
	var lastRun, nextRun, runAt any
	if !j.LastRunAt.IsZero() {
		lastRun = j.LastRunAt.UTC()
	}
	if !j.NextRunAt.IsZero() {
		nextRun = j.NextRunAt.UTC()
	}
	if !j.RunAt.IsZero() {
		runAt = j.RunAt.UTC()
	}
	j.TargetKind = j.EffectiveTargetKind()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_cron_jobs SET description=?, schedule_kind=?, cron_expr=?, interval_seconds=?, run_at=?,
			func_file=?, func_export=?, input_json=?, enabled=?, last_run_at=?, next_run_at=?,
			last_status=?, last_error=?, run_count=?, updated_at=?,
			target_kind=?, agent_id=?, agent_thread_id=?, agent_prompt=?
		 WHERE id=? AND project_id=? AND archived_at IS NULL`,
		j.Description, j.ScheduleKind, sqlval.NullString(j.CronExpr), nullInt64IfZero(j.IntervalSeconds), runAt,
		j.FuncFile, j.FuncExport, j.InputJSON, enabled, lastRun, nextRun,
		j.LastStatus, j.LastError, j.RunCount, now,
		j.TargetKind, j.AgentID, j.AgentThreadID, j.AgentPrompt, j.ID, j.ProjectID)
	if err != nil {
		return CronJob{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return CronJob{}, sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return j, nil
}

// ArchiveCronJob 软删并清空排期（不再到期）。
func (s *Store) ArchiveCronJob(ctx context.Context, projectID, id string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_cron_jobs SET archived_at=?, updated_at=?, next_run_at=NULL, enabled=0
		 WHERE id=? AND project_id=? AND archived_at IS NULL`, now, now, id, projectID)
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

// ListDueCronJobs 取启用且 next_run_at 到期（<= now）的任务，最多 limit 条。
func (s *Store) ListDueCronJobs(ctx context.Context, now time.Time, limit int) ([]CronJob, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	limit = pagelimit.Normalize(limit, 100, 500)
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+cronJobColumns+`
		 FROM sys_cron_jobs
		 WHERE archived_at IS NULL AND enabled = 1 AND next_run_at IS NOT NULL AND next_run_at <= ?
		 ORDER BY next_run_at ASC LIMIT ?`, now.UTC(), int64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CronJob, 0)
	for rows.Next() {
		j, err := scanCronJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ClaimCronJob CAS 认领：仅当 next_run_at 仍等于 expectNext 时推进到 newNext 并写 lastRun。
// RowsAffected==0 返回 false（已被其他执行路径认领或修改）。
func (s *Store) ClaimCronJob(ctx context.Context, id string, expectNext, newNext, lastRun time.Time) (bool, error) {
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
		`UPDATE sys_cron_jobs
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

// InsertCronJobRun 写入一次执行记录，返回带 ID 的完整记录。
func (s *Store) InsertCronJobRun(ctx context.Context, r CronJobRun) (CronJobRun, error) {
	if s == nil || s.db == nil {
		return CronJobRun{}, ErrUnavailable
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
		`INSERT INTO sys_cron_job_runs(id, job_id, project_id, trigger, status, error, duration_ms, response_json, started_at, finished_at, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.JobID, r.ProjectID, r.Trigger, r.Status, r.Error, r.DurationMs, sqlval.NullString(r.ResponseJSON), started, finished, now)
	if err != nil {
		return CronJobRun{}, err
	}
	s.notifyWrite(ctx)
	return r, nil
}

// UpdateCronJobRunStatus 更新执行记录状态与结果。
func (s *Store) UpdateCronJobRunStatus(ctx context.Context, projectID, id, status, errMsg string, durationMs int64, response string, finished *time.Time) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	var finAny any
	if finished != nil && !finished.IsZero() {
		finAny = finished.UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE sys_cron_job_runs SET status=?, error=?, duration_ms=?, response_json=?,
		 finished_at = CASE WHEN ? IS NOT NULL THEN ? ELSE finished_at END
		 WHERE id=? AND project_id=?`,
		status, errMsg, durationMs, sqlval.NullString(response), finAny, finAny, id, projectID)
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// ListCronJobRuns 按任务列出执行记录（新→旧），最多 limit 条。
func (s *Store) ListCronJobRuns(ctx context.Context, projectID, jobID string, limit int) ([]CronJobRun, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	limit = pagelimit.Normalize(limit, 20, 100)
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+cronJobRunColumns+`
		 FROM sys_cron_job_runs WHERE project_id = ? AND job_id = ?
		 ORDER BY created_at DESC LIMIT ?`, projectID, jobID, int64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CronJobRun, 0)
	for rows.Next() {
		r, err := scanCronJobRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanCronJob(sc rowScanner) (CronJob, error) {
	var j CronJob
	var enabled int64
	var cronExpr sql.NullString
	var interval sql.NullInt64
	var lastRun, nextRun, runAt sql.NullTime
	// v49 新增列由 ALTER ... DEFAULT 补齐，历史行读到的可能是 NULL，用 NullString 兜底。
	var targetKind, agentID, agentThreadID, agentPrompt sql.NullString
	if err := sc.Scan(&j.ID, &j.ProjectID, &j.Name, &j.Description, &j.ScheduleKind, &cronExpr, &interval, &runAt,
		&j.FuncFile, &j.FuncExport, &j.InputJSON, &enabled, &lastRun, &nextRun,
		&j.LastStatus, &j.LastError, &j.RunCount, &j.CreatedBy, &j.CreatedAt, &j.UpdatedAt,
		&targetKind, &agentID, &agentThreadID, &agentPrompt); err != nil {
		return CronJob{}, err
	}
	j.TargetKind = targetKind.String
	j.AgentID = agentID.String
	j.AgentThreadID = agentThreadID.String
	j.AgentPrompt = agentPrompt.String
	j.TargetKind = j.EffectiveTargetKind()
	j.Enabled = enabled != 0
	j.CronExpr = cronExpr.String
	j.IntervalSeconds = interval.Int64
	if runAt.Valid {
		j.RunAt = runAt.Time
	}
	if lastRun.Valid {
		j.LastRunAt = lastRun.Time
	}
	if nextRun.Valid {
		j.NextRunAt = nextRun.Time
	}
	return j, nil
}

func scanCronJobRun(sc rowScanner) (CronJobRun, error) {
	var r CronJobRun
	var response sql.NullString
	var started, finished sql.NullTime
	if err := sc.Scan(&r.ID, &r.JobID, &r.ProjectID, &r.Trigger, &r.Status, &r.Error, &r.DurationMs, &response,
		&started, &finished, &r.CreatedAt); err != nil {
		return CronJobRun{}, err
	}
	r.ResponseJSON = response.String
	if started.Valid {
		r.StartedAt = started.Time
	}
	if finished.Valid {
		r.FinishedAt = finished.Time
	}
	return r, nil
}

// nullInt64IfZero 0 转 NULL（interval_seconds 可空列）。
func nullInt64IfZero(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
