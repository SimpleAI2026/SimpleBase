package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 是 SimpleBase 单实例服务的全部运行配置。
// 所有敏感字段（S3 AccessKey/SecretKey、Auth.APIKeyHashSecret、LLM API Key）
// 只能来自环境或密钥服务；禁止写入仓库或日志。
type Config struct {
	HTTP          HTTPConfig          `mapstructure:"http"`
	Instance      InstanceConfig      `mapstructure:"instance"`
	Database      DatabaseConfig      `mapstructure:"database"`
	S3            S3Config            `mapstructure:"s3"`
	Catalog       CatalogConfig       `mapstructure:"catalog"`
	Auth          AuthConfig          `mapstructure:"auth"`
	LLM           LLMConfig           `mapstructure:"llm"`
	Limits        LimitsConfig        `mapstructure:"limits"`
	Observability ObservabilityConfig `mapstructure:"observability"`
	// DevMode 启用本地开发模式：catalog 与用户库使用 :memory: SQLite，
	// 旁路 S3 持久层与 preflight S3 检查。仅用于本地开发，禁止生产开启。
	DevMode bool `mapstructure:"dev_mode"`
}

type HTTPConfig struct {
	Address      string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

type InstanceConfig struct {
	ID       string
	Writable bool
}

const (
	EngineDuckLake = "ducklake"
)

type DatabaseConfig struct {
	// Engine 用户库后端；仅允许 ducklake（缺省即 ducklake）。历史 local/turso 已退役。
	Engine            string
	CacheDir          string
	CacheMaxBytes     int64
	CacheMaxDatabases int
	IdleTimeout       time.Duration
	MaxOpen           int
	MaxIdle           int
	DuckLake          DuckLakeConfig
}

// DuckLakeConfig 对应 planv2.0 §4.8 的 database.ducklake section。
type DuckLakeConfig struct {
	MemoryLimit          string
	Threads              int
	ExtensionDir         string
	DataInliningRowLimit int
	ParquetCompression   string
	TargetFileSize       string
	RequireCommitMessage bool
	CatalogSync          CatalogSyncConfig
	Maintenance          DuckLakeMaintenanceConfig
}

type CatalogSyncConfig struct {
	Mode         string
	Debounce     time.Duration
	KeepVersions int
}

type DuckLakeMaintenanceConfig struct {
	CheckpointInterval     time.Duration
	ExpireOlderThan        time.Duration
	DeleteOlderThan        time.Duration
	RewriteDeleteThreshold float64
}

type S3Config struct {
	Endpoint       string
	Region         string
	Bucket         string
	Prefix         string
	AccessKey      string
	SecretKey      string
	KMSKeyID       string
	ForcePathStyle bool
}

type CatalogConfig struct {
	DatabaseID string
}

type AuthConfig struct {
	APIKeyHashSecret string
}

type LLMConfig struct {
	Enabled   bool
	Providers map[string]ProviderConfig
}

type ProviderConfig struct {
	APIKey       string
	BaseURL      string
	DefaultModel string
	AllowedModels []string
	Timeout      time.Duration
}

type LimitsConfig struct {
	MaxRequestBytes       int64
	MaxQueryRows          int
	QueryTimeout          time.Duration
	MaxConcurrentQueries  int
	MaxBatchStatements    int
	MaxSQLBytes           int
}

type ObservabilityConfig struct {
	LogLevel   string
	LogFormat  string
	MetricsPath string
}

// Load 加载配置。加载顺序：
//  1. 若 SIMPLEBASE_CONFIG_PATH 指定的 YAML 文件（默认 config.yaml）存在，先解析为底；
//  2. 再用 SIMPLEBASE_ 前缀环境变量覆盖（环境变量优先级更高）。
//
// 不打印任何密钥。YAML 文件仅供本地开发，生产用环境变量。
func Load() (Config, error) {
	cfg := loadFromEnv()

	if path := configFilePath(); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			var yc yamlConfig
			if err := yaml.Unmarshal(data, &yc); err == nil {
				applyYAML(&cfg, yc)
			}
			// 解析失败或文件不存在时静默回退到纯环境变量配置
		}
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// configFilePath 返回 YAML 配置路径。空表示不加载 YAML。
func configFilePath() string {
	if p := os.Getenv("SIMPLEBASE_CONFIG_PATH"); p != "" {
		return p
	}
	// 默认尝试当前目录下的 config.yaml；不存在则跳过
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	return ""
}

// yamlConfig 镜像 Config 结构，字段名用 YAML 语义命名。
// 环境变量仍是运行时权威；YAML 仅作本地开发底座。
type yamlConfig struct {
	HTTP          yamlHTTP          `yaml:"http"`
	Instance      yamlInstance      `yaml:"instance"`
	Database      yamlDatabase      `yaml:"database"`
	S3            yamlS3            `yaml:"s3"`
	Catalog       yamlCatalog       `yaml:"catalog"`
	Auth          yamlAuth          `yaml:"auth"`
	Limits        yamlLimits        `yaml:"limits"`
	Observability yamlObservability `yaml:"observability"`
	DevMode       bool              `yaml:"dev_mode"`
}

type yamlHTTP struct {
	Address      string        `yaml:"address"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

type yamlInstance struct {
	ID       string `yaml:"id"`
	Writable bool   `yaml:"writable"`
}

type yamlDatabase struct {
	Engine      string        `yaml:"engine"`
	CacheDir    string        `yaml:"cache_dir"`
	IdleTimeout time.Duration `yaml:"idle_timeout"`
	MaxOpen     int           `yaml:"max_open"`
	MaxIdle     int           `yaml:"max_idle"`
	DuckLake    yamlDuckLake  `yaml:"ducklake"`
}

type yamlDuckLake struct {
	MemoryLimit          string                `yaml:"memory_limit"`
	Threads              int                   `yaml:"threads"`
	ExtensionDir         string                `yaml:"extension_dir"`
	DataInliningRowLimit int                   `yaml:"data_inlining_row_limit"`
	ParquetCompression   string                `yaml:"parquet_compression"`
	TargetFileSize       string                `yaml:"target_file_size"`
	RequireCommitMessage bool                  `yaml:"require_commit_message"`
	CatalogSync          yamlCatalogSync       `yaml:"catalog_sync"`
	Maintenance          yamlDuckLakeMaint     `yaml:"maintenance"`
}

type yamlCatalogSync struct {
	Mode         string `yaml:"mode"`
	DebounceMS   int    `yaml:"debounce_ms"`
	KeepVersions int    `yaml:"keep_versions"`
}

type yamlDuckLakeMaint struct {
	CheckpointInterval     time.Duration `yaml:"checkpoint_interval"`
	ExpireOlderThan        string        `yaml:"expire_older_than"`
	DeleteOlderThan        string        `yaml:"delete_older_than"`
	RewriteDeleteThreshold float64       `yaml:"rewrite_delete_threshold"`
}

type yamlS3 struct {
	Endpoint       string `yaml:"endpoint"`
	Region         string `yaml:"region"`
	Bucket         string `yaml:"bucket"`
	Prefix         string `yaml:"prefix"`
	AccessKey      string `yaml:"access_key"`
	SecretKey      string `yaml:"secret_key"`
	ForcePathStyle bool   `yaml:"force_path_style"`
}

type yamlCatalog struct {
	DatabaseID string `yaml:"database_id"`
}

type yamlAuth struct {
	APIKeyHashSecret string `yaml:"api_key_hash_secret"`
}

type yamlLimits struct {
	MaxRequestBytes      int64         `yaml:"max_request_bytes"`
	MaxQueryRows         int           `yaml:"max_query_rows"`
	QueryTimeout         time.Duration `yaml:"query_timeout"`
	MaxConcurrentQueries int           `yaml:"max_concurrent_queries"`
	MaxBatchStatements   int           `yaml:"max_batch_statements"`
	MaxSQLBytes          int           `yaml:"max_sql_bytes"`
}

type yamlObservability struct {
	LogLevel    string `yaml:"log_level"`
	LogFormat   string `yaml:"log_format"`
	MetricsPath string `yaml:"metrics_path"`
}

// applyYAML 将 YAML 值填入 cfg 中仍为默认值的字段。
// 仅当环境变量未显式设置（即字段为 envStr/envInt 的默认值）时才覆盖；
// 这里采用简化策略：YAML 值非零值则覆盖环境变量默认值。
func applyYAML(cfg *Config, yc yamlConfig) {
	if yc.HTTP.Address != "" {
		if os.Getenv("SIMPLEBASE_HTTP_ADDRESS") == "" {
			cfg.HTTP.Address = yc.HTTP.Address
		}
	}
	if yc.HTTP.ReadTimeout > 0 {
		if os.Getenv("SIMPLEBASE_HTTP_READ_TIMEOUT") == "" {
			cfg.HTTP.ReadTimeout = yc.HTTP.ReadTimeout
		}
	}
	if yc.HTTP.WriteTimeout > 0 {
		if os.Getenv("SIMPLEBASE_HTTP_WRITE_TIMEOUT") == "" {
			cfg.HTTP.WriteTimeout = yc.HTTP.WriteTimeout
		}
	}
	if yc.HTTP.IdleTimeout > 0 {
		if os.Getenv("SIMPLEBASE_HTTP_IDLE_TIMEOUT") == "" {
			cfg.HTTP.IdleTimeout = yc.HTTP.IdleTimeout
		}
	}
	if yc.Instance.ID != "" {
		if os.Getenv("SIMPLEBASE_INSTANCE_ID") == "" {
			cfg.Instance.ID = yc.Instance.ID
		}
	}
	// Writable 布尔：YAML 设 false 时需要能覆盖默认 true，用 env lookup 判断
	if os.Getenv("SIMPLEBASE_INSTANCE_WRITABLE") == "" {
		cfg.Instance.Writable = yc.Instance.Writable
	}
	if yc.Database.Engine != "" {
		if os.Getenv("SIMPLEBASE_DB_ENGINE") == "" {
			cfg.Database.Engine = yc.Database.Engine
		}
	}
	if yc.Database.CacheDir != "" {
		if os.Getenv("SIMPLEBASE_DB_CACHE_DIR") == "" {
			cfg.Database.CacheDir = yc.Database.CacheDir
		}
	}
	applyYAMLDuckLake(cfg, yc.Database.DuckLake)
	if yc.Database.IdleTimeout > 0 {
		if os.Getenv("SIMPLEBASE_DB_IDLE_TIMEOUT") == "" {
			cfg.Database.IdleTimeout = yc.Database.IdleTimeout
		}
	}
	if yc.Database.MaxOpen > 0 {
		if os.Getenv("SIMPLEBASE_DB_MAX_OPEN") == "" {
			cfg.Database.MaxOpen = yc.Database.MaxOpen
		}
	}
	if yc.Database.MaxIdle >= 0 && yc.Database.MaxIdle != 0 {
		if os.Getenv("SIMPLEBASE_DB_MAX_IDLE") == "" {
			cfg.Database.MaxIdle = yc.Database.MaxIdle
		}
	}
	if yc.S3.Endpoint != "" {
		if os.Getenv("SIMPLEBASE_S3_ENDPOINT") == "" {
			cfg.S3.Endpoint = yc.S3.Endpoint
		}
	}
	if yc.S3.Region != "" {
		if os.Getenv("SIMPLEBASE_S3_REGION") == "" {
			cfg.S3.Region = yc.S3.Region
		}
	}
	if yc.S3.Bucket != "" {
		if os.Getenv("SIMPLEBASE_S3_BUCKET") == "" {
			cfg.S3.Bucket = yc.S3.Bucket
		}
	}
	if yc.S3.Prefix != "" {
		if os.Getenv("SIMPLEBASE_S3_PREFIX") == "" {
			cfg.S3.Prefix = yc.S3.Prefix
		}
	}
	if yc.S3.AccessKey != "" {
		if os.Getenv("SIMPLEBASE_S3_ACCESS_KEY") == "" {
			cfg.S3.AccessKey = yc.S3.AccessKey
		}
	}
	if yc.S3.SecretKey != "" {
		if os.Getenv("SIMPLEBASE_S3_SECRET_KEY") == "" {
			cfg.S3.SecretKey = yc.S3.SecretKey
		}
	}
	if os.Getenv("SIMPLEBASE_S3_FORCE_PATH_STYLE") == "" {
		cfg.S3.ForcePathStyle = yc.S3.ForcePathStyle
	}
	if yc.Catalog.DatabaseID != "" {
		if os.Getenv("SIMPLEBASE_CATALOG_DATABASE_ID") == "" {
			cfg.Catalog.DatabaseID = yc.Catalog.DatabaseID
		}
	}
	if yc.Auth.APIKeyHashSecret != "" {
		if os.Getenv("SIMPLEBASE_AUTH_APIKEY_SECRET") == "" {
			cfg.Auth.APIKeyHashSecret = yc.Auth.APIKeyHashSecret
		}
	}
	if yc.Limits.MaxRequestBytes > 0 {
		if os.Getenv("SIMPLEBASE_LIMITS_MAX_REQUEST_BYTES") == "" {
			cfg.Limits.MaxRequestBytes = yc.Limits.MaxRequestBytes
		}
	}
	if yc.Limits.MaxQueryRows > 0 {
		if os.Getenv("SIMPLEBASE_LIMITS_MAX_QUERY_ROWS") == "" {
			cfg.Limits.MaxQueryRows = yc.Limits.MaxQueryRows
		}
	}
	if yc.Limits.QueryTimeout > 0 {
		if os.Getenv("SIMPLEBASE_LIMITS_QUERY_TIMEOUT") == "" {
			cfg.Limits.QueryTimeout = yc.Limits.QueryTimeout
		}
	}
	if yc.Limits.MaxConcurrentQueries > 0 {
		if os.Getenv("SIMPLEBASE_LIMITS_MAX_CONCURRENT_QUERIES") == "" {
			cfg.Limits.MaxConcurrentQueries = yc.Limits.MaxConcurrentQueries
		}
	}
	if yc.Limits.MaxBatchStatements > 0 {
		if os.Getenv("SIMPLEBASE_LIMITS_MAX_BATCH_STATEMENTS") == "" {
			cfg.Limits.MaxBatchStatements = yc.Limits.MaxBatchStatements
		}
	}
	if yc.Limits.MaxSQLBytes > 0 {
		if os.Getenv("SIMPLEBASE_LIMITS_MAX_SQL_BYTES") == "" {
			cfg.Limits.MaxSQLBytes = yc.Limits.MaxSQLBytes
		}
	}
	if yc.Observability.LogLevel != "" {
		if os.Getenv("SIMPLEBASE_LOG_LEVEL") == "" {
			cfg.Observability.LogLevel = yc.Observability.LogLevel
		}
	}
	if yc.Observability.LogFormat != "" {
		if os.Getenv("SIMPLEBASE_LOG_FORMAT") == "" {
			cfg.Observability.LogFormat = yc.Observability.LogFormat
		}
	}
	if yc.Observability.MetricsPath != "" {
		if os.Getenv("SIMPLEBASE_METRICS_PATH") == "" {
			cfg.Observability.MetricsPath = yc.Observability.MetricsPath
		}
	}
	// DevMode：YAML 设 true 时启用（env 未显式设置才覆盖）。
	if os.Getenv("SIMPLEBASE_DEV_MODE") == "" {
		cfg.DevMode = yc.DevMode
	}
}

// loadFromEnv 仅从环境变量加载配置（原 Load 逻辑）。
func loadFromEnv() Config {
	return Config{
		DevMode: envBool("SIMPLEBASE_DEV_MODE", false),
		HTTP: HTTPConfig{
			Address:      envStr("SIMPLEBASE_HTTP_ADDRESS", ":8080"),
			ReadTimeout:  envDuration("SIMPLEBASE_HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout: envDuration("SIMPLEBASE_HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:  envDuration("SIMPLEBASE_HTTP_IDLE_TIMEOUT", 60*time.Second),
		},
		Instance: InstanceConfig{
			ID:       envStr("SIMPLEBASE_INSTANCE_ID", ""),
			Writable: envBool("SIMPLEBASE_INSTANCE_WRITABLE", true),
		},
		Database: DatabaseConfig{
			Engine:      envStr("SIMPLEBASE_DB_ENGINE", EngineDuckLake),
			CacheDir:    envStr("SIMPLEBASE_DB_CACHE_DIR", ""),
			IdleTimeout: envDuration("SIMPLEBASE_DB_IDLE_TIMEOUT", 5*time.Minute),
			MaxOpen:     envInt("SIMPLEBASE_DB_MAX_OPEN", 8),
			MaxIdle:     envInt("SIMPLEBASE_DB_MAX_IDLE", 2),
			DuckLake: DuckLakeConfig{
				MemoryLimit:          envStr("SIMPLEBASE_DUCKLAKE_MEMORY_LIMIT", "512MB"),
				Threads:              envInt("SIMPLEBASE_DUCKLAKE_THREADS", 2),
				ExtensionDir:         envStr("SIMPLEBASE_DUCKLAKE_EXTENSION_DIR", ""),
				DataInliningRowLimit: envInt("SIMPLEBASE_DUCKLAKE_DATA_INLINING_ROW_LIMIT", 100),
				ParquetCompression:   envStr("SIMPLEBASE_DUCKLAKE_PARQUET_COMPRESSION", "zstd"),
				TargetFileSize:       envStr("SIMPLEBASE_DUCKLAKE_TARGET_FILE_SIZE", "64MB"),
				RequireCommitMessage: envBool("SIMPLEBASE_DUCKLAKE_REQUIRE_COMMIT_MESSAGE", false),
				CatalogSync: CatalogSyncConfig{
					Mode:         envStr("SIMPLEBASE_DUCKLAKE_SYNC_MODE", "debounce"),
					Debounce:     time.Duration(envInt("SIMPLEBASE_DUCKLAKE_SYNC_DEBOUNCE_MS", 200)) * time.Millisecond,
					KeepVersions: envInt("SIMPLEBASE_DUCKLAKE_SYNC_KEEP_VERSIONS", 10),
				},
				Maintenance: DuckLakeMaintenanceConfig{
					CheckpointInterval:     envDuration("SIMPLEBASE_DUCKLAKE_MAINT_CHECKPOINT_INTERVAL", time.Hour),
					ExpireOlderThan:        envDuration("SIMPLEBASE_DUCKLAKE_MAINT_EXPIRE_OLDER_THAN", 7*24*time.Hour),
					DeleteOlderThan:        envDuration("SIMPLEBASE_DUCKLAKE_MAINT_DELETE_OLDER_THAN", 24*time.Hour),
					RewriteDeleteThreshold: envFloat64("SIMPLEBASE_DUCKLAKE_MAINT_REWRITE_DELETE_THRESHOLD", 0.95),
				},
			},
		},
		S3: S3Config{
			Endpoint:       envStr("SIMPLEBASE_S3_ENDPOINT", ""),
			Region:         envStr("SIMPLEBASE_S3_REGION", ""),
			Bucket:         envStr("SIMPLEBASE_S3_BUCKET", ""),
			Prefix:         envStr("SIMPLEBASE_S3_PREFIX", "simplebase"),
			AccessKey:      envStr("SIMPLEBASE_S3_ACCESS_KEY", ""),
			SecretKey:      envStr("SIMPLEBASE_S3_SECRET_KEY", ""),
			KMSKeyID:       envStr("SIMPLEBASE_S3_KMS_KEY_ID", ""),
			ForcePathStyle: envBool("SIMPLEBASE_S3_FORCE_PATH_STYLE", false),
		},
		Catalog: CatalogConfig{
			DatabaseID: envStr("SIMPLEBASE_CATALOG_DATABASE_ID", "simplebase-catalog"),
		},
		Auth: AuthConfig{
			APIKeyHashSecret: envStr("SIMPLEBASE_AUTH_APIKEY_SECRET", ""),
		},
		LLM: LLMConfig{
			Providers: loadLLMProviders(),
		},
		Limits: LimitsConfig{
			MaxRequestBytes:      envInt64("SIMPLEBASE_LIMITS_MAX_REQUEST_BYTES", 1<<20),
			MaxQueryRows:         envInt("SIMPLEBASE_LIMITS_MAX_QUERY_ROWS", 1000),
			QueryTimeout:         envDuration("SIMPLEBASE_LIMITS_QUERY_TIMEOUT", 30*time.Second),
			MaxConcurrentQueries: envInt("SIMPLEBASE_LIMITS_MAX_CONCURRENT_QUERIES", 64),
			MaxBatchStatements:   envInt("SIMPLEBASE_LIMITS_MAX_BATCH_STATEMENTS", 100),
			MaxSQLBytes:          envInt("SIMPLEBASE_LIMITS_MAX_SQL_BYTES", 65536),
		},
		Observability: ObservabilityConfig{
			LogLevel:    envStr("SIMPLEBASE_LOG_LEVEL", "info"),
			LogFormat:   envStr("SIMPLEBASE_LOG_FORMAT", "json"),
			MetricsPath: envStr("SIMPLEBASE_METRICS_PATH", "/metrics"),
		},
	}
}

func loadLLMProviders() map[string]ProviderConfig {
	// SIMPLEBASE_LLM_PROVIDERS=openai,anthropic
	// SIMPLEBASE_LLM_PROVIDER_OPENAI_API_KEY=...
	// SIMPLEBASE_LLM_PROVIDER_OPENAI_BASE_URL=...
	// SIMPLEBASE_LLM_PROVIDER_OPENAI_DEFAULT_MODEL=gpt-4o-mini
	// SIMPLEBASE_LLM_PROVIDER_OPENAI_ALLOWED_MODELS=gpt-4o-mini,gpt-4o
	namesCsv := envStr("SIMPLEBASE_LLM_PROVIDERS", "")
	if namesCsv == "" {
		return map[string]ProviderConfig{}
	}
	out := map[string]ProviderConfig{}
	for _, name := range strings.Split(namesCsv, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		upper := strings.ToUpper(name)
		out[name] = ProviderConfig{
			APIKey:        envStr("SIMPLEBASE_LLM_PROVIDER_"+upper+"_API_KEY", ""),
			BaseURL:       envStr("SIMPLEBASE_LLM_PROVIDER_"+upper+"_BASE_URL", ""),
			DefaultModel:  envStr("SIMPLEBASE_LLM_PROVIDER_"+upper+"_DEFAULT_MODEL", ""),
			AllowedModels: envList("SIMPLEBASE_LLM_PROVIDER_"+upper+"_ALLOWED_MODELS"),
			Timeout:       envDuration("SIMPLEBASE_LLM_PROVIDER_"+upper+"_TIMEOUT", 60*time.Second),
		}
	}
	return out
}

// Validate 聚合校验字段。Writable 模式下 S3 与 CacheDir 必须完整。
func (c Config) Validate() error {
	var errs []error

	if c.HTTP.Address == "" {
		errs = append(errs, errors.New("http.address is required"))
	}
	if c.HTTP.ReadTimeout <= 0 || c.HTTP.WriteTimeout <= 0 || c.HTTP.IdleTimeout <= 0 {
		errs = append(errs, errors.New("http timeouts must be positive"))
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
	if c.Database.IdleTimeout <= 0 {
		errs = append(errs, errors.New("database.idle_timeout must be positive"))
	}
	if c.Database.MaxOpen <= 0 || c.Database.MaxIdle < 0 {
		errs = append(errs, errors.New("database.max_open must be positive and max_idle non-negative"))
	}
	if c.Catalog.DatabaseID == "" {
		errs = append(errs, errors.New("catalog.database_id is required"))
	}
	if c.Auth.APIKeyHashSecret == "" {
		errs = append(errs, errors.New("auth.api_key_hash_secret is required"))
	}
	if c.Limits.MaxRequestBytes <= 0 || c.Limits.MaxQueryRows <= 0 ||
		c.Limits.QueryTimeout <= 0 || c.Limits.MaxConcurrentQueries <= 0 ||
		c.Limits.MaxBatchStatements <= 0 || c.Limits.MaxSQLBytes <= 0 {
		errs = append(errs, errors.New("limits fields must be positive"))
	}

	if c.Instance.Writable && !c.DevMode {
		// Writable 实例必须拥有完整 S3 配置，否则无法作为在线持久层。
		// DevMode 旁路 S3，使用 :memory: SQLite，不校验 S3 字段。
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
			"has_api_key":  p.APIKey != "",
			"base_url":     p.BaseURL,
			"default_model": p.DefaultModel,
			"allowed_models": p.AllowedModels,
		}
		providers[name] = entry
	}
	return map[string]any{
		"http": map[string]any{
			"address":      c.HTTP.Address,
			"read_timeout":  c.HTTP.ReadTimeout.String(),
			"write_timeout": c.HTTP.WriteTimeout.String(),
			"idle_timeout":  c.HTTP.IdleTimeout.String(),
		},
		"instance": map[string]any{
			"id":       c.Instance.ID,
			"writable": c.Instance.Writable,
		},
		"database": map[string]any{
			"engine":       c.Database.Engine,
			"cache_dir":    c.Database.CacheDir,
			"idle_timeout": c.Database.IdleTimeout.String(),
			"max_open":     c.Database.MaxOpen,
			"max_idle":     c.Database.MaxIdle,
			"ducklake": map[string]any{
				"memory_limit":             c.Database.DuckLake.MemoryLimit,
				"threads":                  c.Database.DuckLake.Threads,
				"extension_dir":            c.Database.DuckLake.ExtensionDir,
				"data_inlining_row_limit":  c.Database.DuckLake.DataInliningRowLimit,
				"parquet_compression":      c.Database.DuckLake.ParquetCompression,
				"target_file_size":         c.Database.DuckLake.TargetFileSize,
				"require_commit_message":   c.Database.DuckLake.RequireCommitMessage,
				"catalog_sync_mode":        c.Database.DuckLake.CatalogSync.Mode,
				"catalog_sync_debounce":    c.Database.DuckLake.CatalogSync.Debounce.String(),
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
		"catalog": map[string]any{
			"database_id": c.Catalog.DatabaseID,
		},
		"limits": map[string]any{
			"max_request_bytes":      c.Limits.MaxRequestBytes,
			"max_query_rows":         c.Limits.MaxQueryRows,
			"query_timeout":          c.Limits.QueryTimeout.String(),
			"max_concurrent_queries": c.Limits.MaxConcurrentQueries,
			"max_batch_statements":   c.Limits.MaxBatchStatements,
			"max_sql_bytes":          c.Limits.MaxSQLBytes,
		},
		"llm_providers": providers,
		"observability": map[string]any{
			"log_level":    c.Observability.LogLevel,
			"log_format":   c.Observability.LogFormat,
			"metrics_path": c.Observability.MetricsPath,
		},
		"dev_mode": c.DevMode,
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

func applyYAMLDuckLake(cfg *Config, y yamlDuckLake) {
	if y.MemoryLimit != "" && os.Getenv("SIMPLEBASE_DUCKLAKE_MEMORY_LIMIT") == "" {
		cfg.Database.DuckLake.MemoryLimit = y.MemoryLimit
	}
	if y.Threads > 0 && os.Getenv("SIMPLEBASE_DUCKLAKE_THREADS") == "" {
		cfg.Database.DuckLake.Threads = y.Threads
	}
	if y.ExtensionDir != "" && os.Getenv("SIMPLEBASE_DUCKLAKE_EXTENSION_DIR") == "" {
		cfg.Database.DuckLake.ExtensionDir = y.ExtensionDir
	}
	if y.DataInliningRowLimit > 0 && os.Getenv("SIMPLEBASE_DUCKLAKE_DATA_INLINING_ROW_LIMIT") == "" {
		cfg.Database.DuckLake.DataInliningRowLimit = y.DataInliningRowLimit
	}
	if y.ParquetCompression != "" && os.Getenv("SIMPLEBASE_DUCKLAKE_PARQUET_COMPRESSION") == "" {
		cfg.Database.DuckLake.ParquetCompression = y.ParquetCompression
	}
	if y.TargetFileSize != "" && os.Getenv("SIMPLEBASE_DUCKLAKE_TARGET_FILE_SIZE") == "" {
		cfg.Database.DuckLake.TargetFileSize = y.TargetFileSize
	}
	if os.Getenv("SIMPLEBASE_DUCKLAKE_REQUIRE_COMMIT_MESSAGE") == "" {
		cfg.Database.DuckLake.RequireCommitMessage = y.RequireCommitMessage
	}
	if y.CatalogSync.Mode != "" && os.Getenv("SIMPLEBASE_DUCKLAKE_SYNC_MODE") == "" {
		cfg.Database.DuckLake.CatalogSync.Mode = y.CatalogSync.Mode
	}
	if y.CatalogSync.DebounceMS > 0 && os.Getenv("SIMPLEBASE_DUCKLAKE_SYNC_DEBOUNCE_MS") == "" {
		cfg.Database.DuckLake.CatalogSync.Debounce = time.Duration(y.CatalogSync.DebounceMS) * time.Millisecond
	}
	if y.CatalogSync.KeepVersions > 0 && os.Getenv("SIMPLEBASE_DUCKLAKE_SYNC_KEEP_VERSIONS") == "" {
		cfg.Database.DuckLake.CatalogSync.KeepVersions = y.CatalogSync.KeepVersions
	}
	if y.Maintenance.CheckpointInterval > 0 && os.Getenv("SIMPLEBASE_DUCKLAKE_MAINT_CHECKPOINT_INTERVAL") == "" {
		cfg.Database.DuckLake.Maintenance.CheckpointInterval = y.Maintenance.CheckpointInterval
	}
	if y.Maintenance.ExpireOlderThan != "" && os.Getenv("SIMPLEBASE_DUCKLAKE_MAINT_EXPIRE_OLDER_THAN") == "" {
		if d, err := parseDuration(y.Maintenance.ExpireOlderThan); err == nil {
			cfg.Database.DuckLake.Maintenance.ExpireOlderThan = d
		}
	}
	if y.Maintenance.DeleteOlderThan != "" && os.Getenv("SIMPLEBASE_DUCKLAKE_MAINT_DELETE_OLDER_THAN") == "" {
		if d, err := parseDuration(y.Maintenance.DeleteOlderThan); err == nil {
			cfg.Database.DuckLake.Maintenance.DeleteOlderThan = d
		}
	}
	if y.Maintenance.RewriteDeleteThreshold > 0 && os.Getenv("SIMPLEBASE_DUCKLAKE_MAINT_REWRITE_DELETE_THRESHOLD") == "" {
		cfg.Database.DuckLake.Maintenance.RewriteDeleteThreshold = y.Maintenance.RewriteDeleteThreshold
	}
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
