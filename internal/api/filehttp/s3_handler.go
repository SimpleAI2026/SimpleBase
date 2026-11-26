package filehttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

// ObjectEventPublisher 是文件写路径发布对象事件的端口（发布语言 objectstore.ObjectEvent）；
// 由 deps.TriggerEvents 注入，nil 表示不发布。订阅方在 app 装配时登记。
// ObjectEventPublisher 是文件写路径发布对象事件的端口（发布语言 objectstore.ObjectEvent）。
type ObjectEventPublisher interface {
	PublishObject(event objectstore.ObjectEvent)
}

// S3Handler 依赖 objectstore.FileStore，可选系统库对象索引。
type S3Handler struct {
	store       objectstore.FileStore
	index       *systemdb.Store
	writable    bool
	tokenSecret string
	allowedMIME []string
	events      ObjectEventPublisher // 对象事件的发布端口；nil 表示不发布
	// planv6.5：发布网站。sites 为 nil 时站点相关字段与接口降级（publishable=false）。
	sites            SiteStore
	sitesCfg         config.SitesConfig
	traffic          SiteTraffic
	cfgHTTPPublicURL string // planv6.7 §1.3：public_base_url 回退用
}

// S3HandlerDeps 是构造 S3Handler 的全部依赖（app 经 api.Dependencies 注入；events / sites 可为 nil）。
type S3HandlerDeps struct {
	Store       objectstore.FileStore
	Index       *systemdb.Store
	Writable    bool
	TokenSecret string
	AllowedMIME []string
	Events      ObjectEventPublisher
	Sites       SiteStore
	SitesCfg    config.SitesConfig
	SiteTraffic SiteTraffic
	PublicURL   string
}

// NewS3Handler 构造用户文件存储 handler。
func NewS3Handler(d S3HandlerDeps) *S3Handler {
	return &S3Handler{store: d.Store, index: d.Index, writable: d.Writable, tokenSecret: d.TokenSecret, allowedMIME: d.AllowedMIME,
		events: d.Events, sites: d.Sites, sitesCfg: d.SitesCfg, traffic: d.SiteTraffic, cfgHTTPPublicURL: d.PublicURL}
}

// s3ObjectDTO 是列表/上传返回的对象元数据。
type s3ObjectDTO struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified,omitempty"`
	// planv6.5：外部访问状态（只在站点可用时返回）。
	Public          bool   `json:"public,omitempty"`
	PublicURL       string `json:"publicUrl,omitempty"`
	PublicInherited string `json:"publicInherited,omitempty"`
}

// presignDTO 是预签名 URL 响应。
type presignDTO struct {
	URL string `json:"url"`
}

// joinKey 把项目 ID 与 files/ 段拼到用户 key 前面，实现项目隔离。
// 物理布局：{projectID}/files/{userKey}（与 storage.data_dir / S3 前缀一致）。
func JoinKey(projectID, key string) string {
	if key == "" {
		return projectID + "/files"
	}
	return projectID + "/files/" + key
}

// stripProject 从物理 key 中去掉 {projectID}/files/ 前缀，还原用户可见的 key。
func StripProject(projectID, key string) string {
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
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	prefix := c.QueryParam("prefix")
	if prefix != "" {
		if err := objectstore.ValidateFileKey(prefix); err != nil {
			return httpkit.WriteError(c, err)
		}
	}
	refresh := c.QueryParam("refresh") == "1" || c.QueryParam("refresh") == "true"
	if refresh && h.index != nil {
		listed, err := h.store.List(c.Request().Context(), JoinKey(pc.ID, prefix), 1000)
		if err != nil {
			return httpkit.WriteError(c, err)
		}
		stripped := make([]objectstore.FileObject, 0, len(listed))
		for _, obj := range listed {
			obj.Key = StripProject(pc.ID, obj.Key)
			stripped = append(stripped, obj)
		}
		_, _, _ = h.index.RefreshS3Index(c.Request().Context(), pc.ID, stripped)
	}
	if h.index != nil && !refresh {
		rows, err := h.index.ListS3Objects(c.Request().Context(), pc.ID, prefix, 1000)
		if err != nil {
			return httpkit.WriteError(c, err)
		}
		dtos := make([]s3ObjectDTO, 0, len(rows))
		for _, obj := range rows {
			dto := s3ObjectDTO{Key: obj.Key, Size: obj.Size}
			if !obj.LastModified.IsZero() {
				dto.LastModified = obj.LastModified.UTC().Format(time.RFC3339)
			}
			dtos = append(dtos, dto)
		}
		return h.listResponse(c, pc.ID, prefix, dtos)
	}
	objects, err := h.store.List(c.Request().Context(), JoinKey(pc.ID, prefix), 1000)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	dtos := make([]s3ObjectDTO, 0, len(objects))
	for _, obj := range objects {
		dtos = append(dtos, toDTO(obj, pc.ID))
	}
	return h.listResponse(c, pc.ID, prefix, dtos)
}

// UploadObject 上传一个文件。使用 multipart/form-data，字段：key + file。
func (h *S3Handler) UploadObject(c echo.Context) error {
	if !h.writable {
		return httpkit.WriteError(c, godatabase.ErrWriterUnavailable)
	}
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	key := c.FormValue("key")
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key is required")
	}
	if err := objectstore.ValidateFileKey(key); err != nil {
		return httpkit.WriteError(c, err)
	}
	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}
	src, err := file.Open()
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	defer src.Close()
	uploadReader, contentType, err := sniffUploadMIME(src, key, file.Header.Get("Content-Type"), h.allowedMIME)
	if err != nil {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusUnsupportedMediaType, "mime_mismatch", "file MIME type is not allowed or does not match", httpkit.RequestID(c)))
	}
	obj, err := h.store.Put(c.Request().Context(), JoinKey(pc.ID, key), uploadReader, file.Size, contentType)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.index != nil {
		_ = h.index.UpsertS3Object(c.Request().Context(), pc.ID, key, obj.Size, "", contentType, obj.LastModified)
	}
	if h.events != nil {
		h.events.PublishObject(objectstore.ObjectEvent{ProjectID: pc.ID, Key: key, Action: "upload"})
	}
	return c.JSON(http.StatusOK, toDTO(obj, pc.ID))
}

// DeleteObject 删除指定 key 的对象。
func (h *S3Handler) DeleteObject(c echo.Context) error {
	if !h.writable {
		return httpkit.WriteError(c, godatabase.ErrWriterUnavailable)
	}
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	key := c.QueryParam("key")
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key is required")
	}
	if err := objectstore.ValidateFileKey(key); err != nil {
		return httpkit.WriteError(c, err)
	}
	if err := h.store.Delete(c.Request().Context(), JoinKey(pc.ID, key)); err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.index != nil {
		_ = h.index.SoftDeleteS3Object(c.Request().Context(), pc.ID, key)
	}
	// planv6.5 §5.6：删除对象时清理 file 精确公开行（folder 行保留）。
	if h.sites != nil {
		_ = h.sites.DeletePublicFile(c.Request().Context(), pc.ID, key)
	}
	if h.events != nil {
		h.events.PublishObject(objectstore.ObjectEvent{ProjectID: pc.ID, Key: key, Action: "delete"})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// PresignObject 生成预签名下载 URL。
func (h *S3Handler) PresignObject(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	key := c.QueryParam("key")
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key is required")
	}
	if err := objectstore.ValidateFileKey(key); err != nil {
		return httpkit.WriteError(c, err)
	}
	url, err := h.store.PresignGet(c.Request().Context(), JoinKey(pc.ID, key), 15*time.Minute)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, presignDTO{URL: url})
}

// toDTO 把 FileObject 转为 DTO，去掉项目前缀。
func toDTO(obj objectstore.FileObject, projectID string) s3ObjectDTO {
	dto := s3ObjectDTO{
		Key:  StripProject(projectID, obj.Key),
		Size: obj.Size,
	}
	if !obj.LastModified.IsZero() {
		dto.LastModified = obj.LastModified.UTC().Format(time.RFC3339)
	}
	return dto
}
