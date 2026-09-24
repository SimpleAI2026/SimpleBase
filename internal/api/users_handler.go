// users_handler.go 提供 /v1/users 用户管理（planv3.0 §4.4）。
package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// UsersHandler 处理用户 CRUD。
type UsersHandler struct {
	users    *auth.UserService
	sessions *auth.SessionService
}

// NewUsersHandler 构造 UsersHandler。
func NewUsersHandler(users *auth.UserService, sessions *auth.SessionService) *UsersHandler {
	return &UsersHandler{users: users, sessions: sessions}
}

type createUserRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

type patchUserRequest struct {
	Role               *string `json:"role"`
	DisplayName        *string `json:"display_name"`
	Email              *string `json:"email"`
	Status             *string `json:"status"`
	Password           *string `json:"password"`
	MustChangePassword *bool   `json:"must_change_password"`
}

type userListResponse struct {
	Users      []userListItem `json:"users"`
	NextCursor string         `json:"next_cursor"`
}

type userListItem struct {
	userResponse
	ProjectCount int `json:"project_count"`
}

// List: GET /v1/users
func (h *UsersHandler) List(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	p, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, NewAPIError(http.StatusUnauthorized, "unauthenticated", "login required", rid))
	}
	if !p.Role.CanViewUsers() {
		return WriteError(c, NewAPIError(http.StatusForbidden, "role_forbidden", "permission denied", rid))
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	cursor := c.QueryParam("cursor")
	list, next, err := h.users.List(c.Request().Context(), limit, cursor)
	if err != nil {
		return WriteError(c, err)
	}
	out := userListResponse{NextCursor: next}
	for _, u := range list {
		item := userListItem{userResponse: toUserResponse(u)}
		if ids, err := h.users.ProjectIDsFor(c.Request().Context(), u); err == nil {
			n := 0
			for id := range ids {
				if id != auth.AdminProjectID {
					n++
				}
			}
			item.ProjectCount = n
		}
		out.Users = append(out.Users, item)
	}
	return c.JSON(http.StatusOK, out)
}

// Create: POST /v1/users（仅 superadminl1）。
func (h *UsersHandler) Create(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	p, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, NewAPIError(http.StatusUnauthorized, "unauthenticated", "login required", rid))
	}
	if !p.Role.CanManageUsers() {
		return WriteError(c, NewAPIError(http.StatusForbidden, "role_forbidden", "permission denied", rid))
	}
	var req createUserRequest
	if err := c.Bind(&req); err != nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid json body", rid))
	}
	u, err := h.users.Create(c.Request().Context(), auth.CreateUserInput{
		Username:    req.Username,
		Password:    req.Password,
		Role:        auth.Role(req.Role),
		DisplayName: req.DisplayName,
		Email:       req.Email,
		CreatedBy:   p.UserID,
	})
	if err != nil {
		return WriteError(c, mapUserErr(err, rid))
	}
	return c.JSON(http.StatusCreated, toUserResponse(u))
}

// Get: GET /v1/users/:id
func (h *UsersHandler) Get(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	p, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, NewAPIError(http.StatusUnauthorized, "unauthenticated", "login required", rid))
	}
	id := c.Param("id")
	if !p.Role.CanViewUsers() && p.UserID != id {
		return WriteError(c, NewAPIError(http.StatusForbidden, "role_forbidden", "permission denied", rid))
	}
	u, err := h.users.GetByID(c.Request().Context(), id)
	if err != nil {
		return WriteError(c, mapUserErr(err, rid))
	}
	return c.JSON(http.StatusOK, toUserResponse(u))
}

// Patch: PATCH /v1/users/:id（仅 superadminl1）。
func (h *UsersHandler) Patch(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	p, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, NewAPIError(http.StatusUnauthorized, "unauthenticated", "login required", rid))
	}
	if !p.Role.CanManageUsers() {
		return WriteError(c, NewAPIError(http.StatusForbidden, "role_forbidden", "permission denied", rid))
	}
	id := c.Param("id")
	if id == p.UserID {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "user_protected", "cannot modify yourself here", rid))
	}
	var req patchUserRequest
	if err := c.Bind(&req); err != nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "invalid json body", rid))
	}
	in := auth.UpdateUserInput{
		DisplayName:        req.DisplayName,
		Email:              req.Email,
		Password:           req.Password,
		MustChangePassword: req.MustChangePassword,
	}
	if req.Role != nil {
		r := auth.Role(*req.Role)
		in.Role = &r
	}
	if req.Status != nil {
		in.Status = req.Status
	}
	u, err := h.users.Update(c.Request().Context(), id, in)
	if err != nil {
		return WriteError(c, mapUserErr(err, rid))
	}
	// 改密/禁用 → 吊销会话。
	if req.Password != nil && *req.Password != "" {
		_ = h.sessions.RevokeAllForUser(c.Request().Context(), id)
	}
	if req.Status != nil && *req.Status == auth.UserStatusDisabled {
		_ = h.sessions.RevokeAllForUser(c.Request().Context(), id)
	}
	return c.JSON(http.StatusOK, toUserResponse(u))
}

// Delete: DELETE /v1/users/:id（软删 = disable）。
func (h *UsersHandler) Delete(c echo.Context) error {
	rid := RequestIDFromContext(c.Request().Context())
	p, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, NewAPIError(http.StatusUnauthorized, "unauthenticated", "login required", rid))
	}
	if !p.Role.CanManageUsers() {
		return WriteError(c, NewAPIError(http.StatusForbidden, "role_forbidden", "permission denied", rid))
	}
	id := c.Param("id")
	if id == p.UserID {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "user_protected", "cannot delete yourself", rid))
	}
	u, err := h.users.GetByID(c.Request().Context(), id)
	if err != nil {
		return WriteError(c, mapUserErr(err, rid))
	}
	if u.Username == auth.BootstrapUsername {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "user_protected", "bootstrap superadmin cannot be deleted", rid))
	}
	if _, err := h.users.Disable(c.Request().Context(), id); err != nil {
		return WriteError(c, mapUserErr(err, rid))
	}
	_ = h.sessions.RevokeAllForUser(c.Request().Context(), id)
	return c.NoContent(http.StatusNoContent)
}

func mapUserErr(err error, rid string) error {
	switch {
	case errors.Is(err, auth.ErrUserNotFound):
		return NewAPIError(http.StatusNotFound, "user_not_found", "user not found", rid)
	case errors.Is(err, auth.ErrUsernameTaken):
		return NewAPIError(http.StatusConflict, "username_taken", "username already exists", rid)
	case errors.Is(err, auth.ErrUserProtected):
		return NewAPIError(http.StatusBadRequest, "user_protected", "user is protected", rid)
	case errors.Is(err, auth.ErrInvalidRole):
		return NewAPIError(http.StatusBadRequest, "invalid_role", "invalid role", rid)
	case errors.Is(err, auth.ErrWeakPassword):
		return NewAPIError(http.StatusBadRequest, "weak_password", "password too weak", rid)
	case errors.Is(err, auth.ErrUserDisabled):
		return NewAPIError(http.StatusForbidden, "user_disabled", "user disabled", rid)
	case errors.Is(err, auth.ErrInvalidCredentials):
		return NewAPIError(http.StatusBadRequest, "invalid_request", "invalid username", rid)
	default:
		return err
	}
}

var _ = time.Now
