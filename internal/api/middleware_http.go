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
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

// routeClass 判定路由类别（§8.1.3）：api / file / console。
// 纯函数，供安全头中间件与测试使用。
func routeClass(path string) string {
	// 文件下载路径优先于通用 /v1 前缀判定。
	if strings.HasPrefix(path, "/files/") ||
		strings.Contains(path, "/s3/objects/content") {
		return "file"
	}
	if strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/go/") ||
		strings.HasPrefix(path, "/health") || strings.HasPrefix(path, "/metrics") {
		return "api"
	}
	return "console"
}

// consoleCSP 是控制台（SPA 与静态资源）的 Content-Security-Policy（§8.1.3）。
// style-src 'unsafe-inline'：Vue :style 绑定与 reka-ui 定位需要内联 style；
// 脚本不放开；connect-src 'self' 覆盖同源 ws(s)://（CSP3 语义）。
const consoleCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; " +
	"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

const apiCSP = "default-src 'none'; frame-ancestors 'none'"

const fileCSP = "sandbox; default-src 'none'"

// securityHeadersMiddleware 按路由类别发送安全响应头（§8.1.3）。
// 四个公共头（nosniff / X-Frame-Options / Referrer-Policy / Permissions-Policy）全类别发送。
// HSTS 只在「确认是 HTTPS」时发：请求本身 TLS，或直连对端在 trusted_proxies 内且 X-Forwarded-Proto: https。
func securityHeadersMiddleware(cfg config.Config) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

			class := routeClass(c.Request().URL.Path)
			cspMode := cfg.HTTP.SecurityHeaders.EffectiveCSP()
			csp := ""
			switch class {
			case "file":
				csp = fileCSP
				h.Set("Cross-Origin-Resource-Policy", "same-site")
			case "api":
				csp = apiCSP
				if strings.HasPrefix(c.Request().URL.Path, "/v1/auth/") {
					h.Set("Cache-Control", "no-store")
				}
			default:
				csp = consoleCSP
				h.Set("Cross-Origin-Opener-Policy", "same-origin")
			}
			if csp != "" && cspMode != config.CSPOff {
				if cspMode == config.CSPReportOnly {
					h.Set("Content-Security-Policy-Report-Only", csp)
				} else {
					h.Set("Content-Security-Policy", csp)
				}
			}

			if cfg.HTTP.SecurityHeaders.HSTSMaxAge > 0 && isHTTPS(c, cfg) {
				v := "max-age=" + strconv.Itoa(cfg.HTTP.SecurityHeaders.HSTSMaxAge)
				if cfg.HTTP.SecurityHeaders.HSTSIncludeSubdomains {
					v += "; includeSubDomains"
				}
				h.Set("Strict-Transport-Security", v)
			}
			return next(c)
		}
	}
}

// isHTTPS 判定请求是否确认经由 HTTPS 到达（§8.1.3）。
// 不用 echo 的 c.Scheme()（它无条件信任 X-Forwarded-Proto）：
// 请求本身 TLS，或直连对端在 trusted_proxies 内且 X-Forwarded-Proto: https。
func isHTTPS(c echo.Context, cfg config.Config) bool {
	if c.Request() != nil && c.Request().TLS != nil {
		return true
	}
	if !isTrustedPeer(c, cfg) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(c.Request().Header.Get("X-Forwarded-Proto")), "https")
}

// isTrustedPeer 判定直连对端地址是否在 trusted_proxies 配置的 CIDR 内。
func isTrustedPeer(c echo.Context, cfg config.Config) bool {
	if len(cfg.HTTP.TrustedProxies) == 0 {
		return false
	}
	ip := c.RealIP()
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, cidr := range cfg.HTTP.TrustedProxies {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(parsed) {
			return true
		}
	}
	return false
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

// trustedIPExtractor 只信任显式配置的 CIDR（§8.1.5）。
// 行为变化：不再隐式信任回环 / 链路本地 / 私网段；本机反代需显式写 127.0.0.1/32。
func trustedIPExtractor(cidrs []string) echo.IPExtractor {
	if len(cidrs) == 0 {
		return echo.ExtractIPDirect()
	}
	// 默认全部不信任，只放开显式 CIDR。
	options := []echo.TrustOption{echo.TrustLoopback(false), echo.TrustLinkLocal(false), echo.TrustPrivateNet(false)}
	for _, value := range cidrs {
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			continue
		}
		options = append(options, echo.TrustIPRange(network))
	}
	return echo.ExtractIPFromXFFHeader(options...)
}
