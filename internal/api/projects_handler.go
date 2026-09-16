package api

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// ProjectResponse 是项目列表项（不含敏感字段）。
type ProjectResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ProjectListResponse 是 GET /v1/projects 的响应。
type ProjectListResponse struct {
	Projects []ProjectResponse `json:"projects"`
}

// ProjectsHandler 提供租户/授权范围内的项目枚举。
type ProjectsHandler struct {
	catalog CatalogService
}

// NewProjectsHandler 构造 ProjectsHandler。
func NewProjectsHandler(cat CatalogService) *ProjectsHandler {
	return &ProjectsHandler{catalog: cat}
}

// ListProjects: GET /v1/projects
// 返回当前 API key 可见的项目（ProjectAdmin 为租户下全部）。
func (h *ProjectsHandler) ListProjects(c echo.Context) error {
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}
	list, err := h.catalog.ListProjects(c.Request().Context(), principal)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]ProjectResponse, 0, len(list))
	for _, p := range list {
		out = append(out, ProjectResponse{
			ID:        p.ID,
			Name:      p.Name,
			CreatedAt: p.CreatedAt,
		})
	}
	return c.JSON(http.StatusOK, ProjectListResponse{Projects: out})
}
