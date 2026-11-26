// 文件说明：defaults.go — 按配置域构造 SimpleBase 的代码默认值
//
// 用法
//
//	defaults() // Load 在 YAML 与环境变量覆盖前调用
//
// 调用架构
//
//	[上游] internal/config/config.go 的 Load
//	         │ 同包调用
//	         ▼
//	[本文件] internal/config/defaults.go
//	         │ 依赖
//	         ▼
//	[下游] 标准库 time 与同包配置类型
//	[同包] config.go, env.go, yaml.go
//
// 主要符号
//
//	defaults() Config
//	defaultHTTPConfig() HTTPConfig
//	defaultDatabaseConfig() DatabaseConfig

package config

import "time"

func defaults() Config {
	return Config{
		Storage:        defaultStorageConfig(),
		HTTP:           defaultHTTPConfig(),
		Instance:       defaultInstanceConfig(),
		Database:       defaultDatabaseConfig(),
		S3:             defaultS3Config(),
		LLM:            defaultLLMConfig(),
		Agent:          defaultAgentConfig(),
		Sandbox:        defaultSandboxConfig(),
		Limits:         defaultLimitsConfig(),
		Observability:  defaultObservabilityConfig(),
		SystemDatabase: defaultSystemDatabaseConfig(),
		Backup:         defaultBackupConfig(),
		Sites:          defaultSitesConfig(),
	}
}

func defaultStorageConfig() StorageConfig {
	return StorageConfig{
		Database: StorageModeLocal,
		Files:    StorageModeLocal,
		DataDir:  DefaultDataDir,
	}
}

func defaultHTTPConfig() HTTPConfig {
	return HTTPConfig{
		Address:         ":8080",
		ReadTimeout:     15 * time.Second,
		WriteTimeout:    30 * time.Second,
		IdleTimeout:     60 * time.Second,
		ShutdownTimeout: 30 * time.Second,
		CORS: CORSConfig{
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Authorization", "Content-Type", "X-Request-Id"},
			ExposedHeaders: []string{"X-Request-Id", "Content-Disposition"},
			MaxAge:         600,
		},
	}
}

func defaultInstanceConfig() InstanceConfig {
	return InstanceConfig{
		Writable: true,
		Lease: InstanceLeaseConfig{
			Enabled: false,
			TTL:     30 * time.Second,
			Grace:   10 * time.Second,
			OnLost:  "release_db",
		},
	}
}

func defaultDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		Engine:            EngineDuckDB,
		CacheMaxBytes:     DefaultCacheMaxBytes,
		CacheMaxDatabases: DefaultCacheMaxDatabases,
		IdleTimeout:       5 * time.Minute,
		MaxOpen:           8,
		DuckLake: DuckLakeConfig{
			CatalogEngine:        CatalogEngineDuckDB,
			MemoryLimit:          "512MB",
			Threads:              2,
			DataInliningRowLimit: 1000,
			ParquetCompression:   "zstd",
			TargetFileSize:       "64MB",
			CatalogSync: CatalogSyncConfig{
				Mode:         "interval",
				Debounce:     200 * time.Millisecond,
				Interval:     15 * time.Second,
				MaxLag:       30 * time.Second,
				KeepVersions: 10,
			},
			Maintenance: DuckLakeMaintenanceConfig{
				CheckpointInterval:     time.Hour,
				ExpireOlderThan:        7 * 24 * time.Hour,
				DeleteOlderThan:        7 * 24 * time.Hour,
				RewriteDeleteThreshold: 0.95,
			},
		},
	}
}

func defaultS3Config() S3Config {
	return S3Config{Prefix: "simplebase"}
}

func defaultLLMConfig() LLMConfig {
	return LLMConfig{
		Enabled:            true,
		Providers:          map[string]ProviderConfig{},
		AgentToolProtocol:  AgentToolProtocolAuto,
		AgentMaxIterations: defaultAgentMaxIterations,
		AgentRunTimeout:    defaultAgentRunTimeout,
	}
}

func defaultAgentConfig() AgentConfig {
	return AgentConfig{
		SkillsCLI:      true,
		DelegationTTL:  15 * time.Minute,
		ConfirmTimeout: 2 * time.Minute,
		MCP:            MCPConfig{Builtin: true},
	}
}

func defaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		Enabled:        false,
		Backend:        SandboxBackendLocal,
		Image:          SandboxDefaultImage,
		CPUs:           1,
		MemoryMiB:      256,
		MaxDuration:    30 * time.Minute,
		IdleTimeout:    5 * time.Minute,
		FreezeAfter:    2 * time.Minute,
		ExecTimeout:    30 * time.Second,
		ExecTimeoutMax: 300 * time.Second,
		MaxOutputBytes: 65536,
		MaxFileBytes:   1 << 20,
		MaxPerProject:  5,
		ReapInterval:   5 * time.Minute,
		Network:        "none",
		Workdir:        "/workspace",
		Terminal: SandboxTerminalConfig{
			Enabled:               true,
			MaxSessionsPerSandbox: 4,
			IdleTimeout:           15 * time.Minute,
		},
		E2B: SandboxE2BConfig{DefaultTemplate: "base"},
	}
}

func defaultLimitsConfig() LimitsConfig {
	return LimitsConfig{
		MaxRequestBytes:      1 << 20,
		MaxQueryRows:         1000,
		QueryTimeout:         30 * time.Second,
		MaxConcurrentQueries: 64,
		MaxBatchStatements:   100,
		MaxSQLBytes:          65536,
		MaxConcurrentGoRuns:  defaultMaxConcurrentGoRuns,
		MaxFilterScanRows:    50000,
		MaxImportBytes:       128 << 20,
		RealtimeMaxConns:     200,
		RateLimit:            RateLimitConfig{Burst: 2, ExemptSSE: true},
	}
}

func defaultObservabilityConfig() ObservabilityConfig {
	return ObservabilityConfig{
		LogLevel:    DefaultLogLevel,
		LogFormat:   "json",
		LogOutput:   "stderr",
		MetricsPath: "/metrics",
	}
}

func defaultSystemDatabaseConfig() SystemDatabaseConfig {
	return SystemDatabaseConfig{
		Name:                 "simplebase-system",
		MetricsFlushInterval: 15 * time.Second,
		LogFlushInterval:     15 * time.Second,
		LogKeepDays:          14,
	}
}

func defaultBackupConfig() BackupConfig {
	return BackupConfig{
		Dir: "./backups", KeepLast: 7, KeepDays: 30, MaxPerProject: 14, SnapshotTimeout: 10 * time.Minute,
	}
}

func defaultSitesConfig() SitesConfig {
	return SitesConfig{
		URLScheme: "https", IndexDocument: DefaultSiteIndexDocument, CacheTTL: DefaultSiteCacheTTL,
	}
}
