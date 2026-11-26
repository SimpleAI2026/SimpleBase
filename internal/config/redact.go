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
//	[下游] 同包配置类型与归一化方法
//	[同包] backup.go, config.go, defaults.go, env.go, http_boundary.go, yaml.go
//
// 主要符号
//
//	func (c Config) Redacted() map[string]any
//	redactedHTTP(c HTTPConfig) map[string]any
//	redactedDatabase(c DatabaseConfig) map[string]any

package config

// Redacted 返回用于启动日志的安全视图。绝不包含密钥/DSN。
func (c Config) Redacted() map[string]any {
	return map[string]any{
		"http":            redactedHTTP(c.HTTP),
		"instance":        redactedInstance(c.Instance),
		"database":        redactedDatabase(c.Database),
		"s3":              redactedS3(c.S3),
		"system_database": redactedSystemDatabase(c.SystemDatabase),
		"limits":          redactedLimits(c.Limits),
		"llm":             redactedLLM(c.LLM),
		"sandbox":         redactedSandbox(c.Sandbox),
		"storage":         redactedStorage(c),
		"sites":           redactedSites(c.Sites),
		"observability":   redactedObservability(c.Observability),
	}
}

func redactedHTTP(c HTTPConfig) map[string]any {
	return map[string]any{
		"address":          c.Address,
		"read_timeout":     c.ReadTimeout.String(),
		"write_timeout":    c.WriteTimeout.String(),
		"idle_timeout":     c.IdleTimeout.String(),
		"shutdown_timeout": c.ShutdownTimeout.String(),
	}
}

func redactedInstance(c InstanceConfig) map[string]any {
	return map[string]any{
		"id":       c.ID,
		"writable": c.Writable,
		"lease": map[string]any{
			"enabled":        c.Lease.Enabled,
			"ttl":            c.Lease.TTL.String(),
			"renew_interval": c.Lease.RenewInterval.String(),
			"grace":          c.Lease.Grace.String(),
			"on_lost":        c.Lease.OnLost,
		},
	}
}

func redactedDatabase(c DatabaseConfig) map[string]any {
	return map[string]any{
		"engine":              c.Engine,
		"cache_dir":           c.CacheDir,
		"cache_max_bytes":     c.CacheMaxBytes,
		"cache_max_databases": c.CacheMaxDatabases,
		"idle_timeout":        c.IdleTimeout.String(),
		"max_open":            c.MaxOpen,
		"ducklake": map[string]any{
			"catalog_engine":             c.DuckLake.CatalogEngine,
			"memory_limit":               c.DuckLake.MemoryLimit,
			"threads":                    c.DuckLake.Threads,
			"extension_dir":              c.DuckLake.ExtensionDir,
			"data_inlining_row_limit":    c.DuckLake.DataInliningRowLimit,
			"parquet_compression":        c.DuckLake.ParquetCompression,
			"target_file_size":           c.DuckLake.TargetFileSize,
			"require_commit_message":     c.DuckLake.RequireCommitMessage,
			"catalog_sync_mode":          c.DuckLake.CatalogSync.Mode,
			"catalog_sync_debounce":      c.DuckLake.CatalogSync.Debounce.String(),
			"catalog_sync_interval":      c.DuckLake.CatalogSync.Interval.String(),
			"catalog_sync_max_lag":       c.DuckLake.CatalogSync.MaxLag.String(),
			"catalog_sync_keep_versions": c.DuckLake.CatalogSync.KeepVersions,
		},
	}
}

func redactedS3(c S3Config) map[string]any {
	return map[string]any{
		"endpoint":         c.Endpoint,
		"region":           c.Region,
		"bucket":           c.Bucket,
		"prefix":           c.Prefix,
		"force_path_style": c.ForcePathStyle,
		"has_access_key":   c.AccessKey != "",
		"has_secret_key":   c.SecretKey != "",
		"kms_key_id":       c.KMSKeyID,
	}
}

func redactedSystemDatabase(c SystemDatabaseConfig) map[string]any {
	return map[string]any{
		"name":                   c.Name,
		"metrics_flush_interval": c.MetricsFlushInterval.String(),
		"log_flush_interval":     c.LogFlushInterval.String(),
		"log_keep_days":          c.LogKeepDays,
	}
}

func redactedLimits(c LimitsConfig) map[string]any {
	return map[string]any{
		"max_request_bytes":      c.MaxRequestBytes,
		"max_query_rows":         c.MaxQueryRows,
		"query_timeout":          c.QueryTimeout.String(),
		"max_concurrent_queries": c.MaxConcurrentQueries,
		"max_batch_statements":   c.MaxBatchStatements,
		"max_sql_bytes":          c.MaxSQLBytes,
	}
}

func redactedLLM(c LLMConfig) map[string]any {
	providers := make(map[string]any, len(c.Providers))
	for name, p := range c.Providers {
		providers[name] = map[string]any{
			"has_api_key":    p.APIKey != "",
			"base_url":       p.BaseURL,
			"default_model":  p.DefaultModel,
			"allowed_models": p.AllowedModels,
		}
	}
	return map[string]any{
		"enabled":   c.Enabled,
		"providers": providers,
	}
}

func redactedSandbox(c SandboxConfig) map[string]any {
	return map[string]any{
		"enabled":          c.Enabled,
		"backend":          c.EffectiveBackend(),
		"images":           c.Images,
		"exec_timeout_max": c.ExecTimeoutMax.String(),
		"max_per_project":  c.MaxPerProject,
		"reap_interval":    c.ReapInterval.String(),
		"root":             c.Root,
		"pids_max":         c.PidsMax,
		"image":            c.Image,
		"cpus":             c.CPUs,
		"memory_mib":       c.MemoryMiB,
		"max_duration":     c.MaxDuration.String(),
		"idle_timeout":     c.IdleTimeout.String(),
		"exec_timeout":     c.ExecTimeout.String(),
		"max_output_bytes": c.MaxOutputBytes,
		"max_file_bytes":   c.MaxFileBytes,
		"network":          c.Network,
		"workdir":          c.Workdir,
	}
}

func redactedStorage(c Config) map[string]any {
	return map[string]any{
		"database":           c.effectiveStorage(c.Storage.Database),
		"files":              c.effectiveStorage(c.Storage.Files),
		"data_dir":           c.Storage.DataDir,
		"database_is_remote": c.IsDBRemote(),
		"files_is_remote":    c.IsFilesRemote(),
	}
}

func redactedSites(c SitesConfig) map[string]any {
	return map[string]any{
		"base_domain": c.BaseDomain,
		"url_scheme":  c.URLScheme,
		"url_port":    c.URLPort,
	}
}

func redactedObservability(c ObservabilityConfig) map[string]any {
	return map[string]any{
		"log_level":         c.LogLevel,
		"log_format":        c.LogFormat,
		"log_output":        c.LogOutput,
		"metrics_path":      c.MetricsPath,
		"metrics_address":   c.MetricsAddress,
		"pprof":             c.PProf,
		"perf_stage_timing": c.PerfStageTiming,
	}
}
