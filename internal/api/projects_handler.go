package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// ProjectResponse 是项目列表项 / 创建结果（不含敏感字段）。
type ProjectResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ProjectListResponse 是 GET /v1/projects 的响应。
type ProjectListResponse struct {
	Projects []ProjectResponse `json:"projects"`
}

// CreateProjectRequest 是 POST /v1/projects 的请求体。
type CreateProjectRequest struct {
	Name string `json:"name"`
	ID   string `json:"id,omitempty"`
}

// ProjectsHandler 提供租户/授权范围内的项目枚举与创建。
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
		out = append(out, toProjectResponse(p))
	}
	return c.JSON(http.StatusOK, ProjectListResponse{Projects: out})
}

// CreateProject: POST /v1/projects
// body { "name": "...", "id": "<optional UUID>" } → 201 { id, name, created_at }
func (h *ProjectsHandler) CreateProject(c echo.Context) error {
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}
	req, err := bindCreateProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	p, err := h.catalog.CreateProject(c.Request().Context(), principal, catalog.CreateProjectInput{
		Name: req.Name,
		ID:   req.ID,
	})
	if err != nil {
		rid := RequestIDFromContext(c.Request().Context())
		if errors.Is(err, catalog.ErrAlreadyExists) {
			return WriteError(c, NewAPIError(http.StatusConflict, "project_already_exists", "project already exists", rid))
		}
		if errors.Is(err, catalog.ErrInvalidName) {
			msg := err.Error()
			code := "invalid_project_name"
			if strings.Contains(msg, "id must be a UUID") || strings.Contains(msg, "reserved project id") {
				code = "invalid_project_id"
			}
			return WriteError(c, NewAPIError(http.StatusBadRequest, code, msg, rid))
		}
		return WriteError(c, err)
	}
	return c.JSON(http.StatusCreated, toProjectResponse(p))
}

func toProjectResponse(p catalog.Project) ProjectResponse {
	return ProjectResponse{
		ID:        p.ID,
		Name:      p.Name,
		CreatedAt: p.CreatedAt,
	}
}

func bindCreateProject(c echo.Context) (CreateProjectRequest, error) {
	var req CreateProjectRequest
	dec := json.NewDecoder(c.Request().Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return CreateProjectRequest{}, NewAPIError(http.StatusBadRequest, "invalid_request", "malformed JSON body", RequestIDFromContext(c.Request().Context()))
	}
	req.Name = strings.TrimSpace(req.Name)
	req.ID = strings.TrimSpace(req.ID)
	if req.Name == "" {
		return CreateProjectRequest{}, NewAPIError(http.StatusBadRequest, "invalid_project_name", "name is required", RequestIDFromContext(c.Request().Context()))
	}
	return req, nil
}
