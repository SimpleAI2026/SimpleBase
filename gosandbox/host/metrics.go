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
