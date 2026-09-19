package api

import (
	"bytes"
	"context"
	"database/sql"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	_ "github.com/uglyer/go-sqlite3"
)

func setupS3IndexRouter(t *testing.T) (*echo.Echo, *systemdb.Store) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.ApplySystemMigrations(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := systemdb.NewStoreForTest(db)
	e := echo.New()
	e.HideBanner = true
	h := &S3Handler{store: objectstore.NewMemoryFileStore(), index: store}
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithPrincipal(c.Request().Context(), auth.Principal{APIKeyID: "k"})
			ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "t"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.GET("/s3/objects", h.ListObjects)
	e.POST("/s3/objects", h.UploadObject)
	e.DELETE("/s3/objects", h.DeleteObject)
	return e, store
}

func TestS3_IndexAndRefresh(t *testing.T) {
	e, store := setupS3IndexRouter(t)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("key", "docs/a.txt")
	fw, _ := mw.CreateFormFile("file", "a.txt")
	_, _ = fw.Write([]byte("hello"))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/s3/objects", body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/s3/objects?prefix=docs", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "docs/a.txt") {
		t.Fatalf("index list %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/s3/objects?refresh=1", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/s3/objects?refresh=true", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh true %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodDelete, "/s3/objects?key=docs/a.txt", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.String())
	}

	// index list error (closed db)
	_ = store
	bad := echo.New()
	h := &S3Handler{store: objectstore.NewMemoryFileStore(), index: systemdb.NewStoreForTest(nil)}
	bad.GET("/s3/objects", h.ListObjects)
	req = httptest.NewRequest(http.MethodGet, "/s3/objects", nil)
	req = req.WithContext(WithProject(req.Context(), ProjectContext{ID: "p"}))
	rec = httptest.NewRecorder()
	c := bad.NewContext(req, rec)
	_ = h.ListObjects(c)
	if rec.Code == http.StatusOK {
		t.Fatal("nil db index list should fail")
	}

	// lastModified formatting
	dto := toDTO(objectstore.FileObject{Key: "p/k", Size: 1, LastModified: time.Unix(1, 0).UTC()}, "p")
	if dto.LastModified == "" || dto.Key != "k" {
		t.Fatalf("%+v", dto)
	}
}

func TestS3_UploadMissingFileAndDeleteInvalid(t *testing.T) {
	e := setupS3TestRouter(t)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("key", "ok.txt")
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/s3/objects", body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing file %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodDelete, "/v1/projects/proj-1/s3/objects?key=../x", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad delete key %d", rec.Code)
	}
}
