// 文件说明：cgroup.go — gosandbox/host 中 Limits 等符号的实现
//
// 用法
//
//	type Limits（导出类型，同包与上层均可使用）
//	同包调用：Limits → policy.go, run.go 等；cgroupNone → probe_linux.go, probe_other.go 等；cgroupDelegated → probe_linux.go, run_linux.go 等
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/cgroup.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, glob.go, mandatory.go, policy.go, probe.go 等 13 个
//	[关键引用] Limits ← policy.go, run.go, run_darwin.go
//
// 主要符号
//
//	type Limits struct {…}
//	... 另有 4 个顶层符号

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
	// Enforced 为真表示这些限额已经由 cgroup 或 systemd 生效。
	// Configured && !Enforced 时调用方必须向模型/日志回显「未限制」。
	Enforced bool
	// Backend 是 "cgroup" | "systemd" | "none"。
	Backend string
}

func limitsConfigured(p Policy) bool {
	return p.MemoryMaxBytes > 0 || p.CPUMax != "" || p.PidsMax > 0
}
