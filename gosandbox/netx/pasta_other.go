//go:build !linux

package netx

import (
	"context"
	"fmt"
)

// Runner 在非 Linux 上是空类型，只为让签名一致。
type Runner struct{}

// Available 恒为 false：netns 与 pasta 都是 Linux 专有。
func Available() bool { return false }

// PastaPath 恒为空。
func PastaPath() string { return "" }

// StartOptions 与 Linux 实现同形。
type StartOptions struct {
	NetnsPID int
	Mode     Mode
	MTU      int
	Log      func(line string)
}

// Start 恒返回 ErrUnsupported。
func Start(context.Context, StartOptions) (*Runner, error) {
	return nil, fmt.Errorf("%w: only available on linux", ErrUnsupported)
}

// Done 实现最小接口。
func (*Runner) Done() <-chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

// Err 恒为 nil。
func (*Runner) Err() error { return nil }

// Stop 无事可做。
func (*Runner) Stop() error { return nil }
