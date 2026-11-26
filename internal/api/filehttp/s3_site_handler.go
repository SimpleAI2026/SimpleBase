package filehttp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

// httpkit.ErrSiteNotPublishable 表示项目当前不能发布网站（未配置域或前缀、系统项目、或子域名方式下 ID 不可作 DNS 标签）。

// delegatedPublicPrefix 是云助手委托令牌唯一允许公开的对象前缀。
const delegatedPublicPrefix = "web/"

// siteReasonUnavailable 是路径方式 + 站点功能未配置的原因码。
const siteReasonUnavailable = "site_unavailable"

// 不可发布原因（只在子域名方式返回）。
const (
	SiteReasonNoDomain      = "site_domain_unconfigured"
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
		return SiteReasonNoDomain
	}
	return config.ProjectIDPublishable(projectID)
}

// siteBase 返回项目站点根 URL（不带结尾 "/"，按当前模式生成）。planv6.7 §1.3。
// 路径方式依赖传入的 context 以取得公共 baseURL。
func (h *S3Handler) siteBase(c echo.Context, projectID string) string {
	if h.sitesCfg.Mode() == config.SiteModeSubdomain {
		return h.sitesCfg.SubdomainBaseURL(projectID)
	}
	return h.sitesCfg.PathBaseURL(httpkit.PublicBaseURL(c, h.cfgHTTPPublicURL, h.sitesCfg.PublicBaseURL), projectID)
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
func (h *S3Handler) loadSite(c echo.Context, projectID string) (SiteState, siteInfoDTO) {
	reason := h.siteReason(projectID)
	info := siteInfoDTO{Publishable: reason == "", Reason: reason, Mode: h.sitesCfg.Mode()}
	if reason != "" {
		return SiteState{}, info
	}
	state, err := h.sites.Lookup(c.Request().Context(), projectID)
	if err != nil {
		return SiteState{}, info
	}
	info.BaseURL = h.siteBase(c, projectID)
	return state, info
}

// decoratePublic 为对象/文件夹补充 public / publicUrl / publicInherited 字段。
func (h *S3Handler) decoratePublic(c echo.Context, projectID string, state SiteState, key string, isFolder bool) (bool, string, string) {
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
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	_, info := h.loadSite(c, pc.ID)
	return c.JSON(http.StatusOK, info)
}

// ListPublic 列出公开路径。
func (h *S3Handler) ListPublic(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
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
	if !h.writable {
		return httpkit.WriteError(c, godatabase.ErrWriterUnavailable)
	}
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if reason := h.siteReason(pc.ID); reason != "" {
		return httpkit.WriteError(c, httpkit.ErrSiteNotPublishable)
	}
	var body struct {
		Key    string `json:"key"`
		Kind   string `json:"kind"`
		Public *bool  `json:"public"`
	}
	if err := httpkit.DecodeJSONBody(c, &body, false); err != nil {
		return httpkit.WriteError(c, err)
	}
	rid := httpkit.RequestID(c)
	if body.Public == nil {
		return httpkit.WriteError(c, httpkit.BadRequest(c, "public is required"))
	}
	// 委托主体（云助手）只能开启 web/ 文件夹：禁止公开对象根或其它目录，
	// 即使模型绕过 CLI 的前缀检查，服务端也在这里兜底。
	if p, ok := httpkit.PrincipalFromContext(c.Request().Context()); ok && p.Delegated {
		if body.Kind != systemdb.PublicPathFolder || body.Key != delegatedPublicPrefix {
			return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusForbidden, "delegated_public_prefix_only", "assistant may only publish the "+delegatedPublicPrefix+" folder", rid))
		}
		if *body.Public {
			store, ok := h.store.(objectstore.ContentStore)
			if !ok {
				return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusServiceUnavailable, "site_content_unavailable", "cannot validate site entry", rid))
			}
			entry, err := store.GetContent(c.Request().Context(), JoinKey(pc.ID, delegatedPublicPrefix+h.sitesCfg.EffectiveIndexDocument()), "")
			if err != nil {
				return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusBadRequest, "site_entry_unavailable", "upload a readable site entry before publishing", rid))
			}
			_ = entry.Body.Close()
			if entry.Size <= 0 || entry.Size > 200*1024 {
				return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusBadRequest, "site_entry_invalid_size", "site entry must be nonempty and at most 200 KiB", rid))
			}
		}
	}
	switch body.Kind {
	case systemdb.PublicPathFile:
		if strings.HasSuffix(body.Key, "/") {
			return httpkit.WriteError(c, httpkit.BadRequest(c, "file key must not end with /"))
		}
		if err := objectstore.ValidateFileKey(body.Key); err != nil {
			return httpkit.WriteError(c, err)
		}
	case systemdb.PublicPathFolder:
		if body.Key != "" {
			if !strings.HasSuffix(body.Key, "/") {
				return httpkit.WriteError(c, httpkit.BadRequest(c, "folder key must end with /"))
			}
			if err := objectstore.ValidateFileKey(strings.TrimSuffix(body.Key, "/")); err != nil {
				return httpkit.WriteError(c, err)
			}
		}
	default:
		return httpkit.WriteError(c, httpkit.BadRequest(c, "kind must be file or folder"))
	}
	actor := httpkit.PrincipalOf(c).Actor()
	if err := h.sites.SetPublicPath(c.Request().Context(), pc.ID, body.Key, body.Kind, *body.Public, actor); err != nil {
		return httpkit.WriteError(c, err)
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
// PutSiteQuota 设置项目的站点每日请求 / 出口配额（0 表示不限），只允许系统管理员。
func (h *S3Handler) PutSiteQuota(c echo.Context) error {
	p := httpkit.PrincipalOf(c)
	if !p.Role.IsAdmin() && !p.Role.IsSuper() {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusForbidden, "forbidden", "admin role required", httpkit.RequestID(c)))
	}
	if h.traffic == nil {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusServiceUnavailable, "site_quota_unavailable", "site quota is not configured", httpkit.RequestID(c)))
	}
	var req struct {
		MaxRequests    int64 `json:"max_requests"`
		MaxEgressBytes int64 `json:"max_egress_bytes"`
	}
	if err := c.Bind(&req); err != nil || req.MaxRequests < 0 || req.MaxEgressBytes < 0 {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusBadRequest, "invalid_request", "max_requests and max_egress_bytes must be >= 0", httpkit.RequestID(c)))
	}
	projectID := c.Param("projectID")
	if err := h.traffic.SetSiteQuota(c.Request().Context(), projectID, req.MaxRequests, req.MaxEgressBytes); err != nil {
		return httpkit.WriteError(c, err)
	}
	h.recordSiteAudit(p.Actor(), projectID, "site_quota_update", fmt.Sprintf("max_requests=%d max_egress_bytes=%d", req.MaxRequests, req.MaxEgressBytes))
	return c.JSON(http.StatusOK, map[string]any{
		"projectID":        projectID,
		"max_requests":     req.MaxRequests,
		"max_egress_bytes": req.MaxEgressBytes,
	})
}

// TakedownSite 让系统管理员一次关闭某个项目的全部公开路径；路径记录保留但 public=false。
func (h *S3Handler) TakedownSite(c echo.Context) error {
	p := httpkit.PrincipalOf(c)
	if !p.Role.IsAdmin() && !p.Role.IsSuper() {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusForbidden, "forbidden", "admin role required", httpkit.RequestID(c)))
	}
	if h.sites == nil {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusServiceUnavailable, "site_unavailable", "site publishing is not configured", httpkit.RequestID(c)))
	}
	projectID := c.Param("projectID")
	state, err := h.sites.Lookup(c.Request().Context(), projectID)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	actor := p.Actor()
	disabled := 0
	for _, path := range state.Paths {
		if err := h.sites.SetPublicPath(c.Request().Context(), projectID, path.Path, path.Kind, false, actor); err != nil {
			return httpkit.WriteError(c, err)
		}
		disabled++
	}
	h.recordSiteAudit(actor, projectID, "site_takedown", fmt.Sprintf("disabled_paths=%d", disabled))
	return c.JSON(http.StatusOK, map[string]any{"projectID": projectID, "disabled": disabled})
}

func (h *S3Handler) recordSiteAudit(actor, projectID, kind, detail string) {
	if h.index == nil {
		return
	}
	h.index.RecordAudit(systemdb.AuditEvent{Actor: actor, ProjectID: projectID, Kind: kind, Detail: detail})
}
