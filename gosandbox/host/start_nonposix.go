//go:build !linux && !darwin

package host

import (
	"os"
	"syscall"
)

func killGroup(int, syscall.Signal) error { return ErrPTYUnsupported }

func exitInfo(_ *os.ProcessState, waitErr error) ExitInfo {
	if waitErr != nil {
		return ExitInfo{ExitCode: -1}
	}
	return ExitInfo{}
}
