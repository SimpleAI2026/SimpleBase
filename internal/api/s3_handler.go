// s3_handler.go 实现用户文件存储的 HTTP handler。
//
// 路由（挂在 /v1/projects/:projectID/s3 下）：
//   GET    /objects?prefix=xxx   列出对象
//   POST   /objects              上传对象（multipart: key + file）
//   DELETE /objects?key=xxx      删除对象
//   GET    /presign?key=xxx      生成预签名下载 URL
//
// 项目隔离：handler 从 ProjectContext 取 projectID，
// 所有 key 前自动拼接 "{projectID}/" 作为物理前缀。
package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// S3Handler 依赖 objectstore.FileStore。
type S3Handler struct {
	store objectstore.FileStore
}

// s3ObjectDTO 是列表/上传返回的对象元数据。
type s3ObjectDTO struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified,omitempty"`
}

// presignDTO 是预签名 URL 响应。
type presignDTO struct {
	URL string `json:"url"`
}

// joinKey 把项目 ID 作为前缀拼到用户 key 前面，实现项目隔离。
func joinKey(projectID, key string) string {
	return projectID + "/" + key
}

// stripProject 从物理 key 中去掉项目前缀，还原用户可见的 key。
func stripProject(projectID, key string) string {
	prefix := projectID + "/"
	if strings.HasPrefix(key, prefix) {
		return key[len(prefix):]
	}
	return key
}

// ListObjects 列出当前项目下的对象。
func (h *S3Handler) ListObjects(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	prefix := c.QueryParam("prefix")
	if prefix != "" {
		if err := objectstore.ValidateFileKey(prefix); err != nil {
			return WriteError(c, err)
		}
	}
	objects, err := h.store.List(c.Request().Context(), joinKey(pc.ID, prefix), 1000)
	if err != nil {
		return WriteError(c, err)
	}
	dtos := make([]s3ObjectDTO, 0, len(objects))
	for _, obj := range objects {
		dtos = append(dtos, toDTO(obj, pc.ID))
	}
	return c.JSON(http.StatusOK, dtos)
}

// UploadObject 上传一个文件。使用 multipart/form-data，字段：key + file。
func (h *S3Handler) UploadObject(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	key := c.FormValue("key")
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key is required")
	}
	if err := objectstore.ValidateFileKey(key); err != nil {
		return WriteError(c, err)
	}
	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}
	src, err := file.Open()
	if err != nil {
		return WriteError(c, err)
	}
	defer src.Close()
	contentType := file.Header.Get("Content-Type")
	obj, err := h.store.Put(c.Request().Context(), joinKey(pc.ID, key), src, file.Size, contentType)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toDTO(obj, pc.ID))
}

// DeleteObject 删除指定 key 的对象。
func (h *S3Handler) DeleteObject(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	key := c.QueryParam("key")
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key is required")
	}
	if err := objectstore.ValidateFileKey(key); err != nil {
		return WriteError(c, err)
	}
	if err := h.store.Delete(c.Request().Context(), joinKey(pc.ID, key)); err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// PresignObject 生成预签名下载 URL。
func (h *S3Handler) PresignObject(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	key := c.QueryParam("key")
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key is required")
	}
	if err := objectstore.ValidateFileKey(key); err != nil {
		return WriteError(c, err)
	}
	url, err := h.store.PresignGet(c.Request().Context(), joinKey(pc.ID, key), 15*time.Minute)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, presignDTO{URL: url})
}

// toDTO 把 FileObject 转为 DTO，去掉项目前缀。
func toDTO(obj objectstore.FileObject, projectID string) s3ObjectDTO {
	dto := s3ObjectDTO{
		Key:  stripProject(projectID, obj.Key),
		Size: obj.Size,
	}
	if !obj.LastModified.IsZero() {
		dto.LastModified = obj.LastModified.UTC().Format(time.RFC3339)
	}
	return dto
}
