//go:build !linux

package agent

import (
	"errors"
	"os"
)

// openPTY 在非 Linux 上不可用：agent 只在 Linux 沙盒里跑。
func openPTY(uint16, uint16) (*os.File, *os.File, error) {
	return nil, nil, errors.New("pty is only available on linux")
}

// setWinsize 在非 Linux 上不可用。
func setWinsize(*os.File, uint16, uint16) error {
	return errors.New("pty is only available on linux")
}
