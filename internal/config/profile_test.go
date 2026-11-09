// 文件说明：profile_test.go — instance.profile 生产默认值注入的表驱动测试（§8.0）
package config

import (
	"os"
	"testing"
	"time"
)

func prodConfig() Config {
	return Config{
		Instance:       InstanceConfig{ID: "t", Writable: true, Profile: ProfileProduction},
		HTTP:           HTTPConfig{Address: ":8080", ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second, ShutdownTimeout: time.Second},
		Storage:        StorageConfig{Database: StorageModeLocal, Files: StorageModeLocal, DataDir: "./data"},
		Database:       DatabaseConfig{Engine: EngineDuckDB, CacheDir: ".cache", CacheMaxBytes: 1, CacheMaxDatabases: 1, IdleTimeout: time.Second, MaxOpen: 1},
		Auth:           AuthConfig{APIKeyHashSecret: "k"},
		SystemDatabase: SystemDatabaseConfig{Name: "s", LogKeepDays: 1, MetricsFlushInterval: time.Second, LogFlushInterval: time.Second},
		Backup:         BackupConfig{Dir: "./backups", KeepLast: 1, MaxPerProject: 2, KeepDays: 1, SnapshotTimeout: time.Minute},
	}
}

func TestApplyProfileDefaults_DevNoop(t *testing.T) {
	cfg := prodConfig()
	cfg.Instance.Profile = ProfileDevelopment
	before := cfg.Limits.RateLimit
	applyProfileDefaults(&cfg, explicitFields{})
	if cfg.Limits.RateLimit.Enabled != before.Enabled || len(cfg.Limits.RateLimit.Rules) != len(before.Rules) {
		t.Fatalf("development profile must not modify config")
	}
	if cfg.Backup.Enabled || cfg.Backup.Schedule != "" {
		t.Fatalf("development profile must not touch backup defaults")
	}
	// 例外（§8.2.3）：会话密钥两档位都派生。
	if cfg.Auth.SessionSecret == "" {
		t.Fatal("session secret should be derived in both profiles")
	}
}

func TestApplyProfileDefaults_ProdFillsDefaults(t *testing.T) {
	cfg := prodConfig()
	cfg.HTTP.PublicURL = "https://api.example.com"
	applyProfileDefaults(&cfg, explicitFields{})
	if !cfg.Limits.RateLimit.Enabled {
		t.Fatal("rate_limit.enabled should default to true in production")
	}
	if len(cfg.Limits.RateLimit.Rules) != 6 {
		t.Fatalf("builtin rules expected 6, got %d", len(cfg.Limits.RateLimit.Rules))
	}
	if cfg.Limits.RateLimit.MaxBuckets != defaultRateLimitMaxBuckets {
		t.Fatalf("max_buckets expected %d, got %d", defaultRateLimitMaxBuckets, cfg.Limits.RateLimit.MaxBuckets)
	}
	if cfg.HTTP.SecurityHeaders.HSTSMaxAge != 31536000 {
		t.Fatalf("hsts_max_age expected 31536000 for https public_url, got %d", cfg.HTTP.SecurityHeaders.HSTSMaxAge)
	}
	if !cfg.Backup.Enabled || cfg.Backup.Schedule != "0 4 * * *" || cfg.Backup.MissedPolicy != MissedPolicyCatchup {
		t.Fatalf("backup defaults wrong: %+v", cfg.Backup)
	}
	if cfg.Auth.SessionSecret == "" || cfg.Auth.SessionSecret == "k" {
		t.Fatal("session secret should be derived from api key secret")
	}
}

func TestApplyProfileDefaults_ExplicitWins(t *testing.T) {
	cfg := prodConfig()
	cfg.Limits.RateLimit.Enabled = false
	cfg.Limits.RateLimit.Rules = []RateLimitRule{{Match: "/v1/x", Max: 1, Window: time.Second}}
	cfg.Backup.Enabled = false
	applyProfileDefaults(&cfg, explicitFields{rateLimitEnabled: true, rateLimitRules: true, backupEnabled: true})
	if cfg.Limits.RateLimit.Enabled {
		t.Fatal("explicit rate_limit.enabled=false must be preserved")
	}
	if len(cfg.Limits.RateLimit.Rules) != 1 {
		t.Fatalf("explicit rules must not be merged with builtin, got %d", len(cfg.Limits.RateLimit.Rules))
	}
	if cfg.Backup.Enabled {
		t.Fatal("explicit backup.enabled=false must be preserved")
	}
}

func TestValidateProduction_MetricsAndBootstrap(t *testing.T) {
	cfg := prodConfig()
	if err := validateProduction(cfg); err == nil {
		t.Fatal("production without metrics_address/token must fail")
	}
	cfg.Observability.MetricsAddress = "127.0.0.1:9090"
	if err := validateProduction(cfg); err == nil {
		t.Fatal("production without bootstrap password must fail")
	}
	cfg.Auth.BootstrapPassword = "short"
	if err := validateProduction(cfg); err == nil {
		t.Fatal("short bootstrap password must fail")
	}
	cfg.Auth.BootstrapPassword = "simplebase2026"
	if err := validateProduction(cfg); err == nil {
		t.Fatal("bootstrap password equal to default must fail")
	}
	cfg.Auth.BootstrapPassword = "a-strong-enough-bootstrap-password"
	if err := validateProduction(cfg); err != nil {
		t.Fatalf("valid production config failed: %v", err)
	}
}

func TestValidateProduction_TrustedProxiesAllRange(t *testing.T) {
	cfg := prodConfig()
	cfg.Limits = LimitsConfig{
		MaxRequestBytes: 1, MaxQueryRows: 1, QueryTimeout: time.Second, MaxConcurrentQueries: 1,
		MaxBatchStatements: 1, MaxSQLBytes: 1,
	}
	cfg.Observability.MetricsToken = "tok"
	cfg.Auth.BootstrapPassword = "a-strong-enough-bootstrap-password"
	cfg.HTTP.TrustedProxies = []string{"0.0.0.0/0"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("0.0.0.0/0 trusted proxy must be rejected in production")
	}
	cfg.HTTP.TrustedProxies = []string{"10.0.0.5/32"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("specific CIDR should pass: %v", err)
	}
}

func TestCollectExplicitFields_EnvWins(t *testing.T) {
	t.Setenv("SIMPLEBASE_BACKUP_ENABLED", "false")
	e := collectExplicitFields(yamlConfig{})
	if !e.backupEnabled {
		t.Fatal("env-set field must be marked explicit")
	}
	os.Unsetenv("SIMPLEBASE_BACKUP_ENABLED")
	e = collectExplicitFields(yamlConfig{})
	if e.backupEnabled {
		t.Fatal("unset env must not mark field explicit")
	}
}

func TestDeriveSessionSecret_StableAndDistinct(t *testing.T) {
	a := DeriveSessionSecret("secret-a")
	if a == "" || a == DeriveSessionSecret("secret-b") {
		t.Fatal("derived session secret must be non-empty and input-dependent")
	}
	if DeriveSessionSecret("secret-a") != a {
		t.Fatal("derivation must be deterministic")
	}
}
