//go:build linux

// 文件说明：loopback_linux.go — gosandbox/agent 的内部实现（bringUpLoopback、ifreq 等）
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
//	[本文件] gosandbox/agent/loopback_linux.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent.go, agent_test.go, fs.go, loopback_other.go, netfilter_linux.go 等 16 个
//
// 主要符号
//
//	bringUpLoopback() error
//	type ifreq struct{…}
//	setIfaceUp(index int) error
//	ioctlIfreq(fd int, req uint, r *ifreq) error

package agent

import (
	"fmt"
	"net"
	"unsafe"

	"golang.org/x/sys/unix"
)

// bringUpLoopback 把 lo 置为 UP。已经是 UP 时什么也不做。
func bringUpLoopback() error {
	iface, err := net.InterfaceByName("lo")
	if err != nil {
		return fmt.Errorf("agent: no loopback interface: %w", err)
	}
	if iface.Flags&net.FlagUp != 0 {
		return nil
	}
	return setIfaceUp(iface.Index)
}

// ifreq 是 ioctl 需要的结构：16 字节名字 + union（这里只用前 4 字节的 flags）。
type ifreq struct {
	Name  [unix.IFNAMSIZ]byte
	Flags uint16
	_     [22]byte
}

// setIfaceUp 用 ioctl 把接口置为 UP。
//
// 走 ioctl 而不是 netlink：只改一个标志位，netlink 的代码量是它的十倍。
// 先读后写：直接写会把内核设置的其他标志（例如 NOARP）清掉。
func setIfaceUp(index int) error {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("agent: socket: %w", err)
	}
	defer unix.Close(fd)
	_ = index // 名字定位已经足够，索引只用于日志
	var req ifreq
	copy(req.Name[:], "lo")
	if err := ioctlIfreq(fd, unix.SIOCGIFFLAGS, &req); err != nil {
		return fmt.Errorf("agent: get iflags: %w", err)
	}
	req.Flags |= unix.IFF_UP | unix.IFF_RUNNING
	if err := ioctlIfreq(fd, unix.SIOCSIFFLAGS, &req); err != nil {
		return fmt.Errorf("agent: set iflags: %w", err)
	}
	return nil
}

// ioctlIfreq 是一次 SIOCxIFFLAGS 调用。
func ioctlIfreq(fd int, req uint, r *ifreq) error {
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(unsafe.Pointer(r))); errno != 0 {
		return errno
	}
	return nil
}
