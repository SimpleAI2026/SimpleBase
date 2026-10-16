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
	s3Access   bool
	s3Secret   bool
	auth       bool
	llmKeys    bool
	sandboxKey bool
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
	if s.sandboxKey {
		which = append(which, "sandbox.api_key")
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
}

type yamlStorage struct {
	Database *string `yaml:"database"`
	Files    *string `yaml:"files"`
	DataDir  *string `yaml:"data_dir"`
}

type yamlHTTP struct {
	Address         *string `yaml:"address"`
	ReadTimeout     yamlDur `yaml:"read_timeout"`
	WriteTimeout    yamlDur `yaml:"write_timeout"`
	IdleTimeout     yamlDur `yaml:"idle_timeout"`
	ShutdownTimeout yamlDur `yaml:"shutdown_timeout"`
}

type yamlInstance struct {
	ID       *string           `yaml:"id"`
	Writable *bool             `yaml:"writable"`
	Lease    yamlInstanceLease `yaml:"lease"`
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
	APIKeyHashSecret *string `yaml:"api_key_hash_secret"`
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
	MaxRequestBytes      *int64  `yaml:"max_request_bytes"`
	MaxQueryRows         *int    `yaml:"max_query_rows"`
	QueryTimeout         yamlDur `yaml:"query_timeout"`
	MaxConcurrentQueries *int    `yaml:"max_concurrent_queries"`
	MaxBatchStatements   *int    `yaml:"max_batch_statements"`
	MaxSQLBytes          *int    `yaml:"max_sql_bytes"`
	MaxConcurrentGoRuns  *int    `yaml:"max_concurrent_go_runs"`
}

type yamlSandbox struct {
	Enabled        *bool    `yaml:"enabled"`
	APIURL         *string  `yaml:"api_url"`
	APIKey         *string  `yaml:"api_key"`
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
}

type yamlObservability struct {
	LogLevel        *string `yaml:"log_level"`
	LogFormat       *string `yaml:"log_format"`
	LogOutput       *string `yaml:"log_output"`
	MetricsPath     *string `yaml:"metrics_path"`
	PerfStageTiming *bool   `yaml:"perf_stage_timing"`
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
	setStr(&cfg.HTTP.Address, yc.HTTP.Address)
	setDur(&cfg.HTTP.ReadTimeout, yc.HTTP.ReadTimeout)
	setDur(&cfg.HTTP.WriteTimeout, yc.HTTP.WriteTimeout)
	setDur(&cfg.HTTP.IdleTimeout, yc.HTTP.IdleTimeout)
	setDur(&cfg.HTTP.ShutdownTimeout, yc.HTTP.ShutdownTimeout)

	setStr(&cfg.Instance.ID, yc.Instance.ID)
	setBool(&cfg.Instance.Writable, yc.Instance.Writable)
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

	setInt64(&cfg.Limits.MaxRequestBytes, yc.Limits.MaxRequestBytes)
	setInt(&cfg.Limits.MaxQueryRows, yc.Limits.MaxQueryRows)
	setDur(&cfg.Limits.QueryTimeout, yc.Limits.QueryTimeout)
	setInt(&cfg.Limits.MaxConcurrentQueries, yc.Limits.MaxConcurrentQueries)
	setInt(&cfg.Limits.MaxBatchStatements, yc.Limits.MaxBatchStatements)
	setInt(&cfg.Limits.MaxSQLBytes, yc.Limits.MaxSQLBytes)
	setInt(&cfg.Limits.MaxConcurrentGoRuns, yc.Limits.MaxConcurrentGoRuns)

	setBool(&cfg.Sandbox.Enabled, yc.Sandbox.Enabled)
	setStr(&cfg.Sandbox.APIURL, yc.Sandbox.APIURL)
	if yc.Sandbox.APIKey != nil {
		cfg.Sandbox.APIKey = *yc.Sandbox.APIKey
		if *yc.Sandbox.APIKey != "" {
			secrets.sandboxKey = true
		}
	}
	setStr(&cfg.Sandbox.Image, yc.Sandbox.Image)
	setInt(&cfg.Sandbox.CPUs, yc.Sandbox.CPUs)
	setInt(&cfg.Sandbox.MemoryMiB, yc.Sandbox.MemoryMiB)
	setDur(&cfg.Sandbox.MaxDuration, yc.Sandbox.MaxDuration)
	setDur(&cfg.Sandbox.IdleTimeout, yc.Sandbox.IdleTimeout)
	setDur(&cfg.Sandbox.ExecTimeout, yc.Sandbox.ExecTimeout)
	setInt(&cfg.Sandbox.MaxOutputBytes, yc.Sandbox.MaxOutputBytes)
	setInt(&cfg.Sandbox.MaxFileBytes, yc.Sandbox.MaxFileBytes)
	setStr(&cfg.Sandbox.Network, yc.Sandbox.Network)
	setStr(&cfg.Sandbox.Workdir, yc.Sandbox.Workdir)
	setStr(&cfg.Sandbox.Backend, yc.Sandbox.Backend)
	if yc.Sandbox.Images != nil {
		cfg.Sandbox.Images = append([]string(nil), yc.Sandbox.Images...)
	}
	setDur(&cfg.Sandbox.ExecTimeoutMax, yc.Sandbox.ExecTimeoutMax)
	setInt(&cfg.Sandbox.MaxPerProject, yc.Sandbox.MaxPerProject)
	setDur(&cfg.Sandbox.ReapInterval, yc.Sandbox.ReapInterval)

	applyYAMLObservability(&cfg.Observability, yc.Observability)

	setStr(&cfg.SystemDatabase.Name, yc.SystemDatabase.Name)
	setDur(&cfg.SystemDatabase.MetricsFlushInterval, yc.SystemDatabase.MetricsFlushInterval)
	setDur(&cfg.SystemDatabase.LogFlushInterval, yc.SystemDatabase.LogFlushInterval)
	setInt(&cfg.SystemDatabase.LogKeepDays, yc.SystemDatabase.LogKeepDays)

	return secrets, nil
}

// applyYAMLObservability 把 YAML 的 Observability 域应用到 cfg。
func applyYAMLObservability(dst *ObservabilityConfig, y yamlObservability) {
	setStr(&dst.LogLevel, y.LogLevel)
	setStr(&dst.LogFormat, y.LogFormat)
	setStr(&dst.LogOutput, y.LogOutput)
	setStr(&dst.MetricsPath, y.MetricsPath)
	setBool(&dst.PerfStageTiming, y.PerfStageTiming)
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
