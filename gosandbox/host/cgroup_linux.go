//go:build linux

// 文件说明：cgroup_linux.go — gosandbox/host 的内部实现（cgroupRoot、cgroupGroup 等）
//
// 用法
//
//	type cgroupGroup（包内使用）
//	同包调用：cgroupGroup → run_linux.go, start_linux.go；newCgroupGroup() → run_linux.go, start_linux.go；apply() → run_linux.go, start_linux.go
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
//	newCgroupGroup() (*cgroupGroup, error)
//	(g *cgroupGroup) apply(p Policy) error
//	... 另有 2 个顶层符号

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
type cgroupGroup struct{ dir string }

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

// newCgroupGroup 在已委派的 cgroup 下建一个子组。
func newCgroupGroup() (*cgroupGroup, error) {
	base, err := delegatedBase()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(base, "gosandbox-")
	if err != nil {
		return nil, err
	}
	return &cgroupGroup{dir: dir}, nil
}

// apply 写入限额。cpu.max 原样写入；非法内容让写入失败，不会被悄悄忽略。
func (g *cgroupGroup) apply(p Policy) error {
	write := func(name, val string) error {
		return os.WriteFile(filepath.Join(g.dir, name), []byte(val), 0o644)
	}
	if p.MemoryMaxBytes > 0 {
		if err := write("memory.max", strconv.FormatInt(p.MemoryMaxBytes, 10)); err != nil {
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

func (g *cgroupGroup) addPID(pid int) error {
	return os.WriteFile(filepath.Join(g.dir, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0o644)
}

// remove 删除子组；组内进程已死时 rmdir 才会成功，失败只是留下空目录。
func (g *cgroupGroup) remove() { _ = os.Remove(g.dir) }
