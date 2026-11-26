// 文件说明：site_handler_test.go — internal/api 的单元测试（TestSiteServeRules、TestSiteHostDispatchBoundaries 等）
//
// 用法
//
//	go test ./internal/api -run 'TestSiteServeRules|TestSiteHostDispatchBoundaries|TestSiteWithoutDomainNotServed|TestSiteContentType'
//	同包调用：siteCfg() → site_path_test.go；siteRouter() → site_path_test.go；siteGet() → site_path_test.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/site_handler_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//	[关键引用] siteCfg ← site_path_test.go；siteRouter ← site_path_test.go；siteGet ← site_path_test.go
//
// 主要符号
//
//	const sitePID
//	timeZero() time.Time
//	siteCfg() config.SitesConfig
//	siteRouter(t *testing.T, mutate func(*Dependencies)) (*echo.Echo, Dependencies, objectstore.FileStore)
//	siteGet(e *echo.Echo, method, host, path string, hdr map[string]string) *httptest.ResponseRecorder
//	TestSiteServeRules(t *testing.T)
//	TestSiteHostDispatchBoundaries(t *testing.T)
//	TestSiteWithoutDomainNotServed(t *testing.T)
//	... 另有 6 个顶层符号

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/linkxzhou/SimpleBase/internal/api/filehttp"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

const sitePID = "prj00001"

func timeZero() time.Time { return time.Time{} }

func siteCfg() config.SitesConfig {
	return config.SitesConfig{Subdomain: true, BaseDomain: "simplebase.top", URLScheme: "https", IndexDocument: "index.html", NotFoundDocument: "404.html"}
}

// siteRouter 构造带站点分流的完整路由（项目 sitePID 已发布，seed 中的路径已公开）。
func siteRouter(t *testing.T, mutate func(*Dependencies)) (*echo.Echo, Dependencies, objectstore.FileStore) {
	t.Helper()
	deps, _ := fullRouterDeps(t)
	deps.Config.Sites = siteCfg()
	deps.Sites = filehttp.NewSiteStore(deps.System, 0)
	if mutate != nil {
		mutate(&deps)
	}
	store := deps.S3FileStore
	ctx := context.Background()
	put := func(key, ct, body string) {
		if _, err := store.Put(ctx, filehttp.JoinKey(sitePID, key), strings.NewReader(body), int64(len(body)), ct); err != nil {
			t.Fatal(err)
		}
	}
	put("index.html", "text/html; charset=utf-8", "<h1>root</h1>")
	put("blog/index.html", "text/html; charset=utf-8", "<h1>blog</h1>")
	put("blog/app.css", "text/plain; charset=utf-8", "body{}")
	put("blog/app.js", "text/plain; charset=utf-8", "console.log(1)")
	put("blog2/secret.html", "text/html", "secret")
	put("private.txt", "text/plain", "private")
	put("404.html", "text/html", "<h1>custom 404</h1>")
	sys := deps.System
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(sys.SetPublicPath(ctx, sitePID, "blog/", systemdb.PublicPathFolder, true, "u"))
	must(sys.SetPublicPath(ctx, sitePID, "index.html", systemdb.PublicPathFile, true, "u"))
	must(sys.SetPublicPath(ctx, sitePID, "404.html", systemdb.PublicPathFile, true, "u"))
	return NewRouter(deps), deps, store
}

func siteGet(e *echo.Echo, method, host, path string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Host = host
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestSiteServeRules(t *testing.T) {
	e, _, _ := siteRouter(t, nil)
	host := sitePID + ".simplebase.top"
	cases := []struct {
		name, path string
		status     int
		ct, body   string
	}{
		{"root index", "/", 200, "text/html", "root"},
		{"folder index", "/blog/", 200, "text/html", "blog"},
		{"css by ext", "/blog/app.css", 200, "text/css", "body{}"},
		{"js by ext", "/blog/app.js", 200, "javascript", "console.log"},
		{"prefix boundary", "/blog2/secret.html", 404, "", "custom 404"},
		{"not public", "/private.txt", 404, "", "custom 404"},
		{"missing", "/blog/nope.html", 404, "", "custom 404"},
		{"traversal encoded", "/blog/%2e%2e/private.txt", 404, "", ""},
		{"backslash", "/blog/..%5cprivate.txt", 404, "", ""},
	}
	for _, tc := range cases {
		rec := siteGet(e, http.MethodGet, host, tc.path, nil)
		if rec.Code != tc.status {
			t.Errorf("%s: status=%d body=%s", tc.name, rec.Code, rec.Body.String())
			continue
		}
		if tc.ct != "" && !strings.Contains(rec.Header().Get(echo.HeaderContentType), tc.ct) {
			t.Errorf("%s: content-type=%q", tc.name, rec.Header().Get(echo.HeaderContentType))
		}
		if tc.body != "" && !strings.Contains(rec.Body.String(), tc.body) {
			t.Errorf("%s: body=%q", tc.name, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "private") || strings.Contains(rec.Body.String(), "secret") {
			t.Errorf("%s: leaked private content", tc.name)
		}
		if rec.Header().Get("Content-Disposition") != "" {
			t.Errorf("%s: site must not send Content-Disposition", tc.name)
		}
	}

	// 目录补斜杠
	rec := siteGet(e, http.MethodGet, host, "/blog?x=1", nil)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/blog/?x=1" {
		t.Fatalf("redirect: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// HEAD / Range / ETag
	rec = siteGet(e, http.MethodHead, host, "/blog/app.js", nil)
	if rec.Code != 200 || rec.Body.Len() != 0 {
		t.Fatalf("HEAD: %d len=%d", rec.Code, rec.Body.Len())
	}
	rec = siteGet(e, http.MethodGet, host, "/blog/app.js", map[string]string{"Range": "bytes=0-6"})
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "console" {
		t.Fatalf("range: %d %q", rec.Code, rec.Body.String())
	}
	// 非 GET/HEAD
	rec = siteGet(e, http.MethodPost, host, "/blog/app.js", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rec.Code)
	}
	// 安全头：site 类
	rec = siteGet(e, http.MethodGet, host, "/", nil)
	if rec.Header().Get("X-Frame-Options") != "SAMEORIGIN" || rec.Header().Get("Content-Security-Policy") != defaultSiteCSP {
		t.Fatalf("site headers: %#v", rec.Header())
	}
	if rec.Header().Get("Cross-Origin-Opener-Policy") != "" || rec.Header().Get("Cross-Origin-Resource-Policy") != "" {
		t.Fatalf("site must not send COOP/CORP: %#v", rec.Header())
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("nosniff missing")
	}
}

func TestSiteSpaFallback(t *testing.T) {
	e, _, _ := siteRouter(t, func(d *Dependencies) { d.Config.Sites.SpaFallback = true })
	host := sitePID + ".simplebase.top"

	rec := siteGet(e, http.MethodGet, host, "/orders/42", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<h1>root</h1>") {
		t.Fatalf("spa fallback: status=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = siteGet(e, http.MethodGet, host, "/assets/missing.js", nil)
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "<h1>root</h1>") {
		t.Fatalf("assets must not fall back: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSiteHostDispatchBoundaries(t *testing.T) {
	e, deps, _ := siteRouter(t, nil)
	// 非站点 Host：走原有路由（未认证 /v1 → 401；/__site 直接访问 → 404）。
	if rec := siteGet(e, http.MethodGet, "console.example.com", "/v1/projects/proj-1/databases", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("console api: %d", rec.Code)
	}
	if rec := siteGet(e, http.MethodGet, "console.example.com", "/__site/index.html", nil); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "root") {
		t.Fatalf("direct /__site must 404: %d %s", rec.Code, rec.Body.String())
	}
	// 站点 Host 上 /v1 不可达（被改写到站点面）。
	if rec := siteGet(e, http.MethodGet, sitePID+".simplebase.top", "/v1/projects/proj-1/databases", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("site host /v1: %d", rec.Code)
	}
	// 大写 Host 与端口
	if rec := siteGet(e, http.MethodGet, "PRJ00001.SimpleBase.top:443", "/", nil); rec.Code != 200 {
		t.Fatalf("upper host: %d", rec.Code)
	}
	// 多级子域 / 裸域 / 非 8 位
	for _, h := range []string{"a." + sitePID + ".simplebase.top", "simplebase.top", "short.simplebase.top", "-prj0001.simplebase.top"} {
		rec := siteGet(e, http.MethodGet, h, "/index.html", nil)
		if strings.Contains(rec.Body.String(), "root") {
			t.Fatalf("host %q must not serve site", h)
		}
	}
	// 撤销全部公开路径 → 404
	if err := deps.System.DeletePublicPathsUnder(context.Background(), sitePID, ""); err != nil {
		t.Fatal(err)
	}
	deps.Sites.Invalidate(sitePID)
	if rec := siteGet(e, http.MethodGet, sitePID+".simplebase.top", "/", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unpublished site: %d", rec.Code)
	}
	// 系统项目永不服务
	if rec := siteGet(e, http.MethodGet, catalog.ReservedSystemProjectID+".simplebase.top", "/", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("admin project: %d", rec.Code)
	}
}

func TestSiteWithoutDomainNotServed(t *testing.T) {
	e, _, _ := siteRouter(t, func(d *Dependencies) { d.Config.Sites.BaseDomain = "" })
	rec := siteGet(e, http.MethodGet, sitePID+".simplebase.top", "/index.html", nil)
	if strings.Contains(rec.Body.String(), "root") {
		t.Fatal("unconfigured site domain must not serve")
	}
}

func TestSiteContentType(t *testing.T) {
	cases := map[string][2]string{
		"a.css":       {"text/plain", "text/css; charset=utf-8"},
		"a.html":      {"", "text/html; charset=utf-8"},
		"a.zzunknown": {"application/x-custom", "application/x-custom"},
		"noext":       {"", "application/octet-stream"},
	}
	for key, tc := range cases {
		if got := filehttp.SiteContentType(key, tc[0]); got != tc[1] {
			t.Errorf("%s: got %q want %q", key, got, tc[1])
		}
	}
}

// --- 管理 API ---

func siteAdminRouter(t *testing.T, perms []auth.Permission, writable bool, pid string, siteDomain bool) (*echo.Echo, *systemdb.Store) {
	t.Helper()
	deps, _ := fullRouterDeps(t)
	store := deps.System
	cfg := siteCfg()
	if !siteDomain {
		cfg.BaseDomain = ""
	}
	w := writable
	h := filehttp.NewS3Handler(filehttp.S3HandlerDeps{Store: objectstore.NewMemoryFileStore(), Index: store, Writable: w, Sites: filehttp.NewSiteStore(store, 0), SitesCfg: cfg})
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			pm := map[auth.Permission]struct{}{}
			for _, p := range perms {
				pm[p] = struct{}{}
			}
			ctx := WithPrincipal(c.Request().Context(), auth.Principal{UserID: "u1", Permissions: pm})
			ctx = WithProject(ctx, ProjectContext{ID: pid})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	req := func(perm auth.Permission) echo.MiddlewareFunc { return auth.Require(perm, PrincipalFromContext) }
	e.GET("/s3/site", h.GetSite, req(auth.DatabaseRead))
	e.GET("/s3/public", h.ListPublic, req(auth.DatabaseRead))
	e.PUT("/s3/public", h.PutPublic, req(auth.DatabaseAdmin))
	e.GET("/s3/objects", h.ListObjects, req(auth.DatabaseRead))
	e.DELETE("/s3/objects", h.DeleteObject, req(auth.DatabaseWrite))
	return e, store
}

func jsonReq(e *echo.Echo, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestSiteAdminAPI(t *testing.T) {
	admin := []auth.Permission{auth.DatabaseAdmin}
	e, store := siteAdminRouter(t, admin, true, sitePID, true)

	rec := jsonReq(e, http.MethodGet, "/s3/site", "")
	var info filehttp.SiteInfoDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &info)
	if rec.Code != 200 || !info.Publishable || info.BaseURL != "https://"+sitePID+".simplebase.top" {
		t.Fatalf("initial site: %d %+v", rec.Code, info)
	}
	rec = jsonReq(e, http.MethodPut, "/s3/public", `{"key":"blog/","kind":"folder","public":true}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"publicUrl":"https://`+sitePID+`.simplebase.top/blog/"`) {
		t.Fatalf("set folder: %d %s", rec.Code, rec.Body.String())
	}
	rec = jsonReq(e, http.MethodPut, "/s3/public", `{"key":"docs/a b#1.html","kind":"file","public":true}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "/docs/a%20b%231.html") {
		t.Fatalf("set file escaped: %d %s", rec.Code, rec.Body.String())
	}
	for _, bad := range []string{
		`{"key":"blog","kind":"folder","public":true}`,
		`{"key":"a/","kind":"file","public":true}`,
		`{"key":"../x","kind":"file","public":true}`,
		`{"key":"x","kind":"dir","public":true}`,
		`{"key":"x","kind":"file"}`,
	} {
		if rec := jsonReq(e, http.MethodPut, "/s3/public", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400 got %d", bad, rec.Code)
		}
	}
	rec = jsonReq(e, http.MethodGet, "/s3/public", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"path":"blog/"`) {
		t.Fatalf("list public: %s", rec.Body.String())
	}
	// 审计只含哈希前缀，不含明文 key
	if err := store.FlushAudits(context.Background()); err != nil {
		t.Fatal(err)
	}
	evs, err := store.QueryAudits(context.Background(), systemdb.AuditQuery{ProjectID: sitePID, Limit: 50})
	if err != nil || len(evs) < 2 {
		t.Fatalf("audits=%v err=%v", evs, err)
	}
	for _, ev := range evs {
		if strings.Contains(ev.Detail, "blog") || strings.Contains(ev.Detail, "docs") {
			t.Fatalf("audit leaked key: %+v", ev)
		}
	}
	// 取消
	rec = jsonReq(e, http.MethodPut, "/s3/public", `{"key":"blog/","kind":"folder","public":false}`)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), `"public":true`) {
		t.Fatalf("unset: %s", rec.Body.String())
	}
}

func TestSiteAdminAPIGuards(t *testing.T) {
	// 写权限不足
	e, _ := siteAdminRouter(t, []auth.Permission{auth.DatabaseRead, auth.DatabaseWrite}, true, sitePID, true)
	if rec := jsonReq(e, http.MethodPut, "/s3/public", `{"key":"a","kind":"file","public":true}`); rec.Code != http.StatusForbidden {
		t.Fatalf("write perm must not publish: %d", rec.Code)
	}
	if rec := jsonReq(e, http.MethodGet, "/s3/site", ""); rec.Code != 200 {
		t.Fatalf("read site: %d", rec.Code)
	}
	// 只读实例
	e, _ = siteAdminRouter(t, []auth.Permission{auth.DatabaseAdmin}, false, sitePID, true)
	if rec := jsonReq(e, http.MethodPut, "/s3/public", `{"key":"a","kind":"file","public":true}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readonly public: %d", rec.Code)
	}
	// 系统项目拒绝
	e, _ = siteAdminRouter(t, []auth.Permission{auth.DatabaseAdmin}, true, catalog.ReservedSystemProjectID, true)
	if rec := jsonReq(e, http.MethodPut, "/s3/public", `{"key":"a","kind":"file","public":true}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "site_not_publishable") {
		t.Fatalf("system project: %d %s", rec.Code, rec.Body.String())
	}
	rec := jsonReq(e, http.MethodGet, "/s3/site", "")
	if !strings.Contains(rec.Body.String(), `"reason":"system_project"`) {
		t.Fatalf("system reason: %s", rec.Body.String())
	}
	// 含大写的项目 ID
	e, _ = siteAdminRouter(t, []auth.Permission{auth.DatabaseAdmin}, true, "PRJ00001", true)
	if rec := jsonReq(e, http.MethodPut, "/s3/public", `{"key":"a","kind":"file","public":true}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("upper id: %d", rec.Code)
	}
	// 未配置站点域名
	e, _ = siteAdminRouter(t, []auth.Permission{auth.DatabaseAdmin}, true, sitePID, false)
	rec = jsonReq(e, http.MethodGet, "/s3/site", "")
	if !strings.Contains(rec.Body.String(), `"publishable":false`) || !strings.Contains(rec.Body.String(), filehttp.SiteReasonNoDomain) {
		t.Fatalf("no domain cfg: %s", rec.Body.String())
	}
	if rec := jsonReq(e, http.MethodPut, "/s3/public", `{"key":"a","kind":"file","public":true}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("no domain cfg put: %d", rec.Code)
	}
}

func TestSiteListDecorationAndDeleteCleanup(t *testing.T) {
	e, store := siteAdminRouter(t, []auth.Permission{auth.DatabaseAdmin}, true, sitePID, true)
	ctx := context.Background()
	for _, k := range []string{"blog/index.html", "blog/a.css", "top.html", "other/x.txt"} {
		if err := store.UpsertS3Object(ctx, sitePID, k, 1, "", "text/plain", timeZero()); err != nil {
			t.Fatal(err)
		}
	}
	_ = store.SetPublicPath(ctx, sitePID, "blog/", systemdb.PublicPathFolder, true, "")
	_ = store.SetPublicPath(ctx, sitePID, "top.html", systemdb.PublicPathFile, true, "")
	rec := jsonReq(e, http.MethodGet, "/s3/objects?delimiter=/", "")
	var out struct {
		Folders    []string                   `json:"folders"`
		Objects    []filehttp.S3ObjectDTO     `json:"objects"`
		FolderMeta []filehttp.S3FolderMetaDTO `json:"folderMeta"`
		Site       filehttp.SiteInfoDTO       `json:"site"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Folders) != 2 || out.Folders[0] != "blog/" || !out.Site.Publishable {
		t.Fatalf("folders=%v site=%+v", out.Folders, out.Site)
	}
	if !out.FolderMeta[0].Public || out.FolderMeta[0].PublicInherited != "" || out.FolderMeta[1].Public {
		t.Fatalf("folderMeta=%+v", out.FolderMeta)
	}
	if len(out.Objects) != 1 || !out.Objects[0].Public || out.Objects[0].PublicURL != "https://"+sitePID+".simplebase.top/top.html" {
		t.Fatalf("objects=%+v", out.Objects)
	}
	rec = jsonReq(e, http.MethodGet, "/s3/objects?prefix=blog/&delimiter=/", "")
	if !strings.Contains(rec.Body.String(), `"publicInherited":"blog/"`) {
		t.Fatalf("inherited: %s", rec.Body.String())
	}
	// 删除对象清理 file 行
	jsonReq(e, http.MethodDelete, "/s3/objects?key=top.html", "")
	paths, _ := store.ListPublicPaths(ctx, sitePID)
	for _, p := range paths {
		if p.Path == "top.html" {
			t.Fatal("public file row must be removed on delete")
		}
	}
	if len(paths) != 1 {
		t.Fatalf("folder row must stay: %+v", paths)
	}
}
