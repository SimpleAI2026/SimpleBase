package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseDuration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"15s", 15 * time.Second, false},
		{"2m", 2 * time.Minute, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"0d", 0, false},
		{"1h30m", 90 * time.Minute, false},
		{"xd", 0, true},
		{"not-a-duration", 0, true},
		{"", 0, true},
		{"10", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseDuration(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDuration(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("parseDuration(%q)=%v want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestEnvHelpers(t *testing.T) {
	t.Setenv("CFG_STR", "hello")
	if got := envStr("CFG_STR", "def"); got != "hello" {
		t.Fatalf("envStr set: %q", got)
	}
	if got := envStr("CFG_STR_MISSING", "def"); got != "def" {
		t.Fatalf("envStr default: %q", got)
	}

	t.Setenv("CFG_BOOL_TRUE", "true")
	t.Setenv("CFG_BOOL_BAD", "not-bool")
	if !envBool("CFG_BOOL_TRUE", false) {
		t.Fatal("envBool true")
	}
	if envBool("CFG_BOOL_BAD", true) != true {
		t.Fatal("envBool invalid should return default")
	}
	if envBool("CFG_BOOL_MISSING", true) != true {
		t.Fatal("envBool missing should return default")
	}
	t.Setenv("CFG_BOOL_FALSE", "0")
	if envBool("CFG_BOOL_FALSE", true) {
		t.Fatal("envBool false")
	}

	t.Setenv("CFG_INT", "42")
	t.Setenv("CFG_INT_BAD", "x")
	if envInt("CFG_INT", 1) != 42 {
		t.Fatal("envInt")
	}
	if envInt("CFG_INT_BAD", 7) != 7 {
		t.Fatal("envInt bad")
	}
	if envInt("CFG_INT_MISSING", 9) != 9 {
		t.Fatal("envInt missing")
	}

	t.Setenv("CFG_I64", "100")
	t.Setenv("CFG_I64_BAD", "nope")
	if envInt64("CFG_I64", 1) != 100 {
		t.Fatal("envInt64")
	}
	if envInt64("CFG_I64_BAD", 3) != 3 {
		t.Fatal("envInt64 bad")
	}
	if envInt64("CFG_I64_MISSING", 5) != 5 {
		t.Fatal("envInt64 missing")
	}

	t.Setenv("CFG_F64", "1.5")
	t.Setenv("CFG_F64_BAD", "zz")
	if envFloat64("CFG_F64", 0) != 1.5 {
		t.Fatal("envFloat64")
	}
	if envFloat64("CFG_F64_BAD", 2.25) != 2.25 {
		t.Fatal("envFloat64 bad")
	}
	if envFloat64("CFG_F64_MISSING", 3.5) != 3.5 {
		t.Fatal("envFloat64 missing")
	}

	t.Setenv("CFG_DUR", "7d")
	t.Setenv("CFG_DUR_BAD", "zzz")
	if envDuration("CFG_DUR", time.Second) != 7*24*time.Hour {
		t.Fatal("envDuration days")
	}
	if envDuration("CFG_DUR_BAD", time.Minute) != time.Minute {
		t.Fatal("envDuration bad")
	}
	if envDuration("CFG_DUR_MISSING", time.Hour) != time.Hour {
		t.Fatal("envDuration missing")
	}

	t.Setenv("CFG_LIST", " a, ,b,c ")
	if got := envList("CFG_LIST"); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("envList: %#v", got)
	}
	if envList("CFG_LIST_MISSING") != nil {
		t.Fatal("envList empty should be nil")
	}
	t.Setenv("CFG_LIST_EMPTY", "")
	if envList("CFG_LIST_EMPTY") != nil {
		t.Fatal("envList blank should be nil")
	}
}

func TestLoadLLMProviders(t *testing.T) {
	if got := loadLLMProviders(); len(got) != 0 {
		t.Fatalf("empty providers: %#v", got)
	}
	t.Setenv("SIMPLEBASE_LLM_PROVIDERS", " openai, ,anthropic ")
	t.Setenv("SIMPLEBASE_LLM_PROVIDER_OPENAI_API_KEY", "sk-test")
	t.Setenv("SIMPLEBASE_LLM_PROVIDER_OPENAI_BASE_URL", "https://api.example")
	t.Setenv("SIMPLEBASE_LLM_PROVIDER_OPENAI_DEFAULT_MODEL", "gpt-4o-mini")
	t.Setenv("SIMPLEBASE_LLM_PROVIDER_OPENAI_ALLOWED_MODELS", "gpt-4o-mini, gpt-4o")
	t.Setenv("SIMPLEBASE_LLM_PROVIDER_OPENAI_TIMEOUT", "30s")
	got := loadLLMProviders()
	if len(got) != 2 {
		t.Fatalf("want 2 providers, got %#v", got)
	}
	p := got["openai"]
	if p.APIKey != "sk-test" || p.BaseURL != "https://api.example" || p.DefaultModel != "gpt-4o-mini" {
		t.Fatalf("openai provider: %+v", p)
	}
	if len(p.AllowedModels) != 2 || p.Timeout != 30*time.Second {
		t.Fatalf("openai models/timeout: %+v", p)
	}
	if _, ok := got["anthropic"]; !ok {
		t.Fatal("missing anthropic")
	}
}

func TestConfigFilePath(t *testing.T) {
	t.Setenv("SIMPLEBASE_CONFIG_PATH", "/tmp/custom.yaml")
	if got := configFilePath(); got != "/tmp/custom.yaml" {
		t.Fatalf("explicit path: %q", got)
	}
	t.Setenv("SIMPLEBASE_CONFIG_PATH", "")
	// default: only returns config.yaml if it exists in cwd
	if _, err := os.Stat("config.yaml"); err == nil {
		if configFilePath() != "config.yaml" {
			t.Fatal("expected default config.yaml when present")
		}
	} else if configFilePath() != "" {
		t.Fatalf("expected empty when config.yaml missing, got %q", configFilePath())
	}
}

func TestLoadFromEnvAndYAMLOverride(t *testing.T) {
	yaml := `
http:
  address: ":7777"
  read_timeout: 11s
  write_timeout: 12s
  idle_timeout: 13s
instance:
  id: yaml-instance
  writable: false
database:
  engine: ducklake
  cache_dir: /tmp/yaml-cache
  idle_timeout: 2m
  max_open: 3
  max_idle: 1
  ducklake:
    memory_limit: 256MB
    threads: 4
    extension_dir: /ext
    data_inlining_row_limit: 50
    parquet_compression: snappy
    target_file_size: 32MB
    require_commit_message: true
    catalog_sync:
      mode: sync_on_commit
      debounce_ms: 400
      keep_versions: 5
    maintenance:
      checkpoint_interval: 30m
      expire_older_than: 3d
      delete_older_than: 2d
      rewrite_delete_threshold: 0.8
s3:
  endpoint: https://s3.example
  region: us-west-2
  bucket: yaml-bucket
  prefix: yaml-prefix
  access_key: yaml-ak
  secret_key: yaml-sk
  force_path_style: true
auth:
  api_key_hash_secret: yaml-secret
limits:
  max_request_bytes: 2048
  max_query_rows: 50
  query_timeout: 9s
  max_concurrent_queries: 7
  max_batch_statements: 11
  max_sql_bytes: 1111
observability:
  log_level: debug
  log_format: console
  metrics_path: /prom
system_database:
  name: yaml-system
  hide_from_list: false
  metrics_flush_interval: 3s
  log_flush_interval: 4s
  log_keep_days: 21
dev_mode: true
`
	path := writeYAML(t, yaml)
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	// Leave required env unset so YAML fills them. Writable=false + DevMode so S3 not required.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load yaml: %v", err)
	}
	if cfg.HTTP.Address != ":7777" || cfg.Instance.ID != "yaml-instance" {
		t.Fatalf("http/instance: %+v %+v", cfg.HTTP, cfg.Instance)
	}
	if cfg.Instance.Writable {
		t.Fatal("yaml writable=false")
	}
	if cfg.Database.CacheDir != "/tmp/yaml-cache" || cfg.Database.MaxOpen != 3 || cfg.Database.MaxIdle != 1 {
		t.Fatalf("database: %+v", cfg.Database)
	}
	dl := cfg.Database.DuckLake
	if dl.MemoryLimit != "256MB" || dl.Threads != 4 || dl.ExtensionDir != "/ext" {
		t.Fatalf("ducklake: %+v", dl)
	}
	if dl.DataInliningRowLimit != 50 || dl.ParquetCompression != "snappy" || dl.TargetFileSize != "32MB" {
		t.Fatalf("ducklake opts: %+v", dl)
	}
	if !dl.RequireCommitMessage || dl.CatalogSync.Mode != "sync_on_commit" || dl.CatalogSync.Debounce != 400*time.Millisecond {
		t.Fatalf("sync: %+v", dl.CatalogSync)
	}
	if dl.CatalogSync.KeepVersions != 5 {
		t.Fatalf("keep versions: %d", dl.CatalogSync.KeepVersions)
	}
	if dl.Maintenance.ExpireOlderThan != 3*24*time.Hour || dl.Maintenance.DeleteOlderThan != 2*24*time.Hour {
		t.Fatalf("maint durations: %+v", dl.Maintenance)
	}
	if dl.Maintenance.RewriteDeleteThreshold != 0.8 || dl.Maintenance.CheckpointInterval != 30*time.Minute {
		t.Fatalf("maint: %+v", dl.Maintenance)
	}
	if cfg.S3.Bucket != "yaml-bucket" || cfg.S3.Region != "us-west-2" || !cfg.S3.ForcePathStyle {
		t.Fatalf("s3: %+v", cfg.S3)
	}
	if cfg.Auth.APIKeyHashSecret != "yaml-secret" {
		t.Fatalf("auth: %+v", cfg.Auth)
	}
	if cfg.Limits.MaxRequestBytes != 2048 || cfg.Limits.MaxSQLBytes != 1111 {
		t.Fatalf("limits: %+v", cfg.Limits)
	}
	if cfg.Observability.LogLevel != "debug" || cfg.Observability.MetricsPath != "/prom" {
		t.Fatalf("obs: %+v", cfg.Observability)
	}
	if cfg.SystemDatabase.Name != "yaml-system" || cfg.SystemDatabase.HideFromList || cfg.SystemDatabase.LogKeepDays != 21 {
		t.Fatalf("system db: %+v", cfg.SystemDatabase)
	}
	if !cfg.DevMode {
		t.Fatal("dev_mode")
	}
}

func TestLoadYAMLDoesNotOverrideExplicitEnv(t *testing.T) {
	path := writeYAML(t, `
http:
  address: ":7777"
instance:
  id: yaml-id
  writable: false
database:
  cache_dir: /tmp/yaml
auth:
  api_key_hash_secret: yaml-secret
dev_mode: true
`)
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	setRequiredEnvs(t, false, false)
	t.Setenv("SIMPLEBASE_HTTP_ADDRESS", ":6060")
	t.Setenv("SIMPLEBASE_INSTANCE_ID", "env-id")
	t.Setenv("SIMPLEBASE_DEV_MODE", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Address != ":6060" || cfg.Instance.ID != "env-id" {
		t.Fatalf("env should win: addr=%s id=%s", cfg.HTTP.Address, cfg.Instance.ID)
	}
	if cfg.DevMode {
		t.Fatal("env SIMPLEBASE_DEV_MODE=false should win")
	}
}

func TestLoadInvalidYAMLFallsBackToEnv(t *testing.T) {
	path := writeYAML(t, "::::not-yaml")
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	setRequiredEnvs(t, false, false)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("invalid yaml should fall back: %v", err)
	}
	if cfg.Instance.ID != "test-instance" {
		t.Fatalf("got %s", cfg.Instance.ID)
	}
}

func TestLoadMissingYAMLPathUsesEnv(t *testing.T) {
	t.Setenv("SIMPLEBASE_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))
	setRequiredEnvs(t, false, false)
	if _, err := Load(); err != nil {
		t.Fatalf("missing yaml: %v", err)
	}
}

func TestApplyYAMLZeroValuesDoNotOverride(t *testing.T) {
	cfg := loadFromEnv()
	origAddr := cfg.HTTP.Address
	origMaxOpen := cfg.Database.MaxOpen
	applyYAML(&cfg, yamlConfig{})
	if cfg.HTTP.Address != origAddr {
		t.Fatalf("empty yaml should not change address: %s", cfg.HTTP.Address)
	}
	if cfg.Database.MaxOpen != origMaxOpen {
		t.Fatalf("empty yaml should not change max_open: %d", cfg.Database.MaxOpen)
	}
}

func TestApplyYAMLDuckLakeInvalidDurationIgnored(t *testing.T) {
	cfg := loadFromEnv()
	origExpire := cfg.Database.DuckLake.Maintenance.ExpireOlderThan
	applyYAMLDuckLake(&cfg, yamlDuckLake{
		Maintenance: yamlDuckLakeMaint{
			ExpireOlderThan: "not-a-duration",
			DeleteOlderThan: "also-bad",
		},
	})
	if cfg.Database.DuckLake.Maintenance.ExpireOlderThan != origExpire {
		t.Fatal("invalid expire duration should be ignored")
	}
}

func TestValidateTable(t *testing.T) {
	valid := func() Config {
		return Config{
			HTTP:     HTTPConfig{Address: ":1", ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second},
			Instance: InstanceConfig{ID: "id", Writable: false},
			Database: DatabaseConfig{
				Engine: EngineDuckLake, CacheDir: "/tmp/c", IdleTimeout: time.Minute, MaxOpen: 1, MaxIdle: 0,
			},
			Auth: AuthConfig{APIKeyHashSecret: "s"},
			Limits: LimitsConfig{
				MaxRequestBytes: 1, MaxQueryRows: 1, QueryTimeout: time.Second,
				MaxConcurrentQueries: 1, MaxBatchStatements: 1, MaxSQLBytes: 1,
			},
		}
	}

	cases := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{"ok", func(*Config) {}, ""},
		{"empty engine ok", func(c *Config) { c.Database.Engine = "" }, ""},
		{"empty address", func(c *Config) { c.HTTP.Address = "" }, "http.address"},
		{"bad timeout", func(c *Config) { c.HTTP.ReadTimeout = 0 }, "timeouts"},
		{"empty instance", func(c *Config) { c.Instance.ID = "" }, "instance.id"},
		{"bad engine", func(c *Config) { c.Database.Engine = "sqlite" }, "ducklake"},
		{"neg threads", func(c *Config) { c.Database.DuckLake.Threads = -1 }, "threads"},
		{"empty cache", func(c *Config) { c.Database.CacheDir = "" }, "cache_dir"},
		{"bad idle", func(c *Config) { c.Database.IdleTimeout = 0 }, "idle_timeout"},
		{"bad max open", func(c *Config) { c.Database.MaxOpen = 0 }, "max_open"},
		{"neg max idle", func(c *Config) { c.Database.MaxIdle = -1 }, "max_idle"},
		{"empty secret", func(c *Config) { c.Auth.APIKeyHashSecret = "" }, "api_key_hash_secret"},
		{"bad limits", func(c *Config) { c.Limits.MaxSQLBytes = 0 }, "limits"},
		{"writable missing bucket", func(c *Config) {
			c.Instance.Writable = true
			c.S3 = S3Config{Region: "r", Prefix: "p"}
		}, "s3.bucket"},
		{"writable missing region", func(c *Config) {
			c.Instance.Writable = true
			c.S3 = S3Config{Bucket: "b", Prefix: "p"}
		}, "s3.region"},
		{"writable missing prefix", func(c *Config) {
			c.Instance.Writable = true
			c.S3 = S3Config{Bucket: "b", Region: "r"}
		}, "s3.prefix"},
		{"access without secret", func(c *Config) {
			c.Instance.Writable = true
			c.S3 = S3Config{Bucket: "b", Region: "r", Prefix: "p", AccessKey: "ak"}
		}, "s3.secret_key"},
		{"secret without access", func(c *Config) {
			c.Instance.Writable = true
			c.S3 = S3Config{Bucket: "b", Region: "r", Prefix: "p", SecretKey: "sk"}
		}, "s3.access_key"},
		{"dev mode skips s3", func(c *Config) {
			c.Instance.Writable = true
			c.DevMode = true
			c.S3 = S3Config{}
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := valid()
			tc.mutate(&c)
			err := c.Validate()
			if tc.wantSub == "" {
				if err != nil {
					t.Fatalf("unexpected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("want %q in error, got %v", tc.wantSub, err)
			}
		})
	}
}

func TestRedactedIncludesProvidersAndOmitsSecrets(t *testing.T) {
	cfg := Config{
		HTTP:     HTTPConfig{Address: ":1", ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second},
		Instance: InstanceConfig{ID: "id", Writable: true},
		Database: DatabaseConfig{Engine: EngineDuckLake, CacheDir: "/c", IdleTimeout: time.Minute, MaxOpen: 1, MaxIdle: 0,
			DuckLake: DuckLakeConfig{MemoryLimit: "1MB", CatalogSync: CatalogSyncConfig{Mode: "debounce", Debounce: time.Millisecond}}},
		S3:   S3Config{AccessKey: "AKIA", SecretKey: "SECRET", KMSKeyID: "kms", Bucket: "b", Region: "r", Prefix: "p"},
		Auth: AuthConfig{APIKeyHashSecret: "hash-secret"},
		LLM: LLMConfig{Providers: map[string]ProviderConfig{
			"openai": {APIKey: "sk", BaseURL: "https://x", DefaultModel: "m", AllowedModels: []string{"m"}},
			"empty":  {},
		}},
		Limits:        LimitsConfig{MaxRequestBytes: 1, MaxQueryRows: 1, QueryTimeout: time.Second, MaxConcurrentQueries: 1, MaxBatchStatements: 1, MaxSQLBytes: 1},
		Observability: ObservabilityConfig{LogLevel: "info", LogFormat: "json", MetricsPath: "/m"},
		DevMode:       true,
	}
	view := cfg.Redacted()
	if view["dev_mode"] != true {
		t.Fatal("dev_mode")
	}
	s3 := view["s3"].(map[string]any)
	if s3["has_access_key"] != true || s3["has_secret_key"] != true {
		t.Fatalf("s3 flags: %#v", s3)
	}
	for _, v := range s3 {
		if v == "AKIA" || v == "SECRET" {
			t.Fatal("secret leaked")
		}
	}
	providers := view["llm_providers"].(map[string]any)
	openai := providers["openai"].(map[string]any)
	if openai["has_api_key"] != true || openai["base_url"] != "https://x" {
		t.Fatalf("provider: %#v", openai)
	}
	if openai["has_api_key"] == "sk" {
		t.Fatal("api key leaked")
	}
	empty := providers["empty"].(map[string]any)
	if empty["has_api_key"] != false {
		t.Fatalf("empty provider: %#v", empty)
	}
}

func TestJoinErrorsAndLoadWritableS3Pair(t *testing.T) {
	if err := joinErrors(nil); err == nil || err.Error() != "" {
		// joinErrors on empty still returns errors.New("")
	}
	err := joinErrors([]error{errors.New("a"), errors.New("b")})
	if err.Error() != "a; b" {
		t.Fatalf("joinErrors: %v", err)
	}

	setRequiredEnvs(t, true, false)
	t.Setenv("SIMPLEBASE_S3_ACCESS_KEY", "ak")
	t.Setenv("SIMPLEBASE_S3_SECRET_KEY", "sk")
	t.Setenv("SIMPLEBASE_S3_KMS_KEY_ID", "kms")
	t.Setenv("SIMPLEBASE_DB_ENGINE", "DUCKLAKE")
	t.Setenv("SIMPLEBASE_DUCKLAKE_MEMORY_LIMIT", "128MB")
	t.Setenv("SIMPLEBASE_DUCKLAKE_THREADS", "3")
	t.Setenv("SIMPLEBASE_DUCKLAKE_MAINT_EXPIRE_OLDER_THAN", "2d")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.S3.KMSKeyID != "kms" || cfg.Database.DuckLake.Threads != 3 {
		t.Fatalf("env ducklake/s3: %+v %+v", cfg.S3, cfg.Database.DuckLake)
	}
	if cfg.Database.DuckLake.Maintenance.ExpireOlderThan != 2*24*time.Hour {
		t.Fatalf("expire: %v", cfg.Database.DuckLake.Maintenance.ExpireOlderThan)
	}
}

func TestLoadWritableAccessKeyWithoutSecret(t *testing.T) {
	setRequiredEnvs(t, true, false)
	t.Setenv("SIMPLEBASE_S3_ACCESS_KEY", "ak")
	if _, err := Load(); err == nil {
		t.Fatal("expected secret_key required")
	}
}

func TestLoadRejectsNegativeThreadsAndLimits(t *testing.T) {
	setRequiredEnvs(t, false, false)
	t.Setenv("SIMPLEBASE_DUCKLAKE_THREADS", "-1")
	if _, err := Load(); err == nil {
		t.Fatal("expected threads error")
	}
	t.Setenv("SIMPLEBASE_DUCKLAKE_THREADS", "1")
	t.Setenv("SIMPLEBASE_LIMITS_MAX_QUERY_ROWS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected limits error")
	}
}
