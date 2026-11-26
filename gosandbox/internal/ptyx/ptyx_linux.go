//go:build linux

// 文件说明：ptyx_linux.go — Linux 上打开伪终端：/dev/ptmx + TIOCSPTLCK + TIOCGPTN
//
// 用法：host / agent 经 ptyx.Open 调用；其它平台见 ptyx_darwin.go、ptyx_other.go。
// 调用架构：Open → 打开 /dev/ptmx → 解锁从端 → 取从端编号 → 打开 /dev/pts/N。
// 主要符号：Open。

package ptyx

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Open 打开一对伪终端：master 留给调用方，slave 作为子进程的 stdin/stdout/stderr。
//
// 用 /dev/ptmx 而不是 posix_openpt：沙盒内的 /dev 是 bwrap 的私有 devpts，
// 两者等价，但前者路径固定。
func Open() (master, slave *os.File, err error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	// 解锁从端：没有这一步，open("/dev/pts/N") 会 EIO。
	if err := unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		m.Close()
		return nil, nil, fmt.Errorf("unlock pty: %w", err)
	}
	n, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
	if err != nil {
		m.Close()
		return nil, nil, fmt.Errorf("pty number: %w", err)
	}
	s, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		m.Close()
		return nil, nil, fmt.Errorf("open pty slave: %w", err)
	}
	return m, s, nil
}
