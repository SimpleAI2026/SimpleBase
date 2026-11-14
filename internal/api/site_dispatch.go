// 文件说明：site_dispatch.go — 对象存储发布网站（planv6.5 §5.1）：按 Host 分流到站点面
//
// 用法
//
//	e.Pre(siteHostMiddleware(cfg.Sites))   // sites.enabled 时注册
//	isSiteRequest(c) / siteProjectFrom(c)  // 安全头、站点 handler 读取分流结果
//
// <pid>.<base_domain> 的请求在路由匹配前把 URL.Path 改写为 /__site + 原路径，
// 从而绕开 /v1 鉴权、SPA fallback 与控制台 CSP；只信任 Host，不读 X-Forwarded-Host。

package api

import (
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

// sitePathPrefix 是站点面的内部路由前缀；只有经过 Host 分流的请求才会被服务。
const sitePathPrefix = "/__site"

// siteProjectCtxKey 是 echo context 中站点项目 ID 的键。
const siteProjectCtxKey = "sb_site_project"

// siteHostMiddleware 识别站点 Host 并改写路径。非站点 Host 原样放行。
func siteHostMiddleware(cfg config.SitesConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			pid, ok := cfg.SiteHostLabel(req.Host)
			if !ok {
				return next(c)
			}
			c.Set(siteProjectCtxKey, pid)
			req.URL.Path = sitePathPrefix + req.URL.Path
			if req.URL.RawPath != "" {
				req.URL.RawPath = sitePathPrefix + req.URL.RawPath
			}
			return next(c)
		}
	}
}

// siteProjectFrom 返回经 Host 分流注入的站点项目 ID。
func siteProjectFrom(c echo.Context) string {
	if c == nil {
		return ""
	}
	pid, _ := c.Get(siteProjectCtxKey).(string)
	return pid
}

// isSiteRequest 判定当前请求是否来自站点子域。
func isSiteRequest(c echo.Context) bool { return siteProjectFrom(c) != "" }
