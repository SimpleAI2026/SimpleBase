// audit_handler.go 实现 Plan 9 审计查询 handler。
package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

// AuditHandler 依赖 AuditService。
type AuditHandler struct {
	svc AuditService
}

// ListOperations 返回当前 project 的审计事件列表（占位）。
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
	_ = h.svc
	_ = pc
	return c.JSON(http.StatusOK, map[string]any{
		"operations": []any{},
		"limit":      limit,
	})
}
