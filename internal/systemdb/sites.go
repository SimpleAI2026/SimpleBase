// 文件说明：sites.go — 对象存储发布网站（planv6.5）：路径级外部访问开关
//
// 用法
//
//	(s *Store) ListPublicPaths(ctx, projectID) ([]PublicPath, error)
//	(s *Store) SetPublicPath(ctx, projectID, path, kind, public, actor) error
//	(s *Store) DeletePublicPathsUnder(ctx, projectID, prefix) error
//	MatchPublicPath(paths, key) (PublicPath, bool)                       // 纯函数：file 精确 / folder 最长前缀
//
// 站点访问是只读面，只读实例同样可服务；写入只来自管理 API（已过 writeGate）。

package systemdb

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// 公开路径种类。
const (
	PublicPathFile   = "file"
	PublicPathFolder = "folder"
)

// PublicPath 是 sys_site_published_paths 一行。
type PublicPath struct {
	ID        string
	ProjectID string
	Path      string
	Kind      string
	CreatedBy string
	CreatedAt time.Time
}

// DeleteProjectSite 删除项目全部公开路径（项目删除时调用）。
func (s *Store) DeleteProjectSite(ctx context.Context, projectID string) error {
	_, connErr := s.conn()
	if connErr != nil {
		return connErr
	}

	return s.DeletePublicPathsUnder(ctx, projectID, "")
}

// ListPublicPaths 列出项目全部公开路径（按 path 升序）。
func (s *Store) ListPublicPaths(ctx context.Context, projectID string) ([]PublicPath, error) {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return nil, connErr
	}

	rows, err := sqlDB.QueryContext(ctx,
		`SELECT id, project_id, path, kind, created_by, created_at
		 FROM sys_site_published_paths WHERE project_id = ? ORDER BY path ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PublicPath, 0)
	for rows.Next() {
		var p PublicPath
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.Path, &p.Kind, &p.CreatedBy, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetPublicPath 设置路径外部访问。public=false 删除该行；重复打开幂等。
// 路径合法性（ValidateFileKey、folder 以 "/" 结尾）由调用方保证。
func (s *Store) SetPublicPath(ctx context.Context, projectID, path, kind string, public bool, actor string) error {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return connErr
	}

	if !public {
		if _, err := sqlDB.ExecContext(ctx,
			`DELETE FROM sys_site_published_paths WHERE project_id = ? AND path = ? AND kind = ?`,
			projectID, path, kind); err != nil {
			return err
		}
		s.notifyWrite(ctx)
		return nil
	}
	var n int64
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_site_published_paths WHERE project_id = ? AND path = ? AND kind = ?`,
		projectID, path, kind).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO sys_site_published_paths(id, project_id, path, kind, created_by, created_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), projectID, path, kind, actor, time.Now().UTC()); err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// DeletePublicFile 删除对象时清理 kind=file 的精确行（文件夹行保留）。
func (s *Store) DeletePublicFile(ctx context.Context, projectID, key string) error {
	return s.SetPublicPath(ctx, projectID, key, PublicPathFile, false, "")
}

// DeletePublicPathsUnder 删除 prefix 下（含 prefix 本身）的全部公开行；prefix="" 删除整个项目。
func (s *Store) DeletePublicPathsUnder(ctx context.Context, projectID, prefix string) error {
	sqlDB, connErr := s.conn()
	if connErr != nil {
		return connErr
	}

	var err error
	if prefix == "" {
		_, err = sqlDB.ExecContext(ctx, `DELETE FROM sys_site_published_paths WHERE project_id = ?`, projectID)
	} else {
		_, err = sqlDB.ExecContext(ctx,
			`DELETE FROM sys_site_published_paths WHERE project_id = ? AND substr(path, 1, ?) = ?`,
			projectID, int64(len(prefix)), prefix)
	}
	if err != nil {
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// MatchPublicPath 判定 key 是否被公开：file 精确匹配优先，否则取覆盖它的最长 folder 前缀。
// folder 路径必须以 "/" 结尾（"" 为整个桶），因此 "blog/" 不会覆盖 "blog2/x"。
func MatchPublicPath(paths []PublicPath, key string) (PublicPath, bool) {
	var best PublicPath
	found := false
	for _, p := range paths {
		switch p.Kind {
		case PublicPathFile:
			if p.Path == key {
				return p, true
			}
		case PublicPathFolder:
			if p.Path != "" && !strings.HasSuffix(p.Path, "/") {
				continue
			}
			if strings.HasPrefix(key, p.Path) && (!found || len(p.Path) > len(best.Path)) {
				best = p
				found = true
			}
		}
	}
	return best, found
}
