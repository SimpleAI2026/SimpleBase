// 文件说明：profile.go — instance.profile=production 时的安全默认值注入（production-readiness-plan §8.0）
//
// 用法
//
//	applyProfileDefaults(cfg *Config, explicit explicitFields)：只在用户未显式设置时填生产默认值。
//	deriveSessionSecret(apiKeySecret string) string：会话密钥派生（§8.2.3）。
//
// 约束：不新增第三方依赖；密钥不落 YAML；显式值优先于生产默认值。
package config

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// sessionSecretDerivationLabel 是从 API Key 盐派生会话密钥的域分离标签（§8.2.3）。
const sessionSecretDerivationLabel = "simplebase-session-v1"

// DeriveSessionSecret 从 API Key 盐派生会话 JWT 密钥。
// 未显式配置 SIMPLEBASE_AUTH_SESSION_SECRET 时使用；轮换 Key 盐会使派生值变化（文档已写明）。
func DeriveSessionSecret(apiKeySecret string) string {
	mac := hmac.New(sha256.New, []byte(sessionSecretDerivationLabel))
	mac.Write([]byte(apiKeySecret))
	return hex.EncodeToString(mac.Sum(nil))
}

// explicitFields 记录「用户显式设置过」的字段集合；生产默认值只填其余字段，
// 避免「用户显式写 rate_limit.enabled: false 被生产模式悄悄改成 true」。
type explicitFields struct {
	rateLimitEnabled   bool
	rateLimitRules     bool
	rateMaxBuckets     bool
	backupEnabled      bool
	backupSchedule     bool
	hstsMaxAge         bool
	csp                bool
	metricsAddress     bool
	metricsToken       bool
	backupMissedPolicy bool
}

// collectExplicitFields 汇总两路显式来源：YAML 指针字段非 nil + 环境变量命中。
func collectExplicitFields(yc yamlConfig) explicitFields {
	e := explicitFields{
		rateLimitEnabled:   yc.Limits.RateLimit.Enabled != nil,
		rateLimitRules:     yc.Limits.RateLimit.Rules != nil,
		rateMaxBuckets:     yc.Limits.RateLimit.MaxBuckets != nil,
		backupEnabled:      yc.Backup.Enabled != nil,
		backupSchedule:     yc.Backup.Schedule != nil,
		hstsMaxAge:         yc.HTTP.SecurityHeaders.HSTSMaxAge != nil,
		csp:                yc.HTTP.SecurityHeaders.CSP != nil,
		metricsAddress:     yc.Observability.MetricsAddress != nil,
		metricsToken:       yc.Observability.MetricsToken != nil,
		backupMissedPolicy: yc.Backup.MissedPolicy != nil,
	}
	for _, key := range []string{
		"SIMPLEBASE_RATE_LIMIT_ENABLED",
		"SIMPLEBASE_RATE_LIMIT_MAX_BUCKETS",
		"SIMPLEBASE_BACKUP_ENABLED",
		"SIMPLEBASE_BACKUP_SCHEDULE",
		"SIMPLEBASE_BACKUP_MISSED_POLICY",
		"SIMPLEBASE_HTTP_HSTS_MAX_AGE",
		"SIMPLEBASE_HTTP_SECURITY_HEADERS_CSP",
		"SIMPLEBASE_METRICS_ADDRESS",
		"SIMPLEBASE_METRICS_TOKEN",
	} {
		if _, ok := os.LookupEnv(key); ok {
			switch key {
			case "SIMPLEBASE_RATE_LIMIT_ENABLED":
				e.rateLimitEnabled = true
			case "SIMPLEBASE_RATE_LIMIT_MAX_BUCKETS":
				e.rateMaxBuckets = true
			case "SIMPLEBASE_BACKUP_ENABLED":
				e.backupEnabled = true
			case "SIMPLEBASE_BACKUP_SCHEDULE":
				e.backupSchedule = true
			case "SIMPLEBASE_BACKUP_MISSED_POLICY":
				e.backupMissedPolicy = true
			case "SIMPLEBASE_HTTP_HSTS_MAX_AGE":
				e.hstsMaxAge = true
			case "SIMPLEBASE_HTTP_SECURITY_HEADERS_CSP":
				e.csp = true
			case "SIMPLEBASE_METRICS_ADDRESS":
				e.metricsAddress = true
			case "SIMPLEBASE_METRICS_TOKEN":
				e.metricsToken = true
			}
		}
	}
	return e
}

// productionRateLimitRules 是 production 下用户未写规则时的内置限流规则（§8.1.6）。
// §3.1 表中的内置限流规则以本表为准（单一来源，避免漂移）。
func productionRateLimitRules() []RateLimitRule {
	return []RateLimitRule{
		{Match: "/v1/auth/refresh", Key: RateLimitKeyIP, Max: 30, Window: time.Minute},
		{Match: "/files/**", Key: RateLimitKeyIP, Max: 300, Window: time.Minute},
		{Match: "/go/**", Key: RateLimitKeyPrincipal, Max: 600, Window: time.Minute},
		{Match: "/v1/projects/*/v1/**", Key: RateLimitKeyPrincipal, Max: 60, Window: time.Minute},
		{Match: "/v1/projects/*/end-users/signup", Key: RateLimitKeyIP, Max: 5, Window: time.Minute},
		{Match: "/v1/**", Key: RateLimitKeyPrincipal, Max: 1200, Window: time.Minute},
	}
}

const defaultRateLimitMaxBuckets = 100000

// applyProfileDefaults 在 production 档位下注入安全默认值。
// 只填用户没有显式设置的字段；development 档位不做任何改动。
// 例外：会话密钥派生（§8.2.3）两档位都执行——dev 也用独立密钥域，
// 保证「显式配置 SIMPLEBASE_AUTH_SESSION_SECRET」与「派生」行为一致。
func applyProfileDefaults(cfg *Config, e explicitFields) {
	// 会话密钥未显式配置时由 Key 盐派生（§8.2.3）；readiness 提示 inherited。
	if cfg.Auth.SessionSecret == "" && cfg.Auth.APIKeyHashSecret != "" {
		cfg.Auth.SessionSecret = DeriveSessionSecret(cfg.Auth.APIKeyHashSecret)
		cfg.Auth.SessionSecretExplicit = false
	} else if cfg.Auth.SessionSecret != "" {
		cfg.Auth.SessionSecretExplicit = true
	}
	if !cfg.IsProduction() {
		return
	}
	// §8.1.6：限流默认开启 + 内置规则（用户写了规则则完全以用户为准，不合并）。
	if !e.rateLimitEnabled && !cfg.Limits.RateLimit.Enabled {
		cfg.Limits.RateLimit.Enabled = true
	}
	if !e.rateLimitRules && len(cfg.Limits.RateLimit.Rules) == 0 {
		cfg.Limits.RateLimit.Rules = productionRateLimitRules()
	}
	if !e.rateMaxBuckets && cfg.Limits.RateLimit.MaxBuckets == 0 {
		cfg.Limits.RateLimit.MaxBuckets = defaultRateLimitMaxBuckets
	}
	// §8.1.3：HSTS —— production 且 public_url 为 https 时默认 31536000 秒。
	if !e.hstsMaxAge && cfg.HTTP.SecurityHeaders.HSTSMaxAge == 0 {
		if isHTTPSURL(cfg.HTTP.PublicURL) {
			cfg.HTTP.SecurityHeaders.HSTSMaxAge = 31536000
		}
	}
	// §8.1.3：development 默认 report-only 由 defaults 补，production 空 = enforce。
	if !e.csp && cfg.HTTP.SecurityHeaders.CSP == "" {
		cfg.HTTP.SecurityHeaders.CSP = CSPEnforce
	}
	// §8.3.2：备份默认开启 + 每日 04:00 + catchup。
	if !e.backupEnabled && !cfg.Backup.Enabled {
		cfg.Backup.Enabled = true
	}
	if !e.backupSchedule && cfg.Backup.Schedule == "" {
		cfg.Backup.Schedule = "0 4 * * *"
	}
	if !e.backupMissedPolicy && cfg.Backup.MissedPolicy == "" {
		cfg.Backup.MissedPolicy = MissedPolicyCatchup
	}
}

// validateProduction 校验 production 档位下的硬性要求（§8.1.7 / §8.2.2）。
// development 档位直接放行。
func validateProduction(cfg Config) error {
	if !cfg.IsProduction() {
		return nil
	}
	var errs []error
	// §8.1.7：观测端点必须收口——独立监听或 token 至少配一个。
	if cfg.Observability.MetricsAddress == "" && cfg.Observability.MetricsToken == "" {
		errs = append(errs, errors.New("production requires observability.metrics_address or SIMPLEBASE_METRICS_TOKEN to protect /metrics"))
	}
	// §8.2.2：引导口令必须注入。
	if strings.TrimSpace(cfg.Auth.BootstrapPassword) == "" {
		errs = append(errs, errors.New("production requires SIMPLEBASE_BOOTSTRAP_PASSWORD for the bootstrap admin"))
	} else if len(cfg.Auth.BootstrapPassword) < 16 || cfg.Auth.BootstrapPassword == "simplebase2026" {
		errs = append(errs, errors.New("SIMPLEBASE_BOOTSTRAP_PASSWORD must be >= 16 chars and not the default"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("production profile validation failed: %w", errors.Join(errs...))
	}
	return nil
}

// isHTTPSURL 判断 URL 是否以 https:// 开头（大小写不敏感、容忍首尾空白）。
func isHTTPSURL(u string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(u)), "https://")
}
