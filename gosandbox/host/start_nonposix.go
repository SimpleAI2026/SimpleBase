//go:build !linux && !darwin

// 文件说明：start_nonposix.go — gosandbox/host 的内部实现（killGroup、exitInfo 等）
//
// 用法
//
//	同包调用：killGroup() → start.go, start_posix.go；exitInfo() → start.go, start_posix.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/start_nonposix.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 14 个
//	[关键引用] killGroup() ← start.go, start_posix.go
//
// 主要符号
//
//	killGroup(int, syscall.Signal) error
//	exitInfo(_ *os.ProcessState, waitErr error) ExitInfo

package host

import (
	"os"
	"syscall"
)

func killGroup(int, syscall.Signal) error { return ErrPTYUnsupported }

func exitInfo(_ *os.ProcessState, waitErr error) ExitInfo {
	if waitErr != nil {
		return ExitInfo{ExitCode: -1}
	}
	return ExitInfo{}
}
