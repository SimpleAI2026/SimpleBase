package config

// r2_yaml_domains_test.go：planv6.0 R2-B1 表征测试（提取前先行）。
// 锁定 HTTP/CORS、Limits/RateLimit、Sandbox 三个 YAML 域的应用语义：
// nil 不覆盖、非 nil（含空切片）覆盖、切片按值拷贝不与输入别名、
// RateLimit 规则整体替换且未给字段为零值。

import (
	"reflect"
	"testing"
	"time"
)

func TestR2_YAMLHTTPDomain(t *testing.T) {
	base := loadFromEnv()

	// 未提供：HTTP 整段不变。
	cfg := loadFromEnv()
	if _, err := applyYAML(&cfg, yamlConfig{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.HTTP, base.HTTP) {
		t.Fatalf("empty yaml changed HTTP: %+v", cfg.HTTP)
	}

	addr := ":9999"
	gz := true
	maxAge := 0
	var yc yamlConfig
	yc.HTTP.Address = &addr
	yc.HTTP.Gzip = &gz
	yc.HTTP.ReadTimeout = yamlDur{d: 3 * time.Second, set: true}
	yc.HTTP.TrustedProxies = []string{"10.0.0.0/8"}
	yc.HTTP.CORS.AllowedOrigins = []string{} // 非 nil 空切片：清空默认值
	yc.HTTP.CORS.AllowedMethods = []string{"GET"}
	yc.HTTP.CORS.MaxAge = &maxAge

	cfg = loadFromEnv()
	if _, err := applyYAML(&cfg, yc); err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Address != addr || !cfg.HTTP.Gzip || cfg.HTTP.ReadTimeout != 3*time.Second {
		t.Fatalf("scalar fields not applied: %+v", cfg.HTTP)
	}
	if cfg.HTTP.CORS.MaxAge != 0 {
		t.Fatalf("explicit zero MaxAge must override: %d", cfg.HTTP.CORS.MaxAge)
	}
	// append([]string(nil), empty...) 结果为 nil：非 nil 空切片把默认值清空为 nil。
	if cfg.HTTP.CORS.AllowedOrigins != nil {
		t.Fatalf("non-nil empty origins must clear to nil: %#v", cfg.HTTP.CORS.AllowedOrigins)
	}
	if !reflect.DeepEqual(cfg.HTTP.CORS.AllowedHeaders, base.HTTP.CORS.AllowedHeaders) {
		t.Fatal("nil headers must keep default")
	}
	// 拷贝语义：修改输入不影响 cfg。
	yc.HTTP.TrustedProxies[0] = "mutated"
	yc.HTTP.CORS.AllowedMethods[0] = "mutated"
	if cfg.HTTP.TrustedProxies[0] != "10.0.0.0/8" || cfg.HTTP.CORS.AllowedMethods[0] != "GET" {
		t.Fatal("slices must be copied, not aliased")
	}
	if cfg.HTTP.WriteTimeout != base.HTTP.WriteTimeout {
		t.Fatal("unset duration must keep default")
	}
}

func TestR2_YAMLLimitsDomain(t *testing.T) {
	base := loadFromEnv()

	cfg := loadFromEnv()
	if _, err := applyYAML(&cfg, yamlConfig{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Limits, base.Limits) {
		t.Fatalf("empty yaml changed Limits: %+v", cfg.Limits)
	}

	rows := 7
	m := "/v1/*"
	mx := 5
	var yc yamlConfig
	yc.Limits.MaxQueryRows = &rows
	yc.Limits.RateLimit.Rules = append(yc.Limits.RateLimit.Rules,
		struct {
			Match  *string `yaml:"match"`
			Max    *int    `yaml:"max"`
			Window yamlDur `yaml:"window"`
			Key    *string `yaml:"key"`
		}{Match: &m, Max: &mx, Window: yamlDur{d: time.Minute, set: true}},
		struct {
			Match  *string `yaml:"match"`
			Max    *int    `yaml:"max"`
			Window yamlDur `yaml:"window"`
			Key    *string `yaml:"key"`
		}{},
	)
	cfg = loadFromEnv()
	if _, err := applyYAML(&cfg, yc); err != nil {
		t.Fatal(err)
	}
	if cfg.Limits.MaxQueryRows != 7 {
		t.Fatalf("MaxQueryRows=%d", cfg.Limits.MaxQueryRows)
	}
	want := []RateLimitRule{{Match: m, Max: 5, Window: time.Minute}, {}}
	if !reflect.DeepEqual(cfg.Limits.RateLimit.Rules, want) {
		t.Fatalf("rules=%+v", cfg.Limits.RateLimit.Rules)
	}
	if cfg.Limits.MaxSQLBytes != base.Limits.MaxSQLBytes {
		t.Fatal("unset limit must keep default")
	}
}

func TestR2_YAMLSandboxDomain(t *testing.T) {
	base := loadFromEnv()

	cfg := loadFromEnv()
	if _, err := applyYAML(&cfg, yamlConfig{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Sandbox, base.Sandbox) {
		t.Fatalf("empty yaml changed Sandbox: %+v", cfg.Sandbox)
	}

	en := true
	img := "alpine:3"
	tmpl := "base"
	var yc yamlConfig
	yc.Sandbox.Enabled = &en
	yc.Sandbox.Image = &img
	yc.Sandbox.Images = []string{"a", "b"}
	yc.Sandbox.E2B.DefaultTemplate = &tmpl
	yc.Sandbox.Terminal.IdleTimeout = yamlDur{d: 0, set: true}
	cfg = loadFromEnv()
	if _, err := applyYAML(&cfg, yc); err != nil {
		t.Fatal(err)
	}
	if !cfg.Sandbox.Enabled || cfg.Sandbox.Image != img || cfg.Sandbox.E2B.DefaultTemplate != tmpl {
		t.Fatalf("sandbox scalars not applied: %+v", cfg.Sandbox)
	}
	if cfg.Sandbox.Terminal.IdleTimeout != 0 {
		t.Fatal("explicit zero terminal idle must override")
	}
	yc.Sandbox.Images[0] = "mutated"
	if !reflect.DeepEqual(cfg.Sandbox.Images, []string{"a", "b"}) {
		t.Fatalf("images must be copied: %v", cfg.Sandbox.Images)
	}
	if cfg.Sandbox.CPUs != base.Sandbox.CPUs {
		t.Fatal("unset CPUs must keep default")
	}
}
