// 文件说明：config.go — internal/config 中 Config、StorageModeLocal 等符号的实现
//
// 用法
//
//	Config、StorageConfig、HTTPConfig（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api, internal/api/perfbench 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/config/config.go
//	         │
//	         ▼
//	[下游] internal/utils/strutil
//	[同包] backup.go, env.go, http_boundary.go, yaml.go
//
// 主要符号
//
//	type Config struct {…}
//	type StorageConfig struct {…}
//	type HTTPConfig struct {…}
//	type HTTPSecurityHeaders struct {…}
//	func (h HTTPSecurityHeaders) EffectiveCSP() string
//	type InstanceConfig struct {…}
//	func (c Config) IsProduction() bool
//	func (c Config) ProfileName() string
//	... 另有 22 个顶层符号

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

// Config 是 SimpleBase 单实例服务的全部运行配置。
//
// 加载顺序（后写覆盖前写）：
//  1. 代码默认值（defaults）
//  2. YAML（SIMPLEBASE_CONFIG_PATH，或 cwd 下存在的 config.yaml）
//  3. SIMPLEBASE_ 环境变量覆盖
//
// 敏感字段（S3 AccessKey/SecretKey、Auth.APIKeyHashSecret、LLM API Key）
// 必须来自环境变量或 .env；YAML 中的非空密钥会被拒绝。
// 禁止把密钥写入仓库或日志。
type Config struct {
	HTTP           HTTPConfig           `yaml:"http"`
	Instance       InstanceConfig       `yaml:"instance"`
	Storage        StorageConfig        `yaml:"storage"`
	Database       DatabaseConfig       `yaml:"database"`
	S3             S3Config             `yaml:"s3"`
	Auth           AuthConfig           `yaml:"auth"`
	LLM            LLMConfig            `yaml:"llm"`
	Agent          AgentConfig          `yaml:"agent"`
	Sandbox        SandboxConfig        `yaml:"sandbox"`
	Limits         LimitsConfig         `yaml:"limits"`
	Observability  ObservabilityConfig  `yaml:"observability"`
	SystemDatabase SystemDatabaseConfig `yaml:"system_database"`
	Files          FilesConfig          `yaml:"files"`
	Backup         BackupConfig         `yaml:"backup"`
	Sites          SitesConfig          `yaml:"sites"`
}

// 存储模式取值（storage.database / storage.files）。
const (
	// StorageModeLocal 使用本地盘作为唯一持久层（默认）。
	StorageModeLocal = "local"
	// StorageModeS3 使用 S3 兼容对象存储作为持久层。
	StorageModeS3 = "s3"

	// DefaultDataDir 是本地持久根目录的默认值。
	DefaultDataDir = "./data"
)

// StorageConfig 决定持久层落在哪里。
//
// database 控制系统库与用户库（catalog + parquet 数据），files 控制用户文件。
// local 模式下 data_dir 是唯一副本，需要自行备份；cache_dir 只放可丢弃的内容。
type StorageConfig struct {
	// Database 数据库持久层：local（默认）| s3。
	Database string `yaml:"database"`
	// Files 用户文件持久层：local（默认）| s3。
	Files string `yaml:"files"`
	// DataDir 本地持久根目录；local 模式下不可与 database.cache_dir 重叠。
	DataDir string `yaml:"data_dir"`
}

type HTTPConfig struct {
	Address         string        `yaml:"address"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
	Gzip            bool          `yaml:"gzip"`
	TrustedProxies  []string      `yaml:"trusted_proxies"`
	CORS            CORSConfig    `yaml:"cors"`
	// PublicURL 对外访问地址（如 https://api.example.com）。
	// 用于 HSTS 判定兜底、readiness 检查与告警消息里的控制台链接。
	PublicURL       string              `yaml:"public_url"`
	SecurityHeaders HTTPSecurityHeaders `yaml:"security_headers"`
}

// HTTPSecurityHeaders 控制安全响应头（production-readiness-plan §8.1.3）。
type HTTPSecurityHeaders struct {
	// HSTSMaxAge 秒；0 = 不发 Strict-Transport-Security。
	HSTSMaxAge int `yaml:"hsts_max_age"`
	// HSTSIncludeSubdomains 是否在 HSTS 里带 includeSubDomains。
	HSTSIncludeSubdomains bool `yaml:"hsts_include_subdomains"`
	// CSP enforce | report-only | off；空 = enforce（development 默认 report-only）。
	CSP string `yaml:"csp"`
}

// CSP 模式取值。
const (
	CSPEnforce    = "enforce"
	CSPReportOnly = "report-only"
	CSPOff        = "off"
)

// EffectiveCSP 归一化 CSP 模式；空视为 enforce。
func (h HTTPSecurityHeaders) EffectiveCSP() string {
	m := strutil.Normalize(h.CSP)
	if m == "" {
		return CSPEnforce
	}
	return m
}

type InstanceConfig struct {
	ID       string              `yaml:"id"`
	Writable bool                `yaml:"writable"`
	Lease    InstanceLeaseConfig `yaml:"lease"`
	// Profile 运行档位：development（默认）| production。
	// production 启用安全默认值（见 profile.go 的 applyProfileDefaults）。
	Profile string `yaml:"profile"`
}

// Profile 取值（instance.profile）。
const (
	ProfileDevelopment = "development"
	ProfileProduction  = "production"
)

// IsProduction 返回当前实例是否以 production 档位运行。
func (c Config) IsProduction() bool {
	return strings.EqualFold(strings.TrimSpace(c.Instance.Profile), ProfileProduction)
}

// ProfileName 返回归一化后的档位名（空值视为 development），用于启动日志。
func (c Config) ProfileName() string {
	p := strutil.Normalize(c.Instance.Profile)
	if p == "" {
		return ProfileDevelopment
	}
	return p
}

// InstanceLeaseConfig 控制 per-database 写租约（multi-instance-consistency-plan §4.8）。
// 仅 writable=true 且 storage.database=s3 时生效；本地模式自动跳过。
type InstanceLeaseConfig struct {
	Enabled       bool          `yaml:"enabled"`
	TTL           time.Duration `yaml:"ttl"`
	RenewInterval time.Duration `yaml:"renew_interval"`
	Grace         time.Duration `yaml:"grace"`
	// OnLost 失租行为：release_db（默认，释放该库句柄）| exit（整实例退出）。
	OnLost string `yaml:"on_lost"`
}

const (
	EngineDuckDB = "duckdb"
	EngineSQLite = "sqlite"

	// catalog 引擎取值（ducklake-duckdb-catalog-plan §2）。
	CatalogEngineDuckDB = "duckdb"
	CatalogEngineSQLite = "sqlite"

	DefaultCacheMaxBytes     int64 = 1 << 30 // 1GiB
	DefaultCacheMaxDatabases       = 256
)

type DatabaseConfig struct {
	// Engine 新建项目默认的单文件引擎：duckdb（缺省）| sqlite。每个项目一个文件，建后不可变更。
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
	// CatalogEngine 本地 catalog 引擎：duckdb（默认）| sqlite。实例级一次性选择；
	// 与已有数据不一致时拒绝启动，只能 `simplebase reset` 后切换。
	CatalogEngine        string                    `yaml:"catalog_engine"`
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
	Interval     time.Duration `yaml:"interval"`
	MaxLag       time.Duration `yaml:"max_lag"`
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
	// SessionSecret 登录态 / 委托 JWT 的独立密钥（production-readiness-plan §8.2.3）。
	// 未设置时由 APIKeyHashSecret 派生（HMAC(apikey_secret, "simplebase-session-v1")）。
	// 只能来自环境变量，写 YAML 拒绝启动。
	SessionSecret string `yaml:"session_secret"`
	// APIKeyHashSecrets 版本化密钥环（SIMPLEBASE_AUTH_APIKEY_SECRETS=v2:<new>,v1:<old>）。
	// 空时退化为单值 APIKeyHashSecret。只能来自环境变量。
	APIKeyHashSecrets string `yaml:"api_key_hash_secrets"`
	// BootstrapPassword 引导超管的初始口令；只在首次种子时读取。
	// 只能来自 SIMPLEBASE_BOOTSTRAP_PASSWORD，写 YAML 拒绝启动。
	BootstrapPassword string `yaml:"bootstrap_password"`
	// SessionSecretExplicit 由 Load 阶段填充：true = 用户显式配置；
	// false = 由 API Key 盐派生（doctor 提示轮换影响）。
	SessionSecretExplicit bool `yaml:"-"`
}

const (
	AgentToolProtocolAuto   = "auto"
	AgentToolProtocolNative = "native"
	AgentToolProtocolText   = "text"

	defaultAgentMaxIterations = 8
	defaultAgentRunTimeout    = 180 * time.Second
)

// SandboxBackendLocal / SandboxBackendFake 是 sandbox.backend 的合法取值。
const (
	SandboxBackendLocal = "local"
	SandboxBackendFake  = "fake"
	// SandboxBackendBox 是常驻 agent 后端：每个沙盒一个 bwrap + 一个 PID 1 agent，
	// 后台进程跨请求存活、热 exec 只是一帧往返、暂停能保留进程。
	SandboxBackendBox = "box"
	// SandboxDefaultImage 是 local 后端的镜像标签。
	SandboxDefaultImage = "host"
)

type LimitsConfig struct {
	MaxRequestBytes      int64         `yaml:"max_request_bytes"`
	MaxQueryRows         int           `yaml:"max_query_rows"`
	QueryTimeout         time.Duration `yaml:"query_timeout"`
	MaxConcurrentQueries int           `yaml:"max_concurrent_queries"`
	MaxBatchStatements   int           `yaml:"max_batch_statements"`
	MaxSQLBytes          int           `yaml:"max_sql_bytes"`
	// MaxConcurrentGoRuns 是 /go、调试试跑、cron 与事件触发器的共享并发上限（GF-RUN-04）。
	// 每次调用都会编译执行源码，满载时返回 503 拒绝，防止高频调用耗尽
	// CPU / 内存。0 视为默认值（见 defaults）。
	MaxConcurrentGoRuns int             `yaml:"max_concurrent_go_runs"`
	MaxFilterScanRows   int             `yaml:"max_filter_scan_rows"`
	MaxImportBytes      int64           `yaml:"max_import_bytes"`
	RealtimeMaxConns    int             `yaml:"realtime_max_conns"`
	RateLimit           RateLimitConfig `yaml:"rate_limit"`
}

// 日志级别取值（observability.log_level）。默认 debug：部署/容器环境下默认
// 输出足够上下文，排查问题时不用改配置重启；用 SIMPLEBASE_LOG_LEVEL=info 收敛。
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"

	// DefaultLogLevel 是未显式配置时的日志级别。
	DefaultLogLevel = LogLevelDebug
)

// ValidLogLevel 判断 level 是否为受支持的日志级别（大小写不敏感）。
func ValidLogLevel(level string) bool {
	switch strutil.Normalize(level) {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		return true
	default:
		return false
	}
}

// normalizeLogLevel 归一化日志级别：去空白、转小写；空值回落到默认级别。
// 未知取值保持原样（observability.parseLevel 会退化为 info），由启动日志提示。
func normalizeLogLevel(level string) string {
	trimmed := strutil.Normalize(level)
	if trimmed == "" {
		return DefaultLogLevel
	}
	return trimmed
}

type ObservabilityConfig struct {
	LogLevel    string `yaml:"log_level"`
	LogFormat   string `yaml:"log_format"`
	LogOutput   string `yaml:"log_output"`
	MetricsPath string `yaml:"metrics_path"`
	// PerfStageTiming 开启请求分段计时（api-db-perf-validation-plan §2.1）。
	// 输出 simplebase_api_stage_seconds 与 debug 日志行；默认关闭。
	PerfStageTiming bool `yaml:"perf_stage_timing"`
	// MetricsAddress 非空时 /metrics 与 pprof 改挂独立监听（如 127.0.0.1:9090），
	// 业务端口上的对应路径返回 404（production-readiness-plan §8.1.7）。
	MetricsAddress string `yaml:"metrics_address"`
	// PProf 是否挂 /debug/pprof；只会挂在独立监听或带 token 的业务路由上。
	PProf bool `yaml:"pprof"`
	// MetricsToken /metrics 的 Bearer token；只能来自环境变量，写 YAML 拒绝启动。
	MetricsToken string `yaml:"metrics_token"`
}

// Load 加载配置：代码默认值 → YAML → SIMPLEBASE_ 环境变量覆盖，然后 Validate。
//
// YAML 路径：
//   - SIMPLEBASE_CONFIG_PATH 若设置，文件必须存在且可解析，否则返回错误；
//   - 否则若 cwd 存在 config.yaml 则加载它（解析失败同样报错）；
//   - 否则仅使用默认值 + 环境变量（兼容纯 env 部署）。
func Load() (Config, error) {
	cfg := defaults()

	ycExplicit := yamlConfig{}
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
			ycExplicit = yc
		}
	}

	applyEnv(&cfg)

	// 日志级别归一化：空值（YAML 显式写空 / env 覆盖成空）回落到默认 debug，
	// 避免 observability.parseLevel 静默退化成 info 而丢掉排查信息。
	normalize(&cfg)

	// production 档位：注入安全默认值（只填用户未显式设置的字段），再做校验。
	applyProfileDefaults(&cfg, collectExplicitFields(ycExplicit))

	if err := rejectYAMLSecrets(yamlSecrets); err != nil {
		return Config{}, err
	}
	if err := validateProduction(cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// defaultMaxConcurrentGoRuns 是 limits.max_concurrent_go_runs 为 0 时的回落值，与 defaults() 一致。
const defaultMaxConcurrentGoRuns = 16

// normalize 把 YAML / env 合并后的空值回落到默认值。
// 归一化只能在 Load 里对指针做：Validate 是值接收者，在里面赋值对调用方无效。
func normalize(cfg *Config) {
	cfg.Observability.LogLevel = normalizeLogLevel(cfg.Observability.LogLevel)
	if cfg.Limits.MaxConcurrentGoRuns == 0 {
		cfg.Limits.MaxConcurrentGoRuns = defaultMaxConcurrentGoRuns
	}
}

// ConfigFilePath 返回当前生效的 YAML 路径（可能为空：表示没有配置文件）。
// UI 写回 storage.* 时使用；与加载阶段用的是同一套规则。
func ConfigFilePath() string {
	p, _ := configFilePath()
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
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
		Storage: StorageConfig{
			Database: StorageModeLocal,
			Files:    StorageModeLocal,
			DataDir:  DefaultDataDir,
		},
		HTTP: HTTPConfig{
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
		},
		Instance: InstanceConfig{
			Writable: true,
			Lease: InstanceLeaseConfig{
				Enabled: false,
				TTL:     30 * time.Second,
				Grace:   10 * time.Second,
				OnLost:  "release_db",
			},
		},
		Database: DatabaseConfig{
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
		},
		S3: S3Config{
			Prefix: "simplebase",
		},
		LLM: LLMConfig{
			Enabled:            true,
			Providers:          map[string]ProviderConfig{},
			AgentToolProtocol:  AgentToolProtocolAuto,
			AgentMaxIterations: defaultAgentMaxIterations,
			AgentRunTimeout:    defaultAgentRunTimeout,
		},
		Agent: AgentConfig{
			SkillsCLI:      true,
			DelegationTTL:  15 * time.Minute,
			ConfirmTimeout: 2 * time.Minute,
			MCP:            MCPConfig{Builtin: true},
		},
		Sandbox: SandboxConfig{
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
		},
		Limits: LimitsConfig{
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
		},
		Observability: ObservabilityConfig{
			LogLevel:    DefaultLogLevel,
			LogFormat:   "json",
			LogOutput:   "stderr",
			MetricsPath: "/metrics",
		},
		SystemDatabase: SystemDatabaseConfig{
			Name:                 "simplebase-system",
			MetricsFlushInterval: 15 * time.Second,
			LogFlushInterval:     15 * time.Second,
			LogKeepDays:          14,
		},
		Backup: BackupConfig{
			Dir: "./backups", KeepLast: 7, KeepDays: 30, MaxPerProject: 14, SnapshotTimeout: 10 * time.Minute,
		},
		Sites: SitesConfig{
			URLScheme: "https", IndexDocument: DefaultSiteIndexDocument, CacheTTL: DefaultSiteCacheTTL,
		},
	}
}

// IsDBRemote 返回数据库持久层是否为 S3。
func (c Config) IsDBRemote() bool {
	return strings.EqualFold(strings.TrimSpace(c.Storage.Database), StorageModeS3)
}

// IsFilesRemote 返回用户文件持久层是否为 S3。
func (c Config) IsFilesRemote() bool {
	return strings.EqualFold(strings.TrimSpace(c.Storage.Files), StorageModeS3)
}

// effectiveStorage 把空值归一化为默认的本地模式，便于日志与 UI 展示生效值。
func (c Config) effectiveStorage(mode string) string {
	m := strutil.Normalize(mode)
	if m == "" {
		return StorageModeLocal
	}
	return m
}

// EffectiveDBMode 返回数据库持久层的生效模式（空视为 local）。
func (c Config) EffectiveDBMode() string { return c.effectiveStorage(c.Storage.Database) }

// EffectiveFilesMode 返回用户文件持久层的生效模式（空视为 local）。
func (c Config) EffectiveFilesMode() string { return c.effectiveStorage(c.Storage.Files) }
