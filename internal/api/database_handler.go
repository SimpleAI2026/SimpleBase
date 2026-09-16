// database_handler.go 实现 Plan 5 的数据库管理 API handler。
//
// 所有 handler 遵循同一流程：
//   1. 从 context 取已校验的 ProjectContext（由 projectContextMiddleware 注入）。
//   2. 从 context 取 Principal（由 auth.APIKeyMiddleware 注入）。
//   3. 解析 path 参数；bind body 后显式校验。
//   4. 调用 service 层；将 typed error 交给 WriteError 统一映射。
//   5. 响应绝不包含 S3 key、DSN、凭据。
//
// handler 不直接访问 catalog.Repository 或 registry.Registry，只通过
// DatabaseService 接口依赖，便于测试用假实现替换。
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
)

// DatabaseService 抽象 handler 所需的 catalog + registry 能力。
// handler 只依赖此接口，不直接引用具体类型，便于测试替换。
type DatabaseService interface {
	// CreateDatabase 创建 logical database 记录并写 S3 descriptor。
	CreateDatabase(ctx context.Context, in catalog.CreateDatabaseInput) (catalog.Database, error)
	// GetDatabase 返回指定数据库的 catalog 记录（已校验 project 归属）。
	GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	// ListDatabases 分页列出 project 内的数据库。
	ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error)
	// BeginDeleteDatabase 将数据库转入 deleting 状态（测试/兼容）。
	BeginDeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	// DeleteDatabase 软删并同步清理平面 B 存储，返回最终记录（通常 status=deleted）。
	DeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error)
	// Acquire 获取数据库访问租约（预热/打开）。
	Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (Lease, error)
	// CloseDatabase 主动关闭数据库的本地连接（不删除 S3 数据）。
	CloseDatabase(ctx context.Context, databaseID string) error
	// SetDatabaseReady 将 creating/opening/recovering 转为 ready。
	SetDatabaseReady(ctx context.Context, databaseID string) error
}

// Lease 是数据库访问租约的抽象接口。registry.Lease 自动满足此接口。
type Lease interface {
	Release()
}

// DatabaseHandler 实现 Plan 5 的全部数据库管理路由。
type DatabaseHandler struct {
	svc      DatabaseService
	writable bool
	// SnapshotFor 可选：填充 DuckLake 同步水位。
	SnapshotFor func(databaseID string) *DatabaseSnapshot
}

// NewDatabaseHandler 构造 handler。writable 为 false 时所有写操作返回 503。
func NewDatabaseHandler(svc DatabaseService, writable bool) *DatabaseHandler {
	return &DatabaseHandler{svc: svc, writable: writable}
}

// CreateDatabaseRequest 是创建数据库的请求体。
type CreateDatabaseRequest struct {
	Name string `json:"name"`
}

// DatabaseResponse 是数据库资源的对外表示。绝不包含 StoragePrefix/DSN/凭据。
type DatabaseResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	// Snapshot 是 DuckLake catalog 同步水位（Phase 2）；未启用远程时为零值省略。
	Snapshot  *DatabaseSnapshot `json:"snapshot,omitempty"`
}

// DatabaseSnapshot 暴露 last_synced / sync_lag（§八 API）。
type DatabaseSnapshot struct {
	LastSyncedSnapshot int64 `json:"last_synced_snapshot"`
	SyncLag            int64 `json:"sync_lag"`
}

// DatabaseListResponse 是分页列表响应。NextCursor 为空表示无更多数据。
type DatabaseListResponse struct {
	Databases  []DatabaseResponse `json:"databases"`
	NextCursor string             `json:"next_cursor,omitempty"`
}

// DeleteDatabaseResponse 是删除操作响应（HTTP 202；同步清理完成后 status 多为 deleted）。
type DeleteDatabaseResponse struct {
	DatabaseID string `json:"database_id"`
	Status     string `json:"status"`
}

// CreateDatabase: POST /v1/projects/:projectID/databases
func (h *DatabaseHandler) CreateDatabase(c echo.Context) error {
	if !h.writable {
		return WriteError(c, database.ErrWriterUnavailable)
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}
	// 权限校验由 Require(DatabaseAdmin) 中间件完成，但 handler 需 principal 传递给 service
	_ = principal

	var req CreateDatabaseRequest
	if err := bindAndValidateCreate(c, &req); err != nil {
		return WriteError(c, err)
	}

	db, err := h.svc.CreateDatabase(c.Request().Context(), catalog.CreateDatabaseInput{
		TenantID:  project.TenantID,
		ProjectID: project.ID,
		Name:      req.Name,
	})
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusCreated, toDatabaseResponse(db))
}

// ListDatabases: GET /v1/projects/:projectID/databases
func (h *DatabaseHandler) ListDatabases(c echo.Context) error {
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}

	page, err := parseListParams(c)
	if err != nil {
		return WriteError(c, err)
	}

	dbs, nextCursor, err := h.svc.ListDatabases(c.Request().Context(), principal, project.ID, page)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]DatabaseResponse, 0, len(dbs))
	for _, db := range dbs {
		out = append(out, toDatabaseResponse(db))
	}
	return c.JSON(http.StatusOK, DatabaseListResponse{Databases: out, NextCursor: nextCursor})
}

// GetDatabase: GET /v1/projects/:projectID/databases/:databaseID
func (h *DatabaseHandler) GetDatabase(c echo.Context) error {
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	databaseID := c.Param("databaseID")
	if databaseID == "" {
		return c.JSON(http.StatusBadRequest, APIErrorBody{Error: APIErrorDetail{Code: "invalid_request", Message: "database_id required"}})
	}

	db, err := h.svc.GetDatabase(c.Request().Context(), principal, project.ID, databaseID)
	if err != nil {
		return WriteError(c, err)
	}
	resp := toDatabaseResponse(db)
	if h.SnapshotFor != nil {
		resp.Snapshot = h.SnapshotFor(databaseID)
	}
	return c.JSON(http.StatusOK, resp)
}

// OpenDatabase: POST /v1/projects/:projectID/databases/:databaseID/open
// 预热数据库：获取租约后立即释放，触发 registry 打开连接。
func (h *DatabaseHandler) OpenDatabase(c echo.Context) error {
	if !h.writable {
		return WriteError(c, database.ErrWriterUnavailable)
	}
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	databaseID := c.Param("databaseID")
	if databaseID == "" {
		return c.JSON(http.StatusBadRequest, APIErrorBody{Error: APIErrorDetail{Code: "invalid_request", Message: "database_id required"}})
	}

	db, err := h.svc.GetDatabase(c.Request().Context(), principal, project.ID, databaseID)
	if err != nil {
		return WriteError(c, err)
	}
	lease, err := h.svc.Acquire(c.Request().Context(), db, database.ReadWrite)
	if err != nil {
		return WriteError(c, err)
	}
	lease.Release()
	// 预热成功后推进状态（兼容历史卡在 creating 的库）。
	_ = h.svc.SetDatabaseReady(c.Request().Context(), databaseID)
	if refreshed, err := h.svc.GetDatabase(c.Request().Context(), principal, project.ID, databaseID); err == nil {
		db = refreshed
	}
	return c.JSON(http.StatusOK, toDatabaseResponse(db))
}

// CloseDatabase: POST /v1/projects/:projectID/databases/:databaseID/close
// 释放本地资源（关闭连接），不删除 S3 数据。
func (h *DatabaseHandler) CloseDatabase(c echo.Context) error {
	if !h.writable {
		return WriteError(c, database.ErrWriterUnavailable)
	}
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	databaseID := c.Param("databaseID")
	if databaseID == "" {
		return c.JSON(http.StatusBadRequest, APIErrorBody{Error: APIErrorDetail{Code: "invalid_request", Message: "database_id required"}})
	}

	// 先校验归属，再关闭
	_, err := h.svc.GetDatabase(c.Request().Context(), principal, project.ID, databaseID)
	if err != nil {
		return WriteError(c, err)
	}
	if err := h.svc.CloseDatabase(c.Request().Context(), databaseID); err != nil {
		return WriteError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteDatabase: DELETE /v1/projects/:projectID/databases/:databaseID
// 软删除并同步清理平面 B（DuckLake S3 前缀 / descriptor）；返回 202。
// status 在清理成功后为 deleted（契约仍用 202；前端以列表刷新为准）。
func (h *DatabaseHandler) DeleteDatabase(c echo.Context) error {
	if !h.writable {
		return WriteError(c, database.ErrWriterUnavailable)
	}
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	databaseID := c.Param("databaseID")
	if databaseID == "" {
		return c.JSON(http.StatusBadRequest, APIErrorBody{Error: APIErrorDetail{Code: "invalid_request", Message: "database_id required"}})
	}

	db, err := h.svc.DeleteDatabase(c.Request().Context(), principal, project.ID, databaseID)
	if err != nil {
		return WriteError(c, err)
	}
	status := string(db.Status)
	if status == "" {
		status = "deleted"
	}
	return c.JSON(http.StatusAccepted, DeleteDatabaseResponse{
		DatabaseID: databaseID,
		Status:     status,
	})
}

// CreateBackup 和 RestoreDatabase 属于 Plan 7（备份恢复）范围，首期返回 501。
func (h *DatabaseHandler) CreateBackup(c echo.Context) error {
	return c.JSON(http.StatusNotImplemented, APIErrorBody{Error: APIErrorDetail{
		Code: "not_implemented", Message: "backup API not yet implemented",
	}})
}

func (h *DatabaseHandler) RestoreDatabase(c echo.Context) error {
	return c.JSON(http.StatusNotImplemented, APIErrorBody{Error: APIErrorDetail{
		Code: "not_implemented", Message: "restore API not yet implemented",
	}})
}

// projectContextMiddleware 解析 :projectID path 参数并通过 catalog 解析其 tenant，
// 构造 ProjectContext 注入 context。project 不存在返回 404。
// 使用 echo.Context 以便传递请求 context 给 catalog。
func projectContextMiddlewareEcho(deps Dependencies) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			projectID := c.Param("projectID")
			if projectID == "" {
				return c.JSON(http.StatusBadRequest, APIErrorBody{Error: APIErrorDetail{Code: "invalid_request", Message: "project_id required"}})
			}
			if deps.Catalog == nil {
				return c.JSON(http.StatusServiceUnavailable, APIErrorBody{Error: APIErrorDetail{Code: "service_unavailable", Message: "catalog not configured"}})
			}
			tenantID, err := deps.Catalog.ResolveProjectTenant(c.Request().Context(), projectID)
			if err != nil {
				return WriteError(c, err)
			}
			ctx := WithProject(c.Request().Context(), ProjectContext{ID: projectID, TenantID: tenantID})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

// bindAndValidateCreate 绑定并校验创建请求。不依赖第三方 validator。
// decode/校验失败返回 APIError(400)，由 WriteError 直接使用。
func bindAndValidateCreate(c echo.Context, req *CreateDatabaseRequest) error {
	dec := json.NewDecoder(c.Request().Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(req); err != nil {
		return NewAPIError(http.StatusBadRequest, "invalid_request", "malformed JSON body", RequestIDFromContext(c.Request().Context()))
	}
	if err := validateDatabaseName(req.Name); err != nil {
		return err
	}
	return nil
}

// validateDatabaseName 校验数据库名称（plan5 约束：1-63 字节，无路径分隔符/控制字符）。
func validateDatabaseName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: name is empty", catalog.ErrInvalidName)
	}
	if len(name) > 63 {
		return fmt.Errorf("%w: name exceeds 63 bytes", catalog.ErrInvalidName)
	}
	for _, r := range name {
		if r == '/' || r == '\\' || r < 0x20 {
			return fmt.Errorf("%w: name contains invalid character", catalog.ErrInvalidName)
		}
	}
	return nil
}

// parseListParams 解析分页参数 limit 和 cursor。
func parseListParams(c echo.Context) (catalog.Page, error) {
	page := catalog.Page{Limit: 50}
	if l := c.QueryParam("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 || n > 200 {
			return catalog.Page{}, fmt.Errorf("invalid limit parameter")
		}
		page.Limit = n
	}
	page.Cursor = c.QueryParam("cursor")
	return page, nil
}

// toDatabaseResponse 将 catalog.Database 转为对外响应，屏蔽 StoragePrefix 等敏感字段。
func toDatabaseResponse(db catalog.Database) DatabaseResponse {
	return DatabaseResponse{
		ID:        db.ID,
		Name:      db.Name,
		Status:    string(db.Status),
		CreatedAt: db.CreatedAt,
		UpdatedAt: db.UpdatedAt,
	}
}
