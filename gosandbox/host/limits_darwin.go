//go:build darwin

// 文件说明：limits_darwin.go — macOS 上的资源限额：父进程采样监控沙盒进程树
//
// 用法
//
//	startLimitMonitor(pid int, p Policy) *limitMonitor  // 配置了限额时启动监控；未配置返回 nil
//	(m *limitMonitor) stop() string                     // 停止监控，返回触发的限额名（"memory" / "pids"）或空串
//
// macOS 没有 cgroup，也不提供按进程树生效的 rlimit（RLIMIT_AS 不生效，RLIMIT_NPROC 按用户计）。
// 这里按固定间隔用 proc_info 枚举「同进程组 ∪ 子孙进程」，并据此执行：
//
//   - MemoryMaxBytes：常驻内存（RSS）之和超限 → SIGKILL 整棵树；
//   - PidsMax：进程数超限 → SIGKILL 整棵树（Linux 是让 fork 失败，macOS 做不到，只能终止）；
//   - CPUMax：按令牌桶核算 CPU 时间，超额时 SIGSTOP 整棵树，额度恢复后 SIGCONT。
//
// 已知精度限制：采样间隔内的瞬时峰值可能漏检；被 launchd 收养且换了进程组的孙进程不在统计内；
// 已退出进程的 CPU 时间不再计入，所以「每次 fork 一个很快退出的子进程」的负载基本不受 CPU 配额约束。

package host

import (
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// limitsMonitor 是 Limits.Backend 与 Caps.Cgroup 在 macOS 上的取值。
const limitsMonitor = "monitor"

// monitorInterval 是采样间隔，测试可替换。
var monitorInterval = 100 * time.Millisecond

// proc_info 调用号与类型（<sys/proc_info.h>）。
const (
	procInfoCallListPids = 1
	procInfoCallPidInfo  = 2
	procPgrpOnly         = 2
	procPPIDOnly         = 6
	procPidTaskInfo      = 4
)

// procTaskInfo 对应 struct proc_taskinfo（96 字节）。
type procTaskInfo struct {
	VirtualSize      uint64
	ResidentSize     uint64
	TotalUser        uint64
	TotalSystem      uint64
	ThreadsUser      uint64
	ThreadsSystem    uint64
	Policy           int32
	Faults           int32
	Pageins          int32
	CowFaults        int32
	MessagesSent     int32
	MessagesReceived int32
	SyscallsMach     int32
	SyscallsUnix     int32
	Csw              int32
	ThreadNum        int32
	NumRunning       int32
	Priority         int32
}

// listPids 返回指定类型的 pid 列表（typ = procPgrpOnly / procPPIDOnly）。
func listPids(typ int, arg int) []int {
	buf := make([]int32, 256)
	for {
		size := uintptr(len(buf) * 4)
		r, _, errno := unix.Syscall6(unix.SYS_PROC_INFO, procInfoCallListPids, uintptr(typ), uintptr(arg), 0,
			uintptr(unsafe.Pointer(&buf[0])), size)
		if errno != 0 {
			return nil
		}
		n := int(r) / 4
		if n < len(buf) {
			out := make([]int, 0, n)
			for _, pid := range buf[:n] {
				if pid > 0 {
					out = append(out, int(pid))
				}
			}
			return out
		}
		if len(buf) >= 1<<16 {
			return nil
		}
		buf = make([]int32, len(buf)*2)
	}
}

// taskInfo 读取单个进程的内存与 CPU 统计；进程已退出或不可访问时 ok=false。
func taskInfo(pid int) (ti procTaskInfo, ok bool) {
	size := unsafe.Sizeof(ti)
	r, _, errno := unix.Syscall6(unix.SYS_PROC_INFO, procInfoCallPidInfo, uintptr(pid), procPidTaskInfo, 0,
		uintptr(unsafe.Pointer(&ti)), size)
	return ti, errno == 0 && r == size
}

// machTicksPerSecond 是 pti_total_user/system 的计时频率：Apple Silicon 为 24 MHz，Intel 为 1 GHz（即纳秒）。
var machTicksPerSecond = func() uint64 {
	if f, err := unix.SysctlUint64("hw.tbfrequency"); err == nil && f > 0 {
		return f
	}
	return 1_000_000_000
}()

func ticksToDuration(t uint64) time.Duration {
	return time.Duration(float64(t) * 1e9 / float64(machTicksPerSecond))
}

// cpuShare 把 cgroup 的 cpu.max 原文解析为可用核数；"max" 或未配置返回 0（不限）。
func cpuShare(cpuMax string) float64 {
	f := strings.Fields(cpuMax)
	if len(f) == 0 || f[0] == "max" {
		return 0
	}
	quota, err := strconv.ParseFloat(f[0], 64)
	if err != nil || quota <= 0 {
		return 0
	}
	period := 100000.0
	if len(f) == 2 {
		if v, err := strconv.ParseFloat(f[1], 64); err == nil && v > 0 {
			period = v
		}
	}
	return quota / period
}

// limitMonitor 监控以 root 为首的进程树。
type limitMonitor struct {
	root     int
	memMax   uint64
	pidsMax  int
	cpus     float64
	stopCh   chan struct{}
	done     chan struct{}
	mu       sync.Mutex
	exceeded string
	// 以下仅在监控协程内访问。
	lastCPU map[int]uint64
	budget  time.Duration
	paused  []int
}

// startLimitMonitor 在配置了限额时启动监控；未配置返回 nil（调用方可直接对 nil 调用 stop）。
func startLimitMonitor(root int, p Policy) *limitMonitor {
	if !limitsConfigured(p) {
		return nil
	}
	m := &limitMonitor{root: root, memMax: uint64(p.MemoryMaxBytes), pidsMax: int(p.PidsMax), cpus: cpuShare(p.CPUMax),
		stopCh: make(chan struct{}), done: make(chan struct{}), lastCPU: map[int]uint64{}}
	m.budget = m.burst()
	go m.loop()
	return m
}

// burst 是令牌桶容量：允许 100ms 内突发使用满额核数。
func (m *limitMonitor) burst() time.Duration {
	return time.Duration(m.cpus * float64(100*time.Millisecond))
}

// stop 停止监控并恢复被暂停的进程，返回触发的限额名或空串。可对 nil 调用、可重复调用。
func (m *limitMonitor) stop() string {
	if m == nil {
		return ""
	}
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
	<-m.done
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.exceeded
}

func (m *limitMonitor) loop() {
	defer close(m.done)
	defer m.resume()
	t := time.NewTicker(monitorInterval)
	defer t.Stop()
	last := time.Now()
	m.sample(0)
	for {
		select {
		case <-m.stopCh:
			return
		case now := <-t.C:
			elapsed := now.Sub(last)
			last = now
			if m.sample(elapsed) {
				return
			}
		}
	}
}

// tree 返回 root 的进程组成员与所有子孙进程（去重，含 root）。
func (m *limitMonitor) tree() []int {
	seen := map[int]bool{}
	var out []int
	queue := []int{m.root}
	queue = append(queue, listPids(procPgrpOnly, m.root)...)
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		out = append(out, pid)
		queue = append(queue, listPids(procPPIDOnly, pid)...)
	}
	return out
}

// sample 做一次采样；返回 true 表示已因超限杀掉进程树，监控结束。
func (m *limitMonitor) sample(elapsed time.Duration) bool {
	pids := m.tree()
	var alive []int
	var rss uint64
	var cpuDelta time.Duration
	cur := make(map[int]uint64, len(pids))
	for _, pid := range pids {
		ti, ok := taskInfo(pid)
		if !ok {
			continue
		}
		alive = append(alive, pid)
		rss += ti.ResidentSize
		total := ti.TotalUser + ti.TotalSystem
		cur[pid] = total
		if prev, seen := m.lastCPU[pid]; seen && total >= prev {
			cpuDelta += ticksToDuration(total - prev)
		} else if !seen {
			cpuDelta += ticksToDuration(total)
		}
	}
	m.lastCPU = cur
	if len(alive) == 0 {
		return false
	}
	if m.pidsMax > 0 && len(alive) > m.pidsMax {
		m.killAll("pids", alive)
		return true
	}
	if m.memMax > 0 && rss > m.memMax {
		m.killAll("memory", alive)
		return true
	}
	if m.cpus > 0 && elapsed > 0 {
		m.budget += time.Duration(m.cpus*float64(elapsed)) - cpuDelta
		if b := m.burst(); m.budget > b {
			m.budget = b
		}
		switch {
		case m.budget < 0 && m.paused == nil:
			m.paused = alive
			m.signal(alive, syscall.SIGSTOP)
		case m.budget >= 0 && m.paused != nil:
			m.resume()
		}
	}
	return false
}

func (m *limitMonitor) signal(pids []int, sig syscall.Signal) {
	_ = syscall.Kill(-m.root, sig)
	for _, pid := range pids {
		_ = syscall.Kill(pid, sig)
	}
}

func (m *limitMonitor) resume() {
	if m.paused != nil {
		m.signal(m.paused, syscall.SIGCONT)
		m.paused = nil
	}
}

func (m *limitMonitor) killAll(reason string, pids []int) {
	m.mu.Lock()
	m.exceeded = reason
	m.mu.Unlock()
	m.signal(pids, syscall.SIGKILL)
	m.paused = nil
}
