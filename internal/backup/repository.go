// 文件说明：repository.go — internal/backup 中 Repository、NewRepository 等符号的实现
//
// 用法
//
//	NewRepository(db *sql.DB, notify func(context.Context)) *Repository
//	(r *Repository) Create(ctx context.Context, item Item) error
//	(r *Repository) List(ctx context.Context, projectID string) ([]Item, error)
//	同包调用：Get() → restore.go, service.go；Repository → scheduler.go, service.go；Create() → scheduler.go, service.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/backup/repository.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] model.go, restore.go, scheduler.go, service.go
//	[关键引用] Get() ← restore.go, service.go
//
// 主要符号
//
//	type Repository struct {…}
//	NewRepository(db *sql.DB, notify func(context.Context)) *Repository
//	(r *Repository) Create(ctx context.Context, item Item) error
//	(r *Repository) List(ctx context.Context, projectID string) ([]Item, error)
//	(r *Repository) Get(ctx context.Context, projectID, id string) (Item, error)
//	(r *Repository) Delete(ctx context.Context, projectID, id string) error
//	... 另有 6 个顶层符号

package backup

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type Repository struct {
	db     *sql.DB
	notify func(context.Context)
}

func NewRepository(db *sql.DB, notify func(context.Context)) *Repository {
	return &Repository{db: db, notify: notify}
}

func (r *Repository) Create(ctx context.Context, item Item) error {
	if r == nil || r.db == nil {
		return ErrUnavailable
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO sys_backups(id, project_id, storage_key, size_bytes, sha256, engine, format_version, status, error, created_at, expires_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		item.ID, item.ProjectID, item.StorageKey, item.SizeBytes, item.SHA256, item.Engine, item.FormatVersion, item.Status, item.CreatedAt, item.ExpiresAt)
	if err == nil && r.notify != nil {
		r.notify(ctx)
	}
	return repositoryError(err)
}

func (r *Repository) List(ctx context.Context, projectID string) ([]Item, error) {
	if r == nil || r.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, project_id, storage_key, size_bytes, sha256, engine, format_version, status, error, created_at, expires_at FROM sys_backups WHERE project_id = ? ORDER BY created_at DESC, id DESC`, projectID)
	if err != nil {
		return nil, repositoryError(err)
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		item, err := scanBackup(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Get(ctx context.Context, projectID, id string) (Item, error) {
	if r == nil || r.db == nil {
		return Item{}, ErrUnavailable
	}
	row := r.db.QueryRowContext(ctx, `SELECT id, project_id, storage_key, size_bytes, sha256, engine, format_version, status, error, created_at, expires_at FROM sys_backups WHERE project_id = ? AND id = ?`, projectID, id)
	item, err := scanBackup(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	return item, repositoryError(err)
}

func (r *Repository) Delete(ctx context.Context, projectID, id string) error {
	if r == nil || r.db == nil {
		return ErrUnavailable
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM sys_backups WHERE project_id = ? AND id = ?`, projectID, id)
	if err == nil && r.notify != nil {
		r.notify(ctx)
	}
	return repositoryError(err)
}

type rowScanner interface{ Scan(...any) error }

func scanBackup(row rowScanner) (Item, error) {
	var item Item
	err := row.Scan(&item.ID, &item.ProjectID, &item.StorageKey, &item.SizeBytes, &item.SHA256, &item.Engine, &item.FormatVersion, &item.Status, &item.Error, &item.CreatedAt, &item.ExpiresAt)
	return item, err
}

type ProjectTarget struct {
	TenantID  string
	ProjectID string
}

func (r *Repository) ListProjects(ctx context.Context) ([]ProjectTarget, error) {
	if r == nil || r.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := r.db.QueryContext(ctx, `SELECT tenant_id, id FROM sys_projects ORDER BY created_at`)
	if err != nil {
		return nil, repositoryError(err)
	}
	defer rows.Close()
	out := []ProjectTarget{}
	for rows.Next() {
		var target ProjectTarget
		if err := rows.Scan(&target.TenantID, &target.ProjectID); err != nil {
			return nil, err
		}
		out = append(out, target)
	}
	return out, rows.Err()
}

func repositoryError(err error) error {
	if err == nil || errors.Is(err, ErrNotFound) {
		return err
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "no such table") || strings.Contains(message, "does not exist") {
		return ErrUnavailable
	}
	return err
}

func expiry(created time.Time, days int) *time.Time {
	if days <= 0 {
		return nil
	}
	value := created.Add(time.Duration(days) * 24 * time.Hour)
	return &value
}
