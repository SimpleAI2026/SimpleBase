// 文件说明：s3_site_handler.go — 对象存储发布网站（planv6.5 §5.4/§5.5）：站点与外部访问管理 API
//
// 用法
//
//	GET  /v1/projects/:pid/s3/site     → {enabled, base_url, publishable, reason}
//	PUT  /v1/projects/:pid/s3/site     ← {enabled}                     （database:admin + writeGate）
//	GET  /v1/projects/:pid/s3/public   → {paths:[{path,kind,public_url}]}
//	PUT  /v1/projects/:pid/s3/public   ← {key, kind:file|folder, public} （database:admin + writeGate）
//
// 不可委托：公开即对外暴露数据，云助手的委托令牌不得代为开启。

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// ErrSiteNotPublishable 表示项目当前不能发布网站（未开启 sites、系统项目、ID 不可作 DNS 标签）。
var ErrSiteNotPublishable = errors.New("site is not publishable for this project")

// 不可发布原因。
const (
	siteReasonDisabled      = "sites_disabled"
	siteReasonSystemProject = "system_project"
)

// siteInfoDTO 是站点状态（列表接口与 GET /s3/site 共用；S3 域沿用 camelCase 例外）。
type siteInfoDTO struct {
	Enabled     bool   `json:"enabled"`
	BaseURL     string `json:"baseUrl,omitempty"`
	Publishable bool   `json:"publishable"`
	Reason      string `json:"reason,omitempty"`
}

// publicPathDTO 是一条公开路径。
type publicPathDTO struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	PublicURL string `json:"publicUrl,omitempty"`
}

// siteReason 返回不可发布原因；空串表示可发布。
func (h *S3Handler) siteReason(projectID string) string {
	if !h.sitesCfg.Enabled || h.sites == nil {
		return siteReasonDisabled
	}
	if systemdb.IsAdminProject(projectID) {
		return siteReasonSystemProject
	}
	return config.ProjectIDPublishable(projectID)
}

// siteURL 返回 key 的站点访问 URL；folder 以 "/" 结尾。路径逐段转义。
func (h *S3Handler) siteURL(projectID, key string) string {
	base := h.sitesCfg.SiteBaseURL(projectID)
	if base == "" {
		return ""
	}
	segs := strings.Split(key, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return base + "/" + strings.Join(segs, "/")
}

// loadSite 读取站点快照与状态 DTO；站点功能不可用时 state 为零值。
func (h *S3Handler) loadSite(ctx context.Context, projectID string) (siteState, siteInfoDTO) {
	reason := h.siteReason(projectID)
	info := siteInfoDTO{Publishable: reason == "", Reason: reason}
	if reason != "" {
		return siteState{}, info
	}
	state, err := h.sites.Lookup(ctx, projectID)
	if err != nil {
		return siteState{}, info
	}
	info.Enabled = state.Enabled
	if state.Enabled {
		info.BaseURL = h.sitesCfg.SiteBaseURL(projectID)
	}
	return state, info
}

// decoratePublic 为对象/文件夹补充 public / publicUrl / publicInherited 字段。
func (h *S3Handler) decoratePublic(projectID string, state siteState, enabled bool, key string, isFolder bool) (bool, string, string) {
	match, ok := systemdb.MatchPublicPath(state.Paths, key)
	if !ok {
		return false, "", ""
	}
	inherited := ""
	selfKind := systemdb.PublicPathFile
	if isFolder {
		selfKind = systemdb.PublicPathFolder
	}
	if match.Path != key || match.Kind != selfKind {
		inherited = match.Path
		if inherited == "" {
			inherited = "/"
		}
	}
	u := ""
	if enabled {
		u = h.siteURL(projectID, key)
	}
	return true, u, inherited
}

// GetSite 返回站点状态。
func (h *S3Handler) GetSite(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	_, info := h.loadSite(c.Request().Context(), pc.ID)
	return c.JSON(http.StatusOK, info)
}

// PutSite 发布或下线网站。
func (h *S3Handler) PutSite(c echo.Context) error {
	if h.writable != nil && !*h.writable {
		return WriteError(c, godatabase.ErrWriterUnavailable)
	}
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	if reason := h.siteReason(pc.ID); reason != "" {
		return WriteError(c, ErrSiteNotPublishable)
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeJSONBody(c, &body, false); err != nil {
		return WriteError(c, err)
	}
	if body.Enabled == nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "enabled is required", RequestIDFromContext(c.Request().Context())))
	}
	actor := principalActor(c)
	if err := h.sites.SetSite(c.Request().Context(), pc.ID, *body.Enabled, actor); err != nil {
		return WriteError(c, err)
	}
	kind := "site.disable"
	if *body.Enabled {
		kind = "site.enable"
	}
	h.recordSiteAudit(actor, pc.ID, kind, "")
	_, info := h.loadSite(c.Request().Context(), pc.ID)
	return c.JSON(http.StatusOK, info)
}

// ListPublic 列出公开路径。
func (h *S3Handler) ListPublic(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	state, info := h.loadSite(c.Request().Context(), pc.ID)
	out := make([]publicPathDTO, 0, len(state.Paths))
	for _, p := range state.Paths {
		dto := publicPathDTO{Path: p.Path, Kind: p.Kind}
		if info.Enabled {
			dto.PublicURL = h.siteURL(pc.ID, p.Path)
		}
		out = append(out, dto)
	}
	return c.JSON(http.StatusOK, map[string]any{"site": info, "paths": out})
}

// PutPublic 设置单条路径的外部访问开关。
func (h *S3Handler) PutPublic(c echo.Context) error {
	if h.writable != nil && !*h.writable {
		return WriteError(c, godatabase.ErrWriterUnavailable)
	}
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	if reason := h.siteReason(pc.ID); reason != "" {
		return WriteError(c, ErrSiteNotPublishable)
	}
	var body struct {
		Key    string `json:"key"`
		Kind   string `json:"kind"`
		Public *bool  `json:"public"`
	}
	if err := decodeJSONBody(c, &body, false); err != nil {
		return WriteError(c, err)
	}
	rid := RequestIDFromContext(c.Request().Context())
	if body.Public == nil {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "public is required", rid))
	}
	switch body.Kind {
	case systemdb.PublicPathFile:
		if strings.HasSuffix(body.Key, "/") {
			return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "file key must not end with /", rid))
		}
		if err := objectstore.ValidateFileKey(body.Key); err != nil {
			return WriteError(c, err)
		}
	case systemdb.PublicPathFolder:
		if body.Key != "" {
			if !strings.HasSuffix(body.Key, "/") {
				return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "folder key must end with /", rid))
			}
			if err := objectstore.ValidateFileKey(strings.TrimSuffix(body.Key, "/")); err != nil {
				return WriteError(c, err)
			}
		}
	default:
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request", "kind must be file or folder", rid))
	}
	actor := principalActor(c)
	if err := h.sites.SetPublicPath(c.Request().Context(), pc.ID, body.Key, body.Kind, *body.Public, actor); err != nil {
		return WriteError(c, err)
	}
	kind := "s3.public.unset"
	if *body.Public {
		kind = "s3.public.set"
	}
	sum := sha256.Sum256([]byte(body.Key))
	h.recordSiteAudit(actor, pc.ID, kind, body.Kind+":"+hex.EncodeToString(sum[:])[:16])

	state, info := h.loadSite(c.Request().Context(), pc.ID)
	public, publicURL, inherited := h.decoratePublic(pc.ID, state, info.Enabled, body.Key, body.Kind == systemdb.PublicPathFolder)
	return c.JSON(http.StatusOK, map[string]any{"public": public, "publicUrl": publicURL, "publicInherited": inherited})
}

// recordSiteAudit 写系统审计：只记 kind 与 path 的 SHA-256 前缀，不记明文 key。
func (h *S3Handler) recordSiteAudit(actor, projectID, kind, detail string) {
	if h.index == nil {
		return
	}
	h.index.RecordAudit(systemdb.AuditEvent{Actor: actor, ProjectID: projectID, Kind: kind, Detail: detail})
}

// principalActor 返回审计 actor（user:<id> | key:<id>）。
func principalActor(c echo.Context) string {
	p, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return ""
	}
	if p.UserID != "" {
		return "user:" + p.UserID
	}
	if p.APIKeyID != "" {
		return "key:" + string(p.APIKeyID)
	}
	return ""
}
