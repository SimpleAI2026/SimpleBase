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

func TestLoadLLMProvidersFlatEnvFallback(t *testing.T) {
	t.Setenv("SIMPLEBASE_LLM_PROVIDER_API_KEY", "sk-fallback")
	t.Setenv("SIMPLEBASE_LLM_PROVIDER_BASE_URL", "https://api.example/v1")
	t.Setenv("SIMPLEBASE_LLM_PROVIDER_DEFAULT_MODEL", "zai-org/GLM-5.3-Flash")
	got := loadLLMProviders()
	if len(got) != 1 || got["openai"].APIKey != "sk-fallback" ||
		got["openai"].BaseURL != "https://api.example/v1" ||
		got["openai"].DefaultModel != "zai-org/GLM-5.3-Flash" {
		t.Fatalf("flat env provider: %+v", got)
	}
	t.Setenv("SIMPLEBASE_LLM_PROVIDERS", "anthropic")
	got = loadLLMProviders()
	if len(got) != 1 {
		t.Fatalf("named providers must take precedence: %+v", got)
	}
	if _, ok := got["openai"]; ok {
		t.Fatalf("flat env must not override named providers: %+v", got)
	}
}

func TestLoadLLMProvidersFlatEnvTimeout(t *testing.T) {
	t.Setenv("SIMPLEBASE_LLM_PROVIDER_API_KEY", "sk-fallback")
	if got := loadLLMProviders()["openai"].Timeout; got != 60*time.Second {
		t.Fatalf("default timeout = %v, want 60s", got)
	}
	t.Setenv("SIMPLEBASE_LLM_PROVIDER_TIMEOUT", "120s")
	if got := loadLLMProviders()["openai"].Timeout; got != 120*time.Second {
		t.Fatalf("timeout = %v, want 120s", got)
	}
	for _, bad := range []string{"not-a-duration", "0s", "-5s"} {
		t.Setenv("SIMPLEBASE_LLM_PROVIDER_TIMEOUT", bad)
		if got := loadLLMProviders()["openai"].Timeout; got != 60*time.Second {
			t.Fatalf("timeout %q = %v, want fallback 60s", bad, got)
		}
	}
}

func TestConfigFilePath(t *testing.T) {
	t.Setenv("SIMPLEBASE_CONFIG_PATH", "/tmp/custom.yaml")
	got, required := configFilePath()
	if got != "/tmp/custom.yaml" || !required {
		t.Fatalf("explicit path: %q required=%v", got, required)
	}
	t.Setenv("SIMPLEBASE_CONFIG_PATH", "")
	got, required = configFilePath()
	if _, err := os.Stat("config.yaml"); err == nil {
		if got != "config.yaml" || required {
			t.Fatalf("expected optional config.yaml when present, got %q required=%v", got, required)
		}
	} else if got != "" {
		t.Fatalf("expected empty when config.yaml missing, got %q", got)
	}
}

func TestLoadFromEnvAndYAMLOverride(t *testing.T) {
	yaml := `
http:
  address: ":7777"
  read_timeout: 11s
  write_timeout: 12s
  idle_timeout: 13s
  shutdown_timeout: 9s
instance:
  id: yaml-instance
  writable: false
database:
  engine: duckdb
  cache_dir: /tmp/yaml-cache
  cache_max_bytes: 2048
  cache_max_databases: 4
  idle_timeout: 2m
  max_open: 3
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
      debounce: 400ms
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
  force_path_style: true
  kms_key_id: yaml-kms
auth: {}
llm:
  enabled: true
  providers:
    openai:
      base_url: https://api.example
      default_model: gpt-4o-mini
      allowed_models: ["gpt-4o-mini"]
      timeout: 45s
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
  log_output: stdout
  metrics_path: /prom
system_database:
  name: yaml-system
  metrics_flush_interval: 3s
  log_flush_interval: 4s
  log_keep_days: 21
storage:
  database: s3
  files: local
  data_dir: /srv/sb-data
`
	path := writeYAML(t, yaml)
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	// 只留非密钥字段从 YAML 读取；密钥必须来自环境变量。
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "env-secret")
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
	if cfg.Database.CacheDir != "/tmp/yaml-cache" || cfg.Database.MaxOpen != 3 {
		t.Fatalf("database: %+v", cfg.Database)
	}
	if cfg.Database.CacheMaxBytes != 2048 || cfg.Database.CacheMaxDatabases != 4 {
		t.Fatalf("cache caps: %+v", cfg.Database)
	}
	if cfg.HTTP.ShutdownTimeout != 9*time.Second {
		t.Fatalf("shutdown: %v", cfg.HTTP.ShutdownTimeout)
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
	if cfg.S3.Bucket != "yaml-bucket" || cfg.S3.Region != "us-west-2" || !cfg.S3.ForcePathStyle || cfg.S3.KMSKeyID != "yaml-kms" {
		t.Fatalf("s3: %+v", cfg.S3)
	}
	if cfg.Auth.APIKeyHashSecret != "env-secret" {
		t.Fatalf("auth secret must not be read from YAML: %+v", cfg.Auth)
	}
	if !cfg.LLM.Enabled || cfg.LLM.Providers["openai"].APIKey != "" || cfg.LLM.Providers["openai"].Timeout != 45*time.Second {
		t.Fatalf("llm: %+v", cfg.LLM)
	}
	if cfg.Limits.MaxRequestBytes != 2048 || cfg.Limits.MaxSQLBytes != 1111 {
		t.Fatalf("limits: %+v", cfg.Limits)
	}
	if cfg.Observability.LogLevel != "debug" || cfg.Observability.MetricsPath != "/prom" || cfg.Observability.LogOutput != "stdout" {
		t.Fatalf("obs: %+v", cfg.Observability)
	}
	if cfg.SystemDatabase.Name != "yaml-system" || cfg.SystemDatabase.LogKeepDays != 21 {
		t.Fatalf("system db: %+v", cfg.SystemDatabase)
	}
	if cfg.SystemDatabase.MetricsFlushInterval != 3*time.Second || cfg.SystemDatabase.LogFlushInterval != 4*time.Second {
		t.Fatalf("flush: %+v", cfg.SystemDatabase)
	}
	if cfg.Storage.Database != StorageModeS3 || cfg.Storage.Files != StorageModeLocal ||
		cfg.Storage.DataDir != "/srv/sb-data" {
		t.Fatalf("storage: %+v", cfg.Storage)
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
`)
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	setRequiredEnvs(t, false, false)
	t.Setenv("SIMPLEBASE_HTTP_ADDRESS", ":6060")
	t.Setenv("SIMPLEBASE_INSTANCE_ID", "env-id")
	t.Setenv("SIMPLEBASE_STORAGE_DATABASE", "s3")
	t.Setenv("SIMPLEBASE_DATA_DIR", "/env/data")
	t.Setenv("SIMPLEBASE_S3_BUCKET", "env-bucket")
	t.Setenv("SIMPLEBASE_S3_REGION", "env-region")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Address != ":6060" || cfg.Instance.ID != "env-id" {
		t.Fatalf("env should win: addr=%s id=%s", cfg.HTTP.Address, cfg.Instance.ID)
	}
	if !cfg.IsDBRemote() || cfg.Storage.DataDir != "/env/data" {
		t.Fatalf("env SIMPLEBASE_STORAGE_DATABASE/DATA_DIR should win: %+v", cfg.Storage)
	}
}

func TestLoadInvalidYAMLErrors(t *testing.T) {
	path := writeYAML(t, "::::not-yaml")
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	setRequiredEnvs(t, false, false)
	if _, err := Load(); err == nil {
		t.Fatal("invalid yaml must fail")
	}
}

func TestLoadMissingYAMLPathErrorsWhenConfigured(t *testing.T) {
	t.Setenv("SIMPLEBASE_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))
	setRequiredEnvs(t, false, false)
	if _, err := Load(); err == nil {
		t.Fatal("missing SIMPLEBASE_CONFIG_PATH must fail")
	}
}

func TestLoadEnvOnlyWhenNoYAML(t *testing.T) {
	t.Setenv("SIMPLEBASE_CONFIG_PATH", "")
	setRequiredEnvs(t, false, false)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("env-only: %v", err)
	}
	if cfg.Instance.ID != "test-instance" {
		t.Fatalf("got %s", cfg.Instance.ID)
	}
	if !cfg.LLM.Enabled {
		t.Fatal("llm.enabled default true")
	}
	if cfg.Database.CacheMaxBytes != DefaultCacheMaxBytes {
		t.Fatalf("cache default: %d", cfg.Database.CacheMaxBytes)
	}
}

func TestApplyYAMLZeroValuesDoNotOverride(t *testing.T) {
	cfg := loadFromEnv()
	origAddr := cfg.HTTP.Address
	origMaxOpen := cfg.Database.MaxOpen
	origWritable := cfg.Instance.Writable
	if _, err := applyYAML(&cfg, yamlConfig{}); err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Address != origAddr {
		t.Fatalf("empty yaml should not change address: %s", cfg.HTTP.Address)
	}
	if cfg.Database.MaxOpen != origMaxOpen {
		t.Fatalf("empty yaml should not change max_open: %d", cfg.Database.MaxOpen)
	}
	if cfg.Instance.Writable != origWritable {
		t.Fatal("omitted writable must not flip default")
	}
}

func TestLoadInvalidDurationInYAMLErrors(t *testing.T) {
	path := writeYAML(t, `
instance:
  id: yaml-id
database:
  cache_dir: /tmp/yaml
  ducklake:
    maintenance:
      expire_older_than: not-a-duration
auth: {}
`)
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "s")
	if _, err := Load(); err == nil {
		t.Fatal("invalid duration must fail")
	}
}

func TestOmittedYAMLBoolsKeepDefaults(t *testing.T) {
	path := writeYAML(t, `
http:
  address: ":8081"
instance:
  id: yaml-id
database:
  cache_dir: /tmp/yaml
auth: {}
`)
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "s")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Instance.Writable {
		t.Fatal("omitted writable should stay default true")
	}
	if cfg.S3.ForcePathStyle {
		t.Fatal("omitted force_path_style should stay false")
	}
	if cfg.Database.DuckLake.RequireCommitMessage {
		t.Fatal("omitted require_commit_message should stay false")
	}
}

func TestLoadRejectsYAMLSecretsAlways(t *testing.T) {
	path := writeYAML(t, `
instance:
  id: prod
  writable: false
database:
  cache_dir: /tmp/c
s3:
  access_key: AKIA
  secret_key: secret
auth:
  api_key_hash_secret: from-yaml
`)
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	if _, err := Load(); err == nil {
		t.Fatal("expected secret rejection: secrets must never come from YAML")
	}
}

func TestLoadRejectsYAMLOnlyAuthSecret(t *testing.T) {
	path := writeYAML(t, `
instance:
  id: dev
database:
  cache_dir: /tmp/c
auth:
  api_key_hash_secret: from-yaml
`)
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	if _, err := Load(); err == nil {
		t.Fatal("expected secret rejection: auth secret must come from env")
	}
}

func TestDebounceMSCompatAlias(t *testing.T) {
	path := writeYAML(t, `
instance:
  id: yaml-id
database:
  cache_dir: /tmp/yaml
  ducklake:
    catalog_sync:
      debounce_ms: 350
auth: {}
`)
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "s")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.DuckLake.CatalogSync.Debounce != 350*time.Millisecond {
		t.Fatalf("debounce: %v", cfg.Database.DuckLake.CatalogSync.Debounce)
	}
}

func TestLoadConfigExampleYAML(t *testing.T) {
	example := filepath.Join("..", "..", "config.example.yaml")
	if _, err := os.Stat(example); err != nil {
		t.Skip("config.example.yaml not found")
	}
	t.Setenv("SIMPLEBASE_CONFIG_PATH", example)
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "example-test-secret")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("example yaml: %v", err)
	}
	if cfg.Instance.ID == "" || cfg.Database.CacheDir == "" {
		t.Fatalf("example missing required fields: %+v", cfg)
	}
	if cfg.Observability.LogFormat == "" || cfg.HTTP.ShutdownTimeout <= 0 {
		t.Fatalf("example missing new fields: %+v", cfg.Observability)
	}
}

func TestValidateTable(t *testing.T) {
	valid := func() Config {
		return Config{
			HTTP: HTTPConfig{
				Address: ":1", ReadTimeout: time.Second, WriteTimeout: time.Second,
				IdleTimeout: time.Second, ShutdownTimeout: time.Second,
			},
			Instance: InstanceConfig{ID: "id", Writable: false},
			Storage:  StorageConfig{Database: StorageModeLocal, Files: StorageModeLocal, DataDir: "/tmp/data"},
			Database: DatabaseConfig{
				Engine: EngineDuckDB, CacheDir: "/tmp/c", IdleTimeout: time.Minute, MaxOpen: 1,
				CacheMaxBytes: DefaultCacheMaxBytes, CacheMaxDatabases: DefaultCacheMaxDatabases,
			},
			Auth: AuthConfig{APIKeyHashSecret: "s"},
			Limits: LimitsConfig{
				MaxRequestBytes: 1, MaxQueryRows: 1, QueryTimeout: time.Second,
				MaxConcurrentQueries: 1, MaxBatchStatements: 1, MaxSQLBytes: 1,
			},
			SystemDatabase: SystemDatabaseConfig{LogKeepDays: 14},
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
		{"bad shutdown", func(c *Config) { c.HTTP.ShutdownTimeout = 0 }, "shutdown_timeout"},
		{"empty instance", func(c *Config) { c.Instance.ID = "" }, "instance.id"},
		{"bad engine", func(c *Config) { c.Database.Engine = "postgres" }, "duckdb or sqlite"},
		{"neg threads", func(c *Config) { c.Database.DuckLake.Threads = -1 }, "threads"},
		{"empty cache", func(c *Config) { c.Database.CacheDir = "" }, "cache_dir"},
		{"bad idle", func(c *Config) { c.Database.IdleTimeout = 0 }, "idle_timeout"},
		{"bad max open", func(c *Config) { c.Database.MaxOpen = 0 }, "max_open"},
		{"bad cache bytes", func(c *Config) { c.Database.CacheMaxBytes = 0 }, "cache_max_bytes"},
		{"empty secret", func(c *Config) { c.Auth.APIKeyHashSecret = "" }, "api_key_hash_secret"},
		{"bad limits", func(c *Config) { c.Limits.MaxSQLBytes = 0 }, "limits"},
		{"bad log format", func(c *Config) { c.Observability.LogFormat = "xml" }, "log_format"},
		{"bad log output", func(c *Config) { c.Observability.LogOutput = "file" }, "log_output"},
		{"bad keep days", func(c *Config) { c.SystemDatabase.LogKeepDays = 0 }, "log_keep_days"},
		{"remote storage missing bucket", func(c *Config) {
			c.Storage.Database = StorageModeS3
			c.S3 = S3Config{Region: "r", Prefix: "p"}
		}, "s3.bucket"},
		{"remote storage missing region", func(c *Config) {
			c.Storage.Database = StorageModeS3
			c.S3 = S3Config{Bucket: "b", Prefix: "p"}
		}, "s3.region"},
		{"access without secret", func(c *Config) {
			c.Storage.Database = StorageModeS3
			c.S3 = S3Config{Bucket: "b", Region: "r", Prefix: "p", AccessKey: "ak"}
		}, "s3.secret_key"},
		{"secret without access", func(c *Config) {
			c.Storage.Database = StorageModeS3
			c.S3 = S3Config{Bucket: "b", Region: "r", Prefix: "p", SecretKey: "sk"}
		}, "s3.access_key"},
		{"bad storage mode", func(c *Config) { c.Storage.Database = "ftp" }, "storage.database"},
		{"bad files mode", func(c *Config) { c.Storage.Files = "nfs" }, "storage.files"},
		{"empty data dir", func(c *Config) { c.Storage.DataDir = "" }, "data_dir"},
		{"data dir equals cache dir", func(c *Config) { c.Storage.DataDir = "/tmp/c" }, "must differ"},
		{"cache dir inside data dir", func(c *Config) {
			c.Storage.DataDir = "/tmp/root"
			c.Database.CacheDir = "/tmp/root/cache"
		}, "must not contain"},
		{"data dir inside cache dir", func(c *Config) {
			c.Storage.DataDir = "/tmp/c/data"
		}, "must not contain"},
		{"local storage skips s3", func(c *Config) {
			c.Instance.Writable = true
			c.Storage = StorageConfig{Database: StorageModeLocal, Files: StorageModeLocal, DataDir: "./data"}
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
		HTTP:     HTTPConfig{Address: ":1", ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second, ShutdownTimeout: time.Second},
		Instance: InstanceConfig{ID: "id", Writable: true},
		Database: DatabaseConfig{Engine: EngineDuckDB, CacheDir: "/c", IdleTimeout: time.Minute, MaxOpen: 1,
			CacheMaxBytes: 1, CacheMaxDatabases: 1,
			DuckLake: DuckLakeConfig{MemoryLimit: "1MB", CatalogSync: CatalogSyncConfig{Mode: "debounce", Debounce: time.Millisecond}}},
		S3:   S3Config{AccessKey: "AKIA", SecretKey: "SECRET", KMSKeyID: "kms", Bucket: "b", Region: "r", Prefix: "p"},
		Auth: AuthConfig{APIKeyHashSecret: "hash-secret"},
		LLM: LLMConfig{Providers: map[string]ProviderConfig{
			"openai": {APIKey: "sk", BaseURL: "https://x", DefaultModel: "m", AllowedModels: []string{"m"}},
			"empty":  {},
		}},
		Limits:        LimitsConfig{MaxRequestBytes: 1, MaxQueryRows: 1, QueryTimeout: time.Second, MaxConcurrentQueries: 1, MaxBatchStatements: 1, MaxSQLBytes: 1},
		Observability: ObservabilityConfig{LogLevel: "info", LogFormat: "json", MetricsPath: "/m"},
	}
	view := cfg.Redacted()
	store, ok := view["storage"].(map[string]any)
	if !ok {
		t.Fatalf("storage view missing: %+v", view)
	}
	if store["database"] != StorageModeLocal || store["files"] != StorageModeLocal {
		t.Fatalf("storage view: %+v", store)
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
	llm := view["llm"].(map[string]any)
	providers := llm["providers"].(map[string]any)
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
	t.Setenv("SIMPLEBASE_DB_ENGINE", "DUCKDB")
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
