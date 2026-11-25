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
//	[同包] adapter.go, agent_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] Status() ← adapter.go, agent_handler.go
//
// 主要符号
//
//	type MigrationHandler struct {…}
//	NewMigrationHandler(store *systemdb.Store) *MigrationHandler
//	(h *MigrationHandler) Status(c echo.Context) error
//	(h *MigrationHandler) Plan(c echo.Context) error

package api

import (
	"context"
	"database/sql"
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
	return serveAdminMigration(c, h.store, "status", systemdb.Status)
}

func (h *MigrationHandler) Plan(c echo.Context) error {
	return serveAdminMigration(c, h.store, "plan", systemdb.Plan)
}

// serveAdminMigration 合并 Status/Plan 路由的共同检查：系统库可用性、admin 项目判断、调用 systemdb.Status/Plan。
// queryFunc: systemdb.Status / systemdb.Plan 类型为 func(ctx, *sql.DB) (T, error)。
func serveAdminMigration[T any](c echo.Context, store *systemdb.Store, operationName string, queryFunc func(context.Context, *sql.DB) (T, error)) error {
	if store == nil {
		return WriteError(c, systemdb.ErrUnavailable)
	}
	if !systemdb.IsAdminProject(projectID(c)) {
		msg := "migration " + operationName + " requires the admin project"
		return WriteError(c, NewAPIError(http.StatusForbidden, "system_project_required", msg, requestID(c)))
	}
	result, err := queryFunc(c.Request().Context(), store.DB())
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, result)
}
