// 文件说明：log_stats.go — internal/systemdb 中 LogStat、LogStats 等符号的实现
//
// 用法
//
//	(s *Store) LogStats(ctx context.Context, projectID, groupBy string, from, to ...
//	(s *Store) ExportLogs(ctx context.Context, query LogQuery) ([]LogEvent, error)
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/log_stats.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_schedules.go, agents.go, async_flush.go, bootstrap.go, cron_jobs.go 等 22 个
//
// 主要符号
//
//	type LogStat struct {…}
//	(s *Store) LogStats(ctx context.Context, projectID, groupBy string, from, to ...
//	(s *Store) ExportLogs(ctx context.Context, query LogQuery) ([]LogEvent, error)

package systemdb

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type LogStat struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

func (s *Store) LogStats(ctx context.Context, projectID, groupBy string, from, to time.Time) ([]LogStat, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	expression := "level"
	switch groupBy {
	case "hour":
		expression = "date_trunc('hour', occurred_at)"
	case "path":
		expression = "COALESCE(json_extract_string(fields_json, '$.route'), 'unknown')"
	case "level", "":
	default:
		return nil, fmt.Errorf("invalid log group")
	}
	query := `SELECT ` + expression + `, COUNT(*) FROM sys_log_events WHERE occurred_at >= ? AND occurred_at <= ?`
	args := []any{from.UTC(), to.UTC()}
	if projectID != "" && !IsAdminProject(projectID) {
		query += ` AND (project_id = ? OR project_id IS NULL)`
		args = append(args, projectID)
	}
	query += ` GROUP BY ` + expression + ` ORDER BY ` + expression
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LogStat{}
	for rows.Next() {
		var stat LogStat
		var key any
		if err := rows.Scan(&key, &stat.Count); err != nil {
			return nil, err
		}
		switch value := key.(type) {
		case time.Time:
			stat.Key = value.UTC().Format(time.RFC3339)
		case string:
			stat.Key = value
		case []byte:
			stat.Key = string(value)
		default:
			stat.Key = fmt.Sprint(value)
		}
		out = append(out, stat)
	}
	return out, rows.Err()
}

func (s *Store) ExportLogs(ctx context.Context, query LogQuery) ([]LogEvent, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	query.Limit = 100000
	sqlText := `SELECT id, project_id, level, logger, message, fields_json, request_id, occurred_at FROM sys_log_events WHERE occurred_at >= ? AND occurred_at <= ?`
	args := []any{query.From.UTC(), query.To.UTC()}
	if query.ProjectID != "" && !IsAdminProject(query.ProjectID) {
		sqlText += ` AND (project_id = ? OR project_id IS NULL)`
		args = append(args, query.ProjectID)
	}
	if query.Level != "" {
		sqlText += ` AND level = ?`
		args = append(args, query.Level)
	}
	if query.Q != "" {
		sqlText += ` AND message LIKE ?`
		args = append(args, "%"+query.Q+"%")
	}
	sqlText += ` ORDER BY occurred_at DESC LIMIT 100000`
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LogEvent{}
	for rows.Next() {
		var event LogEvent
		var project sql.NullString
		if err := rows.Scan(&event.ID, &project, &event.Level, &event.Logger, &event.Message, &event.FieldsJSON, &event.RequestID, &event.OccurredAt); err != nil {
			return nil, err
		}
		if project.Valid {
			event.ProjectID = project.String
		}
		out = append(out, event)
	}
	return out, rows.Err()
}
