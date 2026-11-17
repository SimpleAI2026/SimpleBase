//go:build linux

// 文件说明：loopback_linux.go — agent 启动时把 netns 里的 lo 拉起来
//
// 为什么必须做：bwrap --unshare-net 建出的 netns 里，lo 存在但处于
// DOWN 状态。没有 lo 的时候：
//
//   - 沙盒内的进程连不上 127.0.0.1，E2B 的 getHost(port) 入站中继全部失效；
//   - 若将来启用白名单 DNS 代理（监听 127.0.0.1:53），它也起不来。
//
// 也就是说：加了网络隔离之后必须顺手把 lo 拉起来，否则等于把入站也关掉了。
//
// 地址不需要配：内核在新建 netns 时已经给 lo 配好 127.0.0.1/8 与 ::1/128，
// 只差一个 IFF_UP。做这件事不需要特权——netns 里的 lo 在 userns 内部，
// CAP_NET_ADMIN 对我们自己的 userns 有效。
//
// 用法
//
//	_ = bringUpLoopback()  // agent 启动早期调用一次，失败只警告
//
// 调用架构
//
//	[上游] gosandbox/agent（run）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/agent/loopback_linux.go
//	         │
//	         ▼
//	[下游] golang.org/x/sys/unix
//	[同包] agent.go
//
// 主要符号
//
//	bringUpLoopback() error

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
