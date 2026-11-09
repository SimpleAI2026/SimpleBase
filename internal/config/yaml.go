// 文件说明：yaml.go — internal/config 的内部实现（yamlSecretPresence、rejectYAMLSecrets 等）
//
// 用法
//
//	type yamlSecretPresence（包内使用）
//	type yamlConfig（包内使用）
//	同包调用：yamlSecretPresence → config.go；rejectYAMLSecrets() → config.go；unmarshalYAML() → config.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api, internal/api/perfbench 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/config/yaml.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backup.go, config.go, env.go, http_boundary.go
//	[关键引用] yamlSecretPresence ← config.go
//
// 主要符号
//
//	type yamlSecretPresence struct {…}
//	rejectYAMLSecrets(s yamlSecretPresence) error
//	type yamlConfig struct {…}
//	type yamlBackup struct {…}
//	type yamlStorage struct {…}
//	type yamlHTTP struct {…}
//	... 另有 31 个顶层符号

package config

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// yamlSecretPresence records whether the YAML file contained non-empty secret values.
// Secrets must come from environment variables or .env, never from YAML.
type yamlSecretPresence struct {
	s3Access         bool
	s3Secret         bool
	auth             bool
	llmKeys          bool
	sessionSecret    bool
	metricsToken     bool
	bootstrapPWD     bool
	apiKeySecretRing bool
}

func rejectYAMLSecrets(s yamlSecretPresence) error {
	var which []string
	if s.s3Access {
		which = append(which, "s3.access_key")
	}
	if s.s3Secret {
		which = append(which, "s3.secret_key")
	}
	if s.auth {
		which = append(which, "auth.api_key_hash_secret")
	}
	if s.llmKeys {
		which = append(which, "llm.providers.*.api_key")
	}
	if s.sessionSecret {
		which = append(which, "auth.session_secret")
	}
	if s.metricsToken {
		which = append(which, "observability.metrics_token")
	}
	if s.bootstrapPWD {
		which = append(which, "auth.bootstrap_password")
	}
	if s.apiKeySecretRing {
		which = append(which, "auth.api_key_hash_secrets")
	}
	if len(which) == 0 {
		return nil
	}
	return fmt.Errorf("config: secret fields %s must come from environment (or IAM); remove them from YAML", strings.Join(which, ", "))
}

type yamlConfig struct {
	HTTP           yamlHTTP           `yaml:"http"`
	Instance       yamlInstance       `yaml:"instance"`
	Storage        yamlStorage        `yaml:"storage"`
	Database       yamlDatabase       `yaml:"database"`
	S3             yamlS3             `yaml:"s3"`
	Auth           yamlAuth           `yaml:"auth"`
	LLM            yamlLLM            `yaml:"llm"`
	Agent          yamlAgent          `yaml:"agent"`
	Sandbox        yamlSandbox        `yaml:"sandbox"`
	Limits         yamlLimits         `yaml:"limits"`
	Observability  yamlObservability  `yaml:"observability"`
	SystemDatabase yamlSystemDatabase `yaml:"system_database"`
	Backup         yamlBackup         `yaml:"backup"`
	Files          struct {
		AllowedMIME []string `yaml:"allowed_mime"`
	} `yaml:"files"`
}

type yamlBackup struct {
	Enabled         *bool   `yaml:"enabled"`
	Dir             *string `yaml:"dir"`
	UploadToS3      *bool   `yaml:"upload_to_s3"`
	Schedule        *string `yaml:"schedule"`
	KeepLast        *int    `yaml:"keep_last"`
	KeepDays        *int    `yaml:"keep_days"`
	MaxPerProject   *int    `yaml:"max_per_project"`
	SnapshotTimeout yamlDur `yaml:"snapshot_timeout"`
	MissedPolicy    *string `yaml:"missed_policy"`
}

type yamlStorage struct {
	Database *string `yaml:"database"`
	Files    *string `yaml:"files"`
	DataDir  *string `yaml:"data_dir"`
}

type yamlHTTP struct {
	Address         *string  `yaml:"address"`
	ReadTimeout     yamlDur  `yaml:"read_timeout"`
	WriteTimeout    yamlDur  `yaml:"write_timeout"`
	IdleTimeout     yamlDur  `yaml:"idle_timeout"`
	ShutdownTimeout yamlDur  `yaml:"shutdown_timeout"`
	Gzip            *bool    `yaml:"gzip"`
	TrustedProxies  []string `yaml:"trusted_proxies"`
	CORS            yamlCORS `yaml:"cors"`
	PublicURL       *string  `yaml:"public_url"`
	SecurityHeaders struct {
		HSTSMaxAge         *int    `yaml:"hsts_max_age"`
		HSTSIncludeSubdoms *bool   `yaml:"hsts_include_subdomains"`
		CSP                *string `yaml:"csp"`
	} `yaml:"security_headers"`
}

type yamlCORS struct {
	Enabled          *bool    `yaml:"enabled"`
	AllowedOrigins   []string `yaml:"allowed_origins"`
	AllowedMethods   []string `yaml:"allowed_methods"`
	AllowedHeaders   []string `yaml:"allowed_headers"`
	ExposedHeaders   []string `yaml:"exposed_headers"`
	AllowCredentials *bool    `yaml:"allow_credentials"`
	MaxAge           *int     `yaml:"max_age"`
}

type yamlRateLimit struct {
	Enabled *bool `yaml:"enabled"`
	Rules   []struct {
		Match  *string `yaml:"match"`
		Max    *int    `yaml:"max"`
		Window yamlDur `yaml:"window"`
		Key    *string `yaml:"key"`
	} `yaml:"rules"`
	Burst      *int  `yaml:"burst"`
	ExemptSSE  *bool `yaml:"exempt_sse"`
	MaxBuckets *int  `yaml:"max_buckets"`
}

type yamlInstance struct {
	ID       *string           `yaml:"id"`
	Writable *bool             `yaml:"writable"`
	Lease    yamlInstanceLease `yaml:"lease"`
	Profile  *string           `yaml:"profile"`
}

type yamlInstanceLease struct {
	Enabled       *bool   `yaml:"enabled"`
	TTL           yamlDur `yaml:"ttl"`
	RenewInterval yamlDur `yaml:"renew_interval"`
	Grace         yamlDur `yaml:"grace"`
	OnLost        *string `yaml:"on_lost"`
}

type yamlDatabase struct {
	Engine            *string      `yaml:"engine"`
	CacheDir          *string      `yaml:"cache_dir"`
	CacheMaxBytes     *int64       `yaml:"cache_max_bytes"`
	CacheMaxDatabases *int         `yaml:"cache_max_databases"`
	IdleTimeout       yamlDur      `yaml:"idle_timeout"`
	MaxOpen           *int         `yaml:"max_open"`
	DuckLake          yamlDuckLake `yaml:"ducklake"`
}

type yamlDuckLake struct {
	CatalogEngine        *string           `yaml:"catalog_engine"`
	MemoryLimit          *string           `yaml:"memory_limit"`
	Threads              *int              `yaml:"threads"`
	ExtensionDir         *string           `yaml:"extension_dir"`
	DataInliningRowLimit *int              `yaml:"data_inlining_row_limit"`
	ParquetCompression   *string           `yaml:"parquet_compression"`
	TargetFileSize       *string           `yaml:"target_file_size"`
	RequireCommitMessage *bool             `yaml:"require_commit_message"`
	CatalogSync          yamlCatalogSync   `yaml:"catalog_sync"`
	Maintenance          yamlDuckLakeMaint `yaml:"maintenance"`
}

type yamlCatalogSync struct {
	Mode         *string `yaml:"mode"`
	Debounce     yamlDur `yaml:"debounce"`
	DebounceMS   *int    `yaml:"debounce_ms"` // compat alias for one release
	Interval     yamlDur `yaml:"interval"`
	MaxLag       yamlDur `yaml:"max_lag"`
	KeepVersions *int    `yaml:"keep_versions"`
}

type yamlDuckLakeMaint struct {
	CheckpointInterval     yamlDur  `yaml:"checkpoint_interval"`
	ExpireOlderThan        yamlDur  `yaml:"expire_older_than"`
	DeleteOlderThan        yamlDur  `yaml:"delete_older_than"`
	RewriteDeleteThreshold *float64 `yaml:"rewrite_delete_threshold"`
}

type yamlS3 struct {
	Endpoint       *string `yaml:"endpoint"`
	Region         *string `yaml:"region"`
	Bucket         *string `yaml:"bucket"`
	Prefix         *string `yaml:"prefix"`
	AccessKey      *string `yaml:"access_key"`
	SecretKey      *string `yaml:"secret_key"`
	KMSKeyID       *string `yaml:"kms_key_id"`
	ForcePathStyle *bool   `yaml:"force_path_style"`
}

type yamlAuth struct {
	APIKeyHashSecret  *string `yaml:"api_key_hash_secret"`
	SessionSecret     *string `yaml:"session_secret"`
	APIKeyHashSecrets *string `yaml:"api_key_hash_secrets"`
	BootstrapPassword *string `yaml:"bootstrap_password"`
}

type yamlLLM struct {
	Enabled            *bool                   `yaml:"enabled"`
	Providers          map[string]yamlProvider `yaml:"providers"`
	AgentToolProtocol  *string                 `yaml:"agent_tool_protocol"`
	AgentMaxIterations *int                    `yaml:"agent_max_iterations"`
	AgentRunTimeout    yamlDur                 `yaml:"agent_run_timeout"`
}

type yamlAgent struct {
	SkillsCLI      *bool   `yaml:"skills_cli"`
	CLIPath        *string `yaml:"cli_path"`
	DelegationTTL  yamlDur `yaml:"delegation_ttl"`
	ConfirmTimeout yamlDur `yaml:"confirm_timeout"`
	TraceLog       *bool   `yaml:"trace_log"`

	ShellIsolation yamlShellIsolation `yaml:"shell_isolation"`
}

type yamlShellIsolation struct {
	Enabled          *bool    `yaml:"enabled"`
	Mode             *string  `yaml:"mode"`
	WritableRoots    []string `yaml:"writable_roots"`
	ReadOnlySubpaths []string `yaml:"read_only_subpaths"`
	DenyRead         []string `yaml:"deny_read"`
	MemoryMaxBytes   *int64   `yaml:"memory_max_bytes"`
	CPUMax           *string  `yaml:"cpu_max"`
	PidsMax          *int64   `yaml:"pids_max"`
	Timeout          yamlDur  `yaml:"timeout"`
	BwrapPath        *string  `yaml:"bwrap_path"`
	HelperPath       *string  `yaml:"helper_path"`
}

type yamlProvider struct {
	APIKey        *string  `yaml:"api_key"`
	BaseURL       *string  `yaml:"base_url"`
	DefaultModel  *string  `yaml:"default_model"`
	AllowedModels []string `yaml:"allowed_models"`
	Timeout       yamlDur  `yaml:"timeout"`
}

type yamlLimits struct {
	MaxRequestBytes      *int64        `yaml:"max_request_bytes"`
	MaxQueryRows         *int          `yaml:"max_query_rows"`
	QueryTimeout         yamlDur       `yaml:"query_timeout"`
	MaxConcurrentQueries *int          `yaml:"max_concurrent_queries"`
	MaxBatchStatements   *int          `yaml:"max_batch_statements"`
	MaxSQLBytes          *int          `yaml:"max_sql_bytes"`
	MaxConcurrentGoRuns  *int          `yaml:"max_concurrent_go_runs"`
	MaxFilterScanRows    *int          `yaml:"max_filter_scan_rows"`
	MaxImportBytes       *int64        `yaml:"max_import_bytes"`
	RealtimeMaxConns     *int          `yaml:"realtime_max_conns"`
	RateLimit            yamlRateLimit `yaml:"rate_limit"`
}

type yamlSandbox struct {
	Enabled        *bool    `yaml:"enabled"`
	Image          *string  `yaml:"image"`
	CPUs           *int     `yaml:"cpus"`
	MemoryMiB      *int     `yaml:"memory_mib"`
	MaxDuration    yamlDur  `yaml:"max_duration"`
	IdleTimeout    yamlDur  `yaml:"idle_timeout"`
	ExecTimeout    yamlDur  `yaml:"exec_timeout"`
	MaxOutputBytes *int     `yaml:"max_output_bytes"`
	MaxFileBytes   *int     `yaml:"max_file_bytes"`
	Network        *string  `yaml:"network"`
	Workdir        *string  `yaml:"workdir"`
	Backend        *string  `yaml:"backend"`
	Images         []string `yaml:"images"`
	ExecTimeoutMax yamlDur  `yaml:"exec_timeout_max"`
	MaxPerProject  *int     `yaml:"max_per_project"`
	ReapInterval   yamlDur  `yaml:"reap_interval"`
	Root           *string  `yaml:"root"`
	PidsMax        *int64   `yaml:"pids_max"`
	BwrapPath      *string  `yaml:"bwrap_path"`
	HelperPath     *string  `yaml:"helper_path"`

	Terminal struct {
		Enabled               *bool   `yaml:"enabled"`
		MaxSessionsPerSandbox *int    `yaml:"max_sessions_per_sandbox"`
		IdleTimeout           yamlDur `yaml:"idle_timeout"`
	} `yaml:"terminal"`
	SSH struct {
		Listen      *string `yaml:"listen"`
		HostKeyPath *string `yaml:"host_key_path"`
	} `yaml:"ssh"`
	E2B struct {
		DefaultTemplate *string `yaml:"default_template"`
		EnvdVersion     *string `yaml:"envd_version"`
	} `yaml:"e2b"`
}

type yamlObservability struct {
	LogLevel        *string `yaml:"log_level"`
	LogFormat       *string `yaml:"log_format"`
	LogOutput       *string `yaml:"log_output"`
	MetricsPath     *string `yaml:"metrics_path"`
	PerfStageTiming *bool   `yaml:"perf_stage_timing"`
	MetricsAddress  *string `yaml:"metrics_address"`
	PProf           *bool   `yaml:"pprof"`
	MetricsToken    *string `yaml:"metrics_token"`
}

type yamlSystemDatabase struct {
	Name                 *string `yaml:"name"`
	MetricsFlushInterval yamlDur `yaml:"metrics_flush_interval"`
	LogFlushInterval     yamlDur `yaml:"log_flush_interval"`
	LogKeepDays          *int    `yaml:"log_keep_days"`
}

// yamlDur unmarshals Go durations plus the "Nd" day suffix used in DuckLake maintenance.
type yamlDur struct {
	set bool
	d   time.Duration
}

func (d *yamlDur) UnmarshalYAML(n *yaml.Node) error {
	if n == nil || n.Kind == 0 {
		return nil
	}
	if n.Kind == yaml.ScalarNode && (n.Tag == "!!null" || n.Value == "" || n.Value == "null" || n.Value == "~") {
		return nil
	}
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("invalid duration (want string, got %s)", n.ShortTag())
	}
	parsed, err := parseDuration(n.Value)
	if err != nil {
		return err
	}
	d.set = true
	d.d = parsed
	return nil
}

func unmarshalYAML(data []byte) (yamlConfig, error) {
	var yc yamlConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		return yamlConfig{}, err
	}
	return yc, nil
}

func applyYAML(cfg *Config, yc yamlConfig) (yamlSecretPresence, error) {
	var secrets yamlSecretPresence
	applyYAMLHTTP(&cfg.HTTP, yc.HTTP)

	setStr(&cfg.Instance.ID, yc.Instance.ID)
	setBool(&cfg.Instance.Writable, yc.Instance.Writable)
	setStr(&cfg.Instance.Profile, yc.Instance.Profile)
	if yc.Instance.Lease.Enabled != nil {
		cfg.Instance.Lease.Enabled = *yc.Instance.Lease.Enabled
	}
	if yc.Instance.Lease.TTL.set {
		cfg.Instance.Lease.TTL = yc.Instance.Lease.TTL.d
	}
	if yc.Instance.Lease.RenewInterval.set {
		cfg.Instance.Lease.RenewInterval = yc.Instance.Lease.RenewInterval.d
	}
	if yc.Instance.Lease.Grace.set {
		cfg.Instance.Lease.Grace = yc.Instance.Lease.Grace.d
	}
	setStr(&cfg.Instance.Lease.OnLost, yc.Instance.Lease.OnLost)

	setStr(&cfg.Storage.Database, yc.Storage.Database)
	setStr(&cfg.Storage.Files, yc.Storage.Files)
	setStr(&cfg.Storage.DataDir, yc.Storage.DataDir)

	setStr(&cfg.Database.Engine, yc.Database.Engine)
	setStr(&cfg.Database.CacheDir, yc.Database.CacheDir)
	setInt64(&cfg.Database.CacheMaxBytes, yc.Database.CacheMaxBytes)
	setInt(&cfg.Database.CacheMaxDatabases, yc.Database.CacheMaxDatabases)
	setDur(&cfg.Database.IdleTimeout, yc.Database.IdleTimeout)
	setInt(&cfg.Database.MaxOpen, yc.Database.MaxOpen)
	applyYAMLDuckLake(&cfg.Database.DuckLake, yc.Database.DuckLake)

	applyYAMLS3(&cfg.S3, yc.S3, &secrets)

	if yc.Auth.APIKeyHashSecret != nil {
		cfg.Auth.APIKeyHashSecret = *yc.Auth.APIKeyHashSecret
		if *yc.Auth.APIKeyHashSecret != "" {
			secrets.auth = true
		}
	}
	if yc.Auth.SessionSecret != nil {
		cfg.Auth.SessionSecret = *yc.Auth.SessionSecret
		if *yc.Auth.SessionSecret != "" {
			secrets.sessionSecret = true
		}
	}
	if yc.Auth.APIKeyHashSecrets != nil {
		cfg.Auth.APIKeyHashSecrets = *yc.Auth.APIKeyHashSecrets
		if *yc.Auth.APIKeyHashSecrets != "" {
			secrets.apiKeySecretRing = true
		}
	}
	if yc.Auth.BootstrapPassword != nil {
		cfg.Auth.BootstrapPassword = *yc.Auth.BootstrapPassword
		if *yc.Auth.BootstrapPassword != "" {
			secrets.bootstrapPWD = true
		}
	}

	if yc.LLM.Enabled != nil {
		cfg.LLM.Enabled = *yc.LLM.Enabled
	}
	setBool(&cfg.Agent.SkillsCLI, yc.Agent.SkillsCLI)
	setStr(&cfg.Agent.CLIPath, yc.Agent.CLIPath)
	setDur(&cfg.Agent.DelegationTTL, yc.Agent.DelegationTTL)
	setDur(&cfg.Agent.ConfirmTimeout, yc.Agent.ConfirmTimeout)
	setBool(&cfg.Agent.TraceLog, yc.Agent.TraceLog)
	si := yc.Agent.ShellIsolation
	setBool(&cfg.Agent.ShellIsolation.Enabled, si.Enabled)
	setStr(&cfg.Agent.ShellIsolation.Mode, si.Mode)
	if si.WritableRoots != nil {
		cfg.Agent.ShellIsolation.WritableRoots = si.WritableRoots
	}
	if si.ReadOnlySubpaths != nil {
		cfg.Agent.ShellIsolation.ReadOnlySubpaths = si.ReadOnlySubpaths
	}
	if si.DenyRead != nil {
		cfg.Agent.ShellIsolation.DenyRead = si.DenyRead
	}
	setInt64(&cfg.Agent.ShellIsolation.MemoryMaxBytes, si.MemoryMaxBytes)
	setStr(&cfg.Agent.ShellIsolation.CPUMax, si.CPUMax)
	setInt64(&cfg.Agent.ShellIsolation.PidsMax, si.PidsMax)
	setDur(&cfg.Agent.ShellIsolation.Timeout, si.Timeout)
	setStr(&cfg.Agent.ShellIsolation.BwrapPath, si.BwrapPath)
	setStr(&cfg.Agent.ShellIsolation.HelperPath, si.HelperPath)

	setStr(&cfg.LLM.AgentToolProtocol, yc.LLM.AgentToolProtocol)
	setInt(&cfg.LLM.AgentMaxIterations, yc.LLM.AgentMaxIterations)
	setDur(&cfg.LLM.AgentRunTimeout, yc.LLM.AgentRunTimeout)
	if yc.LLM.Providers != nil {
		if cfg.LLM.Providers == nil {
			cfg.LLM.Providers = map[string]ProviderConfig{}
		}
		for name, yp := range yc.LLM.Providers {
			p := cfg.LLM.Providers[name]
			if yp.APIKey != nil {
				p.APIKey = *yp.APIKey
				if *yp.APIKey != "" {
					secrets.llmKeys = true
				}
			}
			setStr(&p.BaseURL, yp.BaseURL)
			setStr(&p.DefaultModel, yp.DefaultModel)
			if yp.AllowedModels != nil {
				p.AllowedModels = append([]string(nil), yp.AllowedModels...)
			}
			setDur(&p.Timeout, yp.Timeout)
			if p.Timeout == 0 {
				p.Timeout = 60 * time.Second
			}
			cfg.LLM.Providers[name] = p
		}
	}

	applyYAMLLimits(&cfg.Limits, yc.Limits)

	applyYAMLSandbox(&cfg.Sandbox, yc.Sandbox)

	applyYAMLObservability(&cfg.Observability, yc.Observability, &secrets)

	setStr(&cfg.SystemDatabase.Name, yc.SystemDatabase.Name)
	setDur(&cfg.SystemDatabase.MetricsFlushInterval, yc.SystemDatabase.MetricsFlushInterval)
	setDur(&cfg.SystemDatabase.LogFlushInterval, yc.SystemDatabase.LogFlushInterval)
	setInt(&cfg.SystemDatabase.LogKeepDays, yc.SystemDatabase.LogKeepDays)
	setBool(&cfg.Backup.Enabled, yc.Backup.Enabled)
	setStr(&cfg.Backup.Dir, yc.Backup.Dir)
	setBool(&cfg.Backup.UploadToS3, yc.Backup.UploadToS3)
	setStr(&cfg.Backup.Schedule, yc.Backup.Schedule)
	setInt(&cfg.Backup.KeepLast, yc.Backup.KeepLast)
	setInt(&cfg.Backup.KeepDays, yc.Backup.KeepDays)
	setInt(&cfg.Backup.MaxPerProject, yc.Backup.MaxPerProject)
	setDur(&cfg.Backup.SnapshotTimeout, yc.Backup.SnapshotTimeout)
	setStr(&cfg.Backup.MissedPolicy, yc.Backup.MissedPolicy)
	if yc.Files.AllowedMIME != nil {
		cfg.Files.AllowedMIME = append([]string(nil), yc.Files.AllowedMIME...)
	}

	return secrets, nil
}

// applyYAMLHTTP 把 YAML 的 HTTP/CORS 域应用到 cfg；切片按值拷贝。
func applyYAMLHTTP(dst *HTTPConfig, y yamlHTTP) {
	setStr(&dst.Address, y.Address)
	setDur(&dst.ReadTimeout, y.ReadTimeout)
	setDur(&dst.WriteTimeout, y.WriteTimeout)
	setDur(&dst.IdleTimeout, y.IdleTimeout)
	setDur(&dst.ShutdownTimeout, y.ShutdownTimeout)
	setBool(&dst.Gzip, y.Gzip)
	setStr(&dst.PublicURL, y.PublicURL)
	setStr(&dst.SecurityHeaders.CSP, y.SecurityHeaders.CSP)
	setInt(&dst.SecurityHeaders.HSTSMaxAge, y.SecurityHeaders.HSTSMaxAge)
	setBool(&dst.SecurityHeaders.HSTSIncludeSubdomains, y.SecurityHeaders.HSTSIncludeSubdoms)
	setBool(&dst.CORS.Enabled, y.CORS.Enabled)
	setBool(&dst.CORS.AllowCredentials, y.CORS.AllowCredentials)
	setInt(&dst.CORS.MaxAge, y.CORS.MaxAge)
	if y.TrustedProxies != nil {
		dst.TrustedProxies = append([]string(nil), y.TrustedProxies...)
	}
	if y.CORS.AllowedOrigins != nil {
		dst.CORS.AllowedOrigins = append([]string(nil), y.CORS.AllowedOrigins...)
	}
	if y.CORS.AllowedMethods != nil {
		dst.CORS.AllowedMethods = append([]string(nil), y.CORS.AllowedMethods...)
	}
	if y.CORS.AllowedHeaders != nil {
		dst.CORS.AllowedHeaders = append([]string(nil), y.CORS.AllowedHeaders...)
	}
	if y.CORS.ExposedHeaders != nil {
		dst.CORS.ExposedHeaders = append([]string(nil), y.CORS.ExposedHeaders...)
	}
}

// applyYAMLLimits 把 YAML 的 Limits/RateLimit 域应用到 cfg；规则列表整体替换。
func applyYAMLLimits(dst *LimitsConfig, y yamlLimits) {
	setInt64(&dst.MaxRequestBytes, y.MaxRequestBytes)
	setInt(&dst.MaxQueryRows, y.MaxQueryRows)
	setDur(&dst.QueryTimeout, y.QueryTimeout)
	setInt(&dst.MaxConcurrentQueries, y.MaxConcurrentQueries)
	setInt(&dst.MaxBatchStatements, y.MaxBatchStatements)
	setInt(&dst.MaxSQLBytes, y.MaxSQLBytes)
	setInt(&dst.MaxConcurrentGoRuns, y.MaxConcurrentGoRuns)
	setInt(&dst.MaxFilterScanRows, y.MaxFilterScanRows)
	setInt64(&dst.MaxImportBytes, y.MaxImportBytes)
	setInt(&dst.RealtimeMaxConns, y.RealtimeMaxConns)
	setBool(&dst.RateLimit.Enabled, y.RateLimit.Enabled)
	setInt(&dst.RateLimit.Burst, y.RateLimit.Burst)
	setBool(&dst.RateLimit.ExemptSSE, y.RateLimit.ExemptSSE)
	setInt(&dst.RateLimit.MaxBuckets, y.RateLimit.MaxBuckets)
	if y.RateLimit.Rules != nil {
		dst.RateLimit.Rules = make([]RateLimitRule, 0, len(y.RateLimit.Rules))
		for _, rule := range y.RateLimit.Rules {
			item := RateLimitRule{Max: 0, Window: 0}
			setStr(&item.Match, rule.Match)
			setInt(&item.Max, rule.Max)
			setDur(&item.Window, rule.Window)
			setStr(&item.Key, rule.Key)
			dst.RateLimit.Rules = append(dst.RateLimit.Rules, item)
		}
	}
}

// applyYAMLSandbox 把 YAML 的 Sandbox 域应用到 cfg；镜像列表按值拷贝。
func applyYAMLSandbox(dst *SandboxConfig, y yamlSandbox) {
	setBool(&dst.Enabled, y.Enabled)
	setStr(&dst.Image, y.Image)
	setInt(&dst.CPUs, y.CPUs)
	setInt(&dst.MemoryMiB, y.MemoryMiB)
	setDur(&dst.MaxDuration, y.MaxDuration)
	setDur(&dst.IdleTimeout, y.IdleTimeout)
	setDur(&dst.ExecTimeout, y.ExecTimeout)
	setInt(&dst.MaxOutputBytes, y.MaxOutputBytes)
	setInt(&dst.MaxFileBytes, y.MaxFileBytes)
	setStr(&dst.Network, y.Network)
	setStr(&dst.Workdir, y.Workdir)
	setStr(&dst.Backend, y.Backend)
	if y.Images != nil {
		dst.Images = append([]string(nil), y.Images...)
	}
	setDur(&dst.ExecTimeoutMax, y.ExecTimeoutMax)
	setInt(&dst.MaxPerProject, y.MaxPerProject)
	setDur(&dst.ReapInterval, y.ReapInterval)
	setStr(&dst.Root, y.Root)
	setInt64(&dst.PidsMax, y.PidsMax)
	setStr(&dst.BwrapPath, y.BwrapPath)
	setStr(&dst.HelperPath, y.HelperPath)
	setBool(&dst.Terminal.Enabled, y.Terminal.Enabled)
	setInt(&dst.Terminal.MaxSessionsPerSandbox, y.Terminal.MaxSessionsPerSandbox)
	setDur(&dst.Terminal.IdleTimeout, y.Terminal.IdleTimeout)
	setStr(&dst.SSH.Listen, y.SSH.Listen)
	setStr(&dst.SSH.HostKeyPath, y.SSH.HostKeyPath)
	setStr(&dst.E2B.DefaultTemplate, y.E2B.DefaultTemplate)
	setStr(&dst.E2B.EnvdVersion, y.E2B.EnvdVersion)
}

// applyYAMLObservability 把 YAML 的 Observability 域应用到 cfg。
func applyYAMLObservability(dst *ObservabilityConfig, y yamlObservability, secrets *yamlSecretPresence) {
	setStr(&dst.LogLevel, y.LogLevel)
	setStr(&dst.LogFormat, y.LogFormat)
	setStr(&dst.LogOutput, y.LogOutput)
	setStr(&dst.MetricsPath, y.MetricsPath)
	setBool(&dst.PerfStageTiming, y.PerfStageTiming)
	setStr(&dst.MetricsAddress, y.MetricsAddress)
	setBool(&dst.PProf, y.PProf)
	if y.MetricsToken != nil {
		dst.MetricsToken = *y.MetricsToken
		if *y.MetricsToken != "" {
			secrets.metricsToken = true
		}
	}
}

// applyYAMLS3 把 YAML 的 S3 域应用到 cfg；非空密钥同时标记 secrets presence，
// 供 rejectYAMLSecrets 在加载尾部拒绝。
func applyYAMLS3(dst *S3Config, y yamlS3, secrets *yamlSecretPresence) {
	setStr(&dst.Endpoint, y.Endpoint)
	setStr(&dst.Region, y.Region)
	setStr(&dst.Bucket, y.Bucket)
	setStr(&dst.Prefix, y.Prefix)
	if y.AccessKey != nil {
		dst.AccessKey = *y.AccessKey
		if *y.AccessKey != "" {
			secrets.s3Access = true
		}
	}
	if y.SecretKey != nil {
		dst.SecretKey = *y.SecretKey
		if *y.SecretKey != "" {
			secrets.s3Secret = true
		}
	}
	setStr(&dst.KMSKeyID, y.KMSKeyID)
	setBool(&dst.ForcePathStyle, y.ForcePathStyle)
}

func applyYAMLDuckLake(dst *DuckLakeConfig, y yamlDuckLake) {
	setStr(&dst.CatalogEngine, y.CatalogEngine)
	setStr(&dst.MemoryLimit, y.MemoryLimit)
	setInt(&dst.Threads, y.Threads)
	setStr(&dst.ExtensionDir, y.ExtensionDir)
	setInt(&dst.DataInliningRowLimit, y.DataInliningRowLimit)
	setStr(&dst.ParquetCompression, y.ParquetCompression)
	setStr(&dst.TargetFileSize, y.TargetFileSize)
	setBool(&dst.RequireCommitMessage, y.RequireCommitMessage)
	setStr(&dst.CatalogSync.Mode, y.CatalogSync.Mode)
	if y.CatalogSync.Debounce.set {
		dst.CatalogSync.Debounce = y.CatalogSync.Debounce.d
	} else if y.CatalogSync.DebounceMS != nil {
		dst.CatalogSync.Debounce = time.Duration(*y.CatalogSync.DebounceMS) * time.Millisecond
	}
	setDur(&dst.CatalogSync.Interval, y.CatalogSync.Interval)
	setDur(&dst.CatalogSync.MaxLag, y.CatalogSync.MaxLag)
	setInt(&dst.CatalogSync.KeepVersions, y.CatalogSync.KeepVersions)
	setDur(&dst.Maintenance.CheckpointInterval, y.Maintenance.CheckpointInterval)
	setDur(&dst.Maintenance.ExpireOlderThan, y.Maintenance.ExpireOlderThan)
	setDur(&dst.Maintenance.DeleteOlderThan, y.Maintenance.DeleteOlderThan)
	setFloat64(&dst.Maintenance.RewriteDeleteThreshold, y.Maintenance.RewriteDeleteThreshold)
}

func setStr(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}

func setBool(dst *bool, src *bool) {
	if src != nil {
		*dst = *src
	}
}

func setInt(dst *int, src *int) {
	if src != nil {
		*dst = *src
	}
}

func setInt64(dst *int64, src *int64) {
	if src != nil {
		*dst = *src
	}
}

func setFloat64(dst *float64, src *float64) {
	if src != nil {
		*dst = *src
	}
}

func setDur(dst *time.Duration, src yamlDur) {
	if src.set {
		*dst = src.d
	}
}
