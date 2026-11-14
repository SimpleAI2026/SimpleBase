// 文件说明：s3_content_handler.go — internal/api 中 DownloadObject、CreateLink 等符号的实现
//
// 用法
//
//	(h *S3Handler) DownloadObject(c echo.Context) error
//	(h *S3Handler) CreateLink(c echo.Context) error
//	同包调用：DownloadObject() → router.go；CreateLink() → router.go；DownloadLinkedObject() → router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/s3_content_handler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/objectstore
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] DownloadObject() ← router.go
//
// 主要符号
//
//	(h *S3Handler) DownloadObject(c echo.Context) error
//	(h *S3Handler) CreateLink(c echo.Context) error
//	(h *S3Handler) DownloadLinkedObject(c echo.Context) error
//	... 另有 7 个顶层符号

package api

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// plainTextSubtypes 是内容嗅探只能识别为 text/plain 的文本格式：声明这些类型、嗅探结果为
// text/plain 时视为一致（例如 CSV 声明 text/csv）。不含 text/html 等可执行类型，
// 落盘的 Content-Type 仍取嗅探结果。
var plainTextSubtypes = map[string]bool{
	"text/csv": true, "text/tab-separated-values": true, "text/markdown": true,
	"text/yaml": true, "application/yaml": true, "application/x-yaml": true,
	"application/json": true, "application/x-ndjson": true,
}

func sniffUploadMIME(src io.Reader, key, declared string, allowed []string) (io.Reader, string, error) {
	reader := bufio.NewReader(src)
	head, _ := reader.Peek(512)
	detected := http.DetectContentType(head)
	detectedBase, _, _ := mime.ParseMediaType(detected)
	declaredBase, _, _ := mime.ParseMediaType(declared)
	if declaredBase != "" && declaredBase != "application/octet-stream" && declaredBase != detectedBase &&
		!(detectedBase == "text/plain" && plainTextSubtypes[declaredBase]) {
		return nil, "", fmt.Errorf("declared MIME mismatch")
	}
	if extension := mime.TypeByExtension(filepath.Ext(key)); extension != "" {
		expected, _, _ := mime.ParseMediaType(extension)
		if expected != detectedBase && expected != "text/plain" && detectedBase != "text/plain" {
			return nil, "", fmt.Errorf("extension MIME mismatch")
		}
	}
	if len(allowed) > 0 && !mimeAllowed(detectedBase, allowed) {
		return nil, "", fmt.Errorf("MIME denied")
	}
	return reader, detected, nil
}

func mimeAllowed(value string, allowed []string) bool {
	for _, item := range allowed {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == value || strings.HasSuffix(item, "/*") && strings.HasPrefix(value, strings.TrimSuffix(item, "*")) {
			return true
		}
	}
	return false
}

func s3ListResponse(c echo.Context, prefix string, objects []s3ObjectDTO) error {
	return s3ListResponseWith(c, prefix, objects, nil)
}

// s3FolderMetaDTO 是文件夹的外部访问状态（planv6.5 §5.5；folders 本身保持 string[] 不变）。
type s3FolderMetaDTO struct {
	Key             string `json:"key"`
	Public          bool   `json:"public"`
	PublicURL       string `json:"publicUrl,omitempty"`
	PublicInherited string `json:"publicInherited,omitempty"`
}

// s3ListDecor 为列表响应追加站点字段；nil 时输出与旧版完全一致。
type s3ListDecor struct {
	site   siteInfoDTO
	folder func(key string) s3FolderMetaDTO
}

// listResponse 为对象补充外部访问字段后输出（站点不可用时与旧版一致，仅追加 site 段）。
func (h *S3Handler) listResponse(c echo.Context, projectID, prefix string, objects []s3ObjectDTO) error {
	if h.sites == nil || !h.sitesCfg.Enabled {
		return s3ListResponse(c, prefix, objects)
	}
	state, info := h.loadSite(c.Request().Context(), projectID)
	for i := range objects {
		objects[i].Public, objects[i].PublicURL, objects[i].PublicInherited = h.decoratePublic(projectID, state, info.Enabled, objects[i].Key, false)
	}
	return s3ListResponseWith(c, prefix, objects, &s3ListDecor{
		site: info,
		folder: func(key string) s3FolderMetaDTO {
			public, u, inherited := h.decoratePublic(projectID, state, info.Enabled, key, true)
			return s3FolderMetaDTO{Key: key, Public: public, PublicURL: u, PublicInherited: inherited}
		},
	})
}

func s3ListResponseWith(c echo.Context, prefix string, objects []s3ObjectDTO, decor *s3ListDecor) error {
	if c.QueryParam("delimiter") == "" {
		return c.JSON(http.StatusOK, objects)
	}
	folderSet := map[string]struct{}{}
	items := make([]s3ObjectDTO, 0)
	for _, object := range objects {
		rest := strings.TrimPrefix(object.Key, prefix)
		if slash := strings.Index(rest, "/"); slash >= 0 {
			folderSet[prefix+rest[:slash+1]] = struct{}{}
			continue
		}
		items = append(items, object)
	}
	folders := make([]string, 0, len(folderSet))
	for folder := range folderSet {
		folders = append(folders, folder)
	}
	sort.Strings(folders)
	out := map[string]any{"folders": folders, "objects": items}
	if decor != nil {
		meta := make([]s3FolderMetaDTO, 0, len(folders))
		for _, f := range folders {
			meta = append(meta, decor.folder(f))
		}
		out["folderMeta"] = meta
		out["site"] = decor.site
	}
	return c.JSON(http.StatusOK, out)
}

func (h *S3Handler) DownloadObject(c echo.Context) error {
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	return h.streamObject(c, project.ID, c.QueryParam("key"))
}

func (h *S3Handler) CreateLink(c echo.Context) error {
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	if h.tokenSecret == "" {
		return WriteError(c, NewAPIError(http.StatusServiceUnavailable, "file_token_unavailable", "file token is not configured", RequestIDFromContext(c.Request().Context())))
	}
	var body struct {
		Key        string `json:"key"`
		TTLSeconds int    `json:"ttl_seconds"`
	}
	if err := decodeJSONBody(c, &body, false); err != nil {
		return WriteError(c, err)
	}
	if err := objectstore.ValidateFileKey(body.Key); err != nil {
		return WriteError(c, err)
	}
	if body.TTLSeconds == 0 {
		body.TTLSeconds = 900
	}
	if body.TTLSeconds < 1 || body.TTLSeconds > 86400 {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "ttl_seconds must be between 1 and 86400", RequestIDFromContext(c.Request().Context())))
	}
	token := signFileToken(h.tokenSecret, fileToken{ProjectID: project.ID, Key: body.Key, ExpiresAt: time.Now().UTC().Add(time.Duration(body.TTLSeconds) * time.Second).Unix()})
	return c.JSON(http.StatusOK, map[string]any{"url": "/files/" + token, "expires_at": time.Now().UTC().Add(time.Duration(body.TTLSeconds) * time.Second).Format(time.RFC3339)})
}

type fileToken struct {
	ProjectID string `json:"p"`
	Key       string `json:"k"`
	ExpiresAt int64  `json:"e"`
}

func signFileToken(secret string, token fileToken) string {
	payload, _ := json.Marshal(token)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(append(payload, mac.Sum(nil)...))
}

func parseFileToken(secret, value string) (fileToken, error) {
	var token fileToken
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(data) <= sha256.Size {
		return token, fmt.Errorf("invalid token")
	}
	payload, signature := data[:len(data)-sha256.Size], data[len(data)-sha256.Size:]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) || json.Unmarshal(payload, &token) != nil || token.ExpiresAt < time.Now().UTC().Unix() {
		return token, fmt.Errorf("invalid token")
	}
	if err := objectstore.ValidateFileKey(token.Key); err != nil {
		return token, err
	}
	return token, nil
}

func (h *S3Handler) DownloadLinkedObject(c echo.Context) error {
	if h.tokenSecret == "" {
		return WriteError(c, NewAPIError(http.StatusNotFound, "not_found", "file not found", RequestIDFromContext(c.Request().Context())))
	}
	token, err := parseFileToken(h.tokenSecret, c.Param("token"))
	if err != nil {
		return WriteError(c, NewAPIError(http.StatusNotFound, "not_found", "file not found", RequestIDFromContext(c.Request().Context())))
	}
	return h.streamObject(c, token.ProjectID, token.Key)
}

// inlineSafeMIME 判定 MIME 是否允许 inline 渲染（§8.1.4 白名单）。
// image/svg+xml 与 text/html 等一律不在白名单内（可执行脚本）。
func inlineSafeMIME(contentType string) bool {
	mt := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch mt {
	case "image/png", "image/jpeg", "image/gif", "image/webp",
		"application/pdf", "text/plain":
		return true
	}
	return false
}

func (h *S3Handler) streamObject(c echo.Context, projectID, key string) error {
	if err := objectstore.ValidateFileKey(key); err != nil {
		return WriteError(c, err)
	}
	store, ok := h.store.(objectstore.ContentStore)
	if !ok {
		return WriteError(c, NewAPIError(http.StatusNotImplemented, "file_content_unavailable", "file content is unavailable", RequestIDFromContext(c.Request().Context())))
	}
	rangeValue := c.Request().Header.Get("Range")
	content, err := store.GetContent(c.Request().Context(), joinKey(projectID, key), rangeValue)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "invalid range") {
			return WriteError(c, NewAPIError(http.StatusRequestedRangeNotSatisfiable, "invalid_range", "invalid range", RequestIDFromContext(c.Request().Context())))
		}
		return WriteError(c, err)
	}
	defer content.Body.Close()
	header := c.Response().Header()
	if content.ContentType != "" {
		header.Set(echo.HeaderContentType, content.ContentType)
	}
	header.Set("Accept-Ranges", "bytes")
	// §8.1.4：inline 白名单——只有这些类型浏览器内联渲染是安全的；
	// 其余（含 image/svg+xml）一律 attachment，防止上传文件在控制台源下执行脚本。
	disposition := "attachment"
	if inlineSafeMIME(content.ContentType) {
		disposition = "inline"
	}
	header.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filepath.Base(key)}))
	if content.ETag != "" {
		header.Set("ETag", content.ETag)
		if c.Request().Header.Get("If-None-Match") == content.ETag {
			return c.NoContent(http.StatusNotModified)
		}
	}
	status := http.StatusOK
	if rangeValue != "" {
		status = http.StatusPartialContent
		header.Set("Content-Range", "bytes "+strconv.FormatInt(content.RangeStart, 10)+"-"+strconv.FormatInt(content.RangeEnd, 10)+"/"+strconv.FormatInt(content.Size, 10))
	}
	header.Set(echo.HeaderContentLength, strconv.FormatInt(content.RangeEnd-content.RangeStart+1, 10))
	return c.Stream(status, header.Get(echo.HeaderContentType), content.Body)
}
