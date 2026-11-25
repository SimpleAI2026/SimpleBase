//go:build unix

// 文件说明：open_unix.go — gosandbox/vm/local 的内部实现（nonBlock 等）
//
// 用法
//
//	包内实现，无导出与跨文件调用
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/local/open_unix.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] exec.go, export_test.go, files.go, local.go, local_test.go 等 8 个
//
// 主要符号
//
//	const nonBlock

package local

import "syscall"

// nonBlock 让打开 FIFO 不阻塞；调用方随后用 Stat 确认是普通文件。
const nonBlock = syscall.O_NONBLOCK
