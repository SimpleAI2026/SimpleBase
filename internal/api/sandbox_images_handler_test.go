// 文件说明：sandbox_images_handler_test.go — internal/api 的单元测试（TestImagesWithoutStore、TestImagesListEmpty 等）
//
// 用法
//
//	go test ./internal/api -run 'TestImagesWithoutStore|TestImagesListEmpty|TestImageRemoveNotFound|TestImageRemoveNeedsReference'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sandbox_images_handler_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	imagesRouter(t *testing.T) (*echo.Echo, string, *image.Store)
//	TestImagesWithoutStore(t *testing.T)
//	TestImagesListEmpty(t *testing.T)
//	TestImageRemoveNotFound(t *testing.T)
//	TestImageRemoveNeedsReference(t *testing.T)
//	TestImagePullNeedsReference(t *testing.T)
//	TestImagePullBadReference(t *testing.T)
//	TestImagesListReturnsIndexed(t *testing.T)

package api

// sandbox_images_handler_test.go：镜像管理端点的形状。
//
//   1. 没有 store 时三个端点都 501（不是 404）；
//   2. 有 store 时列出/删除走 index；拉取的参数校验在最前面；
//   3. 引用非法 400，找不到 404。
//
// 真实拉取需要网络，这里不覆盖：协议路径在 gosandbox/image 的
// pull_test.go 里用本地假 registry 测过。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gosandbox/image"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// imagesRouter 起一个带镜像 store 的路由。
func imagesRouter(t *testing.T) (*echo.Echo, string, *image.Store) {
	t.Helper()
	deps, key := fullRouterDeps(t)
	store, err := image.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deps.Images = store
	// 镜像端点走 ext 组，需要 e2b 的凭据与项目中间件。
	if deps.Auth == nil {
		t.Fatal("fullRouterDeps must provide an auth service")
	}
	return NewRouter(deps), key, store
}

// 没有 store：501。
func TestImagesWithoutStore(t *testing.T) {
	deps, key := fullRouterDeps(t)
	e := NewRouter(deps)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/e2b/ext/images"},
		{http.MethodPost, "/e2b/ext/images/pull"},
		{http.MethodDelete, "/e2b/ext/images?reference=alpine"},
	} {
		rec := call(e, tc.method, tc.path, key, "", nil)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s %s: status = %d", tc.method, tc.path, rec.Code)
		}
	}
}

// 空 store：列表返回空数组，不是 null。
func TestImagesListEmpty(t *testing.T) {
	e, key, _ := imagesRouter(t)
	rec := call(e, http.MethodGet, "/e2b/ext/images", key, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"images":[]`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

// 删除不存在的镜像：404。
func TestImageRemoveNotFound(t *testing.T) {
	e, key, _ := imagesRouter(t)
	rec := call(e, http.MethodDelete, "/e2b/ext/images?reference=alpine:3.19", key, "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

// 删除缺少 reference：400。
func TestImageRemoveNeedsReference(t *testing.T) {
	e, key, _ := imagesRouter(t)
	rec := call(e, http.MethodDelete, "/e2b/ext/images", key, "", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

// 拉取缺少 reference：400（在校验通过前不该打网络）。
func TestImagePullNeedsReference(t *testing.T) {
	e, key, _ := imagesRouter(t)
	rec := call(e, http.MethodPost, "/e2b/ext/images/pull", key, `{}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

// 拉取非法引用：400（同样不打网络）。
func TestImagePullBadReference(t *testing.T) {
	e, key, _ := imagesRouter(t)
	rec := call(e, http.MethodPost, "/e2b/ext/images/pull", key, `{"reference":"Alpine"}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

// 已索引的镜像出现在列表里。
func TestImagesListReturnsIndexed(t *testing.T) {
	e, key, store := imagesRouter(t)
	img := image.Image{
		Reference:      "docker.io/library/alpine:3.19",
		ManifestDigest: "sha256:" + strings.Repeat("a", 64),
		ConfigDigest:   "sha256:" + strings.Repeat("b", 64),
		Platform:       "linux/arm64",
		Layers:         []image.Layer{{Digest: "sha256:" + strings.Repeat("c", 64), Size: 100}},
	}
	if err := store.PutImage(img); err != nil {
		t.Fatal(err)
	}
	rec := call(e, http.MethodGet, "/e2b/ext/images", key, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "alpine:3.19") || !strings.Contains(body, "linux/arm64") {
		t.Fatalf("body = %s", body)
	}
}

var (
	_ = context.Background
	_ = httptest.NewRecorder
	_ = auth.DatabaseRead
)
