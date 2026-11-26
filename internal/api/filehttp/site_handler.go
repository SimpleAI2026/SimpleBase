package filehttp

import (
	"context"
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

// SiteTraffic 是站点配额与出口统计端口（由 usage.Service 实现）。
type SiteTraffic interface {
	AllowSite(ctx context.Context, projectID string) error
	RecordSite(ctx context.Context, projectID string, bytes int64) error
	SetSiteQuota(ctx context.Context, projectID string, maxRequests, maxEgressBytes int64) error
}

// SiteHandler 提供站点的匿名只读访问（子域名 <pid>.<base_domain> 或路径 <prefix>/<pid>/ 两种方式共用）。
type SiteHandler struct {
	store   objectstore.FileStore
	sites   SiteStore
	cfg     config.SitesConfig
	traffic SiteTraffic
}

// NewSiteHandler 构造匿名站点读取 handler（站点分流由 api.NewRouter 的 e.Pre 注入）。
func NewSiteHandler(store objectstore.FileStore, sites SiteStore, cfg config.SitesConfig, traffic SiteTraffic) *SiteHandler {
	return &SiteHandler{store: store, sites: sites, cfg: cfg, traffic: traffic}
}

// countingWriter 统计站点响应实际写出的字节数（HEAD 为 0）。
type countingWriter struct {
	http.ResponseWriter
	bytes int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

const siteNotFoundHTML = "<!doctype html><html><head><meta charset=\"utf-8\"><title>404 Not Found</title></head>" +
	"<body><h1>404 Not Found</h1></body></html>\n"

// Serve 处理站点请求。
func (h *SiteHandler) Serve(c echo.Context) error {
	sr, _ := RequestFrom(c)
	pid := sr.ProjectID
	if pid == "" || systemdb.IsAdminProject(pid) || h.sites == nil {
		return siteNotFound(c)
	}
	method := c.Request().Method
	if method != http.MethodGet && method != http.MethodHead {
		c.Response().Header().Set("Allow", "GET, HEAD")
		return c.String(http.StatusMethodNotAllowed, "method not allowed\n")
	}
	if h.traffic != nil {
		if err := h.traffic.AllowSite(c.Request().Context(), pid); err != nil {
			c.Response().Header().Set("Retry-After", "60")
			return c.String(http.StatusTooManyRequests, "site quota exceeded\n")
		}
		counter := &countingWriter{ResponseWriter: c.Response().Writer}
		c.Response().Writer = counter
		defer func() { _ = h.traffic.RecordSite(c.Request().Context(), pid, counter.bytes) }()
	}
	contentStore, ok := h.store.(objectstore.ContentStore)
	if !ok {
		return siteNotFound(c)
	}
	state, err := h.sites.Lookup(c.Request().Context(), pid)
	if err != nil {
		return siteNotFound(c)
	}

	reqPath := strings.TrimPrefix(c.Request().URL.Path, PathPrefix)
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
				target := sr.Mount + "/" + rel + "/"
				if q := c.Request().URL.RawQuery; q != "" {
					target += "?" + q
				}
				return c.Redirect(http.StatusMovedPermanently, target)
			}
		}
	}

	// SPA 回退：无扩展名的未命中路径返回站点根 index（只对页面路由生效，静态资源仍 404）。
	if h.cfg.SpaFallback && rel != "" && path.Ext(rel) == "" {
		if objectstore.ValidateFileKey(index) == nil {
			if _, public := systemdb.MatchPublicPath(state.Paths, index); public {
				c.Request().Header.Del("Range")
				c.Request().Header.Del("If-None-Match")
				if served, err := h.serveKey(c, contentStore, pid, index, http.StatusOK); served || err != nil {
					return err
				}
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
	content, err := store.GetContent(c.Request().Context(), JoinKey(pid, key), "")
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
	content, err := store.GetContent(c.Request().Context(), JoinKey(pid, key), rangeValue)
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
	contentType := SiteContentType(key, content.ContentType)
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
func SiteContentType(key, stored string) string {
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
