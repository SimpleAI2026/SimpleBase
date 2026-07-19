package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// HealthHandler 实现 /health/live 与 /health/ready。
// live 只检查进程存活；ready 在 Writable 模式下必须包含 catalog+S3。
type HealthHandler struct {
	checker HealthChecker
}

func (h *HealthHandler) Live(c echo.Context) error {
	if h.checker == nil {
		return c.NoContent(http.StatusOK)
	}
	if err := h.checker.Live(c.Request().Context()); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unhealthy"})
	}
	return c.NoContent(http.StatusOK)
}

func (h *HealthHandler) Ready(c echo.Context) error {
	if h.checker == nil {
		return c.NoContent(http.StatusOK)
	}
	if err := h.checker.Ready(c.Request().Context()); err != nil {
		rid := RequestIDFromContext(c.Request().Context())
		return c.JSON(http.StatusServiceUnavailable, APIErrorBody{Error: APIErrorDetail{
			Code: "not_ready", Message: "dependency check failed", RequestID: rid,
		}})
	}
	return c.NoContent(http.StatusOK)
}
