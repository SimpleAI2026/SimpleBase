// gofunctions.go 兼容层：旧 GoFunction 读写 API 映射到 sys_go_funcs / sys_go_func_versions
// （gofunction-versions-testplan：对外 HTTP 已是版本化契约；此文件供既有测试与内部调用过渡）。
package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// GoFunction 是旧「一文件一源码」视图；底层为实体 + 生效/最新版本。
type GoFunction struct {
	ID        string
	ProjectID string
	Name      string
	Source    string
	Exports   []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func fromGoFuncParts(f GoFunc, v GoFuncVersion) GoFunction {
	src := v.Source
	exports := v.Exports
	if exports == nil {
		exports = []string{}
	}
	return GoFunction{
		ID:        f.ID,
		ProjectID: f.ProjectID,
		Name:      f.Name,
		Source:    src,
		Exports:   exports,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}
}

// ListGoFunctions 列出项目内云函数（源码取生效版，否则最新版）。
func (s *Store) ListGoFunctions(ctx context.Context, projectID string) ([]GoFunction, error) {
	funcs, err := s.ListGoFuncs(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]GoFunction, 0, len(funcs))
	for _, f := range funcs {
		v, err := s.viewVersion(ctx, projectID, f)
		if err != nil {
			continue
		}
		out = append(out, fromGoFuncParts(f, v))
	}
	return out, nil
}

func (s *Store) viewVersion(ctx context.Context, projectID string, f GoFunc) (GoFuncVersion, error) {
	if f.ActiveVersion > 0 {
		return s.GetGoFuncVersion(ctx, projectID, f.Name, f.ActiveVersion)
	}
	return s.LatestGoFuncVersion(ctx, projectID, f.Name)
}

// GetGoFunction 按项目 + 名称取（源码同上）。
func (s *Store) GetGoFunction(ctx context.Context, projectID, name string) (GoFunction, error) {
	f, err := s.GetGoFunc(ctx, projectID, name)
	if err != nil {
		return GoFunction{}, err
	}
	v, err := s.viewVersion(ctx, projectID, f)
	if errors.Is(err, sql.ErrNoRows) {
		return fromGoFuncParts(f, GoFuncVersion{Exports: []string{}}), nil
	}
	if err != nil {
		return GoFunction{}, err
	}
	return fromGoFuncParts(f, v), nil
}

// CreateGoFunction 创建实体 + v1 并默认生效。
func (s *Store) CreateGoFunction(ctx context.Context, g GoFunction) (GoFunction, error) {
	f, err := s.CreateGoFunc(ctx, GoFunc{
		ProjectID: g.ProjectID,
		Name:      g.Name,
	})
	if err != nil {
		return GoFunction{}, err
	}
	v, err := s.AppendGoFuncVersion(ctx, GoFuncVersion{
		FuncID: f.ID, ProjectID: g.ProjectID, Name: g.Name,
		Source: g.Source, Exports: g.Exports,
	})
	if err != nil {
		return GoFunction{}, err
	}
	if err := s.ActivateGoFuncVersion(ctx, g.ProjectID, g.Name, v.Version); err != nil {
		return GoFunction{}, err
	}
	f.ActiveVersion = v.Version
	return fromGoFuncParts(f, v), nil
}

// UpdateGoFunction 追加新版本并设为生效（旧语义「覆盖源码」→ 新语义「存新版」）。
func (s *Store) UpdateGoFunction(ctx context.Context, g GoFunction) (GoFunction, error) {
	f, err := s.GetGoFunc(ctx, g.ProjectID, g.Name)
	if err != nil {
		return GoFunction{}, err
	}
	v, err := s.AppendGoFuncVersion(ctx, GoFuncVersion{
		FuncID: f.ID, ProjectID: g.ProjectID, Name: g.Name,
		Source: g.Source, Exports: g.Exports,
	})
	if err != nil {
		return GoFunction{}, err
	}
	if err := s.ActivateGoFuncVersion(ctx, g.ProjectID, g.Name, v.Version); err != nil {
		return GoFunction{}, err
	}
	f.ActiveVersion = v.Version
	f.UpdatedAt = time.Now().UTC()
	return fromGoFuncParts(f, v), nil
}

// ArchiveGoFunction 软删实体。
func (s *Store) ArchiveGoFunction(ctx context.Context, projectID, name string) error {
	return s.ArchiveGoFunc(ctx, projectID, name)
}

// CountGoFunctions 未归档数量。
func (s *Store) CountGoFunctions(ctx context.Context, projectID string) (int, error) {
	return s.CountGoFuncs(ctx, projectID)
}
