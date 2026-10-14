package config

// b1_config_domains_test.go：planv6.0 B1 表征测试（提取前先行）。
// 覆盖计划 §8 B1 第 2 条要求的域语义：未提供值、显式零值、有效非默认值、
// YAML pointer 与 env 空串回退差异、密钥标记（secrets）传递。
// 本文件只锁行为，不改生产逻辑；后续提取 applyYAMLS3 等域函数后必须原样通过。

import (
	"testing"
	"time"
)

// yamlS3For 构造仅含 S3 域的 yamlConfig 输入。
func yamlS3For(addr, key, secret string) yamlConfig {
	var yc yamlConfig
	yc.S3.Endpoint = &addr
	yc.S3.AccessKey = &key
	yc.S3.SecretKey = &secret
	return yc
}

// TestB1_YAMLS3DomainValues：S3 域非指针字段 setStr 语义（非空覆盖、空串不覆盖）
// 与指针字段语义（nil 不覆盖；非 nil 空串覆盖且不置 secrets 标记）。
func TestB1_YAMLS3DomainValues(t *testing.T) {
	cfg := loadFromEnv()
	origEndpoint := cfg.S3.Endpoint

	// 全空输入：不改变任何字段。
	before := cfg.S3
	if _, err := applyYAML(&cfg, yamlConfig{}); err != nil {
		t.Fatal(err)
	}
	if cfg.S3 != before {
		t.Fatalf("empty yaml must not change S3: %+v", cfg.S3)
	}
	if cfg.S3.Endpoint != origEndpoint {
		t.Fatal("endpoint changed")
	}

	// 非空 endpoint 覆盖；非 nil 空密钥覆盖为空串但标记为 false。
	ep := "http://minio:9000"
	empty := ""
	cfg = loadFromEnv()
	secrets, err := applyYAML(&cfg, yamlS3For(ep, empty, empty))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.S3.Endpoint != ep {
		t.Fatalf("endpoint not applied: %s", cfg.S3.Endpoint)
	}
	if cfg.S3.AccessKey != "" || cfg.S3.SecretKey != "" {
		t.Fatalf("explicit empty secrets must overwrite to empty: %q %q", cfg.S3.AccessKey, cfg.S3.SecretKey)
	}
	if secrets.s3Access || secrets.s3Secret {
		t.Fatal("empty secret strings must not set presence flags")
	}

	// 非空密钥：覆盖并置 secrets 标记。
	k := "AKIAEXAMPLE"
	s := "topsecret"
	cfg = loadFromEnv()
	secrets, err = applyYAML(&cfg, yamlS3For("", k, s))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.S3.AccessKey != k || cfg.S3.SecretKey != s {
		t.Fatalf("secrets not applied: %q %q", cfg.S3.AccessKey, cfg.S3.SecretKey)
	}
	if !secrets.s3Access || !secrets.s3Secret {
		t.Fatal("non-empty secrets must set presence flags")
	}
	if cfg.S3.Endpoint != origEndpoint {
		t.Fatal("empty endpoint string must not overwrite")
	}
}

// TestB1_YAMLObservabilityDomain：Observability 域语义——
// nil 指针不覆盖默认；显式空串覆盖为零值（setStr 非 nil 即赋值）。
func TestB1_YAMLObservabilityDomain(t *testing.T) {
	cfg := loadFromEnv()
	if cfg.Observability.LogLevel != "info" {
		t.Fatalf("test precondition: default LogLevel must be info, got %q", cfg.Observability.LogLevel)
	}

	// 显式空串：非 nil 指针覆盖默认值为空串。
	var yc yamlConfig
	empty := ""
	yc.Observability.LogLevel = &empty
	if _, err := applyYAML(&cfg, yc); err != nil {
		t.Fatal(err)
	}
	if cfg.Observability.LogLevel != "" {
		t.Fatalf("explicit empty LogLevel must override to empty: %q", cfg.Observability.LogLevel)
	}

	// 有效非默认值覆盖。
	lvl := "debug"
	cfg = loadFromEnv()
	yc = yamlConfig{}
	yc.Observability.LogLevel = &lvl
	if _, err := applyYAML(&cfg, yc); err != nil {
		t.Fatal(err)
	}
	if cfg.Observability.LogLevel != "debug" {
		t.Fatalf("LogLevel not applied: %s", cfg.Observability.LogLevel)
	}
}

// TestB1_YAMLInstanceLeaseDomain：Instance/Lease 域混合语义——
// 字符串 setStr、布尔 setBool、yamlDur set 标记与显式 nil 判断并存。
func TestB1_YAMLInstanceLeaseDomain(t *testing.T) {
	cfg := loadFromEnv()
	origTTL := cfg.Instance.Lease.TTL

	// 未提供：全部保持默认。
	before := cfg.Instance
	if _, err := applyYAML(&cfg, yamlConfig{}); err != nil {
		t.Fatal(err)
	}
	if cfg.Instance != before {
		t.Fatalf("empty yaml must not change Instance: %+v", cfg.Instance)
	}

	// 显式 TTL=0：set 标记为 true，覆盖为零值（与省略不同）。
	cfg = loadFromEnv()
	var yc yamlConfig
	yc.Instance.Lease.TTL = yamlDur{d: 0, set: true}
	if _, err := applyYAML(&cfg, yc); err != nil {
		t.Fatal(err)
	}
	if cfg.Instance.Lease.TTL != 0 {
		t.Fatalf("explicit zero TTL must override: %v", cfg.Instance.Lease.TTL)
	}
	if origTTL == 0 {
		t.Fatal("test precondition: default Lease.TTL must be non-zero")
	}

	// 有效非默认 TTL 覆盖。
	cfg = loadFromEnv()
	yc = yamlConfig{}
	yc.Instance.Lease.TTL = yamlDur{d: 9 * time.Second, set: true}
	if _, err := applyYAML(&cfg, yc); err != nil {
		t.Fatal(err)
	}
	if cfg.Instance.Lease.TTL != 9*time.Second {
		t.Fatalf("TTL not applied: %v", cfg.Instance.Lease.TTL)
	}
}
