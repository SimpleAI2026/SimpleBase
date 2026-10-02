// api_keys_handler.go 提供项目 API Key 管理端点（创建 / 列表 / 吊销）。
//
// 背景：生产实例（dev_mode=false）没有种子 Key，且平台此前没有任何签发入口，
// SDK / examples 无法获得合法凭据。本 handler 允许：
//   - 登录态：superadminl1 / user 角色可为自己有权的项目签发 Key；
//   - API Key 通道：持 ProjectAdmin 权限的 Key 可再签发同项目 Key。
//
// 安全约定：Key 明文仅在创建响应中出现一次（服务端只存 HMAC 摘要）；
// 列表只返回 id / 权限 / 时间，绝不回显明文。
package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// APIKeysHandler 处理 /v1/projects/:projectID/api-keys。
type APIKeysHandler struct {
	auth *auth.Service
	db   *sql.DB
}

// NewAPIKeysHandler 构造 APIKeysHandler。db 为已应用系统迁移的系统库连接。
func NewAPIKeysHandler(authSvc *auth.Service, db *sql.DB) *APIKeysHandler {
	return &APIKeysHandler{auth: authSvc, db: db}
}

type createAPIKeyRequest struct {
	Permissions []string `json:"permissions"`
}

type apiKeyResponse struct {
	ID          string   `json:"id"`
	ProjectID   string   `json:"project_id"`
	Permissions []string `json:"permissions"`
	CreatedAt   string   `json:"created_at"`
	RevokedAt   *string  `json:"revoked_at,omitempty"`
	// Secret 仅出现在创建响应中，一次性回显。
	Secret *string `json:"secret,omitempty"`
}

type apiKeyListResponse struct {
	Keys []apiKeyResponse `json:"keys"`
}

// Create: POST /v1/projects/:projectID/api-keys
// body { "permissions": ["database:read", ...] } → 201 { id, permissions, secret }
func (h *APIKeysHandler) Create(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	p, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, NewAPIError(http.StatusUnauthorized, "unauthenticated", "login required", rid))
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, NewAPIError(http.StatusInternalServerError, "internal", "project context missing", rid))
	}
	if !p.CanAccessProject(project.ID) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "cross_project_denied", "cross project access denied", rid))
	}
	// 登录态 user/super 或持 ProjectAdmin 的 Key 才能签发；admin 只读角色拒绝。
	if !h.canIssue(p) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "forbidden", "permission denied", rid))
	}

	var req createAPIKeyRequest
	dec := json.NewDecoder(c.Request().Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil && err.Error() != "EOF" {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "malformed JSON body", rid))
	}

	perms, err := parseAPIKeyPermissions(req.Permissions)
	if err != nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_permission", "unknown or malformed permission", rid))
	}

	secret, err := generateAPIKeySecret()
	if err != nil {
		return WriteError(c, err)
	}
	now := time.Now().UTC()
	id := uuid.NewString()
	if err := auth.CreateAPIKey(c.Request().Context(), h.db, id, project.ID,
		h.auth.HashKey(secret), perms, now); err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusCreated, apiKeyResponse{
		ID:          id,
		ProjectID:   project.ID,
		Permissions: permissionStrings(perms),
		CreatedAt:   now.Format(time.RFC3339),
		Secret:      &secret,
	})
}

// List: GET /v1/projects/:projectID/api-keys → { keys: [...] }（无明文）
func (h *APIKeysHandler) List(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	p, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, NewAPIError(http.StatusUnauthorized, "unauthenticated", "login required", rid))
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, NewAPIError(http.StatusInternalServerError, "internal", "project context missing", rid))
	}
	if !p.CanAccessProject(project.ID) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "cross_project_denied", "cross project access denied", rid))
	}
	rows, err := h.db.QueryContext(c.Request().Context(),
		`SELECT id, permissions, created_at, revoked_at FROM sys_api_keys WHERE project_id = ? ORDER BY created_at DESC`,
		project.ID)
	if err != nil {
		return WriteError(c, err)
	}
	defer rows.Close()
	out := apiKeyListResponse{Keys: []apiKeyResponse{}}
	for rows.Next() {
		var (
			id, permsCSV string
			createdAt    time.Time
			revokedAt    sql.NullTime
		)
		if err := rows.Scan(&id, &permsCSV, &createdAt, &revokedAt); err != nil {
			return WriteError(c, err)
		}
		item := apiKeyResponse{
			ID:          id,
			ProjectID:   project.ID,
			Permissions: splitNonEmpty(permsCSV),
			CreatedAt:   createdAt.Format(time.RFC3339),
		}
		if revokedAt.Valid {
			t := revokedAt.Time.UTC().Format(time.RFC3339)
			item.RevokedAt = &t
		}
		out.Keys = append(out.Keys, item)
	}
	if err := rows.Err(); err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// Delete: DELETE /v1/projects/:projectID/api-keys/:keyID（吊销）
func (h *APIKeysHandler) Delete(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	p, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, NewAPIError(http.StatusUnauthorized, "unauthenticated", "login required", rid))
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, NewAPIError(http.StatusInternalServerError, "internal", "project context missing", rid))
	}
	if !p.CanAccessProject(project.ID) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "cross_project_denied", "cross project access denied", rid))
	}
	if !h.canIssue(p) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "forbidden", "permission denied", rid))
	}
	keyID := c.Param("keyID")
	if keyID == "" {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "key id required", rid))
	}
	// RevokeAPIKeyByID 同时失效本进程认证缓存，吊销立即生效。
	if err := h.auth.RevokeAPIKeyByID(c.Request().Context(), h.db, keyID, time.Now().UTC()); err != nil {
		return WriteError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// canIssue 判断能否签发 / 吊销：登录态 super/user 角色，或 API Key 持 ProjectAdmin。
func (h *APIKeysHandler) canIssue(p auth.Principal) bool {
	if p.UserID != "" {
		return p.Role == auth.RoleSuperAdmin || p.Role == auth.RoleUser
	}
	return p.HasPermission(auth.ProjectAdmin)
}

// parseAPIKeyPermissions 校验权限列表；空列表授予全量业务权限（与种子 Key 一致）。
func parseAPIKeyPermissions(in []string) ([]auth.Permission, error) {
	if len(in) == 0 {
		return []auth.Permission{
			auth.DatabaseRead, auth.DatabaseWrite, auth.DatabaseAdmin,
			auth.LLMInvoke, auth.ProjectAdmin,
		}, nil
	}
	seen := make(map[auth.Permission]struct{}, len(in))
	out := make([]auth.Permission, 0, len(in))
	for _, s := range in {
		perm := auth.Permission(strings.TrimSpace(s))
		switch perm {
		case auth.DatabaseRead, auth.DatabaseWrite, auth.DatabaseAdmin,
			auth.LLMInvoke, auth.ProjectAdmin:
		default:
			return nil, echo.NewHTTPError(http.StatusBadRequest, "unknown permission: %s", s)
		}
		if _, dup := seen[perm]; dup {
			continue
		}
		seen[perm] = struct{}{}
		out = append(out, perm)
	}
	return out, nil
}

func permissionStrings(perms []auth.Permission) []string {
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		out = append(out, string(p))
	}
	return out
}

func splitNonEmpty(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// generateAPIKeySecret 生成 sb_live_<32 hex> 形态的明文 Key（与种子 Key 前缀一致）。
func generateAPIKeySecret() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "sb_live_" + hex.EncodeToString(buf), nil
}
