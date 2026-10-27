// 文件说明：env.go — internal/config 的内部实现（applyEnv、applyEnvAgent 等）
//
// 用法
//
//	同包调用：applyEnv() → config.go；parseDuration() → yaml.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api, internal/api/perfbench 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/config/env.go
//	         │ 依赖
//	         ▼
//	[下游] internal/utils/strutil
//	[同包] backup.go, config.go, http_boundary.go, yaml.go
//	[关键引用] applyEnv() ← config.go
//
// 主要符号
//
//	applyEnv(cfg *Config)
//	applyEnvAgent(cfg *Config)
//	applyEnvSandbox(cfg *Config)
//	applyEnvLLM(cfg *Config)
//	setEnvBool(dst *bool, key string)
//	setEnvInt(dst *int, key string)
//	... 另有 12 个顶层符号

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

func applyEnv(cfg *Config) {
	if v, ok := os.LookupEnv("SIMPLEBASE_HTTP_ADDRESS"); ok {
		cfg.HTTP.Address = v
	}
	setEnvDuration(&cfg.HTTP.ReadTimeout, "SIMPLEBASE_HTTP_READ_TIMEOUT")
	setEnvDuration(&cfg.HTTP.WriteTimeout, "SIMPLEBASE_HTTP_WRITE_TIMEOUT")
	setEnvDuration(&cfg.HTTP.IdleTimeout, "SIMPLEBASE_HTTP_IDLE_TIMEOUT")
	setEnvDuration(&cfg.HTTP.ShutdownTimeout, "SIMPLEBASE_HTTP_SHUTDOWN_TIMEOUT")
	setEnvBool(&cfg.HTTP.Gzip, "SIMPLEBASE_HTTP_GZIP")
	setEnvBool(&cfg.HTTP.CORS.Enabled, "SIMPLEBASE_HTTP_CORS_ENABLED")
	setEnvBool(&cfg.HTTP.CORS.AllowCredentials, "SIMPLEBASE_HTTP_CORS_ALLOW_CREDENTIALS")
	setEnvInt(&cfg.HTTP.CORS.MaxAge, "SIMPLEBASE_HTTP_CORS_MAX_AGE")
	if v, ok := os.LookupEnv("SIMPLEBASE_HTTP_CORS_ALLOWED_ORIGINS"); ok {
		cfg.HTTP.CORS.AllowedOrigins = strutil.SplitCSV(v)
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_HTTP_TRUSTED_PROXIES"); ok {
		cfg.HTTP.TrustedProxies = strutil.SplitCSV(v)
	}
	setEnvBool(&cfg.Limits.RateLimit.Enabled, "SIMPLEBASE_RATE_LIMIT_ENABLED")
	setEnvInt(&cfg.Limits.RateLimit.Burst, "SIMPLEBASE_RATE_LIMIT_BURST")
	setEnvBool(&cfg.Limits.RateLimit.ExemptSSE, "SIMPLEBASE_RATE_LIMIT_EXEMPT_SSE")
	if v, ok := os.LookupEnv("SIMPLEBASE_FILES_TOKEN_SECRET"); ok {
		cfg.Files.TokenSecret = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_FILES_ALLOWED_MIME"); ok {
		cfg.Files.AllowedMIME = strutil.SplitCSV(v)
	}
	setEnvBool(&cfg.Backup.Enabled, "SIMPLEBASE_BACKUP_ENABLED")
	setEnvBool(&cfg.Backup.UploadToS3, "SIMPLEBASE_BACKUP_UPLOAD_TO_S3")
	setEnvInt(&cfg.Backup.KeepLast, "SIMPLEBASE_BACKUP_KEEP_LAST")
	setEnvInt(&cfg.Backup.KeepDays, "SIMPLEBASE_BACKUP_KEEP_DAYS")
	setEnvInt(&cfg.Backup.MaxPerProject, "SIMPLEBASE_BACKUP_MAX_PER_PROJECT")
	setEnvDuration(&cfg.Backup.SnapshotTimeout, "SIMPLEBASE_BACKUP_SNAPSHOT_TIMEOUT")
	if v, ok := os.LookupEnv("SIMPLEBASE_BACKUP_DIR"); ok {
		cfg.Backup.Dir = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_BACKUP_SCHEDULE"); ok {
		cfg.Backup.Schedule = v
	}

	if v, ok := os.LookupEnv("SIMPLEBASE_INSTANCE_ID"); ok {
		cfg.Instance.ID = v
	}
	setEnvBool(&cfg.Instance.Writable, "SIMPLEBASE_INSTANCE_WRITABLE")
	setEnvBool(&cfg.Instance.Lease.Enabled, "SIMPLEBASE_INSTANCE_LEASE_ENABLED")
	setEnvDuration(&cfg.Instance.Lease.TTL, "SIMPLEBASE_INSTANCE_LEASE_TTL")
	setEnvDuration(&cfg.Instance.Lease.RenewInterval, "SIMPLEBASE_INSTANCE_LEASE_RENEW_INTERVAL")
	setEnvDuration(&cfg.Instance.Lease.Grace, "SIMPLEBASE_INSTANCE_LEASE_GRACE")
	if v, ok := os.LookupEnv("SIMPLEBASE_INSTANCE_LEASE_ON_LOST"); ok {
		cfg.Instance.Lease.OnLost = v
	}

	if v, ok := os.LookupEnv("SIMPLEBASE_STORAGE_DATABASE"); ok {
		cfg.Storage.Database = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_STORAGE_FILES"); ok {
		cfg.Storage.Files = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_DATA_DIR"); ok {
		cfg.Storage.DataDir = v
	}

	if v, ok := os.LookupEnv("SIMPLEBASE_DB_ENGINE"); ok {
		cfg.Database.Engine = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_DB_CACHE_DIR"); ok {
		cfg.Database.CacheDir = v
	}
	setEnvInt64(&cfg.Database.CacheMaxBytes, "SIMPLEBASE_DB_CACHE_MAX_BYTES")
	setEnvInt(&cfg.Database.CacheMaxDatabases, "SIMPLEBASE_DB_CACHE_MAX_DATABASES")
	setEnvDuration(&cfg.Database.IdleTimeout, "SIMPLEBASE_DB_IDLE_TIMEOUT")
	setEnvInt(&cfg.Database.MaxOpen, "SIMPLEBASE_DB_MAX_OPEN")

	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_CATALOG_ENGINE"); ok {
		cfg.Database.DuckLake.CatalogEngine = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_MEMORY_LIMIT"); ok {
		cfg.Database.DuckLake.MemoryLimit = v
	}
	setEnvInt(&cfg.Database.DuckLake.Threads, "SIMPLEBASE_DUCKLAKE_THREADS")
	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_EXTENSION_DIR"); ok {
		cfg.Database.DuckLake.ExtensionDir = v
	}
	setEnvInt(&cfg.Database.DuckLake.DataInliningRowLimit, "SIMPLEBASE_DUCKLAKE_DATA_INLINING_ROW_LIMIT")
	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_PARQUET_COMPRESSION"); ok {
		cfg.Database.DuckLake.ParquetCompression = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_TARGET_FILE_SIZE"); ok {
		cfg.Database.DuckLake.TargetFileSize = v
	}
	setEnvBool(&cfg.Database.DuckLake.RequireCommitMessage, "SIMPLEBASE_DUCKLAKE_REQUIRE_COMMIT_MESSAGE")
	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_SYNC_MODE"); ok {
		cfg.Database.DuckLake.CatalogSync.Mode = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_SYNC_DEBOUNCE_MS"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Database.DuckLake.CatalogSync.Debounce = time.Duration(n) * time.Millisecond
		}
	}
	setEnvDuration(&cfg.Database.DuckLake.CatalogSync.Interval, "SIMPLEBASE_DUCKLAKE_SYNC_INTERVAL")
	setEnvDuration(&cfg.Database.DuckLake.CatalogSync.MaxLag, "SIMPLEBASE_DUCKLAKE_SYNC_MAX_LAG")
	setEnvInt(&cfg.Database.DuckLake.CatalogSync.KeepVersions, "SIMPLEBASE_DUCKLAKE_SYNC_KEEP_VERSIONS")
	setEnvDuration(&cfg.Database.DuckLake.Maintenance.CheckpointInterval, "SIMPLEBASE_DUCKLAKE_MAINT_CHECKPOINT_INTERVAL")
	setEnvDuration(&cfg.Database.DuckLake.Maintenance.ExpireOlderThan, "SIMPLEBASE_DUCKLAKE_MAINT_EXPIRE_OLDER_THAN")
	setEnvDuration(&cfg.Database.DuckLake.Maintenance.DeleteOlderThan, "SIMPLEBASE_DUCKLAKE_MAINT_DELETE_OLDER_THAN")
	setEnvFloat64(&cfg.Database.DuckLake.Maintenance.RewriteDeleteThreshold, "SIMPLEBASE_DUCKLAKE_MAINT_REWRITE_DELETE_THRESHOLD")

	if v, ok := os.LookupEnv("SIMPLEBASE_S3_ENDPOINT"); ok {
		cfg.S3.Endpoint = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_S3_REGION"); ok {
		cfg.S3.Region = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_S3_BUCKET"); ok {
		cfg.S3.Bucket = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_S3_PREFIX"); ok {
		cfg.S3.Prefix = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_S3_ACCESS_KEY"); ok {
		cfg.S3.AccessKey = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_S3_SECRET_KEY"); ok {
		cfg.S3.SecretKey = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_S3_KMS_KEY_ID"); ok {
		cfg.S3.KMSKeyID = v
	}
	setEnvBool(&cfg.S3.ForcePathStyle, "SIMPLEBASE_S3_FORCE_PATH_STYLE")

	if v, ok := os.LookupEnv("SIMPLEBASE_AUTH_APIKEY_SECRET"); ok {
		cfg.Auth.APIKeyHashSecret = v
	}

	applyEnvSandbox(cfg)

	setEnvInt64(&cfg.Limits.MaxRequestBytes, "SIMPLEBASE_LIMITS_MAX_REQUEST_BYTES")
	setEnvInt(&cfg.Limits.MaxQueryRows, "SIMPLEBASE_LIMITS_MAX_QUERY_ROWS")
	setEnvDuration(&cfg.Limits.QueryTimeout, "SIMPLEBASE_LIMITS_QUERY_TIMEOUT")
	setEnvInt(&cfg.Limits.MaxConcurrentQueries, "SIMPLEBASE_LIMITS_MAX_CONCURRENT_QUERIES")
	setEnvInt(&cfg.Limits.MaxBatchStatements, "SIMPLEBASE_LIMITS_MAX_BATCH_STATEMENTS")
	setEnvInt(&cfg.Limits.MaxSQLBytes, "SIMPLEBASE_LIMITS_MAX_SQL_BYTES")
	setEnvInt(&cfg.Limits.MaxConcurrentGoRuns, "SIMPLEBASE_LIMITS_MAX_CONCURRENT_GO_RUNS")
	setEnvInt(&cfg.Limits.MaxFilterScanRows, "SIMPLEBASE_LIMITS_MAX_FILTER_SCAN_ROWS")
	setEnvInt64(&cfg.Limits.MaxImportBytes, "SIMPLEBASE_LIMITS_MAX_IMPORT_BYTES")
	setEnvInt(&cfg.Limits.RealtimeMaxConns, "SIMPLEBASE_LIMITS_REALTIME_MAX_CONNS")

	if v, ok := os.LookupEnv("SIMPLEBASE_LOG_LEVEL"); ok {
		cfg.Observability.LogLevel = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_LOG_FORMAT"); ok {
		cfg.Observability.LogFormat = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_LOG_OUTPUT"); ok {
		cfg.Observability.LogOutput = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_METRICS_PATH"); ok {
		cfg.Observability.MetricsPath = v
	}
	setEnvBool(&cfg.Observability.PerfStageTiming, "SIMPLEBASE_PERF_STAGE_TIMING")

	if v, ok := os.LookupEnv("SIMPLEBASE_SYSTEM_DB_NAME"); ok {
		cfg.SystemDatabase.Name = v
	}
	setEnvDuration(&cfg.SystemDatabase.MetricsFlushInterval, "SIMPLEBASE_METRICS_FLUSH_INTERVAL")
	setEnvDuration(&cfg.SystemDatabase.LogFlushInterval, "SIMPLEBASE_LOG_FLUSH_INTERVAL")
	setEnvInt(&cfg.SystemDatabase.LogKeepDays, "SIMPLEBASE_LOG_KEEP_DAYS")

	applyEnvLLM(cfg)
	applyEnvAgent(cfg)
}

func applyEnvAgent(cfg *Config) {
	setEnvBool(&cfg.Agent.SkillsCLI, "SIMPLEBASE_AGENT_SKILLS_CLI")
	if v, ok := os.LookupEnv("SIMPLEBASE_AGENT_CLI_PATH"); ok {
		cfg.Agent.CLIPath = v
	}
	setEnvDuration(&cfg.Agent.DelegationTTL, "SIMPLEBASE_AGENT_DELEGATION_TTL")
	setEnvDuration(&cfg.Agent.ConfirmTimeout, "SIMPLEBASE_AGENT_CONFIRM_TIMEOUT")
	setEnvBool(&cfg.Agent.TraceLog, "SIMPLEBASE_AGENT_TRACE_LOG")
	// 列表型字段只放 YAML，不做含糊的分隔符环境变量。
	setEnvBool(&cfg.Agent.ShellIsolation.Enabled, "SIMPLEBASE_AGENT_SHELL_ISOLATION_ENABLED")
	if v, ok := os.LookupEnv("SIMPLEBASE_AGENT_SHELL_ISOLATION_MODE"); ok {
		cfg.Agent.ShellIsolation.Mode = v
	}
	setEnvDuration(&cfg.Agent.ShellIsolation.Timeout, "SIMPLEBASE_AGENT_SHELL_ISOLATION_TIMEOUT")
}

// applyEnvSandbox 映射 SIMPLEBASE_SANDBOX_* 环境变量。
func applyEnvSandbox(cfg *Config) {
	setEnvBool(&cfg.Sandbox.Enabled, "SIMPLEBASE_SANDBOX_ENABLED")
	if v, ok := os.LookupEnv("SIMPLEBASE_SANDBOX_IMAGE"); ok {
		cfg.Sandbox.Image = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_SANDBOX_NETWORK"); ok {
		cfg.Sandbox.Network = v
	}
	setEnvDuration(&cfg.Sandbox.ExecTimeout, "SIMPLEBASE_SANDBOX_EXEC_TIMEOUT")
	setEnvDuration(&cfg.Sandbox.IdleTimeout, "SIMPLEBASE_SANDBOX_IDLE_TIMEOUT")
	setEnvDuration(&cfg.Sandbox.MaxDuration, "SIMPLEBASE_SANDBOX_MAX_DURATION")
	if v, ok := os.LookupEnv("SIMPLEBASE_SANDBOX_BACKEND"); ok {
		cfg.Sandbox.Backend = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_SANDBOX_IMAGES"); ok {
		cfg.Sandbox.Images = nil
		cfg.Sandbox.Images = append(cfg.Sandbox.Images, strutil.SplitCSV(v)...)
	}
	setEnvInt(&cfg.Sandbox.MaxPerProject, "SIMPLEBASE_SANDBOX_MAX_PER_PROJECT")
	if v, ok := os.LookupEnv("SIMPLEBASE_SANDBOX_ROOT"); ok {
		cfg.Sandbox.Root = v
	}
	setEnvBool(&cfg.Sandbox.Terminal.Enabled, "SIMPLEBASE_SANDBOX_TERMINAL_ENABLED")
	if v, ok := os.LookupEnv("SIMPLEBASE_SANDBOX_SSH_LISTEN"); ok {
		cfg.Sandbox.SSH.Listen = v
	}
}

func applyEnvLLM(cfg *Config) {
	setEnvBool(&cfg.LLM.Enabled, "SIMPLEBASE_LLM_ENABLED")
	if v, ok := os.LookupEnv("SIMPLEBASE_LLM_AGENT_TOOL_PROTOCOL"); ok {
		cfg.LLM.AgentToolProtocol = v
	}
	setEnvInt(&cfg.LLM.AgentMaxIterations, "SIMPLEBASE_LLM_AGENT_MAX_ITERATIONS")
	setEnvDuration(&cfg.LLM.AgentRunTimeout, "SIMPLEBASE_LLM_AGENT_RUN_TIMEOUT")
	names := map[string]struct{}{}
	for n := range cfg.LLM.Providers {
		names[n] = struct{}{}
	}
	if csv, ok := os.LookupEnv("SIMPLEBASE_LLM_PROVIDERS"); ok && csv != "" {
		for _, n := range strutil.SplitCSV(csv) {
			names[n] = struct{}{}
		}
	}
	if cfg.LLM.Providers == nil {
		cfg.LLM.Providers = map[string]ProviderConfig{}
	}
	// 无具名实例供应商时，兼容 .env 中的 OpenAI 兼容网关简写配置。
	// 项目级凭证仍由 resolver 优先选择，密钥只通过环境变量注入。
	// 简写默认超时 60s，可用 SIMPLEBASE_LLM_PROVIDER_TIMEOUT 调大（多轮工具调用建议 ≥120s）；
	// 非法或非正值与具名 provider 的 _TIMEOUT 一致，静默回落到 60s。
	if len(names) == 0 {
		if key := strings.TrimSpace(os.Getenv("SIMPLEBASE_LLM_PROVIDER_API_KEY")); key != "" {
			timeout := 60 * time.Second
			if v, ok := os.LookupEnv("SIMPLEBASE_LLM_PROVIDER_TIMEOUT"); ok {
				if d, err := parseDuration(v); err == nil && d > 0 {
					timeout = d
				}
			}
			cfg.LLM.Providers["openai"] = ProviderConfig{
				APIKey:       key,
				BaseURL:      strings.TrimSpace(os.Getenv("SIMPLEBASE_LLM_PROVIDER_BASE_URL")),
				DefaultModel: strings.TrimSpace(os.Getenv("SIMPLEBASE_LLM_PROVIDER_DEFAULT_MODEL")),
				Timeout:      timeout,
			}
		}
	}
	for name := range names {
		upper := strings.ToUpper(name)
		p := cfg.LLM.Providers[name]
		if v, ok := os.LookupEnv("SIMPLEBASE_LLM_PROVIDER_" + upper + "_API_KEY"); ok {
			p.APIKey = v
		}
		if v, ok := os.LookupEnv("SIMPLEBASE_LLM_PROVIDER_" + upper + "_BASE_URL"); ok {
			p.BaseURL = v
		}
		if v, ok := os.LookupEnv("SIMPLEBASE_LLM_PROVIDER_" + upper + "_DEFAULT_MODEL"); ok {
			p.DefaultModel = v
		}
		if _, ok := os.LookupEnv("SIMPLEBASE_LLM_PROVIDER_" + upper + "_ALLOWED_MODELS"); ok {
			p.AllowedModels = envList("SIMPLEBASE_LLM_PROVIDER_" + upper + "_ALLOWED_MODELS")
		}
		if v, ok := os.LookupEnv("SIMPLEBASE_LLM_PROVIDER_" + upper + "_TIMEOUT"); ok {
			if d, err := parseDuration(v); err == nil {
				p.Timeout = d
			}
		}
		if p.Timeout == 0 {
			p.Timeout = 60 * time.Second
		}
		cfg.LLM.Providers[name] = p
	}
}

func setEnvBool(dst *bool, key string) {
	if v, ok := os.LookupEnv(key); ok {
		b, err := strconv.ParseBool(v)
		if err == nil {
			*dst = b
		}
	}
}

func setEnvInt(dst *int, key string) {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.Atoi(v)
		if err == nil {
			*dst = n
		}
	}
}

func setEnvInt64(dst *int64, key string) {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			*dst = n
		}
	}
}

func setEnvFloat64(dst *float64, key string) {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.ParseFloat(v, 64)
		if err == nil {
			*dst = n
		}
	}
}

func setEnvDuration(dst *time.Duration, key string) {
	if v, ok := os.LookupEnv(key); ok {
		d, err := parseDuration(v)
		if err == nil {
			*dst = d
		}
	}
}

func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return def
		}
		return b
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return def
		}
		return n
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return def
		}
		return n
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		d, err := parseDuration(v)
		if err != nil {
			return def
		}
		return d
	}
	return def
}

func envFloat64(key string, def float64) float64 {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return def
		}
		return n
	}
	return def
}

func parseDuration(v string) (time.Duration, error) {
	if d, err := time.ParseDuration(v); err == nil {
		return d, nil
	}
	if strings.HasSuffix(v, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("invalid duration %q", v)
}

func envList(key string) []string {
	v := envStr(key, "")
	if v == "" {
		return nil
	}
	return strutil.SplitCSV(v)
}

// loadLLMProviders is kept for tests that call it directly; it only reads env.
func loadLLMProviders() map[string]ProviderConfig {
	cfg := defaults()
	applyEnvLLM(&cfg)
	if len(cfg.LLM.Providers) == 0 {
		return map[string]ProviderConfig{}
	}
	return cfg.LLM.Providers
}
