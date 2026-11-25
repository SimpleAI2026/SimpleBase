//go:build !linux && !darwin

// 文件说明：start_nonposix.go — gosandbox/host 的内部实现（killGroup、exitInfo 等）
//
// 用法
//
//	包内实现，无导出与跨文件调用
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/box, gosandbox/vm/box 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/start_nonposix.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, cgroup.go, cgroup_linux.go, cgroup_linux_test.go 等 36 个
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
