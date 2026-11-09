// 文件说明：session_repository.go — SessionRepository 的系统 DuckLake 实现（sys_user_sessions）
//
// 用法
//
//	NewSQLSessionRepository(db *sql.DB) SessionRepository
//	type sqlSessionRepository（包内使用）
//	同包调用：s → api_key_repository.go, cache.go 等；GetByID() → observer.go, session.go 等；Create() → session.go, user.go 等
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/auth/session_repository.go
//	         │ 依赖
//	         ▼
//	[下游] internal/utils/sqlval
//	[同包] api_key_repository.go, cache.go, delegation.go, id.go, jwt.go 等 15 个
//	[关键引用] s ← api_key_repository.go, cache.go, password.go
//
// 主要符号
//
//	NewSQLSessionRepository(db *sql.DB) SessionRepository
//	... 另有 11 个顶层符号
//	session_repository.go 是 SessionRepository 的系统 DuckLake 实现（sys_user_sessions）。

package auth

import (
	"context"
	"database/sql"
	"errors"
	"github.com/linkxzhou/SimpleBase/internal/utils/sqlval"
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
		s.ExpiresAt.UTC(), s.CreatedAt.UTC(), sqlval.NullTime(s.RevokedAt),
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
		s.ExpiresAt.UTC(), sqlval.NullTime(s.RevokedAt), s.ID,
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
