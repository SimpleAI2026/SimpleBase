// api_key_repository.go 是 auth.Repository 基于 catalog 系统数据库的实现。
//
// 复用 catalog migrations 已创建的 api_keys 表（id, project_id, key_hash,
// permissions, created_at, revoked_at）。tenant_id 通过 JOIN projects 得到，
// 避免在 api_keys 表中重复存储。permissions 以逗号分隔字符串存储。
//
// 一个 API key 目前只授权单个 project；Principal.ProjectIDs 因此是大小为 1
// 的集合。若未来需要一个 key 对应多个 project，需要拆分出关联表并调整此实现，
// Principal/Service 的接口不需要变化。
package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// sqliteAPIKeyRepository 基于 database/sql 的 Repository 实现。
type sqliteAPIKeyRepository struct {
	db *sql.DB
}

// NewSQLiteAPIKeyRepository 构造 Repository。db 必须是已应用 catalog migrations
// 的系统数据库连接。
func NewSQLiteAPIKeyRepository(db *sql.DB) Repository {
	return &sqliteAPIKeyRepository{db: db}
}

func (r *sqliteAPIKeyRepository) FindByHash(ctx context.Context, keyHash string) (APIKeyRecord, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT k.id, p.tenant_id, k.project_id, k.permissions, k.revoked_at, k.created_at
		 FROM api_keys k JOIN projects p ON p.id = k.project_id
		 WHERE k.key_hash = ?`,
		keyHash)

	var (
		id, tenantID, projectID, permsCSV string
		revokedAt                         sql.NullTime
		createdAt                         time.Time
	)
	if err := row.Scan(&id, &tenantID, &projectID, &permsCSV, &revokedAt, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return APIKeyRecord{}, ErrInvalidCredentials
		}
		return APIKeyRecord{}, err
	}

	rec := APIKeyRecord{
		ID:          id,
		TenantID:    tenantID,
		ProjectIDs:  []string{projectID},
		Permissions: parsePermissions(permsCSV),
		KeyHash:     keyHash,
		CreatedAt:   createdAt,
	}
	if revokedAt.Valid {
		t := revokedAt.Time
		rec.RevokedAt = &t
	}
	return rec, nil
}

// CreateAPIKey 插入一条新的 api_keys 记录。keyHash 由调用方通过 Service.HashKey
// 预先计算；本函数不接受原文 key。
func CreateAPIKey(ctx context.Context, db *sql.DB, id, projectID, keyHash string, perms []Permission, createdAt time.Time) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO api_keys(id, project_id, key_hash, permissions, created_at) VALUES(?, ?, ?, ?, ?)`,
		id, projectID, keyHash, joinPermissions(perms), createdAt)
	return err
}

// RevokeAPIKey 将指定 key 标记为已撤销。
func RevokeAPIKey(ctx context.Context, db *sql.DB, id string, at time.Time) error {
	_, err := db.ExecContext(ctx, `UPDATE api_keys SET revoked_at = ? WHERE id = ?`, at, id)
	return err
}

func parsePermissions(csv string) []Permission {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]Permission, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, Permission(p))
		}
	}
	return out
}

func joinPermissions(perms []Permission) string {
	parts := make([]string, 0, len(perms))
	for _, p := range perms {
		parts = append(parts, string(p))
	}
	return strings.Join(parts, ",")
}
