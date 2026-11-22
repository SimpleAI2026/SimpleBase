//go:build !linux

package host

// MetricsFor 在非 Linux 平台没有 cgroup，恒返回 false。
func MetricsFor(string) (Metrics, bool) { return Metrics{}, false }

// FreezeCgroup 在非 Linux 平台不可用：macOS 的采样监控没有冻结能力。
func FreezeCgroup(string, bool) error {
	return unavailable("cgroup freeze is only available on Linux")
}

// RemoveCgroup 在非 Linux 平台无事可做。
func RemoveCgroup(string) error { return nil }
