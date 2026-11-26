//go:build darwin

// 文件说明：ptyx_darwin.go — macOS 上打开伪终端：/dev/ptmx + TIOCPTYGRANT / TIOCPTYUNLK / TIOCPTYGNAME
//
// 用法：host / agent 经 ptyx.Open 调用；其它平台见 ptyx_linux.go、ptyx_other.go。
// 调用架构：Open → 打开 /dev/ptmx → 授权 → 解锁 → 取从端路径 → 打开从端。
// 主要符号：Open。

package ptyx

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Open 打开一对伪终端：master 留给调用方，slave 作为子进程的 stdin/stdout/stderr。
func Open() (master, slave *os.File, err error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	fd := int(m.Fd())
	if err := ioctlNoArg(fd, unix.TIOCPTYGRANT); err != nil {
		m.Close()
		return nil, nil, fmt.Errorf("grant pty: %w", err)
	}
	if err := ioctlNoArg(fd, unix.TIOCPTYUNLK); err != nil {
		m.Close()
		return nil, nil, fmt.Errorf("unlock pty: %w", err)
	}
	var name [128]byte
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(unix.TIOCPTYGNAME), uintptr(unsafe.Pointer(&name[0]))); errno != 0 {
		m.Close()
		return nil, nil, fmt.Errorf("pty name: %w", errno)
	}
	n := 0
	for n < len(name) && name[n] != 0 {
		n++
	}
	s, err := os.OpenFile(string(name[:n]), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		m.Close()
		return nil, nil, fmt.Errorf("open pty slave: %w", err)
	}
	return m, s, nil
}

func ioctlNoArg(fd int, req uint) error {
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), 0); errno != 0 {
		return errno
	}
	return nil
}
