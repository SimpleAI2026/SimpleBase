// 文件说明：s3_site_handler.go — 对象存储发布网站（planv6.5 §5.4/§5.5）：站点与外部访问管理 API
//
// 用法
//
//	GET  /v1/projects/:pid/s3/site     → {base_url, publishable, reason}
//	GET  /v1/projects/:pid/s3/public   → {paths:[{path,kind,public_url}]}
//	PUT  /v1/projects/:pid/s3/public   ← {key, kind:file|folder, public} （database:admin + writeGate）
//
// 站点没有实例级开关：配置了 sites.base_domain 即可发布，对象默认私有。
// 委托：公开即对外暴露数据。云助手委托令牌只可读站点状态、公开 web/ 文件夹
// （需用户确认）；公开路径列表仍不可委托。

package api

import (
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

// ErrSiteNotPublishable 表示项目当前不能发布网站（未配置域或前缀、系统项目、或子域名方式下 ID 不可作 DNS 标签）。
var ErrSiteNotPublishable = errors.New("site is not publishable for this project")

// delegatedPublicPrefix 是云助手委托令牌唯一允许公开的对象前缀。
const delegatedPublicPrefix = "web/"

// siteReasonUnavailable 是路径方式 + 站点功能未配置的原因码。
const siteReasonUnavailable = "site_unavailable"

// 不可发布原因（只在子域名方式返回）。
const (
	siteReasonNoDomain      = "site_domain_unconfigured"
	siteReasonSystemProject = "system_project"
)

// siteInfoDTO 是站点状态（列表接口与 GET /s3/site 共用；S3 域沿用 camelCase 例外）。
type siteInfoDTO struct {
	BaseURL     string `json:"baseUrl,omitempty"`
	Publishable bool   `json:"publishable"`
	Reason      string `json:"reason,omitempty"`
	Mode        string `json:"mode,omitempty"` // path | subdomain；planv6.7 §1.3
}

// publicPathDTO 是一条公开路径。
type publicPathDTO struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	PublicURL string `json:"publicUrl,omitempty"`
}

// siteReason 返回不可发布原因；空串表示可发布。planv6.7 §1.1 按模式区分。
func (h *S3Handler) siteReason(projectID string) string {
	if systemdb.IsAdminProject(projectID) {
		return siteReasonSystemProject
	}
	if h.sitesCfg.Mode() == config.SiteModePath {
		// 路径方式不依赖 base_domain，但必须有 sites 开关（等于 h.sites != nil）。
		if h.sites == nil {
			return siteReasonUnavailable
		}
		return ""
	}
	// 子域名方式
	if h.sites == nil || !h.sitesCfg.Configured() {
		return siteReasonNoDomain
	}
	return config.ProjectIDPublishable(projectID)
}

// siteBase 返回项目站点根 URL（不带结尾 "/"，按当前模式生成）。planv6.7 §1.3。
// 路径方式依赖传入的 context 以取得公共 baseURL。
func (h *S3Handler) siteBase(c echo.Context, projectID string) string {
	if h.sitesCfg.Mode() == config.SiteModeSubdomain {
		return h.sitesCfg.SubdomainBaseURL(projectID)
	}
	return h.sitesCfg.PathBaseURL(publicBaseURL(c, h.cfgHTTPPublicURL, h.sitesCfg.PublicBaseURL), projectID)
}

// siteURL 返回 key 的站点访问 URL；folder 以 "/" 结尾。路径逐段转义。
func (h *S3Handler) siteURL(c echo.Context, projectID, key string) string {
	base := h.siteBase(c, projectID)
	if base == "" {
		return ""
	}
	segs := strings.Split(key, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return base + "/" + strings.Join(segs, "/")
}

// loadSite 读取站点快照与状态 DTO；站点功能不可用时 state 为零值。planv6.7 §1.3 补 mode。
func (h *S3Handler) loadSite(c echo.Context, projectID string) (siteState, siteInfoDTO) {
	reason := h.siteReason(projectID)
	info := siteInfoDTO{Publishable: reason == "", Reason: reason, Mode: h.sitesCfg.Mode()}
	if reason != "" {
		return siteState{}, info
	}
	state, err := h.sites.Lookup(c.Request().Context(), projectID)
	if err != nil {
		return siteState{}, info
	}
	info.BaseURL = h.siteBase(c, projectID)
	return state, info
}

// decoratePublic 为对象/文件夹补充 public / publicUrl / publicInherited 字段。
func (h *S3Handler) decoratePublic(c echo.Context, projectID string, state siteState, key string, isFolder bool) (bool, string, string) {
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
	return true, h.siteURL(c, projectID, key), inherited
}

// GetSite 返回站点状态（planv6.7 §1.1 补 mode 字段）。
func (h *S3Handler) GetSite(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	_, info := h.loadSite(c, pc.ID)
	return c.JSON(http.StatusOK, info)
}

// ListPublic 列出公开路径。
func (h *S3Handler) ListPublic(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	state, info := h.loadSite(c, pc.ID)
	out := make([]publicPathDTO, 0, len(state.Paths))
	for _, p := range state.Paths {
		dto := publicPathDTO{Path: p.Path, Kind: p.Kind, PublicURL: h.siteURL(c, pc.ID, p.Path)}
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
	// 委托主体（云助手）只能开启 web/ 文件夹：禁止公开对象根或其它目录，
	// 即使模型绕过 CLI 的前缀检查，服务端也在这里兜底。
	if p, ok := PrincipalFromContext(c.Request().Context()); ok && p.Delegated {
		if body.Kind != systemdb.PublicPathFolder || body.Key != delegatedPublicPrefix {
			return WriteError(c, NewAPIError(http.StatusForbidden, "delegated_public_prefix_only", "assistant may only publish the "+delegatedPublicPrefix+" folder", rid))
		}
		if *body.Public {
			store, ok := h.store.(objectstore.ContentStore)
			if !ok {
				return WriteError(c, NewAPIError(http.StatusServiceUnavailable, "site_content_unavailable", "cannot validate site entry", rid))
			}
			entry, err := store.GetContent(c.Request().Context(), joinKey(pc.ID, delegatedPublicPrefix+h.sitesCfg.EffectiveIndexDocument()), "")
			if err != nil {
				return WriteError(c, NewAPIError(http.StatusBadRequest, "site_entry_unavailable", "upload a readable site entry before publishing", rid))
			}
			_ = entry.Body.Close()
			if entry.Size <= 0 || entry.Size > 200*1024 {
				return WriteError(c, NewAPIError(http.StatusBadRequest, "site_entry_invalid_size", "site entry must be nonempty and at most 200 KiB", rid))
			}
		}
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

	state, _ := h.loadSite(c, pc.ID)
	public, publicURL, inherited := h.decoratePublic(c, pc.ID, state, body.Key, body.Kind == systemdb.PublicPathFolder)
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
