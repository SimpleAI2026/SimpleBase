// 文件说明：site_traffic_test.go — 站点配额拦截与出口字节统计测试。
//
// 用法
//
//	go test ./internal/api/filehttp -run TestSiteTraffic
//
// 调用架构
//
//	[上游] 无（同包测试）
//	[本文件] internal/api/filehttp/site_traffic_test.go
//	[下游] site_handler.go
//
// 主要符号
//
//	fakeSiteTraffic / TestSiteTrafficRecordsBytes / TestSiteTrafficRejectsOverQuota

package filehttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type fakeSiteTraffic struct {
	allowErr    error
	allowed     int
	recorded    int
	bytes       int64
	maxRequests int64
	maxEgress   int64
}

func (f *fakeSiteTraffic) AllowSite(context.Context, string) error {
	if f.allowErr != nil {
		return f.allowErr
	}
	f.allowed++
	return nil
}

func (f *fakeSiteTraffic) RecordSite(_ context.Context, _ string, bytes int64) error {
	f.recorded++
	f.bytes += bytes
	return nil
}

func (f *fakeSiteTraffic) SetSiteQuota(_ context.Context, _ string, maxRequests, maxEgress int64) error {
	f.maxRequests = maxRequests
	f.maxEgress = maxEgress
	return nil
}

func siteTrafficContext() echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	c.Set(siteProjectCtxKey, SiteRequest{ProjectID: "p1"})
	return c
}

func TestSiteTrafficRecordsBytes(t *testing.T) {
	store := objectstore.NewMemoryFileStore()
	if _, err := store.Put(context.Background(), JoinKey("p1", "index.html"), strings.NewReader("hello"), 5, "text/html"); err != nil {
		t.Fatal(err)
	}
	traffic := &fakeSiteTraffic{}
	h := NewSiteHandler(store, &fakeTakedownStore{paths: []systemdb.PublicPath{{Path: "index.html", Kind: "file"}}}, config.SitesConfig{}, traffic)
	c := siteTrafficContext()
	if err := h.Serve(c); err != nil {
		t.Fatal(err)
	}
	if c.Response().Status != http.StatusOK {
		t.Fatalf("status=%d", c.Response().Status)
	}
	if traffic.allowed != 1 || traffic.recorded != 1 || traffic.bytes != 5 {
		t.Fatalf("traffic=%+v", traffic)
	}
}

func TestPutSiteQuota(t *testing.T) {
	traffic := &fakeSiteTraffic{}
	h := NewS3Handler(S3HandlerDeps{SiteTraffic: traffic})
	c := takedownContext(auth.RoleAdmin)
	c.SetRequest(httptest.NewRequest(http.MethodPut, "/v1/system/projects/p1/site/quota", strings.NewReader(`{"max_requests":10,"max_egress_bytes":20}`)))
	c.Request().Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	ctx := httpkit.WithPrincipal(c.Request().Context(), auth.Principal{Role: auth.RoleAdmin, TenantID: "t1"})
	c.SetRequest(c.Request().WithContext(ctx))
	rec := httptest.NewRecorder()
	c.SetResponse(echo.NewResponse(rec, c.Echo()))
	if err := h.PutSiteQuota(c); err != nil {
		t.Fatal(err)
	}
	if traffic.maxRequests != 10 || traffic.maxEgress != 20 {
		t.Fatalf("quota not set: %+v status=%d body=%s", traffic, rec.Code, rec.Body.String())
	}
}

func TestSiteTrafficRejectsOverQuota(t *testing.T) {
	traffic := &fakeSiteTraffic{allowErr: errors.New("quota exceeded")}
	h := NewSiteHandler(nil, &fakeTakedownStore{}, config.SitesConfig{}, traffic)
	c := siteTrafficContext()
	if err := h.Serve(c); err != nil {
		t.Fatal(err)
	}
	if c.Response().Status != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", c.Response().Status, c.Response().Writer.(*httptest.ResponseRecorder).Body.String())
	}
	if traffic.recorded != 0 {
		t.Fatalf("rejected request must not record bytes: %+v", traffic)
	}
}
