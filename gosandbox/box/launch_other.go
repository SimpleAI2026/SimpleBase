//go:build !linux

// 文件说明：launch_other.go — gosandbox/box 的内部实现（launch、closeSession 等）
//
// 用法
//
//	包内实现，无导出与跨文件调用
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/launch_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] backpressure_test.go, copytree.go, deadline.go, egress_e2e_test.go, files.go 等 26 个
//
// 主要符号
//
//	launch(context.Context, Config) (net.Conn, error)
//	closeSession(*Session)

package box

import (
	"context"
	"net"
)

// launch 在非 Linux 上直接失败。
func launch(context.Context, Config) (net.Conn, error) { return nil, ErrUnavailable }

// closeSession 在非 Linux 上无事可做。
func closeSession(*Session) {}
