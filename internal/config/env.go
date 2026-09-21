package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func applyEnv(cfg *Config) {
	if v, ok := os.LookupEnv("SIMPLEBASE_DEV_MODE"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.DevMode = b
		}
	}
	if v, ok := os.LookupEnv("SIMPLEBASE_HTTP_ADDRESS"); ok {
		cfg.HTTP.Address = v
	}
	setEnvDuration(&cfg.HTTP.ReadTimeout, "SIMPLEBASE_HTTP_READ_TIMEOUT")
	setEnvDuration(&cfg.HTTP.WriteTimeout, "SIMPLEBASE_HTTP_WRITE_TIMEOUT")
	setEnvDuration(&cfg.HTTP.IdleTimeout, "SIMPLEBASE_HTTP_IDLE_TIMEOUT")
	setEnvDuration(&cfg.HTTP.ShutdownTimeout, "SIMPLEBASE_HTTP_SHUTDOWN_TIMEOUT")

	if v, ok := os.LookupEnv("SIMPLEBASE_INSTANCE_ID"); ok {
		cfg.Instance.ID = v
	}
	setEnvBool(&cfg.Instance.Writable, "SIMPLEBASE_INSTANCE_WRITABLE")

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

	setEnvInt64(&cfg.Limits.MaxRequestBytes, "SIMPLEBASE_LIMITS_MAX_REQUEST_BYTES")
	setEnvInt(&cfg.Limits.MaxQueryRows, "SIMPLEBASE_LIMITS_MAX_QUERY_ROWS")
	setEnvDuration(&cfg.Limits.QueryTimeout, "SIMPLEBASE_LIMITS_QUERY_TIMEOUT")
	setEnvInt(&cfg.Limits.MaxConcurrentQueries, "SIMPLEBASE_LIMITS_MAX_CONCURRENT_QUERIES")
	setEnvInt(&cfg.Limits.MaxBatchStatements, "SIMPLEBASE_LIMITS_MAX_BATCH_STATEMENTS")
	setEnvInt(&cfg.Limits.MaxSQLBytes, "SIMPLEBASE_LIMITS_MAX_SQL_BYTES")

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

	if v, ok := os.LookupEnv("SIMPLEBASE_SYSTEM_DB_NAME"); ok {
		cfg.SystemDatabase.Name = v
	}
	setEnvDuration(&cfg.SystemDatabase.MetricsFlushInterval, "SIMPLEBASE_METRICS_FLUSH_INTERVAL")
	setEnvDuration(&cfg.SystemDatabase.LogFlushInterval, "SIMPLEBASE_LOG_FLUSH_INTERVAL")
	setEnvInt(&cfg.SystemDatabase.LogKeepDays, "SIMPLEBASE_LOG_KEEP_DAYS")

	applyEnvLLM(cfg)
}

func applyEnvLLM(cfg *Config) {
	setEnvBool(&cfg.LLM.Enabled, "SIMPLEBASE_LLM_ENABLED")
	names := map[string]struct{}{}
	for n := range cfg.LLM.Providers {
		names[n] = struct{}{}
	}
	if csv, ok := os.LookupEnv("SIMPLEBASE_LLM_PROVIDERS"); ok && csv != "" {
		for _, n := range strings.Split(csv, ",") {
			n = strings.TrimSpace(n)
			if n != "" {
				names[n] = struct{}{}
			}
		}
	}
	if cfg.LLM.Providers == nil {
		cfg.LLM.Providers = map[string]ProviderConfig{}
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
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
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
