//go:build darwin

// 文件说明：pty_darwin.go — gosandbox/host 的内部实现（openPTY、ioctlNoArg 等）
//
// 用法
//
//	同包调用：openPTY() → pty_linux.go, start_darwin.go 等；setWinsize() → pty_linux.go, start_darwin.go 等
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/pty_darwin.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 13 个
//	[关键引用] openPTY() ← pty_linux.go, start_darwin.go, start_linux.go
//
// 主要符号
//
//	openPTY(size PTYSize) (master, slave *os.File, err error)
//	ioctlNoArg(fd int, req uint) error
//	setWinsize(f *os.File, size PTYSize) error

package host

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPTY 在 macOS 上打开伪终端：/dev/ptmx + TIOCPTYGRANT / TIOCPTYUNLK / TIOCPTYGNAME。
func openPTY(size PTYSize) (master, slave *os.File, err error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	fd := int(m.Fd())
	if err = ioctlNoArg(fd, unix.TIOCPTYGRANT); err != nil {
		m.Close()
		return nil, nil, err
	}
	if err = ioctlNoArg(fd, unix.TIOCPTYUNLK); err != nil {
		m.Close()
		return nil, nil, err
	}
	var name [128]byte
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(unix.TIOCPTYGNAME), uintptr(unsafe.Pointer(&name[0]))); errno != 0 {
		m.Close()
		return nil, nil, errno
	}
	n := 0
	for n < len(name) && name[n] != 0 {
		n++
	}
	s, err := os.OpenFile(string(name[:n]), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		m.Close()
		return nil, nil, err
	}
	if err = setWinsize(m, size); err != nil {
		m.Close()
		s.Close()
		return nil, nil, err
	}
	return m, s, nil
}

func ioctlNoArg(fd int, req uint) error {
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), 0); errno != 0 {
		return errno
	}
	return nil
}

func setWinsize(f *os.File, size PTYSize) error {
	ws := unix.Winsize{Row: size.Rows, Col: size.Cols}
	return unix.IoctlSetWinsize(int(f.Fd()), unix.TIOCSWINSZ, &ws)
}
