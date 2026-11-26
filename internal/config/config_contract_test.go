// 文件说明：config_contract_test.go — internal/config 的配置域覆盖与 Redacted 完整快照测试
//
// 用法
//
//	go test ./internal/config -run 'TestEnvRemainingDomains|TestYAMLRemainingDomains|TestRedactedDefaultsSnapshot'
//
// 调用架构
//
//	[上游] 重构前的配置特征测试
//	         │ 调用包内默认值、YAML 与 env 应用函数
//	         ▼
//	[本文件] internal/config/config_contract_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] config.go, env.go, redact.go, yaml.go
//
// 主要符号
//
//	TestEnvRemainingDomains(t *testing.T)
//	TestYAMLRemainingDomains(t *testing.T)
//	TestRedactedDefaultsSnapshot(t *testing.T)

package config

import (
	"reflect"
	"testing"
	"time"
)

func TestEnvRemainingDomains(t *testing.T) {
	t.Setenv("SIMPLEBASE_INSTANCE_ID", "env-instance")
	t.Setenv("SIMPLEBASE_INSTANCE_WRITABLE", "false")
	t.Setenv("SIMPLEBASE_INSTANCE_LEASE_ENABLED", "true")
	t.Setenv("SIMPLEBASE_INSTANCE_LEASE_TTL", "41s")
	t.Setenv("SIMPLEBASE_INSTANCE_LEASE_RENEW_INTERVAL", "11s")
	t.Setenv("SIMPLEBASE_INSTANCE_LEASE_GRACE", "5s")
	t.Setenv("SIMPLEBASE_INSTANCE_LEASE_ON_LOST", "stop")
	t.Setenv("SIMPLEBASE_INSTANCE_PROFILE", "production")
	t.Setenv("SIMPLEBASE_STORAGE_DATABASE", "s3")
	t.Setenv("SIMPLEBASE_STORAGE_FILES", "local")
	t.Setenv("SIMPLEBASE_DATA_DIR", "/env/data")
	t.Setenv("SIMPLEBASE_DB_ENGINE", "sqlite")
	t.Setenv("SIMPLEBASE_DB_CACHE_DIR", "/env/cache")
	t.Setenv("SIMPLEBASE_DB_CACHE_MAX_BYTES", "123")
	t.Setenv("SIMPLEBASE_DB_CACHE_MAX_DATABASES", "7")
	t.Setenv("SIMPLEBASE_DB_IDLE_TIMEOUT", "2m")
	t.Setenv("SIMPLEBASE_DB_MAX_OPEN", "3")
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "env-hash")
	t.Setenv("SIMPLEBASE_AUTH_SESSION_SECRET", "env-session")
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRETS", "env-ring")
	t.Setenv("SIMPLEBASE_BOOTSTRAP_PASSWORD", "env-bootstrap")
	t.Setenv("SIMPLEBASE_FILES_TOKEN_SECRET", "env-files-token")
	t.Setenv("SIMPLEBASE_FILES_ALLOWED_MIME", "text/plain, image/png")
	t.Setenv("SIMPLEBASE_BACKUP_ENABLED", "true")
	t.Setenv("SIMPLEBASE_BACKUP_UPLOAD_TO_S3", "true")
	t.Setenv("SIMPLEBASE_BACKUP_KEEP_LAST", "2")
	t.Setenv("SIMPLEBASE_BACKUP_KEEP_DAYS", "3")
	t.Setenv("SIMPLEBASE_BACKUP_MAX_PER_PROJECT", "4")
	t.Setenv("SIMPLEBASE_BACKUP_SNAPSHOT_TIMEOUT", "5m")
	t.Setenv("SIMPLEBASE_BACKUP_DIR", "/env/backups")
	t.Setenv("SIMPLEBASE_BACKUP_SCHEDULE", "0 * * * *")
	t.Setenv("SIMPLEBASE_BACKUP_MISSED_POLICY", "run")
	t.Setenv("SIMPLEBASE_SYSTEM_DB_NAME", "env-system")
	t.Setenv("SIMPLEBASE_METRICS_FLUSH_INTERVAL", "8s")
	t.Setenv("SIMPLEBASE_LOG_FLUSH_INTERVAL", "9s")
	t.Setenv("SIMPLEBASE_LOG_KEEP_DAYS", "10")

	cfg := loadFromEnv()
	if cfg.Instance.ID != "env-instance" || cfg.Instance.Writable || !cfg.Instance.Lease.Enabled ||
		cfg.Instance.Lease.TTL != 41*time.Second || cfg.Instance.Lease.RenewInterval != 11*time.Second ||
		cfg.Instance.Lease.Grace != 5*time.Second || cfg.Instance.Lease.OnLost != "stop" || cfg.Instance.Profile != "production" {
		t.Fatalf("instance domain: %+v", cfg.Instance)
	}
	if cfg.Storage.Database != "s3" || cfg.Storage.Files != "local" || cfg.Storage.DataDir != "/env/data" {
		t.Fatalf("storage domain: %+v", cfg.Storage)
	}
	if cfg.Database.Engine != "sqlite" || cfg.Database.CacheDir != "/env/cache" ||
		cfg.Database.CacheMaxBytes != 123 || cfg.Database.CacheMaxDatabases != 7 ||
		cfg.Database.IdleTimeout != 2*time.Minute || cfg.Database.MaxOpen != 3 {
		t.Fatalf("database domain: %+v", cfg.Database)
	}
	if cfg.Auth.APIKeyHashSecret != "env-hash" || cfg.Auth.SessionSecret != "env-session" ||
		cfg.Auth.APIKeyHashSecrets != "env-ring" || cfg.Auth.BootstrapPassword != "env-bootstrap" {
		t.Fatal("auth domain was not applied")
	}
	if cfg.Files.TokenSecret != "env-files-token" || !reflect.DeepEqual(cfg.Files.AllowedMIME, []string{"text/plain", "image/png"}) {
		t.Fatalf("files domain: %+v", cfg.Files)
	}
	if !cfg.Backup.Enabled || !cfg.Backup.UploadToS3 || cfg.Backup.KeepLast != 2 ||
		cfg.Backup.KeepDays != 3 || cfg.Backup.MaxPerProject != 4 || cfg.Backup.SnapshotTimeout != 5*time.Minute ||
		cfg.Backup.Dir != "/env/backups" || cfg.Backup.Schedule != "0 * * * *" || cfg.Backup.MissedPolicy != "run" {
		t.Fatalf("backup domain: %+v", cfg.Backup)
	}
	if cfg.SystemDatabase.Name != "env-system" || cfg.SystemDatabase.MetricsFlushInterval != 8*time.Second ||
		cfg.SystemDatabase.LogFlushInterval != 9*time.Second || cfg.SystemDatabase.LogKeepDays != 10 {
		t.Fatalf("system database domain: %+v", cfg.SystemDatabase)
	}
}

func TestYAMLRemainingDomains(t *testing.T) {
	yc, err := unmarshalYAML([]byte(`
storage:
  database: s3
  files: local
  data_dir: /yaml/data
database:
  engine: sqlite
  cache_dir: /yaml/cache
  cache_max_bytes: 456
  cache_max_databases: 9
  idle_timeout: 4m
  max_open: 6
files:
  allowed_mime: [text/plain, image/png]
backup:
  enabled: true
  dir: /yaml/backups
  upload_to_s3: true
  schedule: "15 * * * *"
  keep_last: 5
  keep_days: 6
  max_per_project: 7
  snapshot_timeout: 8m
  missed_policy: skip
system_database:
  name: yaml-system
  metrics_flush_interval: 12s
  log_flush_interval: 13s
  log_keep_days: 14
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaults()
	if _, err := applyYAML(&cfg, yc); err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.Database != "s3" || cfg.Storage.Files != "local" || cfg.Storage.DataDir != "/yaml/data" {
		t.Fatalf("storage domain: %+v", cfg.Storage)
	}
	if cfg.Database.Engine != "sqlite" || cfg.Database.CacheDir != "/yaml/cache" ||
		cfg.Database.CacheMaxBytes != 456 || cfg.Database.CacheMaxDatabases != 9 ||
		cfg.Database.IdleTimeout != 4*time.Minute || cfg.Database.MaxOpen != 6 {
		t.Fatalf("database domain: %+v", cfg.Database)
	}
	if !reflect.DeepEqual(cfg.Files.AllowedMIME, []string{"text/plain", "image/png"}) {
		t.Fatalf("files domain: %+v", cfg.Files)
	}
	if !cfg.Backup.Enabled || !cfg.Backup.UploadToS3 || cfg.Backup.KeepLast != 5 ||
		cfg.Backup.KeepDays != 6 || cfg.Backup.MaxPerProject != 7 || cfg.Backup.SnapshotTimeout != 8*time.Minute ||
		cfg.Backup.Dir != "/yaml/backups" || cfg.Backup.Schedule != "15 * * * *" || cfg.Backup.MissedPolicy != "skip" {
		t.Fatalf("backup domain: %+v", cfg.Backup)
	}
	if cfg.SystemDatabase.Name != "yaml-system" || cfg.SystemDatabase.MetricsFlushInterval != 12*time.Second ||
		cfg.SystemDatabase.LogFlushInterval != 13*time.Second || cfg.SystemDatabase.LogKeepDays != 14 {
		t.Fatalf("system database domain: %+v", cfg.SystemDatabase)
	}
}

func TestRedactedDefaultsSnapshot(t *testing.T) {
	want := map[string]any{
		"http": map[string]any{
			"address": ":8080", "read_timeout": "15s", "write_timeout": "30s",
			"idle_timeout": "1m0s", "shutdown_timeout": "30s",
		},
		"instance": map[string]any{
			"id": "", "writable": true,
			"lease": map[string]any{
				"enabled": false, "ttl": "30s", "renew_interval": "0s", "grace": "10s", "on_lost": "release_db",
			},
		},
		"database": map[string]any{
			"engine": "duckdb", "cache_dir": "", "cache_max_bytes": int64(1073741824),
			"cache_max_databases": 256, "idle_timeout": "5m0s", "max_open": 8,
			"ducklake": map[string]any{
				"catalog_engine": "duckdb", "memory_limit": "512MB", "threads": 2, "extension_dir": "",
				"data_inlining_row_limit": 1000, "parquet_compression": "zstd", "target_file_size": "64MB",
				"require_commit_message": false, "catalog_sync_mode": "interval", "catalog_sync_debounce": "200ms",
				"catalog_sync_interval": "15s", "catalog_sync_max_lag": "30s", "catalog_sync_keep_versions": 10,
			},
		},
		"s3": map[string]any{
			"endpoint": "", "region": "", "bucket": "", "prefix": "simplebase", "force_path_style": false,
			"has_access_key": false, "has_secret_key": false, "kms_key_id": "",
		},
		"system_database": map[string]any{
			"name": "simplebase-system", "metrics_flush_interval": "15s", "log_flush_interval": "15s", "log_keep_days": 14,
		},
		"limits": map[string]any{
			"max_request_bytes": int64(1048576), "max_query_rows": 1000, "query_timeout": "30s",
			"max_concurrent_queries": 64, "max_batch_statements": 100, "max_sql_bytes": 65536,
		},
		"llm": map[string]any{"enabled": true, "providers": map[string]any{}},
		"sandbox": map[string]any{
			"enabled": false, "backend": "local", "images": []string(nil), "exec_timeout_max": "5m0s",
			"max_per_project": 5, "reap_interval": "5m0s", "root": "", "pids_max": int64(0), "image": "host",
			"cpus": 1, "memory_mib": 256, "max_duration": "30m0s", "idle_timeout": "5m0s", "exec_timeout": "30s",
			"max_output_bytes": 65536, "max_file_bytes": 1048576, "network": "none", "workdir": "/workspace",
		},
		"storage": map[string]any{
			"database": "local", "files": "local", "data_dir": "./data", "database_is_remote": false, "files_is_remote": false,
		},
		"sites": map[string]any{"base_domain": "", "url_scheme": "https", "url_port": 0},
		"observability": map[string]any{
			"log_level": "debug", "log_format": "json", "log_output": "stderr", "metrics_path": "/metrics",
			"metrics_address": "", "pprof": false, "perf_stage_timing": false,
		},
	}
	if got := defaults().Redacted(); !reflect.DeepEqual(got, want) {
		t.Fatalf("redacted map:\n got: %#v\nwant: %#v", got, want)
	}
}
