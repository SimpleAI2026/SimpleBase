//go:build linux || darwin

// 文件说明：ptyx_unix.go — Linux 与 macOS 共用的窗口大小设置（TIOCSWINSZ）
//
// 用法：ptyx.SetWinsize(master 或 slave, rows, cols)。
// 调用架构：调用方 → unix.IoctlSetWinsize → 内核把窗口大小同步给另一端。
// 主要符号：SetWinsize。

package ptyx

import (
	"os"

	"golang.org/x/sys/unix"
)

// SetWinsize 设置伪终端的窗口大小。master 与 slave 指向同一个 tty，对哪一端调用效果相同。
func SetWinsize(f *os.File, rows, cols uint16) error {
	return unix.IoctlSetWinsize(int(f.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: rows, Col: cols})
}
