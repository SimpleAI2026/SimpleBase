//go:build linux

// 文件说明：pty_linux.go — gosandbox/host 的内部实现（openPTY、setWinsize 等）
//
// 用法
//
//	包内实现，无导出与跨文件调用
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/box, gosandbox/vm/box 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/pty_linux.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, cgroup.go, cgroup_linux.go, cgroup_linux_test.go 等 36 个
//
// 主要符号
//
//	openPTY(size PTYSize) (master, slave *os.File, err error)
//	setWinsize(f *os.File, size PTYSize) error
//	itoa(n int) string

package host

import (
	"os"

	"golang.org/x/sys/unix"
)

// openPTY 打开一对伪终端：master 留给父进程，slave 作为子进程的 stdin/stdout/stderr。
func openPTY(size PTYSize) (master, slave *os.File, err error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	fd := int(m.Fd())
	var unlock int32
	if err = unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, int(unlock)); err != nil {
		m.Close()
		return nil, nil, err
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		m.Close()
		return nil, nil, err
	}
	s, err := os.OpenFile("/dev/pts/"+itoa(n), os.O_RDWR|unix.O_NOCTTY, 0)
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

func setWinsize(f *os.File, size PTYSize) error {
	ws := unix.Winsize{Row: size.Rows, Col: size.Cols}
	return unix.IoctlSetWinsize(int(f.Fd()), unix.TIOCSWINSZ, &ws)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
