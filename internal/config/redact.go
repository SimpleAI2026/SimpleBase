// 文件说明：redact.go — 配置脱敏视图：Config.Redacted 输出可安全落日志 / 控制台的 map
//
// 用法
//
//	Redacted（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api, internal/api/perfbench 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/config/redact.go
//	         │
//	         ▼
//	[下游] internal/utils/strutil
//	[同包] backup.go, env.go, http_boundary.go, yaml.go
//
// 主要符号
//
//	func (c Config) Redacted() map[string]any

package config

// Redacted 返回用于启动日志的安全视图。绝不包含密钥/DSN。
func (c Config) Redacted() map[string]any {
	providers := make(map[string]any, len(c.LLM.Providers))
	for name, p := range c.LLM.Providers {
		entry := map[string]any{
			"has_api_key":    p.APIKey != "",
			"base_url":       p.BaseURL,
			"default_model":  p.DefaultModel,
			"allowed_models": p.AllowedModels,
		}
		providers[name] = entry
	}
	return map[string]any{
		"http": map[string]any{
			"address":          c.HTTP.Address,
			"read_timeout":     c.HTTP.ReadTimeout.String(),
			"write_timeout":    c.HTTP.WriteTimeout.String(),
			"idle_timeout":     c.HTTP.IdleTimeout.String(),
			"shutdown_timeout": c.HTTP.ShutdownTimeout.String(),
		},
		"instance": map[string]any{
			"id":       c.Instance.ID,
			"writable": c.Instance.Writable,
			"lease": map[string]any{
				"enabled":        c.Instance.Lease.Enabled,
				"ttl":            c.Instance.Lease.TTL.String(),
				"renew_interval": c.Instance.Lease.RenewInterval.String(),
				"grace":          c.Instance.Lease.Grace.String(),
				"on_lost":        c.Instance.Lease.OnLost,
			},
		},
		"database": map[string]any{
			"engine":              c.Database.Engine,
			"cache_dir":           c.Database.CacheDir,
			"cache_max_bytes":     c.Database.CacheMaxBytes,
			"cache_max_databases": c.Database.CacheMaxDatabases,
			"idle_timeout":        c.Database.IdleTimeout.String(),
			"max_open":            c.Database.MaxOpen,
			"ducklake": map[string]any{
				"catalog_engine":             c.Database.DuckLake.CatalogEngine,
				"memory_limit":               c.Database.DuckLake.MemoryLimit,
				"threads":                    c.Database.DuckLake.Threads,
				"extension_dir":              c.Database.DuckLake.ExtensionDir,
				"data_inlining_row_limit":    c.Database.DuckLake.DataInliningRowLimit,
				"parquet_compression":        c.Database.DuckLake.ParquetCompression,
				"target_file_size":           c.Database.DuckLake.TargetFileSize,
				"require_commit_message":     c.Database.DuckLake.RequireCommitMessage,
				"catalog_sync_mode":          c.Database.DuckLake.CatalogSync.Mode,
				"catalog_sync_debounce":      c.Database.DuckLake.CatalogSync.Debounce.String(),
				"catalog_sync_interval":      c.Database.DuckLake.CatalogSync.Interval.String(),
				"catalog_sync_max_lag":       c.Database.DuckLake.CatalogSync.MaxLag.String(),
				"catalog_sync_keep_versions": c.Database.DuckLake.CatalogSync.KeepVersions,
			},
		},
		"s3": map[string]any{
			"endpoint":         c.S3.Endpoint,
			"region":           c.S3.Region,
			"bucket":           c.S3.Bucket,
			"prefix":           c.S3.Prefix,
			"force_path_style": c.S3.ForcePathStyle,
			"has_access_key":   c.S3.AccessKey != "",
			"has_secret_key":   c.S3.SecretKey != "",
			"kms_key_id":       c.S3.KMSKeyID,
		},
		"system_database": map[string]any{
			"name":                   c.SystemDatabase.Name,
			"metrics_flush_interval": c.SystemDatabase.MetricsFlushInterval.String(),
			"log_flush_interval":     c.SystemDatabase.LogFlushInterval.String(),
			"log_keep_days":          c.SystemDatabase.LogKeepDays,
		},
		"limits": map[string]any{
			"max_request_bytes":      c.Limits.MaxRequestBytes,
			"max_query_rows":         c.Limits.MaxQueryRows,
			"query_timeout":          c.Limits.QueryTimeout.String(),
			"max_concurrent_queries": c.Limits.MaxConcurrentQueries,
			"max_batch_statements":   c.Limits.MaxBatchStatements,
			"max_sql_bytes":          c.Limits.MaxSQLBytes,
		},
		"llm": map[string]any{
			"enabled":   c.LLM.Enabled,
			"providers": providers,
		},
		"sandbox": map[string]any{
			"enabled":          c.Sandbox.Enabled,
			"backend":          c.Sandbox.EffectiveBackend(),
			"images":           c.Sandbox.Images,
			"exec_timeout_max": c.Sandbox.ExecTimeoutMax.String(),
			"max_per_project":  c.Sandbox.MaxPerProject,
			"reap_interval":    c.Sandbox.ReapInterval.String(),
			"root":             c.Sandbox.Root,
			"pids_max":         c.Sandbox.PidsMax,
			"image":            c.Sandbox.Image,
			"cpus":             c.Sandbox.CPUs,
			"memory_mib":       c.Sandbox.MemoryMiB,
			"max_duration":     c.Sandbox.MaxDuration.String(),
			"idle_timeout":     c.Sandbox.IdleTimeout.String(),
			"exec_timeout":     c.Sandbox.ExecTimeout.String(),
			"max_output_bytes": c.Sandbox.MaxOutputBytes,
			"max_file_bytes":   c.Sandbox.MaxFileBytes,
			"network":          c.Sandbox.Network,
			"workdir":          c.Sandbox.Workdir,
		},
		"storage": map[string]any{
			"database":           c.effectiveStorage(c.Storage.Database),
			"files":              c.effectiveStorage(c.Storage.Files),
			"data_dir":           c.Storage.DataDir,
			"database_is_remote": c.IsDBRemote(),
			"files_is_remote":    c.IsFilesRemote(),
		},
		"sites": map[string]any{
			"base_domain": c.Sites.BaseDomain,
			"url_scheme":  c.Sites.URLScheme,
			"url_port":    c.Sites.URLPort,
		},
		"observability": map[string]any{
			"log_level":         c.Observability.LogLevel,
			"log_format":        c.Observability.LogFormat,
			"log_output":        c.Observability.LogOutput,
			"metrics_path":      c.Observability.MetricsPath,
			"metrics_address":   c.Observability.MetricsAddress,
			"pprof":             c.Observability.PProf,
			"perf_stage_timing": c.Observability.PerfStageTiming,
		},
	}
}
