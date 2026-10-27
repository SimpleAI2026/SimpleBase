// 文件说明：config.go — internal/config 中 Config、StorageModeLocal 等符号的实现
//
// 用法
//
//	(l LLMConfig) EffectiveAgentToolProtocol() string  // EffectiveAgentToolProtocol 返回归一后的工具协议，空值视为 auto
//	(l LLMConfig) EffectiveAgentMaxIterations() int  // EffectiveAgentMaxIterations 返回工具迭代上限，零值为 8
//	同包调用：Config → env.go, http_boundary.go 等；ProviderConfig → env.go, yaml.go；defaults() → env.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api, internal/api/perfbench 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/config/config.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backup.go, env.go, http_boundary.go, yaml.go
//	[关键引用] Config ← env.go, http_boundary.go, yaml.go
//
// 主要符号
//
//	type Config struct {…}
//	const StorageModeLocal
//	const StorageModeS3
//	const DefaultDataDir
//	type StorageConfig struct {…}
//	type HTTPConfig struct {…}
//	... 另有 63 个顶层符号

package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
}

type InstanceConfig struct {
	ID       string              `yaml:"id"`
	Writable bool                `yaml:"writable"`
	Lease    InstanceLeaseConfig `yaml:"lease"`
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
}

type LLMConfig struct {
	Enabled   bool                      `yaml:"enabled"`
	Providers map[string]ProviderConfig `yaml:"providers"`
	// 云 Agent 运行参数（planv4.0 cloud-agent-optimization-plan §6）。零值表示默认。
	AgentToolProtocol  string        `yaml:"agent_tool_protocol"`  // auto | native | text
	AgentMaxIterations int           `yaml:"agent_max_iterations"` // 1–20
	AgentRunTimeout    time.Duration `yaml:"agent_run_timeout"`    // 10s–15m
}

const (
	AgentToolProtocolAuto   = "auto"
	AgentToolProtocolNative = "native"
	AgentToolProtocolText   = "text"

	defaultAgentMaxIterations = 8
	defaultAgentRunTimeout    = 180 * time.Second
)

// EffectiveAgentToolProtocol 返回归一后的工具协议，空值视为 auto。
func (l LLMConfig) EffectiveAgentToolProtocol() string {
	switch p := strings.ToLower(strings.TrimSpace(l.AgentToolProtocol)); p {
	case AgentToolProtocolNative, AgentToolProtocolText:
		return p
	default:
		return AgentToolProtocolAuto
	}
}

// EffectiveAgentMaxIterations 返回工具迭代上限，零值为 8。
func (l LLMConfig) EffectiveAgentMaxIterations() int {
	if l.AgentMaxIterations <= 0 {
		return defaultAgentMaxIterations
	}
	return l.AgentMaxIterations
}

// EffectiveAgentRunTimeout 返回单次 run 总时限，零值为 180s。
func (l LLMConfig) EffectiveAgentRunTimeout() time.Duration {
	if l.AgentRunTimeout <= 0 {
		return defaultAgentRunTimeout
	}
	return l.AgentRunTimeout
}

func validateLLMAgent(l LLMConfig) []error {
	var errs []error
	switch strings.ToLower(strings.TrimSpace(l.AgentToolProtocol)) {
	case "", AgentToolProtocolAuto, AgentToolProtocolNative, AgentToolProtocolText:
	default:
		errs = append(errs, fmt.Errorf("llm.agent_tool_protocol must be auto, native or text (got %q)", l.AgentToolProtocol))
	}
	if l.AgentMaxIterations < 0 || l.AgentMaxIterations > 20 {
		errs = append(errs, fmt.Errorf("llm.agent_max_iterations must be 1-20 (got %d)", l.AgentMaxIterations))
	}
	if l.AgentRunTimeout != 0 && (l.AgentRunTimeout < 10*time.Second || l.AgentRunTimeout > 15*time.Minute) {
		errs = append(errs, fmt.Errorf("llm.agent_run_timeout must be 10s-15m (got %s)", l.AgentRunTimeout))
	}
	return errs
}

// AgentConfig 控制云助手 CLI skill（planv5.0 cloud-assistant-skills-cli）。
type AgentConfig struct {
	SkillsCLI      bool          `yaml:"skills_cli"`
	CLIPath        string        `yaml:"cli_path"`
	DelegationTTL  time.Duration `yaml:"delegation_ttl"`
	ConfirmTimeout time.Duration `yaml:"confirm_timeout"`
	// TraceLog 开启云助手的运行日志（planv6.0 v2 §5）。默认关闭：
	// 开启后按「元数据层（默认 info）」+「内容层（脱敏 + 截断）」两级输出，
	// 内容层是 internal/AGENTS.md 脱敏约定的显式例外，仅在此开关为真时生效。
	TraceLog bool `yaml:"trace_log"`
	// ShellIsolation 是本机 shell 的隔离策略（planv6.2）。默认关闭。
	// 服务端云助手的 sandbox_shell 走云沙盒（sandbox.*），不经过它；
	// 目前只有 gocloudagent CLI 会启动本机 shell，而 CLI 读的是 ~/.gocloudagent/config.json，
	// 这一段在服务端只做解析与校验，供操作者对照抄写。
	ShellIsolation ShellIsolationConfig `yaml:"shell_isolation"`
}

// ShellIsolationConfig 是 agent.shell_isolation 段。字段含义见 gosandbox/host.Policy。
// internal/config 不 import gosandbox，所以校验在本包内自行完成。
type ShellIsolationConfig struct {
	Enabled          bool          `yaml:"enabled"`
	Mode             string        `yaml:"mode"` // read-only | workspace-write | danger-full-access
	WritableRoots    []string      `yaml:"writable_roots"`
	ReadOnlySubpaths []string      `yaml:"read_only_subpaths"`
	DenyRead         []string      `yaml:"deny_read"`
	MemoryMaxBytes   int64         `yaml:"memory_max_bytes"`
	CPUMax           string        `yaml:"cpu_max"`
	PidsMax          int64         `yaml:"pids_max"`
	Timeout          time.Duration `yaml:"timeout"`
	BwrapPath        string        `yaml:"bwrap_path"`
	HelperPath       string        `yaml:"helper_path"`
}

// EffectiveMode 返回生效档位；未指定取 workspace-write。
func (c ShellIsolationConfig) EffectiveMode() string {
	if c.Mode == "" {
		return "workspace-write"
	}
	return c.Mode
}

func validateShellIsolation(c ShellIsolationConfig) []error {
	var errs []error
	switch c.EffectiveMode() {
	case "read-only", "workspace-write", "danger-full-access":
	default:
		errs = append(errs, fmt.Errorf("agent.shell_isolation.mode must be read-only, workspace-write or danger-full-access (got %q)", c.Mode))
	}
	if c.MemoryMaxBytes < 0 || c.PidsMax < 0 || c.Timeout < 0 {
		errs = append(errs, errors.New("agent.shell_isolation.memory_max_bytes, pids_max and timeout must not be negative"))
	}
	if c.CPUMax != "" {
		f := strings.Fields(c.CPUMax)
		ok := len(f) >= 1 && len(f) <= 2
		for i, tok := range f {
			if tok == "max" && i == 0 {
				continue
			}
			if tok == "" || strings.Trim(tok, "0123456789") != "" {
				ok = false
			}
		}
		if !ok {
			errs = append(errs, fmt.Errorf("agent.shell_isolation.cpu_max must be \"<quota|max> [period]\" (got %q)", c.CPUMax))
		}
	}
	for _, group := range []struct {
		name  string
		paths []string
	}{{"writable_roots", c.WritableRoots}, {"deny_read", c.DenyRead}} {
		for _, p := range group.paths {
			if !strings.HasPrefix(p, "/") && p != "~" && !strings.HasPrefix(p, "~/") {
				errs = append(errs, fmt.Errorf("agent.shell_isolation.%s entries must be absolute or start with ~ (got %q)", group.name, p))
			}
		}
	}
	for _, p := range []struct{ name, v string }{{"bwrap_path", c.BwrapPath}, {"helper_path", c.HelperPath}} {
		if p.v != "" && !strings.HasPrefix(p.v, "/") {
			errs = append(errs, fmt.Errorf("agent.shell_isolation.%s must be absolute (got %q)", p.name, p.v))
		}
	}
	return errs
}

func (a AgentConfig) EffectiveDelegationTTL() time.Duration {
	if a.DelegationTTL <= 0 {
		return 15 * time.Minute
	}
	return a.DelegationTTL
}

func (a AgentConfig) EffectiveConfirmTimeout() time.Duration {
	if a.ConfirmTimeout <= 0 {
		return 2 * time.Minute
	}
	return a.ConfirmTimeout
}

func validateAgent(a AgentConfig) []error {
	var errs []error
	if a.DelegationTTL != 0 && (a.DelegationTTL < time.Minute || a.DelegationTTL > 30*time.Minute) {
		errs = append(errs, fmt.Errorf("agent.delegation_ttl must be 1m-30m (got %s)", a.DelegationTTL))
	}
	if a.ConfirmTimeout != 0 && (a.ConfirmTimeout < 10*time.Second || a.ConfirmTimeout > 10*time.Minute) {
		errs = append(errs, fmt.Errorf("agent.confirm_timeout must be 10s-10m (got %s)", a.ConfirmTimeout))
	}
	errs = append(errs, validateShellIsolation(a.ShellIsolation)...)
	return errs
}

type ProviderConfig struct {
	APIKey        string        `yaml:"api_key"`
	BaseURL       string        `yaml:"base_url"`
	DefaultModel  string        `yaml:"default_model"`
	AllowedModels []string      `yaml:"allowed_models"`
	Timeout       time.Duration `yaml:"timeout"`
}

// SandboxConfig 控制「云沙盒」：项目级的隔离命令环境。
//
// 后端只有两种：local（本机进程隔离，经 gosandbox/host：Linux bubblewrap + Landlock + seccomp，
// macOS Seatbelt）与 fake（内存假后端，仅测试）。不再依赖任何外部沙盒服务，没有 API Key。
// local 与宿主共享内核，不是 VM，只适合自托管 / 受信任用户。
type SandboxConfig struct {
	Enabled bool `yaml:"enabled"`
	// Image 在 local 后端下只是展示用的标签（不拉取任何镜像），默认 "host"。
	Image          string        `yaml:"image"`
	CPUs           int           `yaml:"cpus"`       // 1–4；cgroup 不可用时不会生效
	MemoryMiB      int           `yaml:"memory_mib"` // 128–4096；cgroup 不可用时不会生效
	MaxDuration    time.Duration `yaml:"max_duration"`
	IdleTimeout    time.Duration `yaml:"idle_timeout"`
	ExecTimeout    time.Duration `yaml:"exec_timeout"`
	MaxOutputBytes int           `yaml:"max_output_bytes"`
	MaxFileBytes   int           `yaml:"max_file_bytes"`
	Network        string        `yaml:"network"` // 仅 none：local 后端不提供网络
	Workdir        string        `yaml:"workdir"`

	Backend        string        `yaml:"backend"`          // local | fake（fake 仅测试环境，须 SIMPLEBASE_SANDBOX_ALLOW_FAKE=1）
	Images         []string      `yaml:"images"`           // 展示用标签白名单；空 = 仅 Image
	ExecTimeoutMax time.Duration `yaml:"exec_timeout_max"` // API 可自定 timeout 的上限
	MaxPerProject  int           `yaml:"max_per_project"`  // 项目内未删除沙盒上限
	ReapInterval   time.Duration `yaml:"reap_interval"`    // 过期清理周期；0 = 关闭
	// Root 是沙盒工作区的父目录；空 = <storage.data_dir>/sandboxes。
	Root string `yaml:"root"`
	// PidsMax 是 cgroup pids.max；0 = 不限。cgroup 不可用时不会生效。
	PidsMax int64 `yaml:"pids_max"`
	// BwrapPath / HelperPath 见 gosandbox/host.Policy；macOS 忽略。
	BwrapPath  string `yaml:"bwrap_path"`
	HelperPath string `yaml:"helper_path"`

	Terminal SandboxTerminalConfig `yaml:"terminal"`
	SSH      SandboxSSHConfig      `yaml:"ssh"`
	E2B      SandboxE2BConfig      `yaml:"e2b"`
}

// SandboxTerminalConfig 控制控制台的浏览器终端（WebSocket + 进程内 SSH，不占端口）。
type SandboxTerminalConfig struct {
	// Enabled 默认 true；后端不支持 PTY 时自动降级为 capabilities.terminal=false。
	Enabled               bool          `yaml:"enabled"`
	MaxSessionsPerSandbox int           `yaml:"max_sessions_per_sandbox"` // 1-32
	IdleTimeout           time.Duration `yaml:"idle_timeout"`             // >= 1m
}

// EffectiveSessionsPerSandbox 返回每个沙盒的会话数上限，零值取默认 4。
func (t SandboxTerminalConfig) EffectiveSessionsPerSandbox() int {
	if t.MaxSessionsPerSandbox <= 0 {
		return 4
	}
	return t.MaxSessionsPerSandbox
}

// EffectiveIdleTimeout 返回终端空闲断开时间，零值取默认 15 分钟。
func (t SandboxTerminalConfig) EffectiveIdleTimeout() time.Duration {
	if t.IdleTimeout <= 0 {
		return 15 * time.Minute
	}
	return t.IdleTimeout
}

// SandboxSSHConfig 控制是否对外开放真实 SSH 端口。默认不开放：它会在宿主上新开一个能拿到 shell 的端口。
type SandboxSSHConfig struct {
	// Listen 为空 = 不开放。例：127.0.0.1:2222；对外开放必须由操作者显式写 0.0.0.0 或经反向代理。
	Listen string `yaml:"listen"`
	// HostKeyPath 为空 = <storage.data_dir>/sandbox_ssh_host_ed25519，不存在时生成。
	HostKeyPath string `yaml:"host_key_path"`
}

// SandboxE2BConfig 是 E2B 兼容 API 的展示配置。E2B 面随 sandbox.enabled 开关，没有单独的开关。
type SandboxE2BConfig struct {
	// DefaultTemplate 是 templateID="base" 对应的镜像别名；空取 sandbox.image。
	DefaultTemplate string `yaml:"default_template"`
	// EnvdVersion 是响应里报告的 envd 版本，SDK 据此判断能力；空取内置值。
	EnvdVersion string `yaml:"envd_version"`
}

// SandboxBackendLocal / SandboxBackendFake 是 sandbox.backend 的合法取值。
const (
	SandboxBackendLocal = "local"
	SandboxBackendFake  = "fake"
	// SandboxDefaultImage 是 local 后端的镜像标签。
	SandboxDefaultImage = "host"
)

// EffectiveBackend 返回归一后的 backend（空 = local）。
func (s SandboxConfig) EffectiveBackend() string {
	b := strings.ToLower(strings.TrimSpace(s.Backend))
	if b == "" {
		return SandboxBackendLocal
	}
	return b
}

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

type ObservabilityConfig struct {
	LogLevel    string `yaml:"log_level"`
	LogFormat   string `yaml:"log_format"`
	LogOutput   string `yaml:"log_output"`
	MetricsPath string `yaml:"metrics_path"`
	// PerfStageTiming 开启请求分段计时（api-db-perf-validation-plan §2.1）。
	// 输出 simplebase_api_stage_seconds 与 debug 日志行；默认关闭。
	PerfStageTiming bool `yaml:"perf_stage_timing"`
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

	if err := rejectYAMLSecrets(yamlSecrets); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
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
		},
		Sandbox: SandboxConfig{
			Enabled:        false,
			Backend:        SandboxBackendLocal,
			Image:          SandboxDefaultImage,
			CPUs:           1,
			MemoryMiB:      256,
			MaxDuration:    30 * time.Minute,
			IdleTimeout:    5 * time.Minute,
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
			MaxConcurrentGoRuns:  16,
			MaxFilterScanRows:    50000,
			MaxImportBytes:       128 << 20,
			RealtimeMaxConns:     200,
			RateLimit:            RateLimitConfig{Burst: 2, ExemptSSE: true},
		},
		Observability: ObservabilityConfig{
			LogLevel:    "info",
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
	}
}

// loadFromEnv 仅从默认值 + 环境变量构造配置（测试辅助；不读 YAML、不 Validate）。
func loadFromEnv() Config {
	cfg := defaults()
	applyEnv(&cfg)
	return cfg
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
	m := strings.ToLower(strings.TrimSpace(mode))
	if m == "" {
		return StorageModeLocal
	}
	return m
}

// EffectiveDBMode 返回数据库持久层的生效模式（空视为 local）。
func (c Config) EffectiveDBMode() string { return c.effectiveStorage(c.Storage.Database) }

// EffectiveFilesMode 返回用户文件持久层的生效模式（空视为 local）。
func (c Config) EffectiveFilesMode() string { return c.effectiveStorage(c.Storage.Files) }

// Validate 聚合校验字段。
// 只有 storage.database=s3 或 storage.files=s3 时才要求 S3 配置完整。
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
	switch strings.ToLower(c.Instance.Lease.OnLost) {
	case "", "release_db", "exit":
	default:
		errs = append(errs, fmt.Errorf("instance.lease.on_lost must be release_db or exit (got %q)", c.Instance.Lease.OnLost))
	}
	if c.Instance.Lease.TTL < 0 || c.Instance.Lease.Grace < 0 || c.Instance.Lease.RenewInterval < 0 {
		errs = append(errs, errors.New("instance.lease durations must be non-negative"))
	}
	switch strings.ToLower(strings.TrimSpace(c.Database.Engine)) {
	case "", EngineDuckDB, EngineSQLite:
	default:
		errs = append(errs, fmt.Errorf("database.engine must be duckdb or sqlite (got %q)", c.Database.Engine))
	}
	if c.Database.DuckLake.Threads < 0 {
		errs = append(errs, errors.New("database.ducklake.threads must be non-negative"))
	}
	switch strings.ToLower(strings.TrimSpace(c.Database.DuckLake.CatalogEngine)) {
	case "", CatalogEngineDuckDB, CatalogEngineSQLite:
	default:
		errs = append(errs, fmt.Errorf("database.ducklake.catalog_engine must be duckdb or sqlite (got %q)", c.Database.DuckLake.CatalogEngine))
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
	// GF-RUN-04：0 用默认值兜底，负数拒绝。
	if c.Limits.MaxConcurrentGoRuns < 0 {
		errs = append(errs, errors.New("limits.max_concurrent_go_runs must not be negative"))
	} else if c.Limits.MaxConcurrentGoRuns == 0 {
		c.Limits.MaxConcurrentGoRuns = 16
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
	if err := validateSandbox(c.Sandbox); err != nil {
		errs = append(errs, err)
	}
	errs = append(errs, validateLLMAgent(c.LLM)...)
	errs = append(errs, validateAgent(c.Agent)...)
	errs = append(errs, validateHTTPBoundary(c)...)
	if err := validateBackup(c.Backup); err != nil {
		errs = append(errs, err)
	}

	if err := validateStorage(c.Storage, c.Database.CacheDir); err != nil {
		errs = append(errs, err)
	}

	// 只要有任一持久层走对象存储，就必须具备完整的 S3 连接信息。
	if c.IsDBRemote() || c.IsFilesRemote() {
		if c.S3.Bucket == "" {
			errs = append(errs, errors.New("s3.bucket is required when storage.database or storage.files is s3"))
		}
		if c.S3.Region == "" {
			errs = append(errs, errors.New("s3.region is required when storage.database or storage.files is s3"))
		}
		if c.S3.AccessKey != "" && c.S3.SecretKey == "" {
			errs = append(errs, errors.New("s3.secret_key is required when s3.access_key is set"))
		}
		if c.S3.SecretKey != "" && c.S3.AccessKey == "" {
			errs = append(errs, errors.New("s3.access_key is required when s3.secret_key is set"))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed: %w", joinErrors(errs))
	}
	return nil
}

// validateStorage 校验 storage 段：模式取值合法，且两个目录不重叠。
// cache_dir 会被缓存淘汰整体删除，因此绝不能落在 data_dir 之内（反之亦然）。
func validateStorage(s StorageConfig, cacheDir string) error {
	var errs []error
	switch strings.ToLower(strings.TrimSpace(s.Database)) {
	case "", StorageModeLocal, StorageModeS3:
	default:
		errs = append(errs, fmt.Errorf("storage.database must be local or s3 (got %q)", s.Database))
	}
	switch strings.ToLower(strings.TrimSpace(s.Files)) {
	case "", StorageModeLocal, StorageModeS3:
	default:
		errs = append(errs, fmt.Errorf("storage.files must be local or s3 (got %q)", s.Files))
	}
	if strings.TrimSpace(s.DataDir) == "" {
		errs = append(errs, errors.New("storage.data_dir is required"))
	}
	if err := validateDirDisjoint(s.DataDir, cacheDir); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("storage validation failed: %w", joinErrors(errs))
	}
	return nil
}

// validateDirDisjoint 拒绝两个目录相等或互为父子目录。
// 缓存目录在容量不足时会被整体删除（cache.Manager.Evict），落在数据目录里会丢数据。
func validateDirDisjoint(dataDir, cacheDir string) error {
	d := filepath.Clean(strings.TrimSpace(dataDir))
	c := filepath.Clean(strings.TrimSpace(cacheDir))
	if d == "" || c == "" || d == "." || c == "." {
		return nil
	}
	if d == c {
		return fmt.Errorf("storage.data_dir and database.cache_dir must differ (both %q)", d)
	}
	if isWithin(d, c) || isWithin(c, d) {
		return fmt.Errorf("storage.data_dir (%q) and database.cache_dir (%q) must not contain each other", d, c)
	}
	return nil
}

// isWithin 返回 inner 是否位于 outer 之内（含相等）。
// 用 filepath.Rel 判断，避免前缀字符串误判（如 /data2 与 /data）。
func isWithin(inner, outer string) bool {
	rel, err := filepath.Rel(outer, inner)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

// validateSandbox 校验 sandbox 配置段。enabled=false 时忽略。
func validateSandbox(s SandboxConfig) error {
	if !s.Enabled {
		return nil
	}
	var errs []error
	switch s.EffectiveBackend() {
	case SandboxBackendLocal:
		if s.BwrapPath != "" && !filepath.IsAbs(s.BwrapPath) {
			errs = append(errs, errors.New("sandbox.bwrap_path must be an absolute path"))
		}
		if s.HelperPath != "" && !filepath.IsAbs(s.HelperPath) {
			errs = append(errs, errors.New("sandbox.helper_path must be an absolute path"))
		}
		if s.Root != "" && !filepath.IsAbs(s.Root) {
			errs = append(errs, errors.New("sandbox.root must be an absolute path"))
		}
	case SandboxBackendFake:
		if !allowFakeSandbox() {
			errs = append(errs, errors.New("sandbox.backend=fake is only allowed when SIMPLEBASE_SANDBOX_ALLOW_FAKE=1 (e2e / unit tests)"))
		}
	default:
		errs = append(errs, fmt.Errorf("sandbox.backend must be local or fake (got %q)", s.Backend))
	}
	if s.PidsMax < 0 {
		errs = append(errs, errors.New("sandbox.pids_max must not be negative"))
	}
	if len(s.Images) > 0 && strings.TrimSpace(s.Image) != "" {
		found := false
		for _, img := range s.Images {
			if strings.TrimSpace(img) == strings.TrimSpace(s.Image) {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Errorf("sandbox.images must contain sandbox.image (%q)", s.Image))
		}
	}
	if s.ExecTimeoutMax != 0 && s.ExecTimeoutMax < s.ExecTimeout {
		errs = append(errs, errors.New("sandbox.exec_timeout must not exceed sandbox.exec_timeout_max"))
	}
	if s.MaxPerProject < 0 || s.MaxPerProject > 100 {
		errs = append(errs, fmt.Errorf("sandbox.max_per_project must be 1-100 (got %d)", s.MaxPerProject))
	}
	if s.ReapInterval < 0 {
		errs = append(errs, errors.New("sandbox.reap_interval must not be negative"))
	}
	switch strings.ToLower(strings.TrimSpace(s.Network)) {
	case "", "none":
	default:
		errs = append(errs, fmt.Errorf("sandbox.network must be none (got %q)", s.Network))
	}
	if s.CPUs < 1 || s.CPUs > 4 {
		errs = append(errs, fmt.Errorf("sandbox.cpus must be 1-4 (got %d)", s.CPUs))
	}
	if s.MemoryMiB < 128 || s.MemoryMiB > 4096 {
		errs = append(errs, fmt.Errorf("sandbox.memory_mib must be 128-4096 (got %d)", s.MemoryMiB))
	}
	if s.ExecTimeout < time.Second {
		errs = append(errs, errors.New("sandbox.exec_timeout must be at least 1s"))
	}
	if s.MaxDuration <= 0 || s.IdleTimeout <= 0 {
		errs = append(errs, errors.New("sandbox.max_duration and sandbox.idle_timeout must be positive"))
	}
	if s.MaxOutputBytes <= 0 || s.MaxFileBytes <= 0 {
		errs = append(errs, errors.New("sandbox.max_output_bytes and sandbox.max_file_bytes must be positive"))
	}
	// 终端子段零值 = 取默认（EffectiveTerminal），所以只校验显式设置过的值。
	if n := s.Terminal.MaxSessionsPerSandbox; n != 0 && (n < 1 || n > 32) {
		errs = append(errs, fmt.Errorf("sandbox.terminal.max_sessions_per_sandbox must be 1-32 (got %d)", n))
	}
	if d := s.Terminal.IdleTimeout; d != 0 && d < time.Minute {
		errs = append(errs, errors.New("sandbox.terminal.idle_timeout must be at least 1m"))
	}
	if l := strings.TrimSpace(s.SSH.Listen); l != "" {
		if _, port, err := net.SplitHostPort(l); err != nil || port == "" {
			errs = append(errs, fmt.Errorf("sandbox.ssh.listen must be host:port (got %q)", s.SSH.Listen))
		}
	}
	if s.SSH.HostKeyPath != "" && !filepath.IsAbs(s.SSH.HostKeyPath) {
		errs = append(errs, errors.New("sandbox.ssh.host_key_path must be an absolute path"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("sandbox validation failed: %w", joinErrors(errs))
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
		"observability": map[string]any{
			"log_level":    c.Observability.LogLevel,
			"log_format":   c.Observability.LogFormat,
			"log_output":   c.Observability.LogOutput,
			"metrics_path": c.Observability.MetricsPath,
		},
	}
}

// allowFakeSandbox 返回是否显式允许 fake 沙盒后端（e2e / 单测）。
func allowFakeSandbox() bool {
	return os.Getenv("SIMPLEBASE_SANDBOX_ALLOW_FAKE") == "1"
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
