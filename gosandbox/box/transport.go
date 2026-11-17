// 文件说明：transport.go — 控制通道的传输抽象
//
// 控制通道之前直接绑死在 socketpair 上（Linux 专有）。但同一个协议栈
// 要被三种传输复用：
//
//	socketpair   Linux 同机（最快，无需路径）
//	unix socket  supervisor 托管（服务重启后可重连）
//	vsock        macOS VM（Virtualization.framework 的 AF_VSOCK）
//
// 上层（Session、agent 的读循环）只依赖 io.ReadWriteCloser，
// 因此这里定义一个最小的传输接口，而不是到处写 net.Conn。
//
// 用法
//
//	var tr Transport = ...
//	io.Copy(dst, tr)
//
// 调用架构
//
//	[上游] gosandbox/box（Session、launch）、gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/transport.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] launch_linux.go、supervise_linux.go
//
// 主要符号
//
//	type Transport interface {…}
//	wrapConn(net.Conn) Transport

package box

import (
	"io"
	"net"
)

// Transport 是控制通道的一条双向字节流。
//
// 协议层（proto）只要求「按顺序、不丢、双向」，与具体传输无关；
// 这正是当初坚持把 agent 做成独立进程而不是库的价值。
type Transport interface {
	io.ReadWriteCloser
}

// wrapConn 把一个 net.Conn 包成 Transport。
//
// net.Conn 已经满足 Transport；这里只是显式转换，
// 让「传输是 net.Conn」这件事只出现在一处。
func wrapConn(c net.Conn) Transport { return c }
