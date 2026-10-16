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
