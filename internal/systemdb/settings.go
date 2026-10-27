// 文件说明：settings.go — internal/systemdb 中 SettingKV、GetGlobalSetting 等符号的实现
//
// 用法
//
//	(s *Store) GetGlobalSetting(ctx context.Context, key string) (SettingKV, error)  // GetGlobalSetting 读取实例级设置
//	(s *Store) ListGlobalSettings(ctx context.Context) ([]SettingKV, error)  // ListGlobalSettings 列出全部实例级设置
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/settings.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_schedules.go, agents.go, async_flush.go, bootstrap.go, cron_jobs.go 等 22 个
//
// 主要符号
//
//	type SettingKV struct {…}
//	(s *Store) GetGlobalSetting(ctx context.Context, key string) (SettingKV, error)
//	(s *Store) ListGlobalSettings(ctx context.Context) ([]SettingKV, error)
//	(s *Store) PutGlobalSetting(ctx context.Context, key, valueJSON string) error
//	(s *Store) ListProjectSettings(ctx context.Context, projectID string) ([]SettingKV ...
//	(s *Store) PutProjectSetting(ctx context.Context, projectID, key, valueJSON string) ...

package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// SettingKV 是设置表的一行。
type SettingKV struct {
	Key       string    `json:"key"`
	ValueJSON string    `json:"value_json"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetGlobalSetting 读取实例级设置。
func (s *Store) GetGlobalSetting(ctx context.Context, key string) (SettingKV, error) {
	if s == nil || s.db == nil {
		return SettingKV{}, ErrUnavailable
	}
	var row SettingKV
	err := s.db.QueryRowContext(ctx,
		`SELECT key, value_json, updated_at FROM sys_settings_global WHERE key = ?`, key).
		Scan(&row.Key, &row.ValueJSON, &row.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SettingKV{Key: key}, nil
	}
	return row, err
}

// ListGlobalSettings 列出全部实例级设置。
func (s *Store) ListGlobalSettings(ctx context.Context) ([]SettingKV, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value_json, updated_at FROM sys_settings_global ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SettingKV, 0)
	for rows.Next() {
		var row SettingKV
		if err := rows.Scan(&row.Key, &row.ValueJSON, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// PutGlobalSetting 写入实例级设置。
func (s *Store) PutGlobalSetting(ctx context.Context, key, valueJSON string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	var existing string
	err := s.db.QueryRowContext(ctx, `SELECT key FROM sys_settings_global WHERE key = ?`, key).Scan(&existing)
	if err == nil {
		_, err = s.db.ExecContext(ctx, `UPDATE sys_settings_global SET value_json=?, updated_at=? WHERE key=?`, valueJSON, now, key)
	} else if errors.Is(err, sql.ErrNoRows) {
		_, err = s.db.ExecContext(ctx, `INSERT INTO sys_settings_global(key, value_json, updated_at) VALUES(?, ?, ?)`, key, valueJSON, now)
	}
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// ListProjectSettings 列出项目级设置。
func (s *Store) ListProjectSettings(ctx context.Context, projectID string) ([]SettingKV, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value_json, updated_at FROM sys_settings_project WHERE project_id = ? ORDER BY key`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SettingKV, 0)
	for rows.Next() {
		var row SettingKV
		if err := rows.Scan(&row.Key, &row.ValueJSON, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// PutProjectSetting 写入项目级设置。
func (s *Store) PutProjectSetting(ctx context.Context, projectID, key, valueJSON string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	var existing string
	err := s.db.QueryRowContext(ctx,
		`SELECT key FROM sys_settings_project WHERE project_id = ? AND key = ?`, projectID, key).Scan(&existing)
	if err == nil {
		_, err = s.db.ExecContext(ctx,
			`UPDATE sys_settings_project SET value_json=?, updated_at=? WHERE project_id=? AND key=?`,
			valueJSON, now, projectID, key)
	} else if errors.Is(err, sql.ErrNoRows) {
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO sys_settings_project(project_id, key, value_json, updated_at) VALUES(?, ?, ?, ?)`,
			projectID, key, valueJSON, now)
	}
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}
