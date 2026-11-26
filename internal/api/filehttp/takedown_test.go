// 文件说明：takedown_test.go — 系统管理员下线站点时关闭全部公开路径的测试。
//
// 用法
//
//	go test ./internal/api/filehttp -run TestTakedownSite
//
// 调用架构
//
//	[上游] 无（同包测试）
//	[本文件] internal/api/filehttp/takedown_test.go
//	[下游] s3_site_handler.go
//
// 主要符号
//
//	fakeTakedownStore / TestTakedownSiteDisablesAllPaths / TestTakedownSiteRequiresAdmin

package filehttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type fakeTakedownStore struct {
	paths  []systemdb.PublicPath
	public map[string]bool
}

func (f *fakeTakedownStore) Lookup(context.Context, string) (SiteState, error) {
	return SiteState{Paths: f.paths}, nil
}

func (f *fakeTakedownStore) SetPublicPath(_ context.Context, _ string, path, _ string, public bool, _ string) error {
	if f.public == nil {
		f.public = map[string]bool{}
	}
	f.public[path] = public
	return nil
}

func (f *fakeTakedownStore) DeletePublicFile(context.Context, string, string) error { return nil }
func (f *fakeTakedownStore) Invalidate(string)                                      {}

func takedownContext(role auth.Role) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/system/projects/proj-1/site/takedown", nil)
	p := auth.Principal{Role: role, TenantID: "tenant-1"}
	ctx := httpkit.WithPrincipal(req.Context(), p)
	c := e.NewContext(req.WithContext(ctx), httptest.NewRecorder())
	c.SetParamNames("projectID")
	c.SetParamValues("proj-1")
	return c
}

func TestTakedownSiteDisablesAllPaths(t *testing.T) {
	store := &fakeTakedownStore{paths: []systemdb.PublicPath{
		{Path: "web/", Kind: "folder"},
		{Path: "index.html", Kind: "file"},
	}}
	h := NewS3Handler(S3HandlerDeps{Sites: store})
	rec := httptest.NewRecorder()
	c := takedownContext(auth.RoleAdmin)
	c.SetResponse(echo.NewResponse(rec, c.Echo()))

	if err := h.TakedownSite(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(store.public) != 2 || store.public["web/"] || store.public["index.html"] {
		t.Fatalf("all paths must be disabled: %+v", store.public)
	}
}

func TestTakedownSiteRequiresAdmin(t *testing.T) {
	store := &fakeTakedownStore{}
	h := NewS3Handler(S3HandlerDeps{Sites: store})
	rec := httptest.NewRecorder()
	c := takedownContext(auth.RoleUser)
	c.SetResponse(echo.NewResponse(rec, c.Echo()))

	if err := h.TakedownSite(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
