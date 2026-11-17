//go:build !linux

// 文件说明：launch_other.go — 非 Linux 上没有 box 后端
//
// 用法
//
//	launch(ctx, cfg)  // 恒返回 ErrUnavailable
//	closeSession(s)  // 空操作
//
// 调用架构
//
//	[上游] gosandbox/box（Start / Close）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/launch_other.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] launch_linux.go, session.go
//
// 主要符号
//
//	launch(ctx context.Context, cfg Config) (net.Conn, error)
//	closeSession(s *Session)

package box

import (
	"context"
	"net"
)

// launch 在非 Linux 上直接失败。
func launch(context.Context, Config) (net.Conn, error) { return nil, ErrUnavailable }

// closeSession 在非 Linux 上无事可做。
func closeSession(*Session) {}
