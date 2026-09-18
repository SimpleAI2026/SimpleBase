// handlers_plan79.go 实现 Plan 7-9 的配额查询与审计查询 handler。
package api

import (
	"errors"
	"net/http"
	"strconv"

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

// AuditHandler 依赖 AuditService。
type AuditHandler struct {
	svc AuditService
}

// ListOperations 返回当前 project 的审计事件列表（读 sys_operations）。
func (h *AuditHandler) ListOperations(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	limit := 50
	if s := c.QueryParam("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	ops, err := h.svc.ListOperations(c.Request().Context(), pc.ID, c.QueryParam("database_id"), limit)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"operations": ops,
		"limit":      limit,
	})
}
