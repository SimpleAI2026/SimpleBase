//go:build linux || darwin

package host

import (
	"os"
	"syscall"
)

// killGroup 向整个进程组发信号（子进程以 Setpgid 启动，pgid == pid）。
func killGroup(pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}

func exitInfo(st *os.ProcessState, waitErr error) ExitInfo {
	if ws, ok := st.Sys().(syscall.WaitStatus); ok {
		if ws.Signaled() {
			return ExitInfo{Signaled: true, ExitCode: 128 + int(ws.Signal())}
		}
		return ExitInfo{ExitCode: ws.ExitStatus()}
	}
	if waitErr != nil {
		return ExitInfo{ExitCode: -1}
	}
	return ExitInfo{}
}
