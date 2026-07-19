// quota_handler.go 实现 Plan 9 配额查询 handler。
package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

// QuotaHandler 依赖 UsageService。
type QuotaHandler struct {
	svc UsageService
}

// GetQuota 返回当前 project 的配额可用状态。
func (h *QuotaHandler) GetQuota(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	llmOK := true
	dbOK := true
	if err := h.svc.CheckQuota(c.Request().Context(), pc.ID, "llm"); err != nil {
		llmOK = false
	}
	if err := h.svc.CheckQuota(c.Request().Context(), pc.ID, "database"); err != nil {
		dbOK = false
	}
	return c.JSON(http.StatusOK, map[string]bool{
		"llm_allowed":      llmOK,
		"database_allowed": dbOK,
	})
}
