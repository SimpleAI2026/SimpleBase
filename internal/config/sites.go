// 文件说明：sites.go — internal/config 中 SitesConfig（planv6.5 对象存储发布网站）
//
// 用法
//
//	cfg.Sites.BaseDomain / URLScheme / URLPort / IndexDocument / NotFoundDocument / CacheTTL / CSP
//	(s SitesConfig) Configured() bool                        // base_domain 是否已配置
//	(s SitesConfig) SiteBaseURL(projectID string) string  // https://<pid>.<base_domain>[:port]
//	(s SitesConfig) SiteHostLabel(host string) (string, bool) // 从 Host 解析项目 ID（已去端口、小写）
//	ProjectIDPublishable(id string) (reason string)          // "" 表示可作为 DNS 标签发布
//
// 站点没有实例级开关：配置了 base_domain 即可发布，对象默认私有，公开仍需显式授权。

package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

// SitesConfig 控制「对象存储发布网站」：匿名访问已公开的对象。
// 两种 URL 方案（planv6.7 §1）：子域名 <pid>.<base_domain>，或路径 <base>/<path_prefix>/<pid>/<key>。
type SitesConfig struct {
	// Subdomain=true：<pid>.<base_domain> 子域名方式（planv6.5 现有实现）；
	// false（默认）：<base>/<path_prefix>/<pid>/<key> 路径方式，不依赖泛域名。
	Subdomain bool `yaml:"subdomain"`
	// PathPrefix 是路径方式的挂载前缀，默认 "/sites"。
	PathPrefix string `yaml:"path_prefix"`
	// PublicBaseURL 是生成访问 URL 用的外部基址（如 https://api.example.com）；
	// 空时回退 http.public_url，再空时用请求的 scheme+Host（只用于展示）。
	PublicBaseURL string `yaml:"public_base_url"`
	// BaseDomain 例 simplebase.top；本地开发可用 localhost（浏览器把 *.localhost 解析到回环）。仅子域名方式使用。
	BaseDomain string `yaml:"base_domain"`
	// URLScheme 生成访问 URL 用：https（默认）| http。
	URLScheme string `yaml:"url_scheme"`
	// URLPort 非 0 时拼进访问 URL（本地开发）。
	URLPort int `yaml:"url_port"`
	// IndexDocument 目录默认文档，默认 index.html。
	IndexDocument string `yaml:"index_document"`
	// NotFoundDocument 例 404.html；必须同样处于公开路径下才会使用。
	NotFoundDocument string `yaml:"not_found_document"`
	// SpaFallback=true：未命中的无扩展名路径回退到站点根 index 文档（SPA 刷新子路由）。
	SpaFallback bool `yaml:"spa_fallback"`
	// CacheTTL 站点开关 / 公开路径的进程内缓存 TTL，默认 30s。
	CacheTTL time.Duration `yaml:"cache_ttl"`
	// CSP 站点响应的 Content-Security-Policy；空 = 内置默认，off = 不发。
	CSP string `yaml:"csp"`
}

// 站点相关默认值。
const (
	DefaultSiteIndexDocument = "index.html"
	DefaultSiteCacheTTL      = 30 * time.Second
)

// EffectiveIndexDocument 返回目录默认文档。
func (s SitesConfig) EffectiveIndexDocument() string {
	if v := strings.TrimSpace(s.IndexDocument); v != "" {
		return v
	}
	return DefaultSiteIndexDocument
}

// EffectiveCacheTTL 返回缓存 TTL（<=0 取默认）。
func (s SitesConfig) EffectiveCacheTTL() time.Duration {
	if s.CacheTTL <= 0 {
		return DefaultSiteCacheTTL
	}
	return s.CacheTTL
}

// normalizedDomain 返回小写、去首尾点的 base_domain。
func (s SitesConfig) normalizedDomain() string {
	return strings.Trim(strutil.Normalize(s.BaseDomain), ".")
}

// Configured 报告站点域名是否已配置；未配置时不能生成访问 URL。
func (s SitesConfig) Configured() bool {
	return s.normalizedDomain() != ""
}

// 站点 URL 方案。
const (
	SiteModeSubdomain = "subdomain"
	SiteModePath      = "path"

	// DefaultSitePathPrefix 是路径方式的默认挂载前缀。
	DefaultSitePathPrefix = "/sites"
)

// Mode 返回当前站点 URL 方案：subdomain | path。
func (s SitesConfig) Mode() string {
	if s.Subdomain {
		return SiteModeSubdomain
	}
	return SiteModePath
}

// EffectivePathPrefix 返回规范化的路径方式挂载前缀（/xxx，无结尾 "/"）；空串取 /sites。
func (s SitesConfig) EffectivePathPrefix() string {
	v := strings.Trim(strings.TrimSpace(s.PathPrefix), "/")
	if v == "" {
		return DefaultSitePathPrefix
	}
	return "/" + v
}

// PathBaseURL 返回路径方式的项目站点根 URL（不带结尾 "/"）：base + prefix + "/" + pid。
func (s SitesConfig) PathBaseURL(base, projectID string) string {
	return strings.TrimRight(base, "/") + s.EffectivePathPrefix() + "/" + url.PathEscape(projectID)
}

// reservedSitePathPrefixes 是 path_prefix 不得占用的顶层前缀（与 router 的顶层路由对账，见测试）。
var reservedSitePathPrefixes = map[string]bool{
	"v1": true, "go": true, "health": true, "metrics": true, "files": true,
	"console": true, "docs": true, "assets": true, "__site": true, "e2b": true,
	"debug": true, "api": true,
}

// ReservedSitePathPrefix 报告单段前缀是否为保留名。
func ReservedSitePathPrefix(seg string) bool { return reservedSitePathPrefixes[strings.ToLower(seg)] }

// SubdomainBaseURL 返回项目站点根 URL（不带结尾 "/"）；未配置域名返回空串。
func (s SitesConfig) SubdomainBaseURL(projectID string) string {
	domain := s.normalizedDomain()
	if domain == "" {
		return ""
	}
	scheme := strutil.Normalize(s.URLScheme)
	if scheme == "" {
		scheme = "https"
	}
	host := strings.ToLower(projectID) + "." + domain
	if s.URLPort > 0 {
		host = net.JoinHostPort(host, strconv.Itoa(s.URLPort))
	}
	return scheme + "://" + host
}

// SiteHostLabel 从请求 Host 中解析站点项目 ID。只认 <label>.<base_domain> 单级子域，
// label 必须是可发布的项目 ID（8 位 [a-z0-9-]，首尾不为 "-"）。
func (s SitesConfig) SiteHostLabel(host string) (string, bool) {
	domain := s.normalizedDomain()
	if domain == "" || host == "" {
		return "", false
	}
	h := strutil.Normalize(host)
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	h = strings.TrimSuffix(h, ".")
	suffix := "." + domain
	if !strings.HasSuffix(h, suffix) {
		return "", false
	}
	label := strings.TrimSuffix(h, suffix)
	if ProjectIDPublishable(label) != "" {
		return "", false
	}
	return label, true
}

// 不可发布原因（GET /s3/site 的 reason 字段）。
const (
	SiteReasonInvalidID = "project_id_not_dns_safe"
)

// ProjectIDPublishable 判定项目 ID 能否作为 DNS 标签：恰好 8 位小写字母/数字/连字符，首尾不为 "-"。
// 返回空串表示可发布。
func ProjectIDPublishable(id string) string {
	if len(id) != 8 || id[0] == '-' || id[len(id)-1] == '-' {
		return SiteReasonInvalidID
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return SiteReasonInvalidID
		}
	}
	return ""
}

// validateSitePathMode 校验路径方式相关字段（两种方式都校验 public_base_url）。
func validateSitePathMode(s SitesConfig) []error {
	var errs []error
	if s.Mode() == SiteModePath {
		seg := strings.Trim(strings.TrimSpace(s.PathPrefix), "/")
		if seg != "" {
			if !validSitePathSegment(seg) {
				errs = append(errs, fmt.Errorf("sites.path_prefix must be a single segment of [a-z0-9_-] (got %q)", s.PathPrefix))
			} else if ReservedSitePathPrefix(seg) {
				errs = append(errs, fmt.Errorf("sites.path_prefix %q is reserved", seg))
			}
		}
		if strings.EqualFold(strings.TrimSpace(s.CSP), CSPOff) {
			errs = append(errs, errors.New("sites.csp=off is not allowed in path mode (the sandbox CSP is mandatory)"))
		}
	}
	if v := strings.TrimSpace(s.PublicBaseURL); v != "" {
		u, err := url.Parse(v)
		switch {
		case err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https"):
			errs = append(errs, fmt.Errorf("sites.public_base_url must be http(s)://host[:port][/path] (got %q)", s.PublicBaseURL))
		case u.RawQuery != "" || u.Fragment != "" || u.User != nil:
			errs = append(errs, fmt.Errorf("sites.public_base_url must not contain query, fragment or userinfo (got %q)", s.PublicBaseURL))
		}
	}
	return errs
}

func validSitePathSegment(seg string) bool {
	if seg == "" {
		return false
	}
	for _, r := range seg {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func validateSites(s SitesConfig) []error {
	errs := validateSitePathMode(s)
	d := s.normalizedDomain()
	if d == "" {
		// 未配置域名时子域名方式不可发布，但实例仍可正常启动。
		return errs
	}
	if strings.ContainsAny(d, "*/:@ ") {
		errs = append(errs, fmt.Errorf("sites.base_domain must be a bare domain without wildcard, path or port (got %q)", s.BaseDomain))
	}
	switch strutil.Normalize(s.URLScheme) {
	case "", "http", "https":
	default:
		errs = append(errs, fmt.Errorf("sites.url_scheme must be http or https (got %q)", s.URLScheme))
	}
	if s.URLPort < 0 || s.URLPort > 65535 {
		errs = append(errs, fmt.Errorf("sites.url_port must be 0-65535 (got %d)", s.URLPort))
	}
	for _, doc := range []struct{ name, v string }{{"index_document", s.IndexDocument}, {"not_found_document", s.NotFoundDocument}} {
		if strings.Contains(doc.v, "..") || strings.HasPrefix(doc.v, "/") || strings.Contains(doc.v, "\\") {
			errs = append(errs, fmt.Errorf("sites.%s must be a relative file name (got %q)", doc.name, doc.v))
		}
	}
	if s.CacheTTL < 0 {
		errs = append(errs, errors.New("sites.cache_ttl must not be negative"))
	}
	return errs
}

// SitesSameSiteWarning 返回控制台与站点共享注册域时的告警文案（§9 同站风险）；无风险返回空串。
func SitesSameSiteWarning(c Config) string {
	d := c.Sites.normalizedDomain()
	if d == "" || strings.TrimSpace(c.HTTP.PublicURL) == "" {
		return ""
	}
	u, err := url.Parse(c.HTTP.PublicURL)
	if err != nil {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	if h == d || strings.HasSuffix(h, "."+d) {
		return "http.public_url shares the registrable domain with sites.base_domain; user sites are same-site with the console (use a separate console domain, never issue parent-domain cookies)"
	}
	return ""
}
