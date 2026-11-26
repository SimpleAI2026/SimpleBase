// 文件说明：s3_content_handler_test.go — internal/api 的单元测试（TestS3ContentMIMERangeLinkAndFolders 等）
//
// 用法
//
//	go test ./internal/api -run 'TestS3ContentMIMERangeLinkAndFolders'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/s3_content_handler_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestS3ContentMIMERangeLinkAndFolders(t *testing.T)

package filehttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

func TestS3ContentMIMERangeLinkAndFolders(t *testing.T) {
	store := objectstore.NewMemoryFileStore()
	h := &S3Handler{store: store, tokenSecret: "test-secret", allowedMIME: []string{"text/plain"}, writable: true}
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.SetRequest(c.Request().WithContext(httpkit.WithProject(c.Request().Context(), httpkit.ProjectContext{ID: "prj00001"})))
			return next(c)
		}
	})
	e.POST("/objects", h.UploadObject)
	e.GET("/objects", h.ListObjects)
	e.GET("/objects/content", h.DownloadObject)
	e.POST("/link", h.CreateLink)
	e.GET("/files/:token", h.DownloadLinkedObject)
	upload := func(key, contentType, content string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		_ = writer.WriteField("key", key)
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, key))
		header.Set("Content-Type", contentType)
		file, _ := writer.CreatePart(header)
		_, _ = file.Write([]byte(content))
		_ = writer.Close()
		req := httptest.NewRequest(http.MethodPost, "/objects", &body)
		req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	if rec := upload("docs/a.txt", "text/plain", "hello"); rec.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	if rec := upload("docs/b.txt", "image/png", "hello"); rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("MIME mismatch: %d %s", rec.Code, rec.Body.String())
	}
	if rec := upload("docs/c.csv", "text/csv", "a,b\n1,2\n"); rec.Code != http.StatusOK {
		t.Fatalf("CSV 声明 text/csv 应被接受: %d %s", rec.Code, rec.Body.String())
	}
	if rec := upload("docs/d.txt", "text/html", "hello"); rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/html 声明与嗅探不一致仍应拒绝: %d %s", rec.Code, rec.Body.String())
	}
	if rec := upload("image.png", "image/png", "not really png"); rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("MIME whitelist: %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/objects?prefix=docs/&delimiter=/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"folders"`) {
		t.Fatalf("folders: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/objects/content?key=docs/a.txt", nil)
	req.Header.Set("Range", "bytes=1-3")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "ell" || rec.Header().Get("Content-Range") != "bytes 1-3/5" {
		t.Fatalf("range: %d %s headers=%v", rec.Code, rec.Body.String(), rec.Header())
	}
	req = httptest.NewRequest(http.MethodPost, "/link", strings.NewReader(`{"key":"docs/a.txt","ttl_seconds":60}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var link struct {
		URL string `json:"url"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &link) != nil {
		t.Fatalf("link: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, link.URL, nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "hello" {
		t.Fatalf("linked download: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, link.URL+"x", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("tampered token: %d", rec.Code)
	}
	expired := signFileToken("test-secret", fileToken{ProjectID: "prj00001", Key: "docs/a.txt", ExpiresAt: time.Now().Add(-time.Minute).Unix()})
	req = httptest.NewRequest(http.MethodGet, "/files/"+expired, nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expired token: %d", rec.Code)
	}
}
