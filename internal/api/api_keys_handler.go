// 文件说明：api_keys_handler.go — 项目 API Key 管理端点（创建 / 列表 / 吊销）
//
// 用法
//
//	NewAPIKeysHandler(authSvc *auth.Service, db *sql.DB, afterWrite func(ctx ...
//	(h *APIKeysHandler) Create(c echo.Context) error  // Create: POST /v1/projects/:projectID/api-keys
//	(h *APIKeysHandler) List(c echo.Context) error  // List: GET /v1/projects/:projectID/api-keys →
//	同包调用：List() → backup_handler.go, cronjob_handler.go 等；Delete() → backup_handler.go, collection_meta_handler.go 等；Create() → backup_handler.go, cronjob_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/api_keys_handler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/auth, internal/utils/strutil
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, auth_handler.go 等 59 个
//	[关键引用] List() ← backup_handler.go, cronjob_handler.go, data_handler.go
//
// 主要符号
//
//	type APIKeysHandler struct {…}
//	NewAPIKeysHandler(authSvc *auth.Service, db *sql.DB, afterWrite func(ctx ...
//	(h *APIKeysHandler) Create(c echo.Context) error
//	(h *APIKeysHandler) List(c echo.Context) error
//	(h *APIKeysHandler) Delete(c echo.Context) error
//	... 另有 12 个顶层符号

package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

// APIKeysHandler 处理 /v1/projects/:projectID/api-keys。
type APIKeysHandler struct {
	auth *auth.Service
	db   *sql.DB
	// afterWrite 在写成功后调用（SYNC-01）：auth 仓储直接写 sys_api_keys，
	// 不经过 systemdb.Store 的 notifyWrite，必须由 handler 显式触发
	// catalog 同步，否则吊销 Key 若为唯一一次写入，崩溃恢复后仍有效。
	afterWrite func(ctx context.Context)
}

// NewAPIKeysHandler 构造 APIKeysHandler。db 为已应用系统迁移的系统库连接。
// afterWrite 可为 nil（测试 / 无同步场景）。
func NewAPIKeysHandler(authSvc *auth.Service, db *sql.DB, afterWrite func(ctx context.Context)) *APIKeysHandler {
	return &APIKeysHandler{auth: authSvc, db: db, afterWrite: afterWrite}
}

type createAPIKeyRequest struct {
	Permissions []string           `json:"permissions"`
	Scope       *apiKeyPolicyInput `json:"scope,omitempty"`
}

type apiKeyResponse struct {
	ID          string             `json:"id"`
	ProjectID   string             `json:"project_id"`
	Permissions []string           `json:"permissions"`
	Scope       *apiKeyPolicyInput `json:"scope,omitempty"`
	CreatedAt   string             `json:"created_at"`
	RevokedAt   *string            `json:"revoked_at,omitempty"`
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
	// SEC-02：签发的权限必须是签发者自身权限的子集，防止低权限 Key 自我提升。
	// 空列表不再授予全量权限，改为继承签发者已有权限。
	perms, err = limitIssuedPermissions(p, perms)
	if err != nil {
		return WriteError(c, NewAPIError(http.StatusForbidden, "forbidden", "cannot issue permissions beyond your own", rid))
	}

	if p.Scope != nil && req.Scope == nil {
		req.Scope = &apiKeyPolicyInput{Resources: p.Scope.Resources, Actions: p.Scope.Actions, HiddenFields: p.Scope.HiddenFields, ReadonlyFields: p.Scope.ReadonlyFields, ExpiresAt: p.Scope.ExpiresAt}
	}
	if err := validateAPIKeyPolicy(req.Scope); err != nil || !scopeSubset(p.Scope, req.Scope.policy()) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "key_scope_denied", "key scope denied", rid))
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
	if err := putAPIKeyPolicy(c.Request().Context(), h.db, id, project.ID, req.Scope); err != nil {
		_ = h.auth.RevokeProjectAPIKeyByID(c.Request().Context(), h.db, id, project.ID, time.Now().UTC())
		return WriteError(c, err)
	}
	if h.afterWrite != nil {
		h.afterWrite(c.Request().Context()) // SYNC-01：写后触发 catalog 同步
	}
	return c.JSON(http.StatusCreated, apiKeyResponse{
		ID:          id,
		ProjectID:   project.ID,
		Permissions: permissionStrings(perms),
		Scope:       req.Scope,
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
	// SEC-01：按 (id, projectID) 吊销；不属于本项目的 Key 与不存在的 ID 一律 404，
	// 不泄露 Key 归属。重复吊销同样 404（幂等）。同时失效本进程认证缓存。
	if err := h.auth.RevokeProjectAPIKeyByID(c.Request().Context(), h.db, keyID, project.ID, time.Now().UTC()); err != nil {
		if errors.Is(err, auth.ErrKeyNotFound) {
			return WriteError(c, NewAPIError(http.StatusNotFound, "not_found", "api key not found", rid))
		}
		return WriteError(c, err)
	}
	if h.afterWrite != nil {
		h.afterWrite(c.Request().Context()) // SYNC-01：吊销是安全敏感写，立即标脏
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

// parseAPIKeyPermissions 校验权限列表；空列表由 limitIssuedPermissions 改为继承签发者权限。
func parseAPIKeyPermissions(in []string) ([]auth.Permission, error) {
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

// limitIssuedPermissions 实施 SEC-02 子集约束：
//   - 登录态按角色展开后的权限求子集；API Key 通道按 Key 自身权限求子集；
//   - 空列表继承签发者已有权限（原行为是授予全量业务权限，属于权限提升漏洞）；
//   - 请求包含签发者不具备的权限时整体拒绝（返回错误），不做静默裁剪，
//     避免调用方误以为拿到了全部请求的权限。
func limitIssuedPermissions(caller auth.Principal, requested []auth.Permission) ([]auth.Permission, error) {
	callerPerms := caller.Permissions
	if caller.UserID != "" {
		callerPerms = auth.PermissionsForRole(caller.Role)
	}
	if callerPerms == nil {
		callerPerms = map[auth.Permission]struct{}{}
	}
	if len(requested) == 0 {
		out := make([]auth.Permission, 0, len(callerPerms))
		for perm := range callerPerms {
			out = append(out, perm)
		}
		if len(out) == 0 {
			return nil, auth.ErrForbidden
		}
		sortPermissions(out)
		return out, nil
	}
	for _, perm := range requested {
		if _, ok := callerPerms[perm]; !ok {
			return nil, auth.ErrForbidden
		}
	}
	return requested, nil
}

// sortPermissions 让继承权限的输出顺序稳定（测试与列表展示可预期）。
func sortPermissions(perms []auth.Permission) {
	sort.Slice(perms, func(i, j int) bool { return perms[i] < perms[j] })
}

func permissionStrings(perms []auth.Permission) []string {
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		out = append(out, string(p))
	}
	return out
}

func splitNonEmpty(csv string) []string {
	return strutil.SplitCSV(csv)
}

// generateAPIKeySecret 生成 sb_live_<32 hex> 形态的明文 Key（与种子 Key 前缀一致）。
func generateAPIKeySecret() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "sb_live_" + hex.EncodeToString(buf), nil
}
