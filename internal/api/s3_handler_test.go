package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// setupS3TestRouter 构造带 S3Handler 的测试路由。
func setupS3TestRouter(t *testing.T) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true

	h := &S3Handler{store: objectstore.NewMemoryFileStore()}

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := auth.Principal{
				APIKeyID:   "key-test",
				TenantID:   "tenant-1",
				ProjectIDs: map[string]struct{}{"proj-1": {}},
				Permissions: map[auth.Permission]struct{}{
					auth.DatabaseRead:  {},
					auth.DatabaseWrite: {},
					auth.DatabaseAdmin: {},
				},
			}
			ctx := WithPrincipal(c.Request().Context(), p)
			ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	v1 := e.Group("/v1")
	p := v1.Group("/projects/:projectID")
	p.GET("/s3/objects", h.ListObjects)
	p.POST("/s3/objects", h.UploadObject)
	p.DELETE("/s3/objects", h.DeleteObject)
	p.GET("/s3/presign", h.PresignObject)

	return e
}

func TestS3_UploadAndList(t *testing.T) {
	e := setupS3TestRouter(t)

	// 上传一个文件
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	mw.WriteField("key", "docs/readme.txt")
	fileWriter, _ := mw.CreateFormFile("file", "readme.txt")
	fileWriter.Write([]byte("hello world"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/s3/objects", body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var obj s3ObjectDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &obj); err != nil {
		t.Fatal(err)
	}
	if obj.Key != "docs/readme.txt" {
		t.Errorf("expected key docs/readme.txt, got %s", obj.Key)
	}
	if obj.Size != 11 {
		t.Errorf("expected size 11, got %d", obj.Size)
	}
}

func TestS3_ListEmpty(t *testing.T) {
	e := setupS3TestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/s3/objects", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var objs []s3ObjectDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &objs); err != nil {
		t.Fatal(err)
	}
	if len(objs) != 0 {
		t.Errorf("expected empty list, got %d", len(objs))
	}
}

func TestS3_UploadAndDelete(t *testing.T) {
	e := setupS3TestRouter(t)

	// 上传
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	mw.WriteField("key", "test.txt")
	fw, _ := mw.CreateFormFile("file", "test.txt")
	fw.Write([]byte("data"))
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/s3/objects", body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload failed: %d %s", rec.Code, rec.Body.String())
	}

	// 删除
	req = httptest.NewRequest(http.MethodDelete, "/v1/projects/proj-1/s3/objects?key=test.txt", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 确认列表为空
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/s3/objects", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var objs []s3ObjectDTO
	json.Unmarshal(rec.Body.Bytes(), &objs)
	if len(objs) != 0 {
		t.Errorf("expected empty after delete, got %d", len(objs))
	}
}

func TestS3_Presign(t *testing.T) {
	e := setupS3TestRouter(t)

	// 先上传
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	mw.WriteField("key", "photo.png")
	fw, _ := mw.CreateFormFile("file", "photo.png")
	fw.Write([]byte("pngdata"))
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/s3/objects", body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload failed: %d", rec.Code)
	}

	// 生成预签名
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/s3/presign?key=photo.png", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp presignDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.URL == "" {
		t.Error("expected non-empty URL")
	}
}

func TestS3_Upload_InvalidKey(t *testing.T) {
	e := setupS3TestRouter(t)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	mw.WriteField("key", "../escape")
	fw, _ := mw.CreateFormFile("file", "x.txt")
	fw.Write([]byte("x"))
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/s3/objects", body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid key, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestS3_Upload_MissingKey(t *testing.T) {
	e := setupS3TestRouter(t)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("file", "x.txt")
	fw.Write([]byte("x"))
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/s3/objects", body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing key, got %d", rec.Code)
	}
}

func TestS3_Delete_MissingKey(t *testing.T) {
	e := setupS3TestRouter(t)
	req := httptest.NewRequest(http.MethodDelete, "/v1/projects/proj-1/s3/objects", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing key param, got %d", rec.Code)
	}
}

func TestS3_ProjectIsolation(t *testing.T) {
	e := setupS3TestRouter(t)

	// 上传到 proj-1
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	mw.WriteField("key", "shared.txt")
	fw, _ := mw.CreateFormFile("file", "shared.txt")
	fw.Write([]byte("from-proj-1"))
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/s3/objects", body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload failed: %d", rec.Code)
	}

	// 用另一个 project context 列出，应看不到 proj-1 的对象
	e2 := echo.New()
	e2.HideBanner = true
	store := objectstore.NewMemoryFileStore()
	// 手动写入 proj-1 前缀的对象
	store.Put(context.Background(), "proj-1/secret.txt", strings.NewReader("secret"), 6, "")
	h2 := &S3Handler{store: store}
	e2.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithPrincipal(c.Request().Context(), auth.Principal{
				APIKeyID:   "key-test2",
				ProjectIDs: map[string]struct{}{"proj-2": {}},
				Permissions: map[auth.Permission]struct{}{
					auth.DatabaseRead: {},
				},
			})
			ctx = WithProject(ctx, ProjectContext{ID: "proj-2", TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	v1 := e2.Group("/v1")
	p := v1.Group("/projects/:projectID")
	p.GET("/s3/objects", h2.ListObjects)

	req = httptest.NewRequest(http.MethodGet, "/v1/projects/proj-2/s3/objects", nil)
	rec = httptest.NewRecorder()
	e2.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var objs []s3ObjectDTO
	json.Unmarshal(rec.Body.Bytes(), &objs)
	if len(objs) != 0 {
		t.Errorf("project isolation broken: proj-2 should not see proj-1 objects, got %d", len(objs))
	}
}

func TestS3_PresignGet_TTL(t *testing.T) {
	// 确保 PresignGet 不因 ttl<=0 崩溃
	ctx := context.Background()
	fs := objectstore.NewMemoryFileStore()
	fs.Put(ctx, "x", strings.NewReader("x"), 1, "")
	url, err := fs.PresignGet(ctx, "x", 0)
	if err != nil {
		t.Fatal(err)
	}
	if url == "" {
		t.Error("expected non-empty URL")
	}
}

// 确保 time 导入被使用
var _ = time.Minute
