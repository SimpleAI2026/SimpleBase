// user_repository.go 是 UserRepository 的系统 DuckLake 实现（sys_users / sys_project_owners）。
package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type sqlUserRepository struct {
	db *sql.DB
}

// NewSQLUserRepository 构造 UserRepository。db 必须已应用 v31–v33 迁移。
func NewSQLUserRepository(db *sql.DB) UserRepository {
	return &sqlUserRepository{db: db}
}

func (r *sqlUserRepository) Create(ctx context.Context, u User) error {
	n, err := r.CountByUsername(ctx, u.Username)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrUsernameTaken
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO sys_users(id, username, password_hash, role, display_name, email, status,
			must_change_password, created_by, created_at, updated_at, last_login_at, disabled_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		u.ID, u.Username, u.PasswordHash, string(u.Role), u.DisplayName, u.Email, u.Status,
		boolToInt(u.MustChangePassword), u.CreatedBy,
		u.CreatedAt.UTC(), u.UpdatedAt.UTC(), nullTime(u.LastLoginAt), nullTime(u.DisabledAt),
	)
	return err
}

func (r *sqlUserRepository) GetByID(ctx context.Context, id string) (User, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, userSelect+` WHERE id = ?`, id))
}

func (r *sqlUserRepository) GetByUsername(ctx context.Context, username string) (User, error) {
	u, err := r.scanOne(r.db.QueryRowContext(ctx, userSelect+` WHERE username = ?`, username))
	if errors.Is(err, ErrUserNotFound) {
		return User{}, err
	}
	return u, err
}

func (r *sqlUserRepository) List(ctx context.Context, limit int, cursor string) ([]User, string, error) {
	args := []any{limit + 1}
	q := userSelect
	if cursor != "" {
		q += ` WHERE username > ?`
		args = append(args, cursor)
	}
	q += ` ORDER BY username ASC LIMIT ?`
	// 重新排参数：LIMIT 在最后
	if cursor != "" {
		q = userSelect + ` WHERE username > ? ORDER BY username ASC LIMIT ?`
		args = []any{cursor, limit + 1}
	} else {
		q = userSelect + ` ORDER BY username ASC LIMIT ?`
		args = []any{limit + 1}
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = out[len(out)-1].Username
	}
	return out, next, nil
}

func (r *sqlUserRepository) Update(ctx context.Context, u User) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE sys_users SET password_hash=?, role=?, display_name=?, email=?, status=?,
			must_change_password=?, updated_at=?, last_login_at=?, disabled_at=? WHERE id = ?`,
		u.PasswordHash, string(u.Role), u.DisplayName, u.Email, u.Status,
		boolToInt(u.MustChangePassword), u.UpdatedAt.UTC(),
		nullTime(u.LastLoginAt), nullTime(u.DisabledAt), u.ID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *sqlUserRepository) CountByUsername(ctx context.Context, username string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_users WHERE username = ?`, username).Scan(&n)
	return n, err
}

func (r *sqlUserRepository) SetProjectOwner(ctx context.Context, projectID, userID string, at time.Time) error {
	// 幂等：已有归属则改写。
	var existing string
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id FROM sys_project_owners WHERE project_id = ?`, projectID).Scan(&existing)
	if err == nil {
		_, err = r.db.ExecContext(ctx,
			`UPDATE sys_project_owners SET user_id = ?, created_at = ? WHERE project_id = ?`,
			userID, at.UTC(), projectID)
		return err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO sys_project_owners(project_id, user_id, created_at) VALUES(?,?,?)`,
		projectID, userID, at.UTC())
	return err
}

func (r *sqlUserRepository) GetProjectOwner(ctx context.Context, projectID string) (string, error) {
	var uid string
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id FROM sys_project_owners WHERE project_id = ?`, projectID).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUserNotFound
	}
	return uid, err
}

func (r *sqlUserRepository) ListProjectIDsByUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT project_id FROM sys_project_owners WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *sqlUserRepository) ListAllProjectIDs(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM sys_projects`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

const userSelect = `SELECT id, username, password_hash, role, display_name, email, status,
	must_change_password, created_by, created_at, updated_at, last_login_at, disabled_at FROM sys_users`

type rowScanner interface {
	Scan(dest ...any) error
}

func (r *sqlUserRepository) scanOne(row rowScanner) (User, error) {
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return u, err
}

func scanUser(row rowScanner) (User, error) {
	var (
		u     User
		role  string
		mustC int64
		last  sql.NullTime
		dis   sql.NullTime
	)
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &role, &u.DisplayName, &u.Email, &u.Status,
		&mustC, &u.CreatedBy, &u.CreatedAt, &u.UpdatedAt, &last, &dis)
	if err != nil {
		return User{}, err
	}
	u.Role = Role(role)
	u.MustChangePassword = mustC != 0
	if last.Valid {
		t := last.Time
		u.LastLoginAt = &t
	}
	if dis.Valid {
		t := dis.Time
		u.DisabledAt = &t
	}
	return u, nil
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}
