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
	applyEnvHTTP(&cfg.HTTP)
	applyEnvFiles(&cfg.Files)
	applyEnvBackup(&cfg.Backup)
	applyEnvSites(&cfg.Sites)
	applyEnvInstance(&cfg.Instance)
	applyEnvStorage(&cfg.Storage)
	applyEnvDatabase(&cfg.Database)
	applyEnvS3(&cfg.S3)
	applyEnvAuth(&cfg.Auth)
	applyEnvSandbox(cfg)
	applyEnvLimits(&cfg.Limits)
	applyEnvObservability(&cfg.Observability)
	applyEnvSystemDatabase(&cfg.SystemDatabase)
	applyEnvLLM(cfg)
	applyEnvAgent(cfg)
}

func applyEnvHTTP(dst *HTTPConfig) {
	if v, ok := os.LookupEnv("SIMPLEBASE_HTTP_ADDRESS"); ok {
		dst.Address = v
	}
	setEnvDuration(&dst.ReadTimeout, "SIMPLEBASE_HTTP_READ_TIMEOUT")
	setEnvDuration(&dst.WriteTimeout, "SIMPLEBASE_HTTP_WRITE_TIMEOUT")
	setEnvDuration(&dst.IdleTimeout, "SIMPLEBASE_HTTP_IDLE_TIMEOUT")
	setEnvDuration(&dst.ShutdownTimeout, "SIMPLEBASE_HTTP_SHUTDOWN_TIMEOUT")
	setEnvBool(&dst.Gzip, "SIMPLEBASE_HTTP_GZIP")
	setEnvBool(&dst.CORS.Enabled, "SIMPLEBASE_HTTP_CORS_ENABLED")
	setEnvBool(&dst.CORS.AllowCredentials, "SIMPLEBASE_HTTP_CORS_ALLOW_CREDENTIALS")
	setEnvInt(&dst.CORS.MaxAge, "SIMPLEBASE_HTTP_CORS_MAX_AGE")
	if v, ok := os.LookupEnv("SIMPLEBASE_HTTP_CORS_ALLOWED_ORIGINS"); ok {
		dst.CORS.AllowedOrigins = strutil.SplitCSV(v)
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_HTTP_TRUSTED_PROXIES"); ok {
		dst.TrustedProxies = strutil.SplitCSV(v)
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_HTTP_PUBLIC_URL"); ok {
		dst.PublicURL = v
	}
	setEnvInt(&dst.SecurityHeaders.HSTSMaxAge, "SIMPLEBASE_HTTP_HSTS_MAX_AGE")
	setEnvBool(&dst.SecurityHeaders.HSTSIncludeSubdomains, "SIMPLEBASE_HTTP_HSTS_INCLUDE_SUBDOMAINS")
	if v, ok := os.LookupEnv("SIMPLEBASE_HTTP_SECURITY_HEADERS_CSP"); ok {
		dst.SecurityHeaders.CSP = v
	}
}

func applyEnvFiles(dst *FilesConfig) {
	if v, ok := os.LookupEnv("SIMPLEBASE_FILES_TOKEN_SECRET"); ok {
		dst.TokenSecret = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_FILES_ALLOWED_MIME"); ok {
		dst.AllowedMIME = strutil.SplitCSV(v)
	}
}

func applyEnvBackup(dst *BackupConfig) {
	setEnvBool(&dst.Enabled, "SIMPLEBASE_BACKUP_ENABLED")
	setEnvBool(&dst.UploadToS3, "SIMPLEBASE_BACKUP_UPLOAD_TO_S3")
	setEnvInt(&dst.KeepLast, "SIMPLEBASE_BACKUP_KEEP_LAST")
	setEnvInt(&dst.KeepDays, "SIMPLEBASE_BACKUP_KEEP_DAYS")
	setEnvInt(&dst.MaxPerProject, "SIMPLEBASE_BACKUP_MAX_PER_PROJECT")
	setEnvDuration(&dst.SnapshotTimeout, "SIMPLEBASE_BACKUP_SNAPSHOT_TIMEOUT")
	if v, ok := os.LookupEnv("SIMPLEBASE_BACKUP_DIR"); ok {
		dst.Dir = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_BACKUP_SCHEDULE"); ok {
		dst.Schedule = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_BACKUP_MISSED_POLICY"); ok {
		dst.MissedPolicy = v
	}
}

func applyEnvSites(dst *SitesConfig) {
	// planv6.5：对象存储发布网站；planv6.7：子域名 / 路径两种方式。
	setEnvBool(&dst.Subdomain, "SIMPLEBASE_SITES_SUBDOMAIN")
	if v, ok := os.LookupEnv("SIMPLEBASE_SITES_PATH_PREFIX"); ok {
		dst.PathPrefix = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_SITES_PUBLIC_BASE_URL"); ok {
		dst.PublicBaseURL = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_SITES_BASE_DOMAIN"); ok {
		dst.BaseDomain = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_SITES_URL_SCHEME"); ok {
		dst.URLScheme = v
	}
	setEnvInt(&dst.URLPort, "SIMPLEBASE_SITES_URL_PORT")
	if v, ok := os.LookupEnv("SIMPLEBASE_SITES_INDEX_DOCUMENT"); ok {
		dst.IndexDocument = v
	}
	setEnvBool(&dst.SpaFallback, "SIMPLEBASE_SITES_SPA_FALLBACK")
	if v, ok := os.LookupEnv("SIMPLEBASE_SITES_NOT_FOUND_DOCUMENT"); ok {
		dst.NotFoundDocument = v
	}
	setEnvDuration(&dst.CacheTTL, "SIMPLEBASE_SITES_CACHE_TTL")
	if v, ok := os.LookupEnv("SIMPLEBASE_SITES_CSP"); ok {
		dst.CSP = v
	}
}

func applyEnvInstance(dst *InstanceConfig) {
	if v, ok := os.LookupEnv("SIMPLEBASE_INSTANCE_ID"); ok {
		dst.ID = v
	}
	setEnvBool(&dst.Writable, "SIMPLEBASE_INSTANCE_WRITABLE")
	setEnvBool(&dst.Lease.Enabled, "SIMPLEBASE_INSTANCE_LEASE_ENABLED")
	setEnvDuration(&dst.Lease.TTL, "SIMPLEBASE_INSTANCE_LEASE_TTL")
	setEnvDuration(&dst.Lease.RenewInterval, "SIMPLEBASE_INSTANCE_LEASE_RENEW_INTERVAL")
	setEnvDuration(&dst.Lease.Grace, "SIMPLEBASE_INSTANCE_LEASE_GRACE")
	if v, ok := os.LookupEnv("SIMPLEBASE_INSTANCE_LEASE_ON_LOST"); ok {
		dst.Lease.OnLost = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_INSTANCE_PROFILE"); ok {
		dst.Profile = v
	}
}

func applyEnvStorage(dst *StorageConfig) {
	if v, ok := os.LookupEnv("SIMPLEBASE_STORAGE_DATABASE"); ok {
		dst.Database = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_STORAGE_FILES"); ok {
		dst.Files = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_DATA_DIR"); ok {
		dst.DataDir = v
	}
}

func applyEnvDatabase(dst *DatabaseConfig) {
	if v, ok := os.LookupEnv("SIMPLEBASE_DB_ENGINE"); ok {
		dst.Engine = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_DB_CACHE_DIR"); ok {
		dst.CacheDir = v
	}
	setEnvInt64(&dst.CacheMaxBytes, "SIMPLEBASE_DB_CACHE_MAX_BYTES")
	setEnvInt(&dst.CacheMaxDatabases, "SIMPLEBASE_DB_CACHE_MAX_DATABASES")
	setEnvDuration(&dst.IdleTimeout, "SIMPLEBASE_DB_IDLE_TIMEOUT")
	setEnvInt(&dst.MaxOpen, "SIMPLEBASE_DB_MAX_OPEN")
	applyEnvDuckLake(&dst.DuckLake)
}

func applyEnvAuth(dst *AuthConfig) {
	if v, ok := os.LookupEnv("SIMPLEBASE_AUTH_APIKEY_SECRET"); ok {
		dst.APIKeyHashSecret = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_AUTH_SESSION_SECRET"); ok {
		dst.SessionSecret = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_AUTH_APIKEY_SECRETS"); ok {
		dst.APIKeyHashSecrets = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_BOOTSTRAP_PASSWORD"); ok {
		dst.BootstrapPassword = v
	}
}

func applyEnvLimits(dst *LimitsConfig) {
	setEnvBool(&dst.RateLimit.Enabled, "SIMPLEBASE_RATE_LIMIT_ENABLED")
	setEnvInt(&dst.RateLimit.Burst, "SIMPLEBASE_RATE_LIMIT_BURST")
	setEnvBool(&dst.RateLimit.ExemptSSE, "SIMPLEBASE_RATE_LIMIT_EXEMPT_SSE")
	setEnvInt64(&dst.MaxRequestBytes, "SIMPLEBASE_LIMITS_MAX_REQUEST_BYTES")
	setEnvInt(&dst.MaxQueryRows, "SIMPLEBASE_LIMITS_MAX_QUERY_ROWS")
	setEnvDuration(&dst.QueryTimeout, "SIMPLEBASE_LIMITS_QUERY_TIMEOUT")
	setEnvInt(&dst.MaxConcurrentQueries, "SIMPLEBASE_LIMITS_MAX_CONCURRENT_QUERIES")
	setEnvInt(&dst.MaxBatchStatements, "SIMPLEBASE_LIMITS_MAX_BATCH_STATEMENTS")
	setEnvInt(&dst.MaxSQLBytes, "SIMPLEBASE_LIMITS_MAX_SQL_BYTES")
	setEnvInt(&dst.MaxConcurrentGoRuns, "SIMPLEBASE_LIMITS_MAX_CONCURRENT_GO_RUNS")
	setEnvInt(&dst.MaxFilterScanRows, "SIMPLEBASE_LIMITS_MAX_FILTER_SCAN_ROWS")
	setEnvInt64(&dst.MaxImportBytes, "SIMPLEBASE_LIMITS_MAX_IMPORT_BYTES")
	setEnvInt(&dst.RealtimeMaxConns, "SIMPLEBASE_LIMITS_REALTIME_MAX_CONNS")
	setEnvInt(&dst.RateLimit.MaxBuckets, "SIMPLEBASE_RATE_LIMIT_MAX_BUCKETS")
}

func applyEnvObservability(dst *ObservabilityConfig) {
	if v, ok := os.LookupEnv("SIMPLEBASE_LOG_LEVEL"); ok {
		dst.LogLevel = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_LOG_FORMAT"); ok {
		dst.LogFormat = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_LOG_OUTPUT"); ok {
		dst.LogOutput = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_METRICS_PATH"); ok {
		dst.MetricsPath = v
	}
	setEnvBool(&dst.PerfStageTiming, "SIMPLEBASE_PERF_STAGE_TIMING")
	if v, ok := os.LookupEnv("SIMPLEBASE_METRICS_ADDRESS"); ok {
		dst.MetricsAddress = v
	}
	setEnvBool(&dst.PProf, "SIMPLEBASE_PPROF")
	if v, ok := os.LookupEnv("SIMPLEBASE_METRICS_TOKEN"); ok {
		dst.MetricsToken = v
	}
}

func applyEnvSystemDatabase(dst *SystemDatabaseConfig) {
	if v, ok := os.LookupEnv("SIMPLEBASE_SYSTEM_DB_NAME"); ok {
		dst.Name = v
	}
	setEnvDuration(&dst.MetricsFlushInterval, "SIMPLEBASE_METRICS_FLUSH_INTERVAL")
	setEnvDuration(&dst.LogFlushInterval, "SIMPLEBASE_LOG_FLUSH_INTERVAL")
	setEnvInt(&dst.LogKeepDays, "SIMPLEBASE_LOG_KEEP_DAYS")
}

// applyEnvDuckLake 应用 SIMPLEBASE_DUCKLAKE_* 环境变量（LookupEnv 语义：设置即覆盖，含空串）。
func applyEnvDuckLake(dst *DuckLakeConfig) {
	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_CATALOG_ENGINE"); ok {
		dst.CatalogEngine = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_MEMORY_LIMIT"); ok {
		dst.MemoryLimit = v
	}
	setEnvInt(&dst.Threads, "SIMPLEBASE_DUCKLAKE_THREADS")
	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_EXTENSION_DIR"); ok {
		dst.ExtensionDir = v
	}
	setEnvInt(&dst.DataInliningRowLimit, "SIMPLEBASE_DUCKLAKE_DATA_INLINING_ROW_LIMIT")
	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_PARQUET_COMPRESSION"); ok {
		dst.ParquetCompression = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_TARGET_FILE_SIZE"); ok {
		dst.TargetFileSize = v
	}
	setEnvBool(&dst.RequireCommitMessage, "SIMPLEBASE_DUCKLAKE_REQUIRE_COMMIT_MESSAGE")
	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_SYNC_MODE"); ok {
		dst.CatalogSync.Mode = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_DUCKLAKE_SYNC_DEBOUNCE_MS"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			dst.CatalogSync.Debounce = time.Duration(n) * time.Millisecond
		}
	}
	setEnvDuration(&dst.CatalogSync.Interval, "SIMPLEBASE_DUCKLAKE_SYNC_INTERVAL")
	setEnvDuration(&dst.CatalogSync.MaxLag, "SIMPLEBASE_DUCKLAKE_SYNC_MAX_LAG")
	setEnvInt(&dst.CatalogSync.KeepVersions, "SIMPLEBASE_DUCKLAKE_SYNC_KEEP_VERSIONS")
	setEnvDuration(&dst.Maintenance.CheckpointInterval, "SIMPLEBASE_DUCKLAKE_MAINT_CHECKPOINT_INTERVAL")
	setEnvDuration(&dst.Maintenance.ExpireOlderThan, "SIMPLEBASE_DUCKLAKE_MAINT_EXPIRE_OLDER_THAN")
	setEnvDuration(&dst.Maintenance.DeleteOlderThan, "SIMPLEBASE_DUCKLAKE_MAINT_DELETE_OLDER_THAN")
	setEnvFloat64(&dst.Maintenance.RewriteDeleteThreshold, "SIMPLEBASE_DUCKLAKE_MAINT_REWRITE_DELETE_THRESHOLD")
}

// applyEnvS3 应用 SIMPLEBASE_S3_* 环境变量（含密钥；LookupEnv 语义）。
func applyEnvS3(dst *S3Config) {
	if v, ok := os.LookupEnv("SIMPLEBASE_S3_ENDPOINT"); ok {
		dst.Endpoint = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_S3_REGION"); ok {
		dst.Region = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_S3_BUCKET"); ok {
		dst.Bucket = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_S3_PREFIX"); ok {
		dst.Prefix = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_S3_ACCESS_KEY"); ok {
		dst.AccessKey = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_S3_SECRET_KEY"); ok {
		dst.SecretKey = v
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_S3_KMS_KEY_ID"); ok {
		dst.KMSKeyID = v
	}
	setEnvBool(&dst.ForcePathStyle, "SIMPLEBASE_S3_FORCE_PATH_STYLE")
}

func applyEnvAgent(cfg *Config) {
	setEnvBool(&cfg.Agent.SkillsCLI, "SIMPLEBASE_AGENT_SKILLS_CLI")
	if v, ok := os.LookupEnv("SIMPLEBASE_AGENT_CLI_PATH"); ok {
		cfg.Agent.CLIPath = v
	}
	setEnvDuration(&cfg.Agent.DelegationTTL, "SIMPLEBASE_AGENT_DELEGATION_TTL")
	setEnvDuration(&cfg.Agent.ConfirmTimeout, "SIMPLEBASE_AGENT_CONFIRM_TIMEOUT")
	setEnvBool(&cfg.Agent.TraceLog, "SIMPLEBASE_AGENT_TRACE_LOG")
	setEnvBool(&cfg.Agent.MCP.Enabled, "SIMPLEBASE_AGENT_MCP_ENABLED")
	setEnvBool(&cfg.Agent.MCP.Builtin, "SIMPLEBASE_AGENT_MCP_BUILTIN")
	setEnvBool(&cfg.Agent.MCP.AllowHTTP, "SIMPLEBASE_AGENT_MCP_ALLOW_HTTP")
	if v, ok := os.LookupEnv("SIMPLEBASE_AGENT_MCP_MAX_SERVERS_PER_PROJECT"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Agent.MCP.MaxServersPerProject = n
		}
	}
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
