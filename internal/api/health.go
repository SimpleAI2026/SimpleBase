// 文件说明：health.go — internal/api 中 HealthHandler、Live 等符号的实现
//
// 用法
//
//	(h *HealthHandler) Live(c echo.Context) error
//	(h *HealthHandler) Ready(c echo.Context) error
//	同包调用：HealthHandler → router.go；Live() → router.go；Ready() → router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/health.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backup_handler.go, data_compat.go, error.go, file_compat.go, function_compat.go, mcp_builtin.go, mcp_builtin_admin.go, mcp_builtin_session.go 等 28 个
//	[关键引用] HealthHandler ← router.go
//
// 主要符号
//
//	type HealthHandler struct {…}
//	(h *HealthHandler) Live(c echo.Context) error
//	(h *HealthHandler) Ready(c echo.Context) error

package api

import (
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
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
		rid := httpkit.RequestID(c)
		return c.JSON(http.StatusServiceUnavailable, httpkit.APIErrorBody{Error: httpkit.APIErrorDetail{
			Code: "not_ready", Message: "dependency check failed", RequestID: rid,
		}})
	}
	return c.NoContent(http.StatusOK)
}
