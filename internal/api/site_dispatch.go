// 文件说明：site_dispatch.go — 对象存储发布网站（planv6.5 §5.1 / planv6.7 §1.2）：分流到站点面
//
// 用法
//
//	e.Pre(siteHostMiddleware(cfg.Sites))   // 子域名方式：sites.subdomain=true 且配置了 base_domain
//	e.Pre(sitePathMiddleware(cfg.Sites))   // 路径方式（默认）：<prefix>/<pid>/...
//	siteRequestFrom(c) / isSiteRequest(c)   // 安全头、站点 handler 读取分流结果
//
// 两种方式都在路由匹配前把 URL.Path 改写为 /__site + 路径，从而绕开 /v1 鉴权与 SPA fallback。
// 子域名方式只信任 Host，不读 X-Forwarded-Host；两种方式互斥，同一进程只注册其中一个。

package api

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// sitePathPrefix 是站点面的内部路由前缀；只有经过分流的请求才会被服务。
const sitePathPrefix = "/__site"

// siteProjectCtxKey 是 echo context 中站点分流结果（siteRequest）的键。
const siteProjectCtxKey = "sb_site_project"

// siteRequest 是 Pre 阶段写入 context 的分流结果。
type siteRequest struct {
	ProjectID string
	Mode      string // config.SiteModeSubdomain | config.SiteModePath
	Mount     string // 对外挂载前缀：子域名模式为 ""，路径模式为 "<prefix>/<pid>"
}

// siteHostMiddleware 识别站点 Host 并改写路径（子域名方式）。非站点 Host 原样放行。
func siteHostMiddleware(cfg config.SitesConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			pid, ok := cfg.SiteHostLabel(req.Host)
			if !ok {
				return next(c)
			}
			c.Set(siteProjectCtxKey, siteRequest{ProjectID: pid, Mode: config.SiteModeSubdomain})
			req.URL.Path = sitePathPrefix + req.URL.Path
			if req.URL.RawPath != "" {
				req.URL.RawPath = sitePathPrefix + req.URL.RawPath
			}
			return next(c)
		}
	}
}

// sitePathMiddleware 识别 <prefix>/<pid>/... 并改写路径（路径方式）。
//
// 只处理 GET / HEAD；pid 必须通过 objectstore.ValidateProjectID（区分大小写）。
// pid 非法时直接由站点面返回 404，不放行给 SPA。
func sitePathMiddleware(cfg config.SitesConfig) echo.MiddlewareFunc {
	prefix := cfg.EffectivePathPrefix()
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			if req.Method != http.MethodGet && req.Method != http.MethodHead {
				return next(c)
			}
			p := req.URL.Path
			if !strings.HasPrefix(p, prefix+"/") {
				return next(c)
			}
			rest := p[len(prefix)+1:]
			pid, tail, _ := strings.Cut(rest, "/")
			hasSlash := strings.Contains(rest, "/")

			rewrite := func(mount string) {
				c.Set(siteProjectCtxKey, siteRequest{ProjectID: pid, Mode: config.SiteModePath, Mount: mount})
				req.URL.Path = sitePathPrefix + "/" + tail
				if raw := req.URL.RawPath; raw != "" {
					// RawPath 只截掉前缀，避免二次解码；无法对齐时清空，交给 Path 重新编码。
					cut := prefix + "/" + pid + "/"
					if strings.HasPrefix(raw, cut) {
						req.URL.RawPath = sitePathPrefix + "/" + raw[len(cut):]
					} else {
						req.URL.RawPath = ""
					}
				}
			}

			if objectstore.ValidateProjectID(pid) != nil {
				// 非法 pid：标记为站点请求但不带项目，Serve 统一 404。
				c.Set(siteProjectCtxKey, siteRequest{Mode: config.SiteModePath})
				req.URL.Path = sitePathPrefix + "/"
				req.URL.RawPath = ""
				return next(c)
			}
			if !hasSlash {
				// /sites/<pid> → /sites/<pid>/
				target := prefix + "/" + pid + "/"
				if q := req.URL.RawQuery; q != "" {
					target += "?" + q
				}
				return c.Redirect(http.StatusMovedPermanently, target)
			}
			rewrite(prefix + "/" + pid)
			return next(c)
		}
	}
}

// siteRequestFrom 返回经分流注入的站点请求信息；非站点请求 ok=false。
func siteRequestFrom(c echo.Context) (siteRequest, bool) {
	if c == nil {
		return siteRequest{}, false
	}
	sr, ok := c.Get(siteProjectCtxKey).(siteRequest)
	return sr, ok
}

// isSiteRequest 判定当前请求是否已被站点面接管（含路径方式下 pid 非法的请求）。
func isSiteRequest(c echo.Context) bool {
	_, ok := siteRequestFrom(c)
	return ok
}
