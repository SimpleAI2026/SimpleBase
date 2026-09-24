// gofuncs.go 云函数实体 + 不可变版本快照（gofunction-versions-testplan §4）。
// 旧 sys_gofunctions 已废弃；本文件只读写 sys_go_funcs / sys_go_func_versions。
package systemdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MaxGoFuncVersions 每函数版本数硬上限（plan D3）。
const MaxGoFuncVersions = 50

// ErrVersionLimit 版本数达到上限。
var ErrVersionLimit = errors.New("systemdb: go func version limit exceeded")

// ErrNoActiveVersion 函数尚未发布任何版本。
var ErrNoActiveVersion = errors.New("systemdb: no active go function version")

// GoFunc 是 sys_go_funcs 一行。
type GoFunc struct {
	ID            string
	ProjectID     string
	Name          string
	ActiveVersion int64 // 0 = 未发布
	Description   string
	CreatedBy     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ArchivedAt    *time.Time
}

// GoFuncVersion 是 sys_go_func_versions 一行（创建后源码不可变）。
type GoFuncVersion struct {
	ID        string
	FuncID    string
	ProjectID string
	Name      string
	Version   int64
	Source    string
	Exports   []string
	Note      string
	CreatedBy string
	CreatedAt time.Time
}

// ListGoFuncs 列出项目内未归档云函数。
func (s *Store) ListGoFuncs(ctx context.Context, projectID string) ([]GoFunc, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, active_version, description, created_by, created_at, updated_at
		 FROM sys_go_funcs WHERE project_id = ? AND archived_at IS NULL
		 ORDER BY created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]GoFunc, 0)
	for rows.Next() {
		var f GoFunc
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Name, &f.ActiveVersion, &f.Description,
			&f.CreatedBy, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// GetGoFunc 按项目 + 名称取未归档实体。
func (s *Store) GetGoFunc(ctx context.Context, projectID, name string) (GoFunc, error) {
	if s == nil || s.db == nil {
		return GoFunc{}, ErrUnavailable
	}
	var f GoFunc
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, active_version, description, created_by, created_at, updated_at
		 FROM sys_go_funcs WHERE name = ? AND project_id = ? AND archived_at IS NULL`,
		name, projectID).Scan(&f.ID, &f.ProjectID, &f.Name, &f.ActiveVersion, &f.Description,
		&f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GoFunc{}, sql.ErrNoRows
	}
	return f, err
}

// CountGoFuncs 未归档实体数。
func (s *Store) CountGoFuncs(ctx context.Context, projectID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_go_funcs WHERE project_id = ? AND archived_at IS NULL`, projectID).Scan(&n)
	return n, err
}

// CreateGoFunc 创建实体（不含版本）。
func (s *Store) CreateGoFunc(ctx context.Context, f GoFunc) (GoFunc, error) {
	if s == nil || s.db == nil {
		return GoFunc{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	f.CreatedAt = now
	f.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_go_funcs(id, project_id, name, active_version, description, created_by, created_at, updated_at, archived_at)
		 VALUES(?,?,?,?,?,?,?,?,NULL)`,
		f.ID, f.ProjectID, f.Name, f.ActiveVersion, f.Description, f.CreatedBy, now, now)
	if err != nil {
		return GoFunc{}, err
	}
	s.notifyWrite(ctx)
	return f, nil
}

// UpdateGoFuncMeta 只改 description / active_version / updated_at。
func (s *Store) UpdateGoFuncMeta(ctx context.Context, f GoFunc) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_go_funcs SET active_version=?, description=?, updated_at=?
		 WHERE id=? AND archived_at IS NULL`,
		f.ActiveVersion, f.Description, now, f.ID)
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

// ActivateGoFuncVersion 设置生效版本。
func (s *Store) ActivateGoFuncVersion(ctx context.Context, projectID, name string, version int64) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	var exists int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_go_func_versions WHERE project_id=? AND name=? AND version=?`,
		projectID, name, version).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return sql.ErrNoRows
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_go_funcs SET active_version=?, updated_at=?
		 WHERE project_id=? AND name=? AND archived_at IS NULL`,
		version, now, projectID, name)
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

// ArchiveGoFunc 软删实体（版本行保留）。
func (s *Store) ArchiveGoFunc(ctx context.Context, projectID, name string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_go_funcs SET archived_at=?, updated_at=?
		 WHERE name=? AND project_id=? AND archived_at IS NULL`, now, now, name, projectID)
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

// AppendGoFuncVersion 追加不可变版本（version=MAX+1，唯一冲突重试 ≤3）。
func (s *Store) AppendGoFuncVersion(ctx context.Context, v GoFuncVersion) (GoFuncVersion, error) {
	if s == nil || s.db == nil {
		return GoFuncVersion{}, ErrUnavailable
	}
	if v.Exports == nil {
		v.Exports = []string{}
	}
	exportsJSON, err := json.Marshal(v.Exports)
	if err != nil {
		return GoFuncVersion{}, err
	}
	var last GoFuncVersion
	for attempt := 0; attempt < 3; attempt++ {
		var maxVer sql.NullInt64
		if err := s.db.QueryRowContext(ctx,
			`SELECT MAX(version) FROM sys_go_func_versions WHERE func_id = ?`, v.FuncID).Scan(&maxVer); err != nil {
			return GoFuncVersion{}, err
		}
		var cnt int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sys_go_func_versions WHERE func_id = ?`, v.FuncID).Scan(&cnt); err != nil {
			return GoFuncVersion{}, err
		}
		if cnt >= MaxGoFuncVersions {
			return GoFuncVersion{}, ErrVersionLimit
		}
		next := int64(1)
		if maxVer.Valid {
			next = maxVer.Int64 + 1
		}
		now := time.Now().UTC()
		last = v
		last.ID = uuid.NewString()
		last.Version = next
		last.CreatedAt = now
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO sys_go_func_versions(id, func_id, project_id, name, version, source, exports_json, note, created_by, created_at)
			 VALUES(?,?,?,?,?,?,?,?,?,?)`,
			last.ID, last.FuncID, last.ProjectID, last.Name, last.Version, last.Source,
			string(exportsJSON), last.Note, last.CreatedBy, now)
		if err != nil {
			if isGoFuncDuplicate(err) {
				continue
			}
			return GoFuncVersion{}, err
		}
		s.notifyWrite(ctx)
		return last, nil
	}
	return GoFuncVersion{}, fmt.Errorf("systemdb: allocate go func version failed after retries")
}

// ListGoFuncVersions 版本摘要（source 置空），按 version 降序。
func (s *Store) ListGoFuncVersions(ctx context.Context, projectID, name string) ([]GoFuncVersion, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, func_id, project_id, name, version, source, exports_json, note, created_by, created_at
		 FROM sys_go_func_versions WHERE project_id = ? AND name = ?
		 ORDER BY version DESC`, projectID, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]GoFuncVersion, 0)
	for rows.Next() {
		v, err := scanGoFuncVersion(rows)
		if err != nil {
			return nil, err
		}
		v.Source = "" // 列表不带源码
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetGoFuncVersion 取指定版本（含 source）。
func (s *Store) GetGoFuncVersion(ctx context.Context, projectID, name string, version int64) (GoFuncVersion, error) {
	if s == nil || s.db == nil {
		return GoFuncVersion{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, func_id, project_id, name, version, source, exports_json, note, created_by, created_at
		 FROM sys_go_func_versions WHERE project_id = ? AND name = ? AND version = ?`,
		projectID, name, version)
	return scanGoFuncVersion(row)
}

// LatestGoFuncVersion 最新版本（含 source）。
func (s *Store) LatestGoFuncVersion(ctx context.Context, projectID, name string) (GoFuncVersion, error) {
	if s == nil || s.db == nil {
		return GoFuncVersion{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, func_id, project_id, name, version, source, exports_json, note, created_by, created_at
		 FROM sys_go_func_versions WHERE project_id = ? AND name = ?
		 ORDER BY version DESC LIMIT 1`, projectID, name)
	return scanGoFuncVersion(row)
}

// ResolveActiveSource 解析生效版源码（/go 与定时任务统一入口，plan §2.1）。
func (s *Store) ResolveActiveSource(ctx context.Context, projectID, name string) (GoFuncVersion, error) {
	f, err := s.GetGoFunc(ctx, projectID, name)
	if err != nil {
		return GoFuncVersion{}, err
	}
	if f.ActiveVersion <= 0 {
		return GoFuncVersion{}, ErrNoActiveVersion
	}
	return s.GetGoFuncVersion(ctx, projectID, name, f.ActiveVersion)
}

// LatestGoFuncVersionNumber 最新版本号（0=无）。
func (s *Store) LatestGoFuncVersionNumber(ctx context.Context, funcID string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	var n sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT MAX(version) FROM sys_go_func_versions WHERE func_id = ?`, funcID).Scan(&n); err != nil {
		return 0, err
	}
	if !n.Valid {
		return 0, nil
	}
	return n.Int64, nil
}

// RecordGoFuncInvoke 写调用流水（不含 body/响应）。
func (s *Store) RecordGoFuncInvoke(ctx context.Context, projectID, funcName, functionName string,
	version int64, channel string, statusCode int, durationMs int64, requestID, actor string) {
	if s == nil || s.db == nil {
		return
	}
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO sys_go_func_invokes(id, project_id, func_name, function_name, version, channel,
			status_code, duration_ms, request_id, actor, created_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		uuid.NewString(), projectID, funcName, functionName, version, channel,
		statusCode, durationMs, requestID, actor, time.Now().UTC())
	s.notifyWrite(ctx)
}

func scanGoFuncVersion(row rowScanner) (GoFuncVersion, error) {
	var (
		v           GoFuncVersion
		exportsJSON string
	)
	err := row.Scan(&v.ID, &v.FuncID, &v.ProjectID, &v.Name, &v.Version, &v.Source,
		&exportsJSON, &v.Note, &v.CreatedBy, &v.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GoFuncVersion{}, sql.ErrNoRows
		}
		return GoFuncVersion{}, err
	}
	v.Exports = []string{}
	if strings.TrimSpace(exportsJSON) != "" {
		_ = json.Unmarshal([]byte(exportsJSON), &v.Exports)
	}
	return v, nil
}

func isGoFuncDuplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "already exists")
}
