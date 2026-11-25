// 文件说明：config_agent_types.go — LLM / Agent / MCP / Shell 隔离 / 沙盒相关配置类型与 Effective* 取值函数
//
// 用法
//
//	LLMConfig、EffectiveAgentToolProtocol、EffectiveAgentMaxIterations（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api, internal/api/perfbench 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/config/config_agent_types.go
//	         │
//	         ▼
//	[下游] internal/utils/strutil
//	[同包] backup.go, env.go, http_boundary.go, yaml.go
//
// 主要符号
//
//	type LLMConfig struct {…}
//	func (l LLMConfig) EffectiveAgentToolProtocol() string
//	func (l LLMConfig) EffectiveAgentMaxIterations() int
//	func (l LLMConfig) EffectiveAgentRunTimeout() time.Duration
//	type AgentConfig struct {…}
//	type MCPConfig struct {…}
//	func (m MCPConfig) EffectiveMaxServersPerProject() int
//	type ShellIsolationConfig struct {…}
//	... 另有 11 个顶层符号

package config

import (
	"time"

	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

type LLMConfig struct {
	Enabled   bool                      `yaml:"enabled"`
	Providers map[string]ProviderConfig `yaml:"providers"`
	// 云 Agent 运行参数（planv4.0 cloud-agent-optimization-plan §6）。零值表示默认。
	AgentToolProtocol  string        `yaml:"agent_tool_protocol"`  // auto | native | text
	AgentMaxIterations int           `yaml:"agent_max_iterations"` // 1–20
	AgentRunTimeout    time.Duration `yaml:"agent_run_timeout"`    // 10s–15m
}

// EffectiveAgentToolProtocol 返回归一后的工具协议，空值视为 auto。
func (l LLMConfig) EffectiveAgentToolProtocol() string {
	switch p := strutil.Normalize(l.AgentToolProtocol); p {
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
	// MCP 是外部 MCP Server 接入开关（planv6.2 §6-4）。默认关闭：
	// 关闭时 Agent 保存 mcp: 引用会被拒绝，run 也不会装配 MCP 工具。
	MCP MCPConfig `yaml:"mcp"`
}

// MCPConfig 是 agent.mcp 段。
type MCPConfig struct {
	// Enabled 是总开关。默认 false。
	Enabled bool `yaml:"enabled"`
	// Builtin 是项目 API 封装的内置 MCP Server 开关（planv6.7 §3）。默认 true。
	Builtin bool `yaml:"builtin"`
	// AllowHTTP 允许 http:// 端点（默认只允许 https）。
	AllowHTTP bool `yaml:"allow_http"`
	// MaxServersPerProject 是单项目可配置的 Server 上限。
	MaxServersPerProject int `yaml:"max_servers_per_project"`
}

// EffectiveMaxServersPerProject 返回生效上限；未配置取 10。
func (m MCPConfig) EffectiveMaxServersPerProject() int {
	if m.MaxServersPerProject <= 0 {
		return 10
	}
	return m.MaxServersPerProject
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
	// Image 在 local 后端下只是展示用的标签（不拉取任何镜像），默认 "host"；
	// 在 box 后端下是沙盒根来源：目前仍只支持 "host"（宿主根只读），
	// 镜像根随 M2（gosandbox/image）落地。
	Image       string        `yaml:"image"`
	CPUs        int           `yaml:"cpus"`       // 1–4；cgroup 不可用时不会生效
	MemoryMiB   int           `yaml:"memory_mib"` // 128–4096；cgroup 不可用时不会生效
	MaxDuration time.Duration `yaml:"max_duration"`
	IdleTimeout time.Duration `yaml:"idle_timeout"`
	// FreezeAfter 是空闲多久后把沙盒冻结（cgroup.freeze）：进程与内存保留，
	// 恢复是毫秒级。0 = 不冻结。只在驱动支持 vm.Pauser 时生效。
	FreezeAfter    time.Duration `yaml:"freeze_after"`
	ExecTimeout    time.Duration `yaml:"exec_timeout"`
	MaxOutputBytes int           `yaml:"max_output_bytes"`
	MaxFileBytes   int           `yaml:"max_file_bytes"`
	// Network 是出网档位：none（默认）/ egress。
	// none 时沙盒 netns 里只有 lo，入站仍可经 agent 中继访问。
	//
	// egress 档同样 unshare netns：pasta 在沙盒自己的 netns 里配网并做用户态转发，
	// 元数据（169.254/16）、私网（10/8、172.16/12、192.168/16）、CGNAT（100.64/10）
	// 与回环（127/8）由沙盒内的黑洞路由拦住，不做域名级白名单。
	// pasta 不可用或进不去 netns 时该档没有出网（fail-closed），不会退回共享宿主网络。
	// 曾有的 egress-allowlist 从未接入生产路径，已于 planv6.6 GSB-10 移除；配置里残留的
	// network_allow / allow_experimental_network 会被忽略。
	Network string `yaml:"network"`
	// Supervise 为真时把沙盒交给独立的 supervisor 进程托管：
	// 服务进程重启（升级、崩溃）后能重新接管，沙盒与其进程不丢。
	//
	// 代价：断开的那一刻，agent 正在发但还没被读走的帧会丢，调用方需要能重试。
	Supervise bool `yaml:"supervise"`
	// DiskLimitMiB 是每个沙盒可写部分（upper + ws）的上限；0 = 不限。
	// rootless 下没有文件系统配额，超限时冻结沙盒而不是杀掉。
	DiskLimitMiB int64  `yaml:"disk_limit_mib"`
	Workdir      string `yaml:"workdir"`

	Backend        string        `yaml:"backend"`          // local | box | fake（fake 仅测试环境，须 SIMPLEBASE_SANDBOX_ALLOW_FAKE=1）
	Images         []string      `yaml:"images"`           // 展示用标签白名单；空 = 仅 Image
	ExecTimeoutMax time.Duration `yaml:"exec_timeout_max"` // API 可自定 timeout 的上限
	MaxPerProject  int           `yaml:"max_per_project"`  // 项目内未删除沙盒上限
	ReapInterval   time.Duration `yaml:"reap_interval"`    // 过期清理周期；0 = 关闭
	// Root 是沙盒工作区的父目录；空 = <storage.data_dir>/sandboxes。
	Root string `yaml:"root"`
	// ImagesDir 是镜像 store 的目录；空表示不分发镜像，只支持 image=host。
	// 配置后按引用从该 store 取镜像并解包成沙盒根（需先拉取或导入）。
	ImagesDir string `yaml:"images_dir"`
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

// EffectiveBackend 返回归一后的 backend（空 = local）。
func (s SandboxConfig) EffectiveBackend() string {
	b := strutil.Normalize(s.Backend)
	if b == "" {
		return SandboxBackendLocal
	}
	return b
}
