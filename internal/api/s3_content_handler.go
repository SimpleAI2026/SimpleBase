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

func sniffUploadMIME(src io.Reader, key, declared string, allowed []string) (io.Reader, string, error) {
	reader := bufio.NewReader(src)
	head, _ := reader.Peek(512)
	detected := http.DetectContentType(head)
	detectedBase, _, _ := mime.ParseMediaType(detected)
	declaredBase, _, _ := mime.ParseMediaType(declared)
	if declaredBase != "" && declaredBase != "application/octet-stream" && declaredBase != detectedBase {
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
	return c.JSON(http.StatusOK, map[string]any{"folders": folders, "objects": items})
}

func (h *S3Handler) DownloadObject(c echo.Context) error {
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, fmt.Errorf("project context missing"))
	}
	return h.streamObject(c, project.ID, c.QueryParam("key"))
}

func (h *S3Handler) CreateLink(c echo.Context) error {
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, fmt.Errorf("project context missing"))
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
	header.Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filepath.Base(key)}))
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
