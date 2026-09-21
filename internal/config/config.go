package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config 是 SimpleBase 单实例服务的全部运行配置。
//
// 加载顺序（后写覆盖前写）：
//  1. 代码默认值（defaults）
//  2. YAML（SIMPLEBASE_CONFIG_PATH，或 cwd 下存在的 config.yaml）
//  3. SIMPLEBASE_ 环境变量覆盖
//
// 敏感字段（S3 AccessKey/SecretKey、Auth.APIKeyHashSecret、LLM API Key）
// 生产（dev_mode=false）必须来自环境或 IAM；YAML 中的非空密钥会被拒绝。
// 禁止把密钥写入仓库或日志。
type Config struct {
	HTTP           HTTPConfig           `yaml:"http"`
	Instance       InstanceConfig       `yaml:"instance"`
	Database       DatabaseConfig       `yaml:"database"`
	S3             S3Config             `yaml:"s3"`
	Auth           AuthConfig           `yaml:"auth"`
	LLM            LLMConfig            `yaml:"llm"`
	Limits         LimitsConfig         `yaml:"limits"`
	Observability  ObservabilityConfig  `yaml:"observability"`
	SystemDatabase SystemDatabaseConfig `yaml:"system_database"`
	// DevMode 启用本地开发模式：系统库与用户库 DATA_PATH 落本地盘，旁路远端 S3。
	// 禁止生产开启。
	DevMode bool `yaml:"dev_mode"`
}

type HTTPConfig struct {
	Address         string        `yaml:"address"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type InstanceConfig struct {
	ID       string `yaml:"id"`
	Writable bool   `yaml:"writable"`
}

const (
	EngineDuckLake = "ducklake"

	DefaultCacheMaxBytes     int64 = 1 << 30 // 1GiB
	DefaultCacheMaxDatabases       = 256
)

type DatabaseConfig struct {
	// Engine 用户库后端；仅允许 ducklake（缺省即 ducklake）。历史 local/turso 已退役。
	Engine            string         `yaml:"engine"`
	CacheDir          string         `yaml:"cache_dir"`
	CacheMaxBytes     int64          `yaml:"cache_max_bytes"`
	CacheMaxDatabases int            `yaml:"cache_max_databases"`
	IdleTimeout       time.Duration  `yaml:"idle_timeout"`
	MaxOpen           int            `yaml:"max_open"`
	DuckLake          DuckLakeConfig `yaml:"ducklake"`
}

// DuckLakeConfig 对应 planv2.0 的 database.ducklake section。
type DuckLakeConfig struct {
	MemoryLimit          string                    `yaml:"memory_limit"`
	Threads              int                       `yaml:"threads"`
	ExtensionDir         string                    `yaml:"extension_dir"`
	DataInliningRowLimit int                       `yaml:"data_inlining_row_limit"`
	ParquetCompression   string                    `yaml:"parquet_compression"`
	TargetFileSize       string                    `yaml:"target_file_size"`
	RequireCommitMessage bool                      `yaml:"require_commit_message"`
	CatalogSync          CatalogSyncConfig         `yaml:"catalog_sync"`
	Maintenance          DuckLakeMaintenanceConfig `yaml:"maintenance"`
}

type CatalogSyncConfig struct {
	Mode         string        `yaml:"mode"`
	Debounce     time.Duration `yaml:"debounce"`
	KeepVersions int           `yaml:"keep_versions"`
}

type DuckLakeMaintenanceConfig struct {
	CheckpointInterval     time.Duration `yaml:"checkpoint_interval"`
	ExpireOlderThan        time.Duration `yaml:"expire_older_than"`
	DeleteOlderThan        time.Duration `yaml:"delete_older_than"`
	RewriteDeleteThreshold float64       `yaml:"rewrite_delete_threshold"`
}

type S3Config struct {
	Endpoint       string `yaml:"endpoint"`
	Region         string `yaml:"region"`
	Bucket         string `yaml:"bucket"`
	Prefix         string `yaml:"prefix"`
	AccessKey      string `yaml:"access_key"`
	SecretKey      string `yaml:"secret_key"`
	KMSKeyID       string `yaml:"kms_key_id"`
	ForcePathStyle bool   `yaml:"force_path_style"`
}

// SystemDatabaseConfig 控制实例级 DuckLake 系统库。
type SystemDatabaseConfig struct {
	Name                 string        `yaml:"name"`
	MetricsFlushInterval time.Duration `yaml:"metrics_flush_interval"`
	LogFlushInterval     time.Duration `yaml:"log_flush_interval"`
	LogKeepDays          int           `yaml:"log_keep_days"`
}

type AuthConfig struct {
	APIKeyHashSecret string `yaml:"api_key_hash_secret"`
}

type LLMConfig struct {
	Enabled   bool                      `yaml:"enabled"`
	Providers map[string]ProviderConfig `yaml:"providers"`
}

type ProviderConfig struct {
	APIKey        string        `yaml:"api_key"`
	BaseURL       string        `yaml:"base_url"`
	DefaultModel  string        `yaml:"default_model"`
	AllowedModels []string      `yaml:"allowed_models"`
	Timeout       time.Duration `yaml:"timeout"`
}

type LimitsConfig struct {
	MaxRequestBytes      int64         `yaml:"max_request_bytes"`
	MaxQueryRows         int           `yaml:"max_query_rows"`
	QueryTimeout         time.Duration `yaml:"query_timeout"`
	MaxConcurrentQueries int           `yaml:"max_concurrent_queries"`
	MaxBatchStatements   int           `yaml:"max_batch_statements"`
	MaxSQLBytes          int           `yaml:"max_sql_bytes"`
}

type ObservabilityConfig struct {
	LogLevel    string `yaml:"log_level"`
	LogFormat   string `yaml:"log_format"`
	LogOutput   string `yaml:"log_output"`
	MetricsPath string `yaml:"metrics_path"`
}

// Load 加载配置：代码默认值 → YAML → SIMPLEBASE_ 环境变量覆盖，然后 Validate。
//
// YAML 路径：
//   - SIMPLEBASE_CONFIG_PATH 若设置，文件必须存在且可解析，否则返回错误；
//   - 否则若 cwd 存在 config.yaml 则加载它（解析失败同样报错）；
//   - 否则仅使用默认值 + 环境变量（兼容纯 env 部署）。
func Load() (Config, error) {
	cfg := defaults()

	path, required := configFilePath()
	var yamlSecrets yamlSecretPresence
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if required || !os.IsNotExist(err) {
				return Config{}, fmt.Errorf("read config file %s: %w", path, err)
			}
		} else {
			yc, err := unmarshalYAML(data)
			if err != nil {
				return Config{}, fmt.Errorf("parse config file %s: %w", path, err)
			}
			yamlSecrets, err = applyYAML(&cfg, yc)
			if err != nil {
				return Config{}, fmt.Errorf("apply config file %s: %w", path, err)
			}
		}
	}

	applyEnv(&cfg)

	if err := rejectYAMLSecrets(cfg.DevMode, yamlSecrets); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// configFilePath 返回 YAML 路径。required 表示 SIMPLEBASE_CONFIG_PATH 显式指定，缺失必须失败。
func configFilePath() (path string, required bool) {
	if p := os.Getenv("SIMPLEBASE_CONFIG_PATH"); p != "" {
		return p, true
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml", false
	}
	return "", false
}

func defaults() Config {
	return Config{
		DevMode: false,
		HTTP: HTTPConfig{
			Address:         ":8080",
			ReadTimeout:     15 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     60 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		Instance: InstanceConfig{
			Writable: true,
		},
		Database: DatabaseConfig{
			Engine:            EngineDuckLake,
			CacheMaxBytes:     DefaultCacheMaxBytes,
			CacheMaxDatabases: DefaultCacheMaxDatabases,
			IdleTimeout:       5 * time.Minute,
			MaxOpen:           8,
			DuckLake: DuckLakeConfig{
				MemoryLimit:          "512MB",
				Threads:              2,
				DataInliningRowLimit: 100,
				ParquetCompression:   "zstd",
				TargetFileSize:       "64MB",
				CatalogSync: CatalogSyncConfig{
					Mode:         "debounce",
					Debounce:     200 * time.Millisecond,
					KeepVersions: 10,
				},
				Maintenance: DuckLakeMaintenanceConfig{
					CheckpointInterval:     time.Hour,
					ExpireOlderThan:        7 * 24 * time.Hour,
					DeleteOlderThan:        24 * time.Hour,
					RewriteDeleteThreshold: 0.95,
				},
			},
		},
		S3: S3Config{
			Prefix: "simplebase",
		},
		LLM: LLMConfig{
			Enabled:   true,
			Providers: map[string]ProviderConfig{},
		},
		Limits: LimitsConfig{
			MaxRequestBytes:      1 << 20,
			MaxQueryRows:         1000,
			QueryTimeout:         30 * time.Second,
			MaxConcurrentQueries: 64,
			MaxBatchStatements:   100,
			MaxSQLBytes:          65536,
		},
		Observability: ObservabilityConfig{
			LogLevel:    "info",
			LogFormat:   "json",
			LogOutput:   "stderr",
			MetricsPath: "/metrics",
		},
		SystemDatabase: SystemDatabaseConfig{
			Name:                 "simplebase-system",
			MetricsFlushInterval: 2 * time.Second,
			LogFlushInterval:     2 * time.Second,
			LogKeepDays:          14,
		},
	}
}

// loadFromEnv 仅从默认值 + 环境变量构造配置（测试辅助；不读 YAML、不 Validate）。
func loadFromEnv() Config {
	cfg := defaults()
	applyEnv(&cfg)
	return cfg
}

// Validate 聚合校验字段。Writable 非 DevMode 下 S3 与 CacheDir 必须完整。
func (c Config) Validate() error {
	var errs []error

	if c.HTTP.Address == "" {
		errs = append(errs, errors.New("http.address is required"))
	}
	if c.HTTP.ReadTimeout <= 0 || c.HTTP.WriteTimeout <= 0 || c.HTTP.IdleTimeout <= 0 {
		errs = append(errs, errors.New("http timeouts must be positive"))
	}
	if c.HTTP.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("http.shutdown_timeout must be positive"))
	}
	if c.Instance.ID == "" {
		errs = append(errs, errors.New("instance.id is required"))
	}
	switch strings.ToLower(c.Database.Engine) {
	case "", EngineDuckLake:
	default:
		errs = append(errs, fmt.Errorf("database.engine must be ducklake (got %q); local/turso engines were removed", c.Database.Engine))
	}
	if c.Database.DuckLake.Threads < 0 {
		errs = append(errs, errors.New("database.ducklake.threads must be non-negative"))
	}
	if c.Database.CacheDir == "" {
		errs = append(errs, errors.New("database.cache_dir is required"))
	}
	if c.Database.CacheMaxBytes <= 0 {
		errs = append(errs, errors.New("database.cache_max_bytes must be positive"))
	}
	if c.Database.CacheMaxDatabases <= 0 {
		errs = append(errs, errors.New("database.cache_max_databases must be positive"))
	}
	if c.Database.IdleTimeout <= 0 {
		errs = append(errs, errors.New("database.idle_timeout must be positive"))
	}
	if c.Database.MaxOpen <= 0 {
		errs = append(errs, errors.New("database.max_open must be positive"))
	}
	if c.Auth.APIKeyHashSecret == "" {
		errs = append(errs, errors.New("auth.api_key_hash_secret is required"))
	}
	if c.Limits.MaxRequestBytes <= 0 || c.Limits.MaxQueryRows <= 0 ||
		c.Limits.QueryTimeout <= 0 || c.Limits.MaxConcurrentQueries <= 0 ||
		c.Limits.MaxBatchStatements <= 0 || c.Limits.MaxSQLBytes <= 0 {
		errs = append(errs, errors.New("limits fields must be positive"))
	}
	switch strings.ToLower(c.Observability.LogFormat) {
	case "", "json", "console":
	default:
		errs = append(errs, fmt.Errorf("observability.log_format must be json or console (got %q)", c.Observability.LogFormat))
	}
	switch strings.ToLower(c.Observability.LogOutput) {
	case "", "stderr", "stdout":
	default:
		errs = append(errs, fmt.Errorf("observability.log_output must be stderr or stdout (got %q)", c.Observability.LogOutput))
	}
	if c.SystemDatabase.LogKeepDays <= 0 {
		errs = append(errs, errors.New("system_database.log_keep_days must be positive"))
	}

	if c.Instance.Writable && !c.DevMode {
		// Writable 实例必须拥有完整 S3 配置，否则无法作为在线持久层。
		// DevMode 旁路 S3，不校验 S3 字段。
		if c.S3.Bucket == "" {
			errs = append(errs, errors.New("s3.bucket is required for writable instance"))
		}
		if c.S3.Region == "" {
			errs = append(errs, errors.New("s3.region is required for writable instance"))
		}
		if c.S3.AccessKey != "" && c.S3.SecretKey == "" {
			errs = append(errs, errors.New("s3.secret_key is required when s3.access_key is set"))
		}
		if c.S3.SecretKey != "" && c.S3.AccessKey == "" {
			errs = append(errs, errors.New("s3.access_key is required when s3.secret_key is set"))
		}
		if c.S3.Prefix == "" {
			errs = append(errs, errors.New("s3.prefix is required for writable instance"))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed: %w", joinErrors(errs))
	}
	return nil
}

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
		},
		"database": map[string]any{
			"engine":              c.Database.Engine,
			"cache_dir":           c.Database.CacheDir,
			"cache_max_bytes":     c.Database.CacheMaxBytes,
			"cache_max_databases": c.Database.CacheMaxDatabases,
			"idle_timeout":        c.Database.IdleTimeout.String(),
			"max_open":            c.Database.MaxOpen,
			"ducklake": map[string]any{
				"memory_limit":               c.Database.DuckLake.MemoryLimit,
				"threads":                    c.Database.DuckLake.Threads,
				"extension_dir":              c.Database.DuckLake.ExtensionDir,
				"data_inlining_row_limit":    c.Database.DuckLake.DataInliningRowLimit,
				"parquet_compression":        c.Database.DuckLake.ParquetCompression,
				"target_file_size":           c.Database.DuckLake.TargetFileSize,
				"require_commit_message":     c.Database.DuckLake.RequireCommitMessage,
				"catalog_sync_mode":          c.Database.DuckLake.CatalogSync.Mode,
				"catalog_sync_debounce":      c.Database.DuckLake.CatalogSync.Debounce.String(),
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
		"observability": map[string]any{
			"log_level":    c.Observability.LogLevel,
			"log_format":   c.Observability.LogFormat,
			"log_output":   c.Observability.LogOutput,
			"metrics_path": c.Observability.MetricsPath,
		},
		"dev_mode": c.DevMode,
	}
}

func joinErrors(errs []error) error {
	var b strings.Builder
	for i, e := range errs {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(e.Error())
	}
	return errors.New(b.String())
}
