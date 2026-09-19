package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// 定时任务调度模式（ui-cronjob-plan §3）。
const (
	CronJobKindCron     = "cron"
	CronJobKindInterval = "interval"
)

// 定时任务运行状态（ui-cronjob-plan §4.2）。
const (
	CronJobRunRunning   = "running"
	CronJobRunCompleted = "completed"
	CronJobRunFailed    = "failed"
	CronJobRunCanceled  = "canceled"
)

// CronJob 是 sys_cron_jobs 一行（云函数定时调用任务）。
type CronJob struct {
	ID              string
	ProjectID       string
	Name            string
	Description     string
	ScheduleKind    string // cron / interval
	CronExpr        string // kind=cron 时非空
	IntervalSeconds int64  // kind=interval 时非空
	FuncFile        string // sys_gofunctions.name
	FuncExport      string // 导出函数名
	InputJSON       string // 固定入参 JSON 原文，默认 "{}"
	Enabled         bool
	LastRunAt       time.Time // 零值表示从未执行
	NextRunAt       time.Time // 服务端计算；零值表示未排期（禁用/暂停）
	LastStatus      string    // 最近一次 run 状态快照
	LastError       string    // 截断 300
	RunCount        int64
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
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
	var lastRun, nextRun any
	if !j.LastRunAt.IsZero() {
		lastRun = j.LastRunAt.UTC()
	}
	if !j.NextRunAt.IsZero() {
		nextRun = j.NextRunAt.UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_cron_jobs(id, project_id, name, description, schedule_kind, cron_expr, interval_seconds,
			func_file, func_export, input_json, enabled, last_run_at, next_run_at, last_status, last_error, run_count,
			created_by, created_at, updated_at, archived_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		j.ID, j.ProjectID, j.Name, j.Description, j.ScheduleKind, nullString(j.CronExpr), nullInt64IfZero(j.IntervalSeconds),
		j.FuncFile, j.FuncExport, j.InputJSON, enabled, lastRun, nextRun, j.LastStatus, j.LastError, j.RunCount,
		j.CreatedBy, now, now)
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
		`SELECT id, project_id, name, description, schedule_kind, cron_expr, interval_seconds,
			func_file, func_export, input_json, enabled, last_run_at, next_run_at, last_status, last_error, run_count,
			created_by, created_at, updated_at
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
		`SELECT id, project_id, name, description, schedule_kind, cron_expr, interval_seconds,
			func_file, func_export, input_json, enabled, last_run_at, next_run_at, last_status, last_error, run_count,
			created_by, created_at, updated_at
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
		`SELECT id, project_id, name, description, schedule_kind, cron_expr, interval_seconds,
			func_file, func_export, input_json, enabled, last_run_at, next_run_at, last_status, last_error, run_count,
			created_by, created_at, updated_at
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
	var lastRun, nextRun any
	if !j.LastRunAt.IsZero() {
		lastRun = j.LastRunAt.UTC()
	}
	if !j.NextRunAt.IsZero() {
		nextRun = j.NextRunAt.UTC()
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_cron_jobs SET description=?, schedule_kind=?, cron_expr=?, interval_seconds=?,
			func_file=?, func_export=?, input_json=?, enabled=?, last_run_at=?, next_run_at=?,
			last_status=?, last_error=?, run_count=?, updated_at=?
		 WHERE id=? AND project_id=? AND archived_at IS NULL`,
		j.Description, j.ScheduleKind, nullString(j.CronExpr), nullInt64IfZero(j.IntervalSeconds),
		j.FuncFile, j.FuncExport, j.InputJSON, enabled, lastRun, nextRun,
		j.LastStatus, j.LastError, j.RunCount, now, j.ID, j.ProjectID)
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
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, description, schedule_kind, cron_expr, interval_seconds,
			func_file, func_export, input_json, enabled, last_run_at, next_run_at, last_status, last_error, run_count,
			created_by, created_at, updated_at
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
		r.ID, r.JobID, r.ProjectID, r.Trigger, r.Status, r.Error, r.DurationMs, nullString(r.ResponseJSON), started, finished, now)
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
		status, errMsg, durationMs, nullString(response), finAny, finAny, id, projectID)
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
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, job_id, project_id, trigger, status, error, duration_ms, response_json, started_at, finished_at, created_at
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
	var lastRun, nextRun sql.NullTime
	if err := sc.Scan(&j.ID, &j.ProjectID, &j.Name, &j.Description, &j.ScheduleKind, &cronExpr, &interval,
		&j.FuncFile, &j.FuncExport, &j.InputJSON, &enabled, &lastRun, &nextRun,
		&j.LastStatus, &j.LastError, &j.RunCount, &j.CreatedBy, &j.CreatedAt, &j.UpdatedAt); err != nil {
		return CronJob{}, err
	}
	j.Enabled = enabled != 0
	j.CronExpr = cronExpr.String
	j.IntervalSeconds = interval.Int64
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

// nullString 空串转 NULL（cron_expr / response_json 可空列）。
func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// nullInt64IfZero 0 转 NULL（interval_seconds 可空列）。
func nullInt64IfZero(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
