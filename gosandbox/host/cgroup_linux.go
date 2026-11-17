//go:build linux

// 文件说明：cgroup_linux.go — gosandbox/host 的 cgroup v2 子组：限额、冻结、统计
//
// 用法
//
//	type cgroupGroup（包内使用）
//	同包调用：cgroupGroup → run_linux.go, start_linux.go；apply() → run_linux.go, start_linux.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/cgroup_linux.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 14 个
//	[关键引用] cgroupGroup ← run_linux.go, start_linux.go
//
// 主要符号
//
//	var cgroupRoot
//	type cgroupGroup struct {…}
//	currentCgroupDir() (string, error)
//	delegatedBase() (string, error)
//	newNamedCgroupGroup(name string) (*cgroupGroup, error)
//	(g *cgroupGroup) apply(p Policy) error
//	... 另有 8 个顶层符号

package host

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cgroupRoot 可在测试里替换。
var cgroupRoot = "/sys/fs/cgroup"

// cgroupGroup 是父进程为一次命令创建的 cgroup v2 子组。
type cgroupGroup struct {
	dir string
	// keep 为真表示这是沙盒的固定子组，命令结束后保留（供 Metrics / Freeze 使用），
	// 由调用方在沙盒删除时 RemoveCgroup。
	keep bool
}

// currentCgroupDir 读 /proc/self/cgroup 的 "0::" 行，得到本进程所在 cgroup 目录。
func currentCgroupDir() (string, error) {
	b, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(line, "0::"); ok {
			return filepath.Join(cgroupRoot, strings.TrimSpace(rest)), nil
		}
	}
	return "", errors.New("cgroup v2 not in use")
}

// delegatedBase 返回 memory/cpu/pids 已经委派给子树的 cgroup 目录；不做任何写入。
// 有进程的 cgroup 不能再开启控制器（no internal process constraint），
// 因此只有 subtree_control 已包含这三个控制器时才可用，且不去改它。
func delegatedBase() (string, error) {
	base, err := currentCgroupDir()
	if err != nil {
		return "", err
	}
	ctl, err := os.ReadFile(filepath.Join(base, "cgroup.subtree_control"))
	if err != nil {
		return "", err
	}
	have := map[string]bool{}
	for _, c := range strings.Fields(string(ctl)) {
		have[c] = true
	}
	for _, need := range []string{"memory", "cpu", "pids"} {
		if !have[need] {
			return "", fmt.Errorf("controller %s is not delegated", need)
		}
	}
	return base, nil
}

// newNamedCgroupGroup 建一个固定名字的子组（gosandbox-<name>），供按名字查询用量与冻结。
// name 为空时退化为随机名（一次性命令用，用完即删）。
//
// 同名子组已存在时先尝试删除：上一次运行可能异常退出留下了空目录。
// 目录里仍有进程时删除失败，此时返回错误而不是复用——复用会让新命令继承
// 上一次的限额与统计。
func newNamedCgroupGroup(name string) (*cgroupGroup, error) {
	base, err := delegatedBase()
	if err != nil {
		return nil, err
	}
	if name == "" {
		dir, err := os.MkdirTemp(base, CgroupNamePrefix)
		if err != nil {
			return nil, err
		}
		return &cgroupGroup{dir: dir}, nil
	}
	dir := filepath.Join(base, CgroupNamePrefix+name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if rerr := os.Remove(dir); rerr != nil {
			return nil, fmt.Errorf("cgroup %s is still in use: %w", dir, rerr)
		}
		if err := os.Mkdir(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return &cgroupGroup{dir: dir, keep: true}, nil
}

// apply 写入限额。cpu.max 原样写入；非法内容让写入失败，不会被悄悄忽略。
//
// 除硬限额外还写三项，都是为了让「超限」表现为可预期的整组终止而不是半死：
//   - memory.high：软限，先触发回收，避免直接 OOM；
//   - memory.swap.max=0：沙盒不换出，否则内存限额形同虚设；
//   - memory.oom.group=1：OOM 时杀整个 cgroup，不留孤儿进程。
func (g *cgroupGroup) apply(p Policy) error {
	write := func(name, val string) error {
		return os.WriteFile(filepath.Join(g.dir, name), []byte(val), 0o644)
	}
	if p.MemoryMaxBytes > 0 {
		if err := write("memory.max", strconv.FormatInt(p.MemoryMaxBytes, 10)); err != nil {
			return err
		}
		// 软限取硬限的 90%：留出回收窗口，同时不改变「超限即失败」的语义。
		// 用 *9/10 而不是 /10*9：后者会先把 10% 截断再乘，结果偏小 1 字节。
		if err := write("memory.high", strconv.FormatInt(p.MemoryMaxBytes*9/10, 10)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := write("memory.swap.max", "0"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := write("memory.oom.group", "1"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if p.CPUMax != "" {
		if err := write("cpu.max", p.CPUMax); err != nil {
			return err
		}
	}
	if p.PidsMax > 0 {
		if err := write("pids.max", strconv.FormatInt(p.PidsMax, 10)); err != nil {
			return err
		}
	}
	return nil
}

// addPID 把进程加入子组。仅在无法用 CLONE_INTO_CGROUP 时作为回退路径使用。
func (g *cgroupGroup) addPID(pid int) error {
	return os.WriteFile(filepath.Join(g.dir, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0o644)
}

// open 返回子组目录的只读句柄，供 SysProcAttr.CgroupFD 使用（CLONE_INTO_CGROUP）。
// 调用方负责关闭。
func (g *cgroupGroup) open() (*os.File, error) {
	return os.Open(g.dir)
}

// freeze 冻结或解冻子组内的全部进程（cgroup.freeze）。
func (g *cgroupGroup) freeze(frozen bool) error {
	v := "0"
	if frozen {
		v = "1"
	}
	return os.WriteFile(filepath.Join(g.dir, "cgroup.freeze"), []byte(v), 0o644)
}

// remove 删除子组；组内进程已死时 rmdir 才会成功，失败只是留下空目录。
// 固定子组（keep）不删：它要留给 Metrics / Freeze 用，由调用方显式 RemoveCgroup。
func (g *cgroupGroup) remove() {
	if g.keep {
		return
	}
	_ = os.Remove(g.dir)
}

// readStat 读一个 cgroup 文件里的单个整数；不存在或不可读时返回 0。
func readStat(dir, name string) int64 {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return 0
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// readKeyed 读 "key value" 逐行的 cgroup 文件（memory.stat、cpu.stat、
// memory.events 等），返回 key → 值。同一 key 出现多次时累加。
//
// io.stat 不是这个排版（字段是 "rbytes=4096"），用 readIOStat。
func readKeyed(dir, name string) map[string]int64 {
	out := map[string]int64{}
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			continue
		}
		out[f[0]] += v
	}
	return out
}

// readIOStat 读 io.stat 的读/写字节数。
//
// io.stat 的排版是「每设备一行，字段用 = 连接」：
//
//	8:0 rbytes=4096 wbytes=8192 rios=1 wios=2
//
// 与 cpu.stat 的 "key value" 不同，readKeyed 那种按空白切列取 f[1] 的
// 解析在这里会全部解析失败（值为 "rbytes=4096"），于是永远报 0。
func readIOStat(dir string) (readBytes, writeBytes int64) {
	b, err := os.ReadFile(filepath.Join(dir, "io.stat"))
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		for _, field := range strings.Fields(line) {
			k, v, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			n, perr := strconv.ParseInt(v, 10, 64)
			if perr != nil {
				continue
			}
			switch k {
			case "rbytes":
				readBytes += n
			case "wbytes":
				writeBytes += n
			}
		}
	}
	return readBytes, writeBytes
}

// cgroupUsage 是子组当前的资源用量，字段与 vm.Metrics 一一对应。
type cgroupUsage struct {
	MemoryBytes  int64
	MemoryPeak   int64
	CPUUsec      int64
	Pids         int64
	OOMKills     int64
	DiskReadByte int64
	DiskWriteByt int64
}

// usage 读子组的用量。任何文件缺失都按 0 处理：不同内核版本暴露的文件不同，
// 缺一项不该让整个 metrics 失败。
func (g *cgroupGroup) usage() cgroupUsage {
	u := cgroupUsage{
		MemoryBytes: readStat(g.dir, "memory.current"),
		MemoryPeak:  readStat(g.dir, "memory.peak"),
		Pids:        readStat(g.dir, "pids.current"),
	}
	cpu := readKeyed(g.dir, "cpu.stat")
	u.CPUUsec = cpu["usage_usec"]
	u.DiskReadByte, u.DiskWriteByt = readIOStat(g.dir)
	events := readKeyed(g.dir, "memory.events")
	u.OOMKills = events["oom_kill"]
	return u
}

// cgroupPathFor 返回某个已存在子组的目录，供按名字查询用量（vm/local 的 Metrics）。
// 名字不是 gosandbox- 前缀时返回空串，避免读到别人的 cgroup。
func cgroupPathFor(name string) string {
	if !strings.HasPrefix(name, "gosandbox-") {
		return ""
	}
	base, err := delegatedBase()
	if err != nil {
		return ""
	}
	dir := filepath.Join(base, name)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return ""
	}
	return dir
}
