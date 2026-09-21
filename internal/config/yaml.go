package config

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// yamlSecretPresence records whether the YAML file contained non-empty secret values.
// Production (dev_mode=false) must not load secrets from YAML.
type yamlSecretPresence struct {
	s3Access bool
	s3Secret bool
	auth     bool
	llmKeys  bool
}

func rejectYAMLSecrets(devMode bool, s yamlSecretPresence) error {
	if devMode {
		return nil
	}
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
	if len(which) == 0 {
		return nil
	}
	return fmt.Errorf("config: secret fields %s must come from environment (or IAM) when dev_mode is false; remove them from YAML", strings.Join(which, ", "))
}

type yamlConfig struct {
	HTTP           yamlHTTP           `yaml:"http"`
	Instance       yamlInstance       `yaml:"instance"`
	Database       yamlDatabase       `yaml:"database"`
	S3             yamlS3             `yaml:"s3"`
	Auth           yamlAuth           `yaml:"auth"`
	LLM            yamlLLM            `yaml:"llm"`
	Limits         yamlLimits         `yaml:"limits"`
	Observability  yamlObservability  `yaml:"observability"`
	SystemDatabase yamlSystemDatabase `yaml:"system_database"`
	DevMode        *bool              `yaml:"dev_mode"`
}

type yamlHTTP struct {
	Address         *string `yaml:"address"`
	ReadTimeout     yamlDur `yaml:"read_timeout"`
	WriteTimeout    yamlDur `yaml:"write_timeout"`
	IdleTimeout     yamlDur `yaml:"idle_timeout"`
	ShutdownTimeout yamlDur `yaml:"shutdown_timeout"`
}

type yamlInstance struct {
	ID       *string `yaml:"id"`
	Writable *bool   `yaml:"writable"`
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
	Enabled   *bool                   `yaml:"enabled"`
	Providers map[string]yamlProvider `yaml:"providers"`
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
}

type yamlObservability struct {
	LogLevel    *string `yaml:"log_level"`
	LogFormat   *string `yaml:"log_format"`
	LogOutput   *string `yaml:"log_output"`
	MetricsPath *string `yaml:"metrics_path"`
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

	setStr(&cfg.Database.Engine, yc.Database.Engine)
	setStr(&cfg.Database.CacheDir, yc.Database.CacheDir)
	setInt64(&cfg.Database.CacheMaxBytes, yc.Database.CacheMaxBytes)
	setInt(&cfg.Database.CacheMaxDatabases, yc.Database.CacheMaxDatabases)
	setDur(&cfg.Database.IdleTimeout, yc.Database.IdleTimeout)
	setInt(&cfg.Database.MaxOpen, yc.Database.MaxOpen)
	applyYAMLDuckLake(&cfg.Database.DuckLake, yc.Database.DuckLake)

	setStr(&cfg.S3.Endpoint, yc.S3.Endpoint)
	setStr(&cfg.S3.Region, yc.S3.Region)
	setStr(&cfg.S3.Bucket, yc.S3.Bucket)
	setStr(&cfg.S3.Prefix, yc.S3.Prefix)
	if yc.S3.AccessKey != nil {
		cfg.S3.AccessKey = *yc.S3.AccessKey
		if *yc.S3.AccessKey != "" {
			secrets.s3Access = true
		}
	}
	if yc.S3.SecretKey != nil {
		cfg.S3.SecretKey = *yc.S3.SecretKey
		if *yc.S3.SecretKey != "" {
			secrets.s3Secret = true
		}
	}
	setStr(&cfg.S3.KMSKeyID, yc.S3.KMSKeyID)
	setBool(&cfg.S3.ForcePathStyle, yc.S3.ForcePathStyle)

	if yc.Auth.APIKeyHashSecret != nil {
		cfg.Auth.APIKeyHashSecret = *yc.Auth.APIKeyHashSecret
		if *yc.Auth.APIKeyHashSecret != "" {
			secrets.auth = true
		}
	}

	if yc.LLM.Enabled != nil {
		cfg.LLM.Enabled = *yc.LLM.Enabled
	}
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

	setStr(&cfg.Observability.LogLevel, yc.Observability.LogLevel)
	setStr(&cfg.Observability.LogFormat, yc.Observability.LogFormat)
	setStr(&cfg.Observability.LogOutput, yc.Observability.LogOutput)
	setStr(&cfg.Observability.MetricsPath, yc.Observability.MetricsPath)

	setStr(&cfg.SystemDatabase.Name, yc.SystemDatabase.Name)
	setDur(&cfg.SystemDatabase.MetricsFlushInterval, yc.SystemDatabase.MetricsFlushInterval)
	setDur(&cfg.SystemDatabase.LogFlushInterval, yc.SystemDatabase.LogFlushInterval)
	setInt(&cfg.SystemDatabase.LogKeepDays, yc.SystemDatabase.LogKeepDays)

	setBool(&cfg.DevMode, yc.DevMode)
	return secrets, nil
}

func applyYAMLDuckLake(dst *DuckLakeConfig, y yamlDuckLake) {
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
