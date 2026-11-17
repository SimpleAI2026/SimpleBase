//go:build !linux

// 文件说明：metrics_other.go — MetricsFor / FreezeCgroup / RemoveCgroup 的非 Linux 实现
//
// 用法
//
//	MetricsFor(name string) (Metrics, bool)  // 非 Linux 恒返回 false
//	FreezeCgroup(name string, frozen bool) error  // 非 Linux 恒返回 ErrIsolationUnavailable
//	RemoveCgroup(name string) error  // 非 Linux 恒返回 nil
//
// 调用架构
//
//	[上游] gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/metrics_other.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] metrics.go
//
// 主要符号
//
//	MetricsFor(name string) (Metrics, bool)
//	FreezeCgroup(name string, frozen bool) error
//	RemoveCgroup(name string) error

package host

// MetricsFor 在非 Linux 平台没有 cgroup，恒返回 false。
func MetricsFor(string) (Metrics, bool) { return Metrics{}, false }

// FreezeCgroup 在非 Linux 平台不可用：macOS 的采样监控没有冻结能力。
func FreezeCgroup(string, bool) error {
	return unavailable("cgroup freeze is only available on Linux")
}

// RemoveCgroup 在非 Linux 平台无事可做。
func RemoveCgroup(string) error { return nil }
