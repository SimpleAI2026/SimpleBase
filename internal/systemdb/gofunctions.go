package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// GoFunction 是 sys_gofunctions 一行（ui-gofunction-plan §5）。
// Source 为权威；Exports 为保存时 ParseFuncList 结果的冗余快照。
type GoFunction struct {
	ID        string
	ProjectID string
	Name      string // {name}.go 的 basename；同项目唯一（未归档）
	Source    string
	Exports   []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ListGoFunctions 列出项目内未归档云函数（不含软删）。
func (s *Store) ListGoFunctions(ctx context.Context, projectID string) ([]GoFunction, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, source, exports_json, created_at, updated_at
		 FROM sys_gofunctions WHERE project_id = ? AND archived_at IS NULL
		 ORDER BY created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]GoFunction, 0)
	for rows.Next() {
		g, err := scanGoFunction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetGoFunction 按项目 + 名称取未归档云函数。
func (s *Store) GetGoFunction(ctx context.Context, projectID, name string) (GoFunction, error) {
	if s == nil || s.db == nil {
		return GoFunction{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, source, exports_json, created_at, updated_at
		 FROM sys_gofunctions WHERE name = ? AND project_id = ? AND archived_at IS NULL`,
		name, projectID)
	g, err := scanGoFunction(row)
	if errors.Is(err, sql.ErrNoRows) {
		return GoFunction{}, sql.ErrNoRows
	}
	return g, err
}

// CreateGoFunction 写入一个云函数。
func (s *Store) CreateGoFunction(ctx context.Context, g GoFunction) (GoFunction, error) {
	if s == nil || s.db == nil {
		return GoFunction{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if g.ID == "" {
		g.ID = uuid.NewString()
	}
	g.CreatedAt = now
	g.UpdatedAt = now
	exportsJSON, err := marshalStringSlice(g.Exports)
	if err != nil {
		return GoFunction{}, err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sys_gofunctions(id, project_id, name, source, exports_json, created_at, updated_at, archived_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, NULL)`,
		g.ID, g.ProjectID, g.Name, g.Source, exportsJSON, now, now)
	if err != nil {
		return GoFunction{}, err
	}
	s.notifyWrite(ctx)
	return g, nil
}

// UpdateGoFunction 更新源码与导出快照（name 不可改）。
func (s *Store) UpdateGoFunction(ctx context.Context, g GoFunction) (GoFunction, error) {
	if s == nil || s.db == nil {
		return GoFunction{}, ErrUnavailable
	}
	now := time.Now().UTC()
	g.UpdatedAt = now
	exportsJSON, err := marshalStringSlice(g.Exports)
	if err != nil {
		return GoFunction{}, err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_gofunctions SET source=?, exports_json=?, updated_at=?
		 WHERE name=? AND project_id=? AND archived_at IS NULL`,
		g.Source, exportsJSON, now, g.Name, g.ProjectID)
	if err != nil {
		return GoFunction{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return GoFunction{}, sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return g, nil
}

// ArchiveGoFunction 软删云函数。
func (s *Store) ArchiveGoFunction(ctx context.Context, projectID, name string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_gofunctions SET archived_at=?, updated_at=? WHERE name=? AND project_id=? AND archived_at IS NULL`,
		now, now, name, projectID)
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

// CountGoFunctions 返回未归档数量（含 0）。
func (s *Store) CountGoFunctions(ctx context.Context, projectID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_gofunctions WHERE project_id = ? AND archived_at IS NULL`, projectID).Scan(&n)
	return int(n), err
}

func scanGoFunction(sc rowScanner) (GoFunction, error) {
	var g GoFunction
	var exportsJSON string
	if err := sc.Scan(&g.ID, &g.ProjectID, &g.Name, &g.Source, &exportsJSON, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return GoFunction{}, err
	}
	exports, err := unmarshalStringSlice(exportsJSON)
	if err != nil {
		return GoFunction{}, err
	}
	g.Exports = exports
	return g, nil
}
