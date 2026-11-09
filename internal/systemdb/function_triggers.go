// 文件说明：function_triggers.go — internal/systemdb 中 FunctionTrigger、CreateFunctionTrigger 等符号的实现
//
// 用法
//
//	(s *Store) CreateFunctionTrigger(ctx context.Context, trigger FunctionTrigger) ...
//	(s *Store) ListFunctionTriggers(ctx context.Context, projectID string) ...
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/function_triggers.go
//	         │ 依赖
//	         ▼
//	[下游] internal/utils/sqlval
//	[同包] agent_schedules.go, agents.go, async_flush.go, bootstrap.go, cron_jobs.go 等 22 个
//
// 主要符号
//
//	type FunctionTrigger struct {…}
//	(s *Store) CreateFunctionTrigger(ctx context.Context, trigger FunctionTrigger) ...
//	(s *Store) ListFunctionTriggers(ctx context.Context, projectID string) ...
//	(s *Store) GetFunctionTrigger(ctx context.Context, projectID, id string) ...
//	(s *Store) UpdateFunctionTrigger(ctx context.Context, trigger FunctionTrigger) ...
//	(s *Store) ArchiveFunctionTrigger(ctx context.Context, projectID, id string) error
//	... 另有 1 个顶层符号

package systemdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/linkxzhou/SimpleBase/internal/utils/sqlval"
	"time"

	"github.com/google/uuid"
)

type FunctionTrigger struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Kind       string    `json:"kind"`
	Target     string    `json:"target"`
	Events     []string  `json:"events"`
	FuncFile   string    `json:"func_file"`
	FuncExport string    `json:"func_export"`
	Filter     string    `json:"filter,omitempty"`
	Enabled    bool      `json:"enabled"`
	CreatedBy  string    `json:"created_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (s *Store) CreateFunctionTrigger(ctx context.Context, trigger FunctionTrigger) (FunctionTrigger, error) {
	if s == nil || s.db == nil {
		return FunctionTrigger{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if trigger.ID == "" {
		trigger.ID = uuid.NewString()
	}
	trigger.CreatedAt, trigger.UpdatedAt = now, now
	events, _ := json.Marshal(trigger.Events)
	_, err := s.db.ExecContext(ctx, `INSERT INTO sys_function_triggers(id, project_id, kind, target, events_json, func_file, func_export, filter, enabled, created_by, created_at, updated_at, archived_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`, trigger.ID, trigger.ProjectID, trigger.Kind, trigger.Target, string(events), trigger.FuncFile, trigger.FuncExport, trigger.Filter, sqlval.BoolInt(trigger.Enabled), trigger.CreatedBy, now, now)
	if err == nil {
		s.notifyWrite(ctx)
	}
	return trigger, err
}

func (s *Store) ListFunctionTriggers(ctx context.Context, projectID string) ([]FunctionTrigger, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, kind, target, events_json, func_file, func_export, filter, enabled, created_by, created_at, updated_at FROM sys_function_triggers WHERE project_id = ? AND archived_at IS NULL ORDER BY created_at, id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FunctionTrigger{}
	for rows.Next() {
		trigger, err := scanFunctionTrigger(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, trigger)
	}
	return out, rows.Err()
}

func (s *Store) GetFunctionTrigger(ctx context.Context, projectID, id string) (FunctionTrigger, error) {
	if s == nil || s.db == nil {
		return FunctionTrigger{}, ErrUnavailable
	}
	return scanFunctionTrigger(s.db.QueryRowContext(ctx, `SELECT id, project_id, kind, target, events_json, func_file, func_export, filter, enabled, created_by, created_at, updated_at FROM sys_function_triggers WHERE project_id = ? AND id = ? AND archived_at IS NULL`, projectID, id))
}

func (s *Store) UpdateFunctionTrigger(ctx context.Context, trigger FunctionTrigger) (FunctionTrigger, error) {
	if s == nil || s.db == nil {
		return FunctionTrigger{}, ErrUnavailable
	}
	trigger.UpdatedAt = time.Now().UTC()
	events, _ := json.Marshal(trigger.Events)
	result, err := s.db.ExecContext(ctx, `UPDATE sys_function_triggers SET kind=?, target=?, events_json=?, func_file=?, func_export=?, filter=?, enabled=?, updated_at=? WHERE project_id=? AND id=? AND archived_at IS NULL`, trigger.Kind, trigger.Target, string(events), trigger.FuncFile, trigger.FuncExport, trigger.Filter, sqlval.BoolInt(trigger.Enabled), trigger.UpdatedAt, trigger.ProjectID, trigger.ID)
	if err != nil {
		return FunctionTrigger{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return FunctionTrigger{}, sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return trigger, nil
}

func (s *Store) ArchiveFunctionTrigger(ctx context.Context, projectID, id string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	result, err := s.db.ExecContext(ctx, `UPDATE sys_function_triggers SET archived_at=?, updated_at=?, enabled=0 WHERE project_id=? AND id=? AND archived_at IS NULL`, time.Now().UTC(), time.Now().UTC(), projectID, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return nil
}

func scanFunctionTrigger(row rowScanner) (FunctionTrigger, error) {
	var trigger FunctionTrigger
	var events string
	var enabled int64
	err := row.Scan(&trigger.ID, &trigger.ProjectID, &trigger.Kind, &trigger.Target, &events, &trigger.FuncFile, &trigger.FuncExport, &trigger.Filter, &enabled, &trigger.CreatedBy, &trigger.CreatedAt, &trigger.UpdatedAt)
	if err != nil {
		return FunctionTrigger{}, err
	}
	trigger.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(events), &trigger.Events); err != nil {
		return FunctionTrigger{}, err
	}
	return trigger, nil
}
