// 文件说明：projects.go — 项目目录的 HTTP 入站适配（internal-ddd-hexagonal-implementation-plan §6.2 cataloghttp）
//
// 用法
//
//	h := cataloghttp.NewProjectsHandler(cat, users)   // app 经 api.Dependencies 注入；路由由 api 的 mountProjectRoutes 注册
//	h.KV / h.KVInit / h.Logger                        // 可选：项目级 KV 的 catalog 行与 schema 初始化、告警日志
//	(h *ProjectsHandler) ListProjects / CreateProject
//
// 调用架构
//
//	[上游] internal/api（路由注册与守卫）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/cataloghttp/projects.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api/httpkit（错误协议 / 请求解析）, internal/auth, internal/catalog
//
// 主要符号
//
//	type CatalogService interface {…}
//	type KVCatalog interface {…}
//	type KVInitializer interface {…}
//	type ProjectResponse / ProjectListResponse / CreateProjectRequest struct {…}
//	type ProjectsHandler struct {…}
//	NewProjectsHandler(cat CatalogService, users *auth.UserService) *ProjectsHandler
//	(h *ProjectsHandler) ListProjects(c echo.Context) error
//	(h *ProjectsHandler) CreateProject(c echo.Context) error
//
// 项目元数据状态机在 catalog；这里只做协议。建项目后的 KV 初始化失败只告警、不影响项目创建结果。

package cataloghttp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// ProjectResponse 是项目列表项 / 创建结果（不含敏感字段）。
type ProjectResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Engine    string    `json:"engine"`
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
	// Engine 为新项目的单文件引擎：duckdb（默认）| sqlite。
	Engine string `json:"engine,omitempty"`
	// OwnerUserID 仅 superadminl1 可指定；默认创建者。
	OwnerUserID string `json:"owner_user_id,omitempty"`
}

// ProjectsHandler 提供租户/授权范围内的项目枚举与创建。
// CatalogService 是项目管理需要的目录查询（消费方定义；api 的 CatalogService 适配器满足）。
type CatalogService interface {
	ListProjects(ctx context.Context, principal auth.Principal) ([]catalog.Project, error)
	CreateProject(ctx context.Context, principal auth.Principal, in catalog.CreateProjectInput) (catalog.Project, error)
}

// KVCatalog 是「建项目后创建 kind=kv 的 catalog 行」的最小端口（项目级 KV，key-value-ducklake-plan §2）。
type KVCatalog interface {
	CreateKVDatabase(ctx context.Context, tenantID, projectID string) (catalog.Database, error)
}

// KVInitializer 在项目 KV catalog 行创建成功后初始化 kv schema（幂等）；实现属于数据面。
type KVInitializer interface {
	InitKV(ctx context.Context, db catalog.Database) error
}

type ProjectsHandler struct {
	catalog CatalogService
	users   *auth.UserService
	// KV 可选：项目级 Key-Value 服务（key-value-ducklake-plan §2）。
	// 非 nil 且可写时，新建项目会同步创建 kind=kv 的 catalog 行并初始化 KV 表。
	KV KVCatalog
	// KVInit 可选：项目 KV catalog 行创建成功后初始化 kv schema。
	KVInit KVInitializer
	// Logger 可选：后置动作失败时记 warn（不影响项目创建结果）。
	Logger observability.Logger
}

// NewProjectsHandler 构造 ProjectsHandler。users 可为 nil（API Key-only 测试）。
func NewProjectsHandler(cat CatalogService, users *auth.UserService) *ProjectsHandler {
	return &ProjectsHandler{catalog: cat, users: users}
}

// ListProjects: GET /v1/projects
// 返回当前 API key 可见的项目（ProjectAdmin 为租户下全部）。
func (h *ProjectsHandler) ListProjects(c echo.Context) error {
	principal, ok := httpkit.PrincipalFromContext(c.Request().Context())
	if !ok {
		return httpkit.WriteError(c, auth.ErrMissingCredentials)
	}
	list, err := h.catalog.ListProjects(c.Request().Context(), principal)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	out := make([]ProjectResponse, 0, len(list))
	for _, p := range list {
		out = append(out, toProjectResponse(p))
	}
	return c.JSON(http.StatusOK, ProjectListResponse{Projects: out})
}

// CreateProject: POST /v1/projects
// body { "name": "...", "id": "<optional 8-char id>" } → 201 { id, name, created_at }
func (h *ProjectsHandler) CreateProject(c echo.Context) error {
	principal, ok := httpkit.PrincipalFromContext(c.Request().Context())
	if !ok {
		return httpkit.WriteError(c, auth.ErrMissingCredentials)
	}
	req, err := bindCreateProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	p, err := h.catalog.CreateProject(c.Request().Context(), principal, catalog.CreateProjectInput{
		Name:   req.Name,
		ID:     req.ID,
		Engine: req.Engine,
	})
	if err != nil {
		rid := httpkit.RequestID(c)
		if errors.Is(err, catalog.ErrAlreadyExists) {
			return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusConflict, "project_already_exists", "project already exists", rid))
		}
		if errors.Is(err, catalog.ErrInvalidName) {
			msg := err.Error()
			code := "invalid_project_name"
			if strings.Contains(msg, "id must be") || strings.Contains(msg, "reserved project id") {
				code = "invalid_project_id"
			}
			return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusBadRequest, code, msg, rid))
		}
		return httpkit.WriteError(c, err)
	}
	if h.users != nil {
		h.users.ProjectsChanged()
	}
	// 记录项目归属（login-auth-plan §4.5）：user/super 创建后 owner=自己或指定用户。
	if h.users != nil && principal.UserID != "" {
		ownerID := principal.UserID
		if req.OwnerUserID != "" && principal.Role.IsSuper() {
			ownerID = req.OwnerUserID
		}
		if err := h.users.AssignProjectOwner(c.Request().Context(), p.ID, ownerID); err != nil {
			// 归属写失败不回滚项目（仍可用）；日志由 access log 兜底。
			_ = err
		}
	}
	resp := toProjectResponse(p)
	// 项目 KV catalog（key-value-ducklake-plan §2）：随项目创建同步建立，
	// 失败不回滚项目（仍 201）；下一次 KV 请求会补建。
	h.ensureProjectKV(c.Request().Context(), p)
	return c.JSON(http.StatusCreated, resp)
}

// ensureProjectKV 为新项目创建 kind=kv 的 catalog 行并初始化 KV 系统表。
// 任一步失败只记 warn（项目本身仍创建成功）。
func (h *ProjectsHandler) ensureProjectKV(ctx context.Context, p catalog.Project) {
	if h.KV == nil {
		return
	}
	db, err := h.KV.CreateKVDatabase(ctx, p.TenantID, p.ID)
	if err != nil {
		if errors.Is(err, catalog.ErrAlreadyExists) {
			return // 幂等：并发创建或重试
		}
		if h.Logger != nil {
			h.Logger.Warn("create project kv catalog failed",
				zap.String("project_id", p.ID), zap.String("err", err.Error()))
		}
		return
	}
	if h.KVInit != nil {
		_ = h.KVInit.InitKV(ctx, db)
	}
}

func toProjectResponse(p catalog.Project) ProjectResponse {
	return ProjectResponse{
		ID:        p.ID,
		Name:      p.Name,
		Engine:    p.Engine(),
		CreatedAt: p.CreatedAt,
	}
}

func bindCreateProject(c echo.Context) (CreateProjectRequest, error) {
	var req CreateProjectRequest
	if err := httpkit.DecodeJSONBody(c, &req, true); err != nil {
		return CreateProjectRequest{}, err
	}
	req.Name = strings.TrimSpace(req.Name)
	req.ID = strings.TrimSpace(req.ID)
	if req.Name == "" {
		return CreateProjectRequest{}, httpkit.NewAPIError(http.StatusBadRequest, "invalid_project_name", "name is required", httpkit.RequestID(c))
	}
	return req, nil
}
