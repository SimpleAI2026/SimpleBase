// 文件说明：migration_handler.go — internal/api 中 MigrationHandler、NewMigrationHandler 等符号的实现
//
// 用法
//
//	NewMigrationHandler(store *systemdb.Store) *MigrationHandler
//	(h *MigrationHandler) Status(c echo.Context) error
//	(h *MigrationHandler) Plan(c echo.Context) error
//	同包调用：Status() → adapter.go, agent_handler.go 等；Plan() → database_handler.go, quota_audit_handler.go 等；NewMigrationHandler() → router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/migration_handler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/systemdb
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] Status() ← adapter.go, agent_handler.go, agent_schedule_handler.go
//
// 主要符号
//
//	type MigrationHandler struct {…}
//	NewMigrationHandler(store *systemdb.Store) *MigrationHandler
//	(h *MigrationHandler) Status(c echo.Context) error
//	(h *MigrationHandler) Plan(c echo.Context) error

package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type MigrationHandler struct {
	store *systemdb.Store
}

func NewMigrationHandler(store *systemdb.Store) *MigrationHandler {
	return &MigrationHandler{store: store}
}

func (h *MigrationHandler) Status(c echo.Context) error {
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	if !systemdb.IsAdminProject(projectID(c)) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "system_project_required", "migration status requires the admin project", RequestIDFromContext(c.Request().Context())))
	}
	status, err := systemdb.Status(c.Request().Context(), h.store.DB())
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, status)
}

func (h *MigrationHandler) Plan(c echo.Context) error {
	if h.store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	if !systemdb.IsAdminProject(projectID(c)) {
		return WriteError(c, NewAPIError(http.StatusForbidden, "system_project_required", "migration plan requires the admin project", RequestIDFromContext(c.Request().Context())))
	}
	plan, err := systemdb.Plan(c.Request().Context(), h.store.DB())
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, plan)
}
