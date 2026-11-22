//go:build linux

package agent

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// openPTY 打开一对伪终端，返回主端与从端。
// 调用方把从端交给 cmd 的 stdio，Start 之后关闭从端；主端自己持有。
//
// 用 /dev/ptmx 而不是 posix_openpt：沙盒内的 /dev 是 bwrap 的私有 devpts，
// 两者等价，但前者路径固定。
func openPTY(rows, cols uint16) (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	// 解锁从端：没有这一步，open("/dev/pts/N") 会 EIO。
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("unlock pty: %w", err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("pty number: %w", err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("open pty slave: %w", err)
	}
	if rows > 0 && cols > 0 {
		_ = setWinsizeFD(int(slave.Fd()), rows, cols)
	}
	return master, slave, nil
}

// setWinsize 改主端的窗口大小；内核会把它同步给从端。
func setWinsize(f *os.File, rows, cols uint16) error {
	return setWinsizeFD(int(f.Fd()), rows, cols)
}

func setWinsizeFD(fd int, rows, cols uint16) error {
	return unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Row: rows, Col: cols})
}
