package filehttp

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// PathPrefix 是站点面的内部路由前缀；只有经过分流的请求才会被服务。
const PathPrefix = "/__site"

// siteProjectCtxKey 是 echo context 中站点分流结果（SiteRequest）的键。
const siteProjectCtxKey = "sb_site_project"

// SiteRequest 是 Pre 阶段写入 context 的分流结果。
type SiteRequest struct {
	ProjectID string
	Mode      string // config.SiteModeSubdomain | config.SiteModePath
	Mount     string // 对外挂载前缀：子域名模式为 ""，路径模式为 "<prefix>/<pid>"
}

// SiteHostMiddleware 识别站点 Host 并改写路径（子域名方式）。非站点 Host 原样放行。
func SiteHostMiddleware(cfg config.SitesConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			pid, ok := cfg.SiteHostLabel(req.Host)
			if !ok {
				return next(c)
			}
			c.Set(siteProjectCtxKey, SiteRequest{ProjectID: pid, Mode: config.SiteModeSubdomain})
			req.URL.Path = PathPrefix + req.URL.Path
			if req.URL.RawPath != "" {
				req.URL.RawPath = PathPrefix + req.URL.RawPath
			}
			return next(c)
		}
	}
}

// SitePathMiddleware 识别 <prefix>/<pid>/... 并改写路径（路径方式）。
//
// 只处理 GET / HEAD；pid 必须通过 objectstore.ValidateProjectID（区分大小写）。
// pid 非法时直接由站点面返回 404，不放行给 SPA。
func SitePathMiddleware(cfg config.SitesConfig) echo.MiddlewareFunc {
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
				c.Set(siteProjectCtxKey, SiteRequest{ProjectID: pid, Mode: config.SiteModePath, Mount: mount})
				req.URL.Path = PathPrefix + "/" + tail
				if raw := req.URL.RawPath; raw != "" {
					// RawPath 只截掉前缀，避免二次解码；无法对齐时清空，交给 Path 重新编码。
					cut := prefix + "/" + pid + "/"
					if strings.HasPrefix(raw, cut) {
						req.URL.RawPath = PathPrefix + "/" + raw[len(cut):]
					} else {
						req.URL.RawPath = ""
					}
				}
			}

			if objectstore.ValidateProjectID(pid) != nil {
				// 非法 pid：标记为站点请求但不带项目，Serve 统一 404。
				c.Set(siteProjectCtxKey, SiteRequest{Mode: config.SiteModePath})
				req.URL.Path = PathPrefix + "/"
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

// RequestFrom 返回经分流注入的站点请求信息；非站点请求 ok=false。
func RequestFrom(c echo.Context) (SiteRequest, bool) {
	if c == nil {
		return SiteRequest{}, false
	}
	sr, ok := c.Get(siteProjectCtxKey).(SiteRequest)
	return sr, ok
}

// IsSiteRequest 判定当前请求是否已被站点面接管（含路径方式下 pid 非法的请求）。
func IsSiteRequest(c echo.Context) bool {
	_, ok := RequestFrom(c)
	return ok
}
