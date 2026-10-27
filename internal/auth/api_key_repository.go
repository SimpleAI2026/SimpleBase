// 文件说明：api_key_repository.go — auth.Repository 基于系统 DuckLake 的实现
//
// 用法
//
//	NewSQLAPIKeyRepository(db *sql.DB) Repository
//	CreateAPIKey(ctx context.Context, db *sql.DB, id, projectID, keyHash string, perms ...  // CreateAPIKey 插入一条新的 sys_api_keys 记录
//	RevokeAPIKey(ctx context.Context, db *sql.DB, id string, at time.Time) error  // RevokeAPIKey 将指定 key 标记为已撤销
//	同包调用：FindByHash() → cache.go, service.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/auth/api_key_repository.go
//	         │ 依赖
//	         ▼
//	[下游] internal/utils/strutil
//	[同包] cache.go, delegation.go, id.go, jwt.go, middleware.go 等 15 个
//	[关键引用] FindByHash() ← cache.go, service.go
//
// 主要符号
//
//	NewSQLAPIKeyRepository(db *sql.DB) Repository
//	CreateAPIKey(ctx context.Context, db *sql.DB, id, projectID, keyHash string, perms ...
//	RevokeAPIKey(ctx context.Context, db *sql.DB, id string, at time.Time) error
//	var ErrKeyNotFound
//	RevokeProjectAPIKey(ctx context.Context, db *sql.DB, id, projectID string, at ...
//	(s *Service) RevokeAPIKeyByID(ctx context.Context, db *sql.DB, id string, at ...
//	... 另有 7 个顶层符号

package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
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

// ErrKeyNotFound 表示按 (id, project_id) 未找到未吊销的 Key。
// handler 据此返回 404，避免跨项目吊销与不存在 ID 均返回 204（SEC-01）。
var ErrKeyNotFound = errors.New("auth: api key not found in project")

// RevokeProjectAPIKey 只吊销属于 projectID 的未吊销 Key（SEC-01）。
// 条件更新限定 project_id 且 revoked_at IS NULL：
//   - 不存在、属于其他项目或已吊销 → ErrKeyNotFound；
//   - 重复吊销按未找到处理（对外 404），保持接口幂等语义。
func RevokeProjectAPIKey(ctx context.Context, db *sql.DB, id, projectID string, at time.Time) error {
	res, err := db.ExecContext(ctx,
		`UPDATE sys_api_keys SET revoked_at = ? WHERE id = ? AND project_id = ? AND revoked_at IS NULL`,
		at.UTC(), id, projectID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrKeyNotFound
	}
	return nil
}

// RevokeAPIKeyByID 吊销并失效本进程缓存（§7.2 P1.2：吊销立即生效契约）。
// 缓存按 keyHash 索引而吊销按 key ID 执行，无法精确定位 hash，
// 因此全量失效（Key 数量有限，重建成本可接受）。
func (s *Service) RevokeAPIKeyByID(ctx context.Context, db *sql.DB, id string, at time.Time) error {
	if err := RevokeAPIKey(ctx, db, id, at); err != nil {
		return err
	}
	s.cache.InvalidateAll()
	return nil
}

// RevokeProjectAPIKeyByID 按 (id, projectID) 吊销并失效缓存（SEC-01）。
// 项目不匹配或已吊销时返回 ErrKeyNotFound，不触发缓存失效。
func (s *Service) RevokeProjectAPIKeyByID(ctx context.Context, db *sql.DB, id, projectID string, at time.Time) error {
	if err := RevokeProjectAPIKey(ctx, db, id, projectID, at); err != nil {
		return err
	}
	s.cache.InvalidateAll()
	return nil
}

func parsePermissions(csv string) []Permission {
	if csv == "" {
		return nil
	}
	parts := strutil.SplitCSV(csv)
	out := make([]Permission, 0, len(parts))
	for _, p := range parts {
		out = append(out, Permission(p))
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
