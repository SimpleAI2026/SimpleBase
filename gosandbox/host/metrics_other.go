//go:build !linux

// 文件说明：metrics_other.go — gosandbox/host 中 MetricsFor、FreezeCgroup 等符号的实现
//
// 用法
//
//	MetricsFor(string) (Metrics, bool)  // MetricsFor 在非 Linux 平台没有 cgroup，恒返回 false
//	FreezeCgroup(string, bool) error  // FreezeCgroup 在非 Linux 平台不可用：macOS 的采样监控没有冻结能力
//	RemoveCgroup(string) error  // RemoveCgroup 在非 Linux 平台无事可做
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/box, gosandbox/vm/box 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/metrics_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, cgroup.go, cgroup_linux.go, cgroup_linux_test.go 等 36 个
//
// 主要符号
//
//	MetricsFor(string) (Metrics, bool)
//	FreezeCgroup(string, bool) error
//	RemoveCgroup(string) error

package host

// MetricsFor 在非 Linux 平台没有 cgroup，恒返回 false。
func MetricsFor(string) (Metrics, bool) { return Metrics{}, false }

// FreezeCgroup 在非 Linux 平台不可用：macOS 的采样监控没有冻结能力。
func FreezeCgroup(string, bool) error {
	return unavailable("cgroup freeze is only available on Linux")
}

// RemoveCgroup 在非 Linux 平台无事可做。
func RemoveCgroup(string) error { return nil }
