// 文件说明：enduser_handler.go — 终端用户路由（production-readiness-plan §8.2.6）
//
// 用法
//
//	POST /v1/projects/:projectID/end-users/signup   免认证（pre_auth 限流 key=ip 5/min）
//	POST /v1/projects/:projectID/end-users/login     免认证（loginGuard 三维度）
//	GET  /v1/projects/:projectID/end-users/me        endUserAuth 守卫
//
// 守卫链：endUserAuth 只认 aud=enduser 的 JWT 并注入 endUserClaims；
// 平台 authMW 对 aud=enduser 的 token 显式拒绝（密钥域隔离，§8.2.6）。
package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// EndUsersHandler 处理 /v1/projects/:projectID/end-users/*。
type EndUsersHandler struct {
	users   *auth.EndUserService
	guard   *loginGuard
	audit   AuditService
}

// NewEndUsersHandler 构造。
func NewEndUsersHandler(users *auth.EndUserService) *EndUsersHandler {
	return &EndUsersHandler{users: users, guard: newLoginGuard(defaultLoginGuardOpts())}
}

// WithAudit 注入审计服务（可为 nil）。
func (h *EndUsersHandler) WithAudit(a AuditService) *EndUsersHandler {
	h.audit = a
	return h
}

type endUserSignupRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

type endUserLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type endUserAuthResponse struct {
	Token     string `json:"token"`
	ExpiresIn int64  `json:"expires_in"`
	User      struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email,omitempty"`
	} `json:"user"`
}

// Signup: POST /v1/projects/:projectID/end-users/signup（免认证，§8.2.6）。
func (h *EndUsersHandler) Signup(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	projectID := c.Param("projectID")
	var req endUserSignupRequest
	if err := c.Bind(&req); err != nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid json body", rid))
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "username required", rid))
	}
	// 注册同样吃 loginGuard（ip 维度挡批量注册爆破）。
	if err := h.guardAllow(c, req.Username); err != nil {
		return err
	}
	u, token, err := h.users.Signup(c.Request().Context(), projectID, auth.EndUserSignupInput{
		ExternalID:  req.Username,
		Password:    req.Password,
		DisplayName: req.DisplayName,
		Email:       req.Email,
	})
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrEndUserExists):
			return WriteError(c, NewAPIError(http.StatusConflict, "user_exists", "username already taken", rid))
		case errors.Is(err, auth.ErrEndUserBadCredentials), errors.Is(err, auth.ErrWeakPassword):
			return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_credentials", "username or password invalid", rid))
		default:
			return WriteError(c, err)
		}
	}
	h.guard.recordSuccess(c.RealIP(), req.Username)
	if h.audit != nil {
		_ = h.audit.Record(c.Request().Context(), AuditEvent{ProjectID: projectID, Kind: "enduser.signup", Detail: "enduser:" + u.ID})
	}
	return c.JSON(http.StatusCreated, endUserTokenResponse(token, u))
}

// Login: POST /v1/projects/:projectID/end-users/login（免认证，§8.2.6）。
func (h *EndUsersHandler) Login(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	projectID := c.Param("projectID")
	var req endUserLoginRequest
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
	u, token, err := h.users.Login(c.Request().Context(), projectID, req.Username, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrEndUserBadCredentials) || errors.Is(err, auth.ErrEndUserDisabled) {
			h.guard.recordFailure(c.RealIP(), req.Username)
			if h.audit != nil {
				_ = h.audit.Record(c.Request().Context(), AuditEvent{ProjectID: projectID, Kind: "enduser.login_failed", Detail: "external:" + req.Username})
			}
			// 统一 401：不泄露「用户存在与否 / 是否被禁用」。
			return WriteError(c, NewAPIError(http.StatusUnauthorized, "invalid_credentials", "invalid username or password", rid))
		}
		return WriteError(c, err)
	}
	h.guard.recordSuccess(c.RealIP(), req.Username)
	if h.audit != nil {
		_ = h.audit.Record(c.Request().Context(), AuditEvent{ProjectID: projectID, Kind: "enduser.login", Detail: "enduser:" + u.ID})
	}
	return c.JSON(http.StatusOK, endUserTokenResponse(token, u))
}

// Me: GET /v1/projects/:projectID/end-users/me（endUserAuth 守卫）。
func (h *EndUsersHandler) Me(c echo.Context) error {
	claims, ok := endUserClaimsFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, NewAPIError(http.StatusUnauthorized, "unauthorized", "end user token required", RequestIDFromContext(c.Request().Context())))
	}
	var body struct {
		ID        string `json:"id"`
		ProjectID string `json:"project_id"`
		Subject   string `json:"sub"`
	}
	body.ID = claims.Subject
	body.ProjectID = claims.ProjectID
	body.Subject = claims.Username
	return c.JSON(http.StatusOK, body)
}

func endUserTokenResponse(token string, u auth.EndUser) endUserAuthResponse {
	out := endUserAuthResponse{Token: token, ExpiresIn: int64(auth.EndUserTokenTTL.Seconds())}
	out.User.ID = u.ID
	out.User.DisplayName = u.DisplayName
	out.User.Email = u.Email
	return out
}

func (h *EndUsersHandler) guardAllow(c echo.Context, username string) error {
	rid := RequestIDFromContext(c.Request().Context())
	switch h.guard.allow(c.RealIP(), username) {
	case loginSoftLocked:
		if h.audit != nil {
			_ = h.audit.Record(c.Request().Context(), AuditEvent{Kind: "enduser.login_locked", Detail: "external:" + username})
		}
		return WriteError(c, NewAPIError(http.StatusTooManyRequests, "login_locked", "account temporarily locked due to repeated failures", rid))
	case loginRateLimited:
		return WriteError(c, NewAPIError(http.StatusTooManyRequests, "rate_limited", "too many attempts", rid))
	}
	return nil
}
