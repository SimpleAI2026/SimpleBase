//go:build !linux

// 文件说明：vsock_other.go — 非 Linux 上没有 vsock 拨号实现
//
// macOS 的 vsock 需要 Virtualization.framework（cgo + Code-Hex/vz），
// 见计划文档 §6.10 / M5-21。这一版先只提供「明确不可用」，
// 而不是一个半成品：假装支持会让 VM 后端在运行时才开始失败，
// 那比启动时就报错难查得多。
//
// 用法
//
//	VSockSupported()  // 恒 false
//	dialVSockOnce()   // 恒 ErrVSockUnsupported
//
// 调用架构
//
//	[上游] gosandbox/box（DialVSock）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/vsock_other.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] vsock.go, vsock_linux.go
//
// 主要符号
//
//	VSockSupported() bool
//	dialVSockOnce(ctx, cid, port) (Transport, error)

package box

import (
	"context"
	"fmt"
)

// VSockSupported 在非 Linux 上恒为 false。
func VSockSupported() bool { return false }

// dialVSockOnce 在非 Linux 上恒返回 ErrVSockUnsupported。
func dialVSockOnce(_ context.Context, cid, port uint32) (Transport, error) {
	return nil, fmt.Errorf("%w: vsock needs Virtualization.framework (macOS VM backend, see plan M5-21)",
		ErrVSockUnsupported)
}
