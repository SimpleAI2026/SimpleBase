// 文件说明：s3_handler.go — 用户文件存储的 HTTP handler
//
// 用法
//
//	(h *S3Handler) ListObjects(c echo.Context) error  // ListObjects 列出当前项目下的对象
//	(h *S3Handler) UploadObject(c echo.Context) error  // UploadObject 上传一个文件
//	同包调用：joinKey() → gocloudagent_access.go, s3_content_handler.go；ListObjects() → gocloudagent_access.go, router.go；S3Handler → router.go, s3_content_handler.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/s3_handler.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, internal/objectstore, internal/systemdb
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] joinKey() ← gocloudagent_access.go, s3_content_handler.go
//
// 主要符号
//
//	type S3Handler struct {…}
//	(h *S3Handler) ListObjects(c echo.Context) error
//	(h *S3Handler) UploadObject(c echo.Context) error
//	(h *S3Handler) DeleteObject(c echo.Context) error
//	(h *S3Handler) PresignObject(c echo.Context) error
//	... 另有 5 个顶层符号

package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// S3Handler 依赖 objectstore.FileStore，可选系统库对象索引。
type S3Handler struct {
	store       objectstore.FileStore
	index       *systemdb.Store
	writable    *bool
	tokenSecret string
	allowedMIME []string
	events      objectEventPublisher
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

// joinKey 把项目 ID 与 files/ 段拼到用户 key 前面，实现项目隔离。
// 物理布局：{projectID}/files/{userKey}（与 storage.data_dir / S3 前缀一致）。
func joinKey(projectID, key string) string {
	if key == "" {
		return projectID + "/files"
	}
	return projectID + "/files/" + key
}

// stripProject 从物理 key 中去掉 {projectID}/files/ 前缀，还原用户可见的 key。
func stripProject(projectID, key string) string {
	prefix := projectID + "/files/"
	if strings.HasPrefix(key, prefix) {
		return key[len(prefix):]
	}
	if key == projectID+"/files" {
		return ""
	}
	return key
}

// ListObjects 列出当前项目下的对象。默认读 sys_s3_objects；refresh=1 时对账平面 A。
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
	refresh := c.QueryParam("refresh") == "1" || c.QueryParam("refresh") == "true"
	if refresh && h.index != nil {
		listed, err := h.store.List(c.Request().Context(), joinKey(pc.ID, prefix), 1000)
		if err != nil {
			return WriteError(c, err)
		}
		stripped := make([]objectstore.FileObject, 0, len(listed))
		for _, obj := range listed {
			obj.Key = stripProject(pc.ID, obj.Key)
			stripped = append(stripped, obj)
		}
		_, _, _ = h.index.RefreshS3Index(c.Request().Context(), pc.ID, stripped)
	}
	if h.index != nil && !refresh {
		rows, err := h.index.ListS3Objects(c.Request().Context(), pc.ID, prefix, 1000)
		if err != nil {
			return WriteError(c, err)
		}
		dtos := make([]s3ObjectDTO, 0, len(rows))
		for _, obj := range rows {
			dto := s3ObjectDTO{Key: obj.Key, Size: obj.Size}
			if !obj.LastModified.IsZero() {
				dto.LastModified = obj.LastModified.UTC().Format(time.RFC3339)
			}
			dtos = append(dtos, dto)
		}
		return s3ListResponse(c, prefix, dtos)
	}
	objects, err := h.store.List(c.Request().Context(), joinKey(pc.ID, prefix), 1000)
	if err != nil {
		return WriteError(c, err)
	}
	dtos := make([]s3ObjectDTO, 0, len(objects))
	for _, obj := range objects {
		dtos = append(dtos, toDTO(obj, pc.ID))
	}
	return s3ListResponse(c, prefix, dtos)
}

// UploadObject 上传一个文件。使用 multipart/form-data，字段：key + file。
func (h *S3Handler) UploadObject(c echo.Context) error {
	if h.writable != nil && !*h.writable {
		return WriteError(c, godatabase.ErrWriterUnavailable)
	}
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
	uploadReader, contentType, err := sniffUploadMIME(src, key, file.Header.Get("Content-Type"), h.allowedMIME)
	if err != nil {
		return WriteError(c, NewAPIError(http.StatusUnsupportedMediaType, "mime_mismatch", "file MIME type is not allowed or does not match", RequestIDFromContext(c.Request().Context())))
	}
	obj, err := h.store.Put(c.Request().Context(), joinKey(pc.ID, key), uploadReader, file.Size, contentType)
	if err != nil {
		return WriteError(c, err)
	}
	if h.index != nil {
		_ = h.index.UpsertS3Object(c.Request().Context(), pc.ID, key, obj.Size, "", contentType, obj.LastModified)
	}
	if h.events != nil {
		h.events.PublishObject(ObjectEvent{ProjectID: pc.ID, Key: key, Action: "upload"})
	}
	return c.JSON(http.StatusOK, toDTO(obj, pc.ID))
}

// DeleteObject 删除指定 key 的对象。
func (h *S3Handler) DeleteObject(c echo.Context) error {
	if h.writable != nil && !*h.writable {
		return WriteError(c, godatabase.ErrWriterUnavailable)
	}
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
	if h.index != nil {
		_ = h.index.SoftDeleteS3Object(c.Request().Context(), pc.ID, key)
	}
	if h.events != nil {
		h.events.PublishObject(ObjectEvent{ProjectID: pc.ID, Key: key, Action: "delete"})
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
