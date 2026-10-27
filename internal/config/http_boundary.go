// 文件说明：http_boundary.go — internal/config 中 CORSConfig、RateLimitConfig 等符号的实现
//
// 用法
//
//	type CORSConfig（导出类型，同包与上层均可使用）
//	type RateLimitConfig（导出类型，同包与上层均可使用）
//	同包调用：CORSConfig → config.go；RateLimitConfig → config.go；FilesConfig → config.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api, internal/api/perfbench 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/config/http_boundary.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backup.go, config.go, env.go, yaml.go
//	[关键引用] CORSConfig ← config.go
//
// 主要符号
//
//	type CORSConfig struct {…}
//	type RateLimitConfig struct {…}
//	type RateLimitRule struct {…}
//	type FilesConfig struct {…}
//	... 另有 1 个顶层符号

package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path"
	"strings"
	"time"
)

type CORSConfig struct {
	Enabled          bool     `yaml:"enabled"`
	AllowedOrigins   []string `yaml:"allowed_origins"`
	AllowedMethods   []string `yaml:"allowed_methods"`
	AllowedHeaders   []string `yaml:"allowed_headers"`
	ExposedHeaders   []string `yaml:"exposed_headers"`
	AllowCredentials bool     `yaml:"allow_credentials"`
	MaxAge           int      `yaml:"max_age"`
}

type RateLimitConfig struct {
	Enabled   bool            `yaml:"enabled"`
	Rules     []RateLimitRule `yaml:"rules"`
	Burst     int             `yaml:"burst"`
	ExemptSSE bool            `yaml:"exempt_sse"`
}

type RateLimitRule struct {
	Match  string        `yaml:"match"`
	Max    int           `yaml:"max"`
	Window time.Duration `yaml:"window"`
}

type FilesConfig struct {
	TokenSecret string   `yaml:"token_secret"`
	AllowedMIME []string `yaml:"allowed_mime"`
}

func validateHTTPBoundary(c Config) []error {
	var errs []error
	for _, origin := range c.HTTP.CORS.AllowedOrigins {
		if origin == "*" {
			if c.HTTP.CORS.AllowCredentials {
				errs = append(errs, errors.New("http.cors.allowed_origins cannot contain * when allow_credentials is true"))
			}
			continue
		}
		u, err := url.Parse(origin)
		if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			errs = append(errs, fmt.Errorf("http.cors.allowed_origins contains invalid origin %q", origin))
		}
	}
	for _, cidr := range c.HTTP.TrustedProxies {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			errs = append(errs, fmt.Errorf("http.trusted_proxies contains invalid CIDR %q", cidr))
		}
	}
	if c.HTTP.CORS.MaxAge < 0 {
		errs = append(errs, errors.New("http.cors.max_age must not be negative"))
	}
	if c.Limits.RateLimit.Burst < 0 {
		errs = append(errs, errors.New("limits.rate_limit.burst must not be negative"))
	}
	for _, rule := range c.Limits.RateLimit.Rules {
		if rule.Match == "" || !strings.HasPrefix(rule.Match, "/") {
			errs = append(errs, errors.New("limits.rate_limit.rules.match must be an absolute path pattern"))
		} else if _, err := path.Match(rule.Match, "/"); err != nil {
			errs = append(errs, fmt.Errorf("invalid rate limit match %q", rule.Match))
		}
		if rule.Max <= 0 || rule.Window <= 0 {
			errs = append(errs, fmt.Errorf("rate limit %q requires positive max and window", rule.Match))
		}
	}
	return errs
}
