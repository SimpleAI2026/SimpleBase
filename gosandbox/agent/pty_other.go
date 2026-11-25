//go:build !linux

// 文件说明：pty_other.go — gosandbox/agent 的内部实现（openPTY、setWinsize 等）
//
// 用法
//
//	包内实现，无导出与跨文件调用
//
// 调用架构
//
//	[上游] gosandbox/cmd
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/agent/pty_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent.go, agent_test.go, fs.go, loopback_linux.go, loopback_other.go 等 16 个
//
// 主要符号
//
//	openPTY(uint16, uint16) (*os.File, *os.File, error)
//	setWinsize(*os.File, uint16, uint16) error

package agent

import (
	"errors"
	"os"
)

// openPTY 在非 Linux 上不可用：agent 只在 Linux 沙盒里跑。
func openPTY(uint16, uint16) (*os.File, *os.File, error) {
	return nil, nil, errors.New("pty is only available on linux")
}

// setWinsize 在非 Linux 上不可用。
func setWinsize(*os.File, uint16, uint16) error {
	return errors.New("pty is only available on linux")
}
