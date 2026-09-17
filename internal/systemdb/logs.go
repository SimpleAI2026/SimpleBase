package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// LogEvent 是 sys_log_events 一行。
type LogEvent struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Level      string    `json:"level"`
	Logger     string    `json:"logger"`
	Message    string    `json:"message"`
	FieldsJSON string    `json:"fields_json,omitempty"`
	RequestID  string    `json:"request_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// LogQuery 是日志查询条件。
type LogQuery struct {
	ProjectID string
	Level     string
	Q         string
	From      time.Time
	To        time.Time
	Limit     int
}

// Retention 是日志保留策略。
type Retention struct {
	Scope     string
	ProjectID string
	KeepDays  int
	UpdatedAt time.Time
}

// RecordLog 缓冲一条运行日志。
func (s *Store) RecordLog(ev LogEvent) {
	if s == nil {
		return
	}
	if ev.ID == "" {
		ev.ID = uuid.NewString()
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}
	if ev.Level == "" {
		ev.Level = "info"
	}
	s.mu.Lock()
	s.logBuf = append(s.logBuf, ev)
	n := len(s.logBuf)
	s.mu.Unlock()
	if n >= 64 {
		_ = s.FlushLogs(context.Background())
	}
}

// FlushLogs 将缓冲日志写入 sys_log_events。
func (s *Store) FlushLogs(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	s.mu.Lock()
	batch := s.logBuf
	s.logBuf = nil
	s.mu.Unlock()
	if len(batch) == 0 {
		return nil
	}
	for _, e := range batch {
		var project any
		if e.ProjectID != "" {
			project = e.ProjectID
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO sys_log_events(id, project_id, level, logger, message, fields_json, request_id, occurred_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			e.ID, project, e.Level, e.Logger, e.Message, e.FieldsJSON, e.RequestID, e.OccurredAt.UTC()); err != nil {
			return err
		}
	}
	s.notifyWrite(ctx)
	return nil
}

// QueryLogs 按条件查询运行日志。ProjectID 为 admin 系统项目时查询全系统。
func (s *Store) QueryLogs(ctx context.Context, q LogQuery) ([]LogEvent, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}
	sqlStr := `SELECT id, project_id, level, logger, message, fields_json, request_id, occurred_at
	           FROM sys_log_events WHERE 1=1`
	args := []any{}
	if q.ProjectID != "" && !IsAdminProject(q.ProjectID) {
		sqlStr += " AND (project_id = ? OR project_id IS NULL)"
		args = append(args, q.ProjectID)
	}
	if q.Level != "" {
		sqlStr += " AND level = ?"
		args = append(args, q.Level)
	}
	if q.Q != "" {
		sqlStr += " AND message LIKE ?"
		args = append(args, "%"+q.Q+"%")
	}
	if !q.From.IsZero() {
		sqlStr += " AND occurred_at >= ?"
		args = append(args, q.From.UTC())
	}
	if !q.To.IsZero() {
		sqlStr += " AND occurred_at <= ?"
		args = append(args, q.To.UTC())
	}
	sqlStr += " ORDER BY occurred_at DESC LIMIT ?"
	args = append(args, int64(q.Limit))
	rows, err := s.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LogEvent, 0)
	for rows.Next() {
		var e LogEvent
		var project sql.NullString
		if err := rows.Scan(&e.ID, &project, &e.Level, &e.Logger, &e.Message, &e.FieldsJSON, &e.RequestID, &e.OccurredAt); err != nil {
			return nil, err
		}
		if project.Valid {
			e.ProjectID = project.String
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LogLevelCount 是按级别聚合的条数。
type LogLevelCount struct {
	Level string
	Count int64
}

// LogLevelStats 按 level 计数当前项目（含 project_id 为空的全局行）。
func (s *Store) LogLevelStats(ctx context.Context, projectID string) ([]LogLevelCount, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT level, COUNT(*) FROM sys_log_events
		 WHERE project_id = ? OR project_id IS NULL
		 GROUP BY level ORDER BY level`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LogLevelCount, 0)
	for rows.Next() {
		var c LogLevelCount
		if err := rows.Scan(&c.Level, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetRetention 读取保留策略；缺省 14 天。
func (s *Store) GetRetention(ctx context.Context, projectID string) (Retention, error) {
	if s == nil || s.db == nil {
		return Retention{}, ErrUnavailable
	}
	var r Retention
	var keep int64
	err := s.db.QueryRowContext(ctx,
		`SELECT scope, project_id, keep_days, updated_at FROM sys_log_retention
		 WHERE (scope = 'project' AND project_id = ?) OR scope = 'global'
		 ORDER BY CASE WHEN scope = 'project' THEN 0 ELSE 1 END LIMIT 1`,
		projectID).Scan(&r.Scope, &r.ProjectID, &keep, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Retention{Scope: "global", KeepDays: 14}, nil
	}
	if err != nil {
		return Retention{}, err
	}
	r.KeepDays = int(keep)
	return r, nil
}

// PutRetention 写入项目保留天数。
func (s *Store) PutRetention(ctx context.Context, projectID string, keepDays int) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if keepDays <= 0 {
		keepDays = 14
	}
	now := time.Now().UTC()
	var existing string
	err := s.db.QueryRowContext(ctx,
		`SELECT scope FROM sys_log_retention WHERE scope = 'project' AND project_id = ?`, projectID).Scan(&existing)
	if err == nil {
		_, err = s.db.ExecContext(ctx,
			`UPDATE sys_log_retention SET keep_days=?, updated_at=? WHERE scope='project' AND project_id=?`,
			int64(keepDays), now, projectID)
	} else if errors.Is(err, sql.ErrNoRows) {
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO sys_log_retention(scope, project_id, keep_days, updated_at) VALUES('project', ?, ?, ?)`,
			projectID, int64(keepDays), now)
	}
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}
