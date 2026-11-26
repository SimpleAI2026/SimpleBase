package filehttp

// site_urls_test.go：站点访问 URL 的解析（自 internal/api/site_path_test.go 随实现迁入；路由级用例留在 api）。

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

const sitePID = "proj-1"

// siteCfg 是子域名方式的站点配置：base_domain 决定访问 URL 的形态。
func siteCfg() config.SitesConfig {
	return config.SitesConfig{Subdomain: true, BaseDomain: "simplebase.top", URLScheme: "https", IndexDocument: "index.html", NotFoundDocument: "404.html"}
}

func TestSitePathAdminURLs(t *testing.T) {
	// 管理 API 返回路径方式 URL，且三级回退：sites.public_base_url > http.public_url > 请求 Host。
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
