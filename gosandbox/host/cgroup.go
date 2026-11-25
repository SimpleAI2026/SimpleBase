// 文件说明：cgroup.go — gosandbox/host 中 Limits 等符号的实现
//
// 用法
//
//	type Limits struct{…}  // Limits 报告配置的资源限额是否真的生效
//	同包调用：limitsConfigured() → limits_darwin.go, run_darwin.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/box, gosandbox/vm/box 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/cgroup.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, cgroup_linux.go, cgroup_linux_test.go, glob.go 等 36 个
//	[关键引用] Limits ← run.go, run_darwin.go；limitsConfigured ← limits_darwin.go, run_darwin.go
//
// 主要符号
//
//	const cgroupDelegated / cgroupSystemd / cgroupNone
//	type Limits struct{…}
//	limitsConfigured(p Policy) bool

package host

// cgroup 限额的可用方式。
const (
	cgroupDelegated = "delegated"
	cgroupSystemd   = "systemd"
	cgroupNone      = "none"
)

// Limits 报告配置的资源限额是否真的生效。
type Limits struct {
	// Configured 为真表示策略里设置了至少一项限额。
	Configured bool
	// Enforced 为真表示这些限额已经由 cgroup、systemd 或 macOS 的采样监控生效。
	// Configured && !Enforced 时调用方必须向模型/日志回显「未限制」。
	Enforced bool
	// Backend 是 "cgroup" | "systemd" | "monitor"（macOS）| "none"。
	Backend string
}

func limitsConfigured(p Policy) bool {
	return p.MemoryMaxBytes > 0 || p.CPUMax != "" || p.PidsMax > 0
}
