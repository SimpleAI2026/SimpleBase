// auth_handler.go 提供登录态 API：login / refresh / logout / me / password（planv3.0 §4.3）。
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
}

// NewAuthHandler 构造 AuthHandler。
func NewAuthHandler(sessions *auth.SessionService, users *auth.UserService) *AuthHandler {
	return &AuthHandler{
		sessions: sessions,
		users:    users,
		limiter:  newLoginLimiter(10, time.Minute),
	}
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
	if !h.limiter.allow(c.RealIP()+"/"+strings.ToLower(req.Username)) {
		return WriteError(c, NewAPIError(http.StatusTooManyRequests, "rate_limited", "too many login attempts", rid))
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
			return WriteError(c, NewAPIError(http.StatusUnauthorized, "invalid_credentials", "invalid username or password", rid))
		}
		return WriteError(c, err)
	}
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
	u, err := h.users.GetByID(c.Request().Context(), p.UserID)
	if err != nil {
		return WriteError(c, err)
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
	mu      sync.Mutex
	max     int
	window  time.Duration
	hits    map[string][]time.Time
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{max: max, window: window, hits: map[string][]time.Time{}}
}

func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
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
