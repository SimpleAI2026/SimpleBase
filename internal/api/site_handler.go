// 文件说明：site_handler.go — 对象存储发布网站（planv6.5 §5.2）：匿名站点读取
//
// 用法
//
//	sh := &SiteHandler{store: fileStore, sites: siteStore, cfg: cfg.Sites}
//	e.Any("/__site/*", sh.Serve)   // 仅经 siteHostMiddleware 分流的请求会被服务
//
// 访问规则：站点开关打开 且 路径被公开（file 精确 / folder 前缀）才返回内容；
// 其余情况（含 key 非法、对象不存在、系统项目）统一 404，不暴露对象是否存在。
// 只读：不写系统库，只读实例同样可服务。

package api

import (
	"errors"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// SiteHandler 提供 <pid>.<base_domain> 的匿名只读访问。
type SiteHandler struct {
	store objectstore.FileStore
	sites SiteStore
	cfg   config.SitesConfig
}

const siteNotFoundHTML = "<!doctype html><html><head><meta charset=\"utf-8\"><title>404 Not Found</title></head>" +
	"<body><h1>404 Not Found</h1></body></html>\n"

// Serve 处理站点请求。
func (h *SiteHandler) Serve(c echo.Context) error {
	pid := siteProjectFrom(c)
	if pid == "" || systemdb.IsAdminProject(pid) || h.sites == nil {
		return siteNotFound(c)
	}
	method := c.Request().Method
	if method != http.MethodGet && method != http.MethodHead {
		c.Response().Header().Set("Allow", "GET, HEAD")
		return c.String(http.StatusMethodNotAllowed, "method not allowed\n")
	}
	contentStore, ok := h.store.(objectstore.ContentStore)
	if !ok {
		return siteNotFound(c)
	}
	state, err := h.sites.Lookup(c.Request().Context(), pid)
	if err != nil || !state.Enabled {
		return siteNotFound(c)
	}

	reqPath := strings.TrimPrefix(c.Request().URL.Path, sitePathPrefix)
	rel := strings.TrimPrefix(reqPath, "/")
	key := rel
	index := h.cfg.EffectiveIndexDocument()
	if key == "" || strings.HasSuffix(key, "/") {
		key += index
	}
	if objectstore.ValidateFileKey(key) != nil {
		return siteNotFound(c)
	}

	if _, public := systemdb.MatchPublicPath(state.Paths, key); public {
		served, err := h.serveKey(c, contentStore, pid, key, http.StatusOK)
		if served || err != nil {
			return err
		}
	}

	// 目录补斜杠：/dir → /dir/（目标 index 存在且公开）。
	if rel != "" && !strings.HasSuffix(rel, "/") && path.Ext(rel) == "" {
		dirIndex := rel + "/" + index
		if objectstore.ValidateFileKey(dirIndex) == nil {
			if _, public := systemdb.MatchPublicPath(state.Paths, dirIndex); public && h.exists(c, contentStore, pid, dirIndex) {
				target := "/" + rel + "/"
				if q := c.Request().URL.RawQuery; q != "" {
					target += "?" + q
				}
				return c.Redirect(http.StatusMovedPermanently, target)
			}
		}
	}

	// 自定义 404 文档：同样必须公开且存在。
	if doc := strings.TrimSpace(h.cfg.NotFoundDocument); doc != "" && objectstore.ValidateFileKey(doc) == nil {
		if _, public := systemdb.MatchPublicPath(state.Paths, doc); public {
			c.Request().Header.Del("Range")
			c.Request().Header.Del("If-None-Match")
			if served, err := h.serveKey(c, contentStore, pid, doc, http.StatusNotFound); served || err != nil {
				return err
			}
		}
	}
	return siteNotFound(c)
}

// exists 判定对象是否存在（只读头部后立即关闭）。
func (h *SiteHandler) exists(c echo.Context, store objectstore.ContentStore, pid, key string) bool {
	content, err := store.GetContent(c.Request().Context(), joinKey(pid, key), "")
	if err != nil {
		return false
	}
	_ = content.Body.Close()
	return true
}

// serveKey 输出对象内容。served=false 表示对象不存在（调用方继续回退）。
func (h *SiteHandler) serveKey(c echo.Context, store objectstore.ContentStore, pid, key string, status int) (bool, error) {
	rangeValue := ""
	if status == http.StatusOK {
		rangeValue = c.Request().Header.Get("Range")
	}
	content, err := store.GetContent(c.Request().Context(), joinKey(pid, key), rangeValue)
	if err != nil {
		if rangeValue != "" && strings.Contains(strings.ToLower(err.Error()), "invalid range") {
			return true, c.String(http.StatusRequestedRangeNotSatisfiable, "invalid range\n")
		}
		if errors.Is(err, objectstore.ErrNotFound) || isNotFoundText(err) {
			return false, nil
		}
		return false, nil
	}
	defer content.Body.Close()

	header := c.Response().Header()
	contentType := siteContentType(key, content.ContentType)
	header.Set(echo.HeaderContentType, contentType)
	header.Set("Accept-Ranges", "bytes")
	if strings.HasPrefix(contentType, "text/html") {
		header.Set("Cache-Control", "no-cache")
	} else {
		header.Set("Cache-Control", "public, max-age=300")
	}
	if content.ETag != "" {
		header.Set("ETag", content.ETag)
		if status == http.StatusOK && c.Request().Header.Get("If-None-Match") == content.ETag {
			return true, c.NoContent(http.StatusNotModified)
		}
	}
	if !content.LastModified.IsZero() {
		header.Set("Last-Modified", content.LastModified.UTC().Format(http.TimeFormat))
	}
	if rangeValue != "" {
		status = http.StatusPartialContent
		header.Set("Content-Range", "bytes "+strconv.FormatInt(content.RangeStart, 10)+"-"+strconv.FormatInt(content.RangeEnd, 10)+"/"+strconv.FormatInt(content.Size, 10))
	}
	header.Set(echo.HeaderContentLength, strconv.FormatInt(content.RangeEnd-content.RangeStart+1, 10))
	if c.Request().Method == http.MethodHead {
		c.Response().WriteHeader(status)
		return true, nil
	}
	return true, c.Stream(status, contentType, content.Body)
}

// isNotFoundText 兜底识别 S3 后端的 NoSuchKey / NotFound（其错误经脱敏后不再 wrap 哨兵）。
func isNotFoundText(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "nosuchkey") || strings.Contains(s, "not found") || strings.Contains(s, "notfound") || strings.Contains(s, "404")
}

// siteContentType 按扩展名推导 Content-Type（S3 模式落盘的是嗅探结果，.css/.js 会被存成 text/plain，
// 在 nosniff 下浏览器会拒绝执行）。查不到扩展名时回退存储里的类型。
func siteContentType(key, stored string) string {
	ct := mime.TypeByExtension(strings.ToLower(path.Ext(key)))
	if ct == "" {
		ct = strings.TrimSpace(stored)
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	base, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return ct
	}
	if _, has := params["charset"]; !has && (strings.HasPrefix(base, "text/") || base == "application/javascript" || base == "application/json" || base == "image/svg+xml") {
		params["charset"] = "utf-8"
	}
	return mime.FormatMediaType(base, params)
}

func siteNotFound(c echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-cache")
	if c.Request().Method == http.MethodHead {
		c.Response().Header().Set(echo.HeaderContentType, "text/html; charset=utf-8")
		c.Response().WriteHeader(http.StatusNotFound)
		return nil
	}
	return c.HTML(http.StatusNotFound, siteNotFoundHTML)
}
