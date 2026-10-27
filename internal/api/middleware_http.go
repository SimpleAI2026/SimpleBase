// 文件说明：middleware_http.go — internal/api 的内部实现（securityHeadersMiddleware、corsMiddleware 等）
//
// 用法
//
//	同包调用：securityHeadersMiddleware() → router.go；corsMiddleware() → router.go；gzipMiddleware() → router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/middleware_http.go
//	         │ 依赖
//	         ▼
//	[下游] internal/config
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] securityHeadersMiddleware() ← router.go
//
// 主要符号
//
//	securityHeadersMiddleware() echo.MiddlewareFunc
//	corsMiddleware(cfg config.CORSConfig) echo.MiddlewareFunc
//	gzipMiddleware() echo.MiddlewareFunc
//	trustedIPExtractor(cidrs []string) echo.IPExtractor

package api

import (
	"net"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

func securityHeadersMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
			return next(c)
		}
	}
}

func corsMiddleware(cfg config.CORSConfig) echo.MiddlewareFunc {
	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     cfg.AllowedMethods,
		AllowHeaders:     cfg.AllowedHeaders,
		ExposeHeaders:    cfg.ExposedHeaders,
		AllowCredentials: cfg.AllowCredentials,
		MaxAge:           cfg.MaxAge,
	})
}

func gzipMiddleware() echo.MiddlewareFunc {
	return middleware.GzipWithConfig(middleware.GzipConfig{Skipper: func(c echo.Context) bool {
		if c.Request() == nil || c.Request().URL == nil {
			return false
		}
		p := c.Request().URL.Path
		return strings.HasSuffix(p, "/events") || strings.HasSuffix(p, "/realtime") ||
			strings.HasSuffix(p, "/objects/content") || strings.HasSuffix(p, "/download") ||
			strings.HasSuffix(p, "/export") || strings.HasPrefix(p, "/files/")
	}})
}

func trustedIPExtractor(cidrs []string) echo.IPExtractor {
	if len(cidrs) == 0 {
		return echo.ExtractIPDirect()
	}
	options := make([]echo.TrustOption, 0, len(cidrs))
	for _, value := range cidrs {
		_, network, err := net.ParseCIDR(value)
		if err == nil {
			options = append(options, echo.TrustIPRange(network))
		}
	}
	return echo.ExtractIPFromXFFHeader(options...)
}
