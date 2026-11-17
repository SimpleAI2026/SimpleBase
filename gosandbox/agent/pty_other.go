//go:build !linux

// 文件说明：pty_other.go — 非 Linux 上 agent 不提供 PTY
//
// 用法
//
//	openPTY(rows, cols uint16) (*os.File, *os.File, error)  // 恒返回错误
//	setWinsize(f *os.File, rows, cols uint16) error  // 恒返回错误
//
// 调用架构
//
//	[上游] gosandbox/agent/proc.go
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/agent/pty_other.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] pty_linux.go
//
// 主要符号
//
//	openPTY(rows, cols uint16) (*os.File, *os.File, error)
//	setWinsize(f *os.File, rows, cols uint16) error

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
