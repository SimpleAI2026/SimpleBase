// 文件说明：auth_handler.go — 登录态 API：login / refresh / logout / me / password（planv3.0 §4.3）
//
// 用法
//
//	NewAuthHandler(sessions *auth.SessionService, users *auth.UserService) *AuthHandler
//	(h *AuthHandler) Login(c echo.Context) error  // Login: POST /v1/auth/login（免认证）
//	(h *AuthHandler) Refresh(c echo.Context) error  // Refresh: POST /v1/auth/refresh（免认证，持 refresh_token）
//	同包调用：allow() → middleware_ratelimit.go；NewAuthHandler() → router.go；Login() → router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/auth_handler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/auth
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] allow() ← middleware_ratelimit.go
//
// 主要符号
//
//	type AuthHandler struct {…}
//	NewAuthHandler(sessions *auth.SessionService, users *auth.UserService) *AuthHandler
//	(h *AuthHandler) Login(c echo.Context) error
//	(h *AuthHandler) Refresh(c echo.Context) error
//	(h *AuthHandler) Logout(c echo.Context) error
//	(h *AuthHandler) Me(c echo.Context) error
//	... 另有 14 个顶层符号
//	auth_handler.go 提供登录态 API：login / refresh / logout / me / password（planv3.0 §4.3）。

package api

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// AuthHandler 处理 /v1/auth/*。
type AuthHandler struct {
	sessions *auth.SessionService
	users    *auth.UserService
	limiter  *loginLimiter
	guard    *loginGuard
	// audit 认证事件审计（§8.2.5：auth.login_failed / auth.login_locked / auth.password_initial）。可为 nil。
	audit AuditService
}

// NewAuthHandler 构造 AuthHandler。
func NewAuthHandler(sessions *auth.SessionService, users *auth.UserService) *AuthHandler {
	return &AuthHandler{
		sessions: sessions,
		users:    users,
		limiter:  newLoginLimiter(10, time.Minute),
		guard:    newLoginGuard(defaultLoginGuardOpts()),
	}
}

// WithLoginGuardAudit 注入审计服务与软锁定回调（§8.2.5 / §8.3.5）；audit 可为 nil。
func (h *AuthHandler) WithLoginGuardAudit(audit AuditService, onLock func(username string)) *AuthHandler {
	h.audit = audit
	if h.guard != nil {
		h.guard.WithLockHook(onLock)
	}
	return h
}

func (h *AuthHandler) guardAllow(c echo.Context, username string) error {
	rid := RequestIDFromContext(c.Request().Context())
	ip := c.RealIP()
	switch h.guard.allow(ip, username) {
	case loginSoftLocked:
		if h.audit != nil {
			_ = h.audit.Record(c.Request().Context(), AuditEvent{Kind: "auth.login_locked", Detail: "username:" + normalizeLoginUsername(username)})
		}
		return WriteError(c, NewAPIError(http.StatusTooManyRequests, "login_locked", "account temporarily locked due to repeated failures", rid))
	case loginRateLimited:
		return WriteError(c, NewAPIError(http.StatusTooManyRequests, "rate_limited", "too many login attempts", rid))
	}
	return nil
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type userResponse struct {
	ID                 string `json:"id"`
	Username           string `json:"username"`
	Role               string `json:"role"`
	DisplayName        string `json:"display_name"`
	Email              string `json:"email"`
	Status             string `json:"status"`
	MustChangePassword bool   `json:"must_change_password"`
	CreatedBy          string `json:"created_by,omitempty"`
	CreatedAt          string `json:"created_at,omitempty"`
	LastLoginAt        string `json:"last_login_at,omitempty"`
}

type tokenResponse struct {
	TokenType    string       `json:"token_type"`
	AccessToken  string       `json:"access_token"`
	ExpiresIn    int64        `json:"expires_in"`
	RefreshToken string       `json:"refresh_token"`
	User         userResponse `json:"user"`
}

type meResponse struct {
	userResponse
	Projects []meProject `json:"projects"`
}

type meProject struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Owner bool   `json:"owner"`
}

// Login: POST /v1/auth/login（免认证）。
func (h *AuthHandler) Login(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid json body", rid))
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "username and password required", rid))
	}
	if err := h.guardAllow(c, req.Username); err != nil {
		return err
	}

	u, pair, err := h.sessions.Login(c.Request().Context(), req.Username, req.Password, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		if errors.Is(err, auth.ErrMustChangePasswd) {
			return WriteError(c, NewAPIError(http.StatusLocked, "must_change_password", "password change required", rid))
		}
		if errors.Is(err, auth.ErrUserDisabled) {
			return WriteError(c, NewAPIError(http.StatusForbidden, "user_disabled", "user disabled", rid))
		}
		if errors.Is(err, auth.ErrInvalidCredentials) {
			h.guard.recordFailure(c.RealIP(), req.Username)
			if h.audit != nil {
				_ = h.audit.Record(c.Request().Context(), AuditEvent{Kind: "auth.login_failed", Detail: "username:" + normalizeLoginUsername(req.Username)})
			}
			return WriteError(c, NewAPIError(http.StatusUnauthorized, "invalid_credentials", "invalid username or password", rid))
		}
		return WriteError(c, err)
	}
	h.guard.recordSuccess(c.RealIP(), req.Username)
	return c.JSON(http.StatusOK, toTokenResponse(pair, u))
}

// Refresh: POST /v1/auth/refresh（免认证，持 refresh_token）。
func (h *AuthHandler) Refresh(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	var req refreshRequest
	if err := c.Bind(&req); err != nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid json body", rid))
	}
	u, pair, err := h.sessions.Refresh(c.Request().Context(), req.RefreshToken, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUserDisabled):
			return WriteError(c, NewAPIError(http.StatusForbidden, "user_disabled", "user disabled", rid))
		case errors.Is(err, auth.ErrSessionRevoked):
			return WriteError(c, NewAPIError(http.StatusUnauthorized, "session_revoked", "session revoked", rid))
		default:
			return WriteError(c, NewAPIError(http.StatusUnauthorized, "invalid_refresh_token", "invalid refresh token", rid))
		}
	}
	return c.JSON(http.StatusOK, toTokenResponse(pair, u))
}

type initialPasswordRequest struct {
	Username    string `json:"username"`
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// InitialPassword: POST /v1/auth/password/initial（免认证，§8.2.2）。
// 流程：验旧密 → 要求 MustChangePassword=true（否则 409）→ 更新并清 flag
// → 吊销该用户全部会话 → 直接签发正式 TokenPair 返回。
// 复用 loginGuard 的 ip / ip+username 两维度（不经 /v1 组 authMW）。
func (h *AuthHandler) InitialPassword(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	var req initialPasswordRequest
	if err := c.Bind(&req); err != nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid json body", rid))
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.OldPassword == "" || req.NewPassword == "" {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "username, old_password and new_password required", rid))
	}
	if err := h.guardAllow(c, req.Username); err != nil {
		return err
	}

	ctx := c.Request().Context()
	// 先验旧密（不签发 token）；Login 对 must_change_password 用户返回 ErrMustChangePasswd + User。
	u, _, err := h.sessions.Login(ctx, req.Username, req.OldPassword, c.Request().UserAgent(), c.RealIP())
	if err != nil && !errors.Is(err, auth.ErrMustChangePasswd) {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			h.guard.recordFailure(c.RealIP(), req.Username)
			if h.audit != nil {
				_ = h.audit.Record(ctx, AuditEvent{Kind: "auth.login_failed", Detail: "username:" + normalizeLoginUsername(req.Username)})
			}
			return WriteError(c, NewAPIError(http.StatusUnauthorized, "invalid_credentials", "invalid username or password", rid))
		}
		return WriteError(c, err)
	}
	if !u.MustChangePassword {
		// 非 must_change_password 用户调此端点：拒绝，避免变成免登录改密后门。
		return WriteError(c, NewAPIError(http.StatusConflict, "password_change_not_required", "password change is not required", rid))
	}
	if err := h.users.ChangePassword(ctx, u.ID, req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, auth.ErrWeakPassword) {
			return WriteError(c, NewAPIError(http.StatusBadRequest, "weak_password", err.Error(), rid))
		}
		if errors.Is(err, auth.ErrInvalidCredentials) {
			return WriteError(c, NewAPIError(http.StatusUnauthorized, "invalid_credentials", "invalid username or password", rid))
		}
		return WriteError(c, err)
	}
	// 吊销该用户全部会话（旧凭据全部失效），再签发新的正式 TokenPair。
	if err := h.sessions.RevokeAllForUser(ctx, u.ID); err != nil {
		return WriteError(c, err)
	}
	u, err = h.users.GetByID(ctx, u.ID)
	if err != nil {
		return WriteError(c, err)
	}
	pair, err := h.sessions.IssueAfterPasswordChange(ctx, u, c.Request().UserAgent(), c.RealIP())
	if err != nil {
		return WriteError(c, err)
	}
	h.guard.recordSuccess(c.RealIP(), req.Username)
	if h.audit != nil {
		_ = h.audit.Record(ctx, AuditEvent{Kind: "auth.password_initial", Detail: "user:" + u.ID})
	}
	return c.JSON(http.StatusOK, toTokenResponse(pair, u))
}

// Logout: POST /v1/auth/logout（需登录态）。
func (h *AuthHandler) Logout(c echo.Context) error {
	p, _ := PrincipalFromContext(c.Request().Context())
	var req logoutRequest
	_ = c.Bind(&req)
	if err := h.sessions.Logout(c.Request().Context(), p.SessionID, req.RefreshToken); err != nil {
		return WriteError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// Me: GET /v1/auth/me（需登录态）。
func (h *AuthHandler) Me(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	p, ok := PrincipalFromContext(c.Request().Context())
	if !ok || p.UserID == "" {
		return WriteError(c, NewAPIError(http.StatusUnauthorized, "unauthenticated", "login required", rid))
	}
	// planv5.0 §4 P1.3：身份字段复用 Principal 缓存条目内同源的 sys_users 行
	//（新鲜度与 Principal 一致：5s TTL + 变更即时失效），消除每请求重复点查；
	// 仅缓存过期/失效时回查仓储。
	var u auth.User
	if cached, hit := h.sessions.CachedUser(p.UserID); hit {
		u = cached
		if timer := StageTimerFrom(c.Request().Context()); timer != nil {
			timer.SetMeta("me_cache", 1)
		}
	} else {
		if timer := StageTimerFrom(c.Request().Context()); timer != nil {
			timer.SetMeta("me_cache", 0)
			scope := timer.StageScope(StageSystemDB)
			var err error
			u, err = h.users.GetByID(c.Request().Context(), p.UserID)
			scope.Done()
			if err != nil {
				return WriteError(c, err)
			}
			timer.SetMeta("me_rows", 1)
		} else {
			var err error
			u, err = h.users.GetByID(c.Request().Context(), p.UserID)
			if err != nil {
				return WriteError(c, err)
			}
		}
	}
	out := meResponse{userResponse: toUserResponse(u)}
	// 可见项目：从 Principal.ProjectIDs 展开（服务端已按角色收敛）。
	// 名称由 catalog 注入可选；此处仅回 ID，UI 另拉 /v1/projects 补名。
	for id := range p.ProjectIDs {
		out.Projects = append(out.Projects, meProject{
			ID:    id,
			Owner: id != auth.AdminProjectID,
		})
	}
	return c.JSON(http.StatusOK, out)
}

// ChangePassword: PUT /v1/auth/password（需登录态）。
func (h *AuthHandler) ChangePassword(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	p, ok := PrincipalFromContext(c.Request().Context())
	if !ok || p.UserID == "" {
		return WriteError(c, NewAPIError(http.StatusUnauthorized, "unauthenticated", "login required", rid))
	}
	var req changePasswordRequest
	if err := c.Bind(&req); err != nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid json body", rid))
	}
	if err := h.users.ChangePassword(c.Request().Context(), p.UserID, req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			return WriteError(c, NewAPIError(http.StatusUnauthorized, "invalid_credentials", "old password incorrect", rid))
		}
		if errors.Is(err, auth.ErrWeakPassword) {
			return WriteError(c, NewAPIError(http.StatusBadRequest, "weak_password", "password too weak", rid))
		}
		return WriteError(c, err)
	}
	// 改密后吊销全部会话，要求重新登录（planv3.0 §4.3.5）。
	_ = h.sessions.RevokeAllForUser(c.Request().Context(), p.UserID)
	return c.NoContent(http.StatusNoContent)
}

func toTokenResponse(pair auth.TokenPair, u auth.User) tokenResponse {
	return tokenResponse{
		TokenType:    "Bearer",
		AccessToken:  pair.AccessToken,
		ExpiresIn:    pair.ExpiresIn,
		RefreshToken: pair.RefreshToken,
		User:         toUserResponse(u),
	}
}

func toUserResponse(u auth.User) userResponse {
	out := userResponse{
		ID:                 u.ID,
		Username:           u.Username,
		Role:               string(u.Role),
		DisplayName:        u.DisplayName,
		Email:              u.Email,
		Status:             u.Status,
		MustChangePassword: u.MustChangePassword,
		CreatedBy:          u.CreatedBy,
	}
	if !u.CreatedAt.IsZero() {
		out.CreatedAt = u.CreatedAt.UTC().Format(time.RFC3339)
	}
	if u.LastLoginAt != nil {
		out.LastLoginAt = u.LastLoginAt.UTC().Format(time.RFC3339)
	}
	return out
}

// loginLimiter 是简单的内存限速（同 key N 次 / 窗口）。
type loginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
	now    func() time.Time
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{max: max, window: window, hits: map[string][]time.Time{}, now: time.Now}
}

func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cutoff := now.Add(-l.window)
	arr := l.hits[key]
	kept := arr[:0]
	for _, t := range arr {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.max {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}
