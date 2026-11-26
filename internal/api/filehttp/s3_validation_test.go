package filehttp

// s3_validation_test.go：对象存储 handler 的请求校验用例（自 internal/api/handler_validation_test.go 随实现迁入）。

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

func TestS3Handler_MissingProjectAndIndex(t *testing.T) {
	h := NewS3Handler(S3HandlerDeps{Store: objectstore.NewMemoryFileStore(), Writable: true})
	e := echo.New()
	e.GET("/s3/objects", h.ListObjects)
	e.POST("/s3/objects", h.UploadObject)
	e.DELETE("/s3/objects", h.DeleteObject)
	e.GET("/s3/presign", h.PresignObject)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/s3/objects"},
		{http.MethodPost, "/s3/objects"},
		{http.MethodDelete, "/s3/objects?key=a"},
		{http.MethodGet, "/s3/presign?key=a"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Fatalf("%s expected missing project", tc.path)
		}
	}

	e2 := setupS3TestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/s3/objects?prefix=../x", nil)
	rec := httptest.NewRecorder()
	e2.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad prefix %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/s3/presign", nil)
	rec = httptest.NewRecorder()
	e2.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("presign missing key %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/s3/presign?key=../x", nil)
	rec = httptest.NewRecorder()
	e2.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("presign bad key %d", rec.Code)
	}
}
