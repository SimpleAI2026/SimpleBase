package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

// pathSiteCfg 是路径方式（默认）的站点配置：不需要 base_domain。
func pathSiteCfg() config.SitesConfig {
	return config.SitesConfig{IndexDocument: "index.html", NotFoundDocument: "404.html"}
}

func pathSiteRouter(t *testing.T, mutate func(*Dependencies)) *echo.Echo {
	t.Helper()
	e, _, _ := siteRouter(t, func(d *Dependencies) {
		d.Config.Sites = pathSiteCfg()
		if mutate != nil {
			mutate(d)
		}
	})
	return e
}

func TestSitePathServeRules(t *testing.T) {
	e := pathSiteRouter(t, nil)
	base := "/sites/" + sitePID
	cases := []struct {
		name, path string
		status     int
		body       string
	}{
		{"root index", base + "/", 200, "root"},
		{"folder index", base + "/blog/", 200, "blog"},
		{"css", base + "/blog/app.css", 200, "body{}"},
		{"prefix boundary", base + "/blog2/secret.html", 404, "custom 404"},
		{"not public", base + "/private.txt", 404, "custom 404"},
		{"traversal encoded", base + "/blog/%2e%2e/private.txt", 404, ""},
		{"backslash", base + "/blog/..%5cprivate.txt", 404, ""},
	}
	for _, tc := range cases {
		rec := siteGet(e, http.MethodGet, "api.example.com", tc.path, nil)
		if rec.Code != tc.status {
			t.Errorf("%s: status=%d body=%s", tc.name, rec.Code, rec.Body.String())
			continue
		}
		if tc.body != "" && !strings.Contains(rec.Body.String(), tc.body) {
			t.Errorf("%s: body=%q", tc.name, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "private") || strings.Contains(rec.Body.String(), "secret") {
			t.Errorf("%s: leaked private content", tc.name)
		}
	}
	// 目录补斜杠带挂载前缀与 query。
	rec := siteGet(e, http.MethodGet, "api.example.com", base+"/blog?x=1", nil)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != base+"/blog/?x=1" {
		t.Fatalf("dir redirect: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// /sites/<pid> → /sites/<pid>/
	rec = siteGet(e, http.MethodGet, "api.example.com", base+"?a=b", nil)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != base+"/?a=b" {
		t.Fatalf("pid redirect: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// HEAD / Range
	rec = siteGet(e, http.MethodHead, "api.example.com", base+"/blog/app.js", nil)
	if rec.Code != 200 || rec.Body.Len() != 0 {
		t.Fatalf("HEAD: %d len=%d", rec.Code, rec.Body.Len())
	}
	rec = siteGet(e, http.MethodGet, "api.example.com", base+"/blog/app.js", map[string]string{"Range": "bytes=0-6"})
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "console" {
		t.Fatalf("range: %d %q", rec.Code, rec.Body.String())
	}
	// 路径模式的 CSP 必须含 sandbox（planv6.7 §1.4）。
	rec = siteGet(e, http.MethodGet, "api.example.com", base+"/", nil)
	csp := rec.Header().Get("Content-Security-Policy")
	xfo := rec.Header().Get("X-Frame-Options")
	if !strings.Contains(csp, "sandbox ") || strings.Contains(csp, "allow-same-origin") || xfo != "DENY" {
		t.Fatalf("path CSP: %q / X-Frame-Options: %q", csp, xfo)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("nosniff: %q", rec.Header().Get("X-Content-Type-Options"))
	}
}

func TestSitePathDispatchBoundaries(t *testing.T) {
	e := pathSiteRouter(t, nil)
	base := "/sites/" + sitePID
	// 非 GET/HEAD 不进入站点面，落到原有路由（不得返回站点内容）。
	rec := siteGet(e, http.MethodPost, "api.example.com", base+"/blog/app.js", nil)
	if strings.Contains(rec.Body.String(), "console.log") {
		t.Fatalf("POST must not reach site: %d", rec.Code)
	}
	// 非法 pid（大写 / 过短 / 含点）一律 404，且不回落 SPA。
	for _, bad := range []string{"/sites/ABCD1234/index.html", "/sites/x/index.html", "/sites/a.b/index.html", "/sites//index.html"} {
		rec := siteGet(e, http.MethodGet, "api.example.com", bad, nil)
		if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), `id="app"`) {
			t.Errorf("%s: %d %q", bad, rec.Code, rec.Body.String())
		}
	}
	// 未发布项目 404；系统项目 404。
	for _, pid := range []string{"prj99999", "sb-admin"} {
		rec := siteGet(e, http.MethodGet, "api.example.com", "/sites/"+pid+"/index.html", nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d", pid, rec.Code)
		}
	}
	// 直接访问 /__site 仍 404；/v1 不受影响。
	if rec := siteGet(e, http.MethodGet, "api.example.com", "/__site/"+sitePID+"/index.html", nil); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "root") {
		t.Fatalf("direct /__site: %d", rec.Code)
	}
	if rec := siteGet(e, http.MethodGet, "api.example.com", "/v1/projects/proj-1/databases", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("v1: %d", rec.Code)
	}
	// 子域名 Host 在路径模式下不被识别。
	if rec := siteGet(e, http.MethodGet, sitePID+".simplebase.top", "/index.html", nil); strings.Contains(rec.Body.String(), "root") {
		t.Fatal("path mode must not honor subdomain host")
	}
}

func TestSitePathCustomPrefix(t *testing.T) {
	e := pathSiteRouter(t, func(d *Dependencies) { d.Config.Sites.PathPrefix = "/pub" })
	if rec := siteGet(e, http.MethodGet, "h", "/pub/"+sitePID+"/", nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), "root") {
		t.Fatalf("custom prefix: %d", rec.Code)
	}
	// 旧前缀不再命中站点面。
	if rec := siteGet(e, http.MethodGet, "h", "/sites/"+sitePID+"/", nil); strings.Contains(rec.Body.String(), "root") {
		t.Fatal("old prefix must not serve")
	}
}

func TestSiteSubdomainModeIgnoresPath(t *testing.T) {
	e, _, _ := siteRouter(t, nil)
	rec := siteGet(e, http.MethodGet, "api.example.com", "/sites/"+sitePID+"/", nil)
	if strings.Contains(rec.Body.String(), "root") {
		t.Fatal("subdomain mode must not serve path style")
	}
}

func TestSitePathAdminURLs(t *testing.T) {
	// 管理 API 返回路径方式 URL，且三级回退：sites.public_base_url > http.public_url > 请求 Host。
	e, _ := siteAdminRouter(t, nil, true, sitePID, true)
	_ = e
	cases := []struct {
		name, override, httpURL, want string
	}{
		{"override wins", "https://o.example.com/", "https://h.example.com", "https://o.example.com/sites/" + sitePID},
		{"http public_url", "", "https://h.example.com/", "https://h.example.com/sites/" + sitePID},
		{"request host", "", "", "http://example.com/sites/" + sitePID},
	}
	for _, tc := range cases {
		h := &S3Handler{sitesCfg: config.SitesConfig{PublicBaseURL: tc.override}, cfgHTTPPublicURL: tc.httpURL}
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
		if got := h.siteBase(c, sitePID); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
	// 子域名方式沿用 <pid>.<base_domain>。
	h := &S3Handler{sitesCfg: siteCfg()}
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	if got := h.siteBase(c, sitePID); got != "https://"+sitePID+".simplebase.top" {
		t.Errorf("subdomain base: %q", got)
	}
}

func TestPathSiteCSPFor(t *testing.T) {
	cases := []struct {
		name, custom string
		must         []string
		mustNot      []string
	}{
		{"default", "", []string{"sandbox allow-scripts", "frame-ancestors 'none'"}, []string{"allow-same-origin"}},
		{"off fallback", "off", []string{"sandbox "}, []string{"allow-same-origin"}},
		{"custom without sandbox", "default-src 'self'", []string{"sandbox ", "default-src 'self'"}, []string{"allow-same-origin"}},
		{"custom strips same-origin", "sandbox allow-scripts allow-same-origin; default-src 'self'", []string{"sandbox allow-scripts", "default-src 'self'"}, []string{"allow-same-origin"}},
		{"bare sandbox kept", "sandbox; img-src 'self'", []string{"sandbox;", "img-src 'self'"}, []string{"allow-same-origin"}},
	}
	for _, tc := range cases {
		cfg := config.Config{Sites: config.SitesConfig{CSP: tc.custom}}
		got := siteCSPFor(cfg)
		for _, m := range tc.must {
			if !strings.Contains(got, m) {
				t.Errorf("%s: %q missing %q", tc.name, got, m)
			}
		}
		for _, m := range tc.mustNot {
			if strings.Contains(got, m) {
				t.Errorf("%s: %q must not contain %q", tc.name, got, m)
			}
		}
	}
	// 子域名方式不受影响：默认值与 off 语义不变。
	if got := siteCSPFor(config.Config{Sites: config.SitesConfig{Subdomain: true}}); got != defaultSiteCSP {
		t.Errorf("subdomain default changed: %q", got)
	}
	if got := siteCSPFor(config.Config{Sites: config.SitesConfig{Subdomain: true, CSP: "off"}}); got != "" {
		t.Errorf("subdomain off: %q", got)
	}
}
