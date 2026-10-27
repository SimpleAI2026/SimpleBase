//go:build linux || darwin

// 文件说明：start_posix.go — gosandbox/host 的内部实现（killGroup、exitInfo 等）
//
// 用法
//
//	同包调用：killGroup() → start.go, start_nonposix.go；exitInfo() → start.go, start_nonposix.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/start_posix.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 13 个
//	[关键引用] killGroup() ← start.go, start_nonposix.go
//
// 主要符号
//
//	killGroup(pid int, sig syscall.Signal) error
//	exitInfo(st *os.ProcessState, waitErr error) ExitInfo

package host

import (
	"os"
	"syscall"
)

// killGroup 向整个进程组发信号（子进程以 Setpgid 启动，pgid == pid）。
func killGroup(pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}

func exitInfo(st *os.ProcessState, waitErr error) ExitInfo {
	if ws, ok := st.Sys().(syscall.WaitStatus); ok {
		if ws.Signaled() {
			return ExitInfo{Signaled: true, ExitCode: 128 + int(ws.Signal())}
		}
		return ExitInfo{ExitCode: ws.ExitStatus()}
	}
	if waitErr != nil {
		return ExitInfo{ExitCode: -1}
	}
	return ExitInfo{}
}
