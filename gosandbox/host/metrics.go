// 文件说明：metrics.go — gosandbox/host 中 Metrics 等符号的实现
//
// 用法
//
//	type Metrics struct{…}  // Metrics 是一个沙盒当前的资源用量快照
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/box, gosandbox/vm/box 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/metrics.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, cgroup.go, cgroup_linux.go, cgroup_linux_test.go 等 36 个
//	[关键引用] Metrics ← metrics_linux.go, metrics_other.go
//
// 主要符号
//
//	type Metrics struct{…}
//	const CgroupNamePrefix

package host

// Metrics 是一个沙盒当前的资源用量快照。
//
// 只有 Linux 且 cgroup v2 已委派时才有值；其他平台 MetricsFor 返回 false，
// 调用方据此在 capabilities 里标注「无指标」而不是报 0。
type Metrics struct {
	// MemoryBytes 是当前占用（memory.current）。
	MemoryBytes int64
	// MemoryPeakBytes 是历史峰值（memory.peak，内核 6.1+；缺失为 0）。
	MemoryPeakBytes int64
	// CPUUsec 是累计 CPU 时间（cpu.stat 的 usage_usec）。
	CPUUsec int64
	// Pids 是当前进程数（pids.current）。
	Pids int64
	// OOMKills 是累计被 OOM 杀掉的进程数（memory.events 的 oom_kill）。
	OOMKills int64
	// DiskReadBytes / DiskWriteBytes 是累计块设备读写字节（io.stat）。
	DiskReadBytes  int64
	DiskWriteBytes int64
	// Backend 是数据来源："cgroup" | "none"。
	Backend string
}

// CgroupNamePrefix 是固定子组名的前缀；MetricsFor / FreezeCgroup 只接受带该前缀的名字，
// 避免误操作别人的 cgroup。
const CgroupNamePrefix = "gosandbox-"
