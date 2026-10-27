//go:build linux

// 文件说明：probe_linux.go — gosandbox/host 的内部实现（landlockABI、seccompSupported 等）
//
// 用法
//
//	同包调用：probeDarwin() → probe.go, run_darwin.go 等；landlockABI() → probe.go, probe_other.go；seccompSupported() → probe.go, probe_other.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/probe_linux.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 14 个
//	[关键引用] probeDarwin() ← probe.go, run_darwin.go, run_other.go
//
// 主要符号
//
//	landlockABI() int
//	seccompSupported() bool
//	detectCgroup() string
//	systemdScopeAvailable() bool
//	probeDarwin() (Caps, error)

package host

import (
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/unix"
)

// landlockABI 用内核的 LANDLOCK_CREATE_RULESET_VERSION 探测 ABI；不可用返回 0。
// go-landlock 没有导出探测函数，syscall 号用 x/sys 的各架构常量，不写死。
func landlockABI() int {
	r, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0,
		uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION))
	if errno != 0 {
		return 0
	}
	return int(r)
}

// seccompSupported 与 go-seccomp-bpf 的 Supported 同一手法：对 strict 模式传
// 非法 flags，syscall 存在且被允许时返回 EINVAL。不链接该库，host 保持轻量。
func seccompSupported() bool {
	_, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_STRICT, 1, 0)
	return errno == unix.EINVAL
}

// detectCgroup 判断 cgroup v2 限额能否生效（见 cgroup_linux.go）。
func detectCgroup() string {
	if _, err := delegatedBase(); err == nil {
		return cgroupDelegated
	}
	if systemdScopeAvailable() {
		return cgroupSystemd
	}
	return cgroupNone
}

func systemdScopeAvailable() bool {
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return false
	}
	path, err := exec.LookPath("systemd-run")
	if err != nil || path == "" {
		return false
	}
	out, err := exec.Command(path, "--version").CombinedOutput()
	return err == nil && strings.HasPrefix(string(out), "systemd")
}

func probeDarwin() (Caps, error) { return Caps{OS: "linux"}, unavailable("not darwin") }
