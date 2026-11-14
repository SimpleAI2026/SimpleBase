// 文件说明：sites.go — internal/config 中 SitesConfig（planv6.5 对象存储发布网站）
//
// 用法
//
//	cfg.Sites.Enabled / BaseDomain / URLScheme / URLPort / IndexDocument / NotFoundDocument / CacheTTL / CSP
//	(s SitesConfig) SiteBaseURL(projectID string) string  // https://<pid>.<base_domain>[:port]
//	(s SitesConfig) SiteHostLabel(host string) (string, bool) // 从 Host 解析项目 ID（已去端口、小写）
//	ProjectIDPublishable(id string) (reason string)          // "" 表示可作为 DNS 标签发布

package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SitesConfig 控制「对象存储发布网站」：<pid>.<base_domain> 匿名访问已公开的对象。
type SitesConfig struct {
	Enabled bool `yaml:"enabled"`
	// BaseDomain 例 simplebase.top；本地开发可用 localhost（浏览器把 *.localhost 解析到回环）。
	BaseDomain string `yaml:"base_domain"`
	// URLScheme 生成访问 URL 用：https（默认）| http。
	URLScheme string `yaml:"url_scheme"`
	// URLPort 非 0 时拼进访问 URL（本地开发）。
	URLPort int `yaml:"url_port"`
	// IndexDocument 目录默认文档，默认 index.html。
	IndexDocument string `yaml:"index_document"`
	// NotFoundDocument 例 404.html；必须同样处于公开路径下才会使用。
	NotFoundDocument string `yaml:"not_found_document"`
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
	return strings.Trim(strings.ToLower(strings.TrimSpace(s.BaseDomain)), ".")
}

// SiteBaseURL 返回项目站点根 URL（不带结尾 "/"）；未启用返回空串。
func (s SitesConfig) SiteBaseURL(projectID string) string {
	domain := s.normalizedDomain()
	if !s.Enabled || domain == "" {
		return ""
	}
	scheme := strings.ToLower(strings.TrimSpace(s.URLScheme))
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
	if !s.Enabled || domain == "" || host == "" {
		return "", false
	}
	h := strings.ToLower(strings.TrimSpace(host))
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

func validateSites(s SitesConfig) []error {
	if !s.Enabled {
		return nil
	}
	var errs []error
	d := s.normalizedDomain()
	if d == "" {
		errs = append(errs, errors.New("sites.base_domain is required when sites.enabled=true"))
	} else if strings.ContainsAny(d, "*/:@ ") {
		errs = append(errs, fmt.Errorf("sites.base_domain must be a bare domain without wildcard, path or port (got %q)", s.BaseDomain))
	}
	switch strings.ToLower(strings.TrimSpace(s.URLScheme)) {
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
	if !c.Sites.Enabled || d == "" || strings.TrimSpace(c.HTTP.PublicURL) == "" {
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
