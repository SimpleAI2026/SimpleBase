// api_key_repository.go 是 auth.Repository 基于系统 DuckLake 的实现。
//
// 复用系统迁移创建的 sys_api_keys / sys_projects。tenant_id 通过 JOIN 得到。
// permissions 以逗号分隔字符串存储。不使用 PRIMARY KEY / 序列。
package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type sqlAPIKeyRepository struct {
	db *sql.DB
}

// NewSQLAPIKeyRepository 构造 Repository。db 必须是已应用系统迁移的连接。
func NewSQLAPIKeyRepository(db *sql.DB) Repository {
	return &sqlAPIKeyRepository{db: db}
}

func (r *sqlAPIKeyRepository) FindByHash(ctx context.Context, keyHash string) (APIKeyRecord, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT k.id, p.tenant_id, k.project_id, k.permissions, k.revoked_at, k.created_at
		 FROM sys_api_keys k JOIN sys_projects p ON p.id = k.project_id
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

// CreateAPIKey 插入一条新的 sys_api_keys 记录。keyHash 由调用方通过 Service.HashKey
// 预先计算；本函数不接受原文 key。已存在时返回错误（调用方可按 duplicate 忽略）。
func CreateAPIKey(ctx context.Context, db *sql.DB, id, projectID, keyHash string, perms []Permission, createdAt time.Time) error {
	var existing string
	err := db.QueryRowContext(ctx, `SELECT id FROM sys_api_keys WHERE id = ?`, id).Scan(&existing)
	if err == nil {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO sys_api_keys(id, project_id, key_hash, permissions, created_at) VALUES(?, ?, ?, ?, ?)`,
		id, projectID, keyHash, joinPermissions(perms), createdAt.UTC())
	return err
}

// RevokeAPIKey 将指定 key 标记为已撤销。
func RevokeAPIKey(ctx context.Context, db *sql.DB, id string, at time.Time) error {
	_, err := db.ExecContext(ctx, `UPDATE sys_api_keys SET revoked_at = ? WHERE id = ?`, at.UTC(), id)
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
