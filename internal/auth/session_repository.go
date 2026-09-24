// session_repository.go 是 SessionRepository 的系统 DuckLake 实现（sys_user_sessions）。
package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type sqlSessionRepository struct {
	db *sql.DB
}

// NewSQLSessionRepository 构造 SessionRepository。
func NewSQLSessionRepository(db *sql.DB) SessionRepository {
	return &sqlSessionRepository{db: db}
}

func (r *sqlSessionRepository) Create(ctx context.Context, s Session) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sys_user_sessions(id, user_id, refresh_token_hash, access_jti, user_agent, ip,
			expires_at, created_at, revoked_at)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		s.ID, s.UserID, s.RefreshTokenHash, s.AccessJTI, s.UserAgent, s.IP,
		s.ExpiresAt.UTC(), s.CreatedAt.UTC(), nullTime(s.RevokedAt),
	)
	return err
}

func (r *sqlSessionRepository) GetByRefreshHash(ctx context.Context, hash string) (Session, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, sessionSelect+` WHERE refresh_token_hash = ?`, hash))
}

func (r *sqlSessionRepository) GetByID(ctx context.Context, id string) (Session, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, sessionSelect+` WHERE id = ?`, id))
}

func (r *sqlSessionRepository) Update(ctx context.Context, s Session) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE sys_user_sessions SET refresh_token_hash=?, access_jti=?, user_agent=?, ip=?,
			expires_at=?, revoked_at=? WHERE id = ?`,
		s.RefreshTokenHash, s.AccessJTI, s.UserAgent, s.IP,
		s.ExpiresAt.UTC(), nullTime(s.RevokedAt), s.ID,
	)
	return err
}

func (r *sqlSessionRepository) RevokeByUser(ctx context.Context, userID string, at time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE sys_user_sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`,
		at.UTC(), userID)
	return err
}

func (r *sqlSessionRepository) RevokeByIDs(ctx context.Context, ids []string, at time.Time) error {
	for _, id := range ids {
		if _, err := r.db.ExecContext(ctx,
			`UPDATE sys_user_sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
			at.UTC(), id); err != nil {
			return err
		}
	}
	return nil
}

const sessionSelect = `SELECT id, user_id, refresh_token_hash, access_jti, user_agent, ip,
	expires_at, created_at, revoked_at FROM sys_user_sessions`

func (r *sqlSessionRepository) scanOne(row rowScanner) (Session, error) {
	var (
		s   Session
		rev sql.NullTime
	)
	err := row.Scan(&s.ID, &s.UserID, &s.RefreshTokenHash, &s.AccessJTI, &s.UserAgent, &s.IP,
		&s.ExpiresAt, &s.CreatedAt, &rev)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrInvalidRefreshToken
	}
	if err != nil {
		return Session{}, err
	}
	if rev.Valid {
		t := rev.Time
		s.RevokedAt = &t
	}
	return s, nil
}
