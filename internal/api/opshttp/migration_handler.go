package opshttp

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
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
		return httpkit.WriteError(c, systemdb.ErrUnavailable)
	}
	if !systemdb.IsAdminProject(httpkit.ProjectID(c)) {
		msg := "migration " + operationName + " requires the admin project"
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusForbidden, "system_project_required", msg, httpkit.RequestID(c)))
	}
	result, err := queryFunc(c.Request().Context(), store.DB())
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, result)
}
