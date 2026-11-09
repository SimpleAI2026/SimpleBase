// 文件说明：enduser_repo.go — sys_end_users 的 SQL 仓储（production-readiness-plan §8.2.6）
package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// SQLEndUserRepository 是 auth.EndUserRepository 的系统库实现。
type SQLEndUserRepository struct {
	db     *sql.DB
	notify func(ctx context.Context)
}

// NewSQLEndUserRepository 构造仓储；notify 为写后同步通知（可 nil）。
func NewSQLEndUserRepository(db *sql.DB, notify func(ctx context.Context)) *SQLEndUserRepository {
	return &SQLEndUserRepository{db: db, notify: notify}
}

func (r *SQLEndUserRepository) Create(ctx context.Context, u auth.EndUser) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO sys_end_users (id, project_id, external_id, password_hash, display_name, email, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.ProjectID, u.ExternalID, u.PasswordHash, u.DisplayName, u.Email, u.Status, u.CreatedAt, u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.ErrEndUserExists
		}
		return err
	}
	if r.notify != nil {
		r.notify(ctx)
	}
	return nil
}

func (r *SQLEndUserRepository) GetByExternalID(ctx context.Context, projectID, externalID string) (auth.EndUser, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, project_id, external_id, password_hash, display_name, email, status, created_at, updated_at
		 FROM sys_end_users WHERE project_id = ? AND external_id = ?`, projectID, externalID)
	var u auth.EndUser
	if err := row.Scan(&u.ID, &u.ProjectID, &u.ExternalID, &u.PasswordHash, &u.DisplayName, &u.Email, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.EndUser{}, auth.ErrEndUserNotFound
		}
		return auth.EndUser{}, err
	}
	return u, nil
}

func (r *SQLEndUserRepository) UpdatePassword(ctx context.Context, id string, hash string, updatedAt time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE sys_end_users SET password_hash = ?, updated_at = ? WHERE id = ?`, hash, updatedAt, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return auth.ErrEndUserNotFound
	}
	if r.notify != nil {
		r.notify(ctx)
	}
	return nil
}

func (r *SQLEndUserRepository) Count(ctx context.Context, projectID string) (int64, error) {
	var n int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_end_users WHERE project_id = ?`, projectID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
