//go:build linux

// 文件说明：vsock_linux.go — Linux 上的 vsock 拨号（AF_VSOCK）
//
// Linux 的 AF_VSOCK 与 macOS 的语义一致：一个 u32 cid + 一个 u32 port。
// 在 Linux 上实现它的价值在于**可测试**：不需要 macOS VM 就能验证
// 「拨号 → 传输 → 协议栈」这条链路。
//
// 用法
//
//	tr, err := dialVSockOnce(ctx, HostCID, VSOCKPortAgent)
//
// 调用架构
//
//	[上游] gosandbox/box（DialVSock）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/vsock_linux.go
//	         │
//	         ▼
//	[下游] golang.org/x/sys/unix
//	[同包] vsock.go
//
// 主要符号
//
//	VSockSupported() bool
//	dialVSockOnce(ctx context.Context, cid, port uint32) (Transport, error)

package box

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

// AF_VSOCK 在部分 Go 版本里没有常量，这里写死（Linux 的固定值）。
const afVSock = 40

// vsockProbe 缓存「本机是否支持 vsock」。
//
// 探测方式是建一个 socket：有 vsock 模块时成功，没有时 EAFNOSUPPORT。
// 比读 /proc/modules 可靠——模块可能内建。
var vsockProbe atomic.Int32 // 0=未探测 1=支持 2=不支持

// VSockSupported 报告本机是否支持 vsock。
func VSockSupported() bool {
	switch vsockProbe.Load() {
	case 1:
		return true
	case 2:
		return false
	}
	fd, err := unix.Socket(afVSock, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		vsockProbe.Store(2)
		return false
	}
	_ = unix.Close(fd)
	vsockProbe.Store(1)
	return true
}

// dialVSockOnce 拨一次，不重试。
//
// 优先用 net.Dial：Go 的 net 包在 Linux 上认识 vsock 的地址格式
// （"vsock" 网络名），而且能直接配合 ctx。运行时不认这个网络名时
// 才退到手写 socket——不认的情况出现在老版本 Go 上。
func dialVSockOnce(ctx context.Context, cid, port uint32) (Transport, error) {
	addr := fmt.Sprintf("%d:%d", cid, port)
	dialer := net.Dialer{Timeout: 5 * time.Second}
	c, err := dialer.DialContext(ctx, "vsock", addr)
	if err == nil {
		return wrapConn(c), nil
	}
	if !isUnknownNetwork(err) {
		// 连接被拒之类：原样返回，让上层的重试逻辑判断是否值得再试。
		return nil, err
	}
	return dialVSockRaw(cid, port)
}

// isUnknownNetwork 判断错误是不是「不认识的网络名」。
func isUnknownNetwork(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsNotFound || dnsErr.Err == "unknown network"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Err != nil {
		msg := opErr.Err.Error()
		return msg == "unknown network "+"vsock" || msg == "unknown network"
	}
	var sysErr *os.SyscallError
	if errors.As(err, &sysErr) {
		return errors.Is(sysErr.Err, unix.EAFNOSUPPORT) || errors.Is(sysErr.Err, unix.EPROTONOSUPPORT)
	}
	return false
}

// dialVSockRaw 手写 socket + connect。
//
// sockaddr_vm 的布局（Linux）：
//
//	uint16 family
//	uint16 reserved
//	uint32 port
//	uint32 cid
//	uint8  flags[4]
//
// 一共 16 字节。
func dialVSockRaw(cid, port uint32) (Transport, error) {
	fd, err := unix.Socket(afVSock, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("box: vsock socket: %w", err)
	}
	sa := &unix.SockaddrVM{Port: port, CID: cid}
	if err := unix.Connect(fd, sa); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("box: vsock connect %d:%d: %w", cid, port, err)
	}
	// 交给 net.FileConn：它会把 fd 包成 net.Conn，我们就能复用协议栈。
	// 注意 FileConn 会 dup fd，所以原 fd 必须自己关。
	f := os.NewFile(uintptr(fd), "vsock")
	defer f.Close()
	conn, cerr := net.FileConn(f)
	if cerr != nil {
		return nil, fmt.Errorf("box: vsock wrap: %w", cerr)
	}
	return wrapConn(conn), nil
}
