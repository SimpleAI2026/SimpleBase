//go:build !linux && !darwin

package host

import (
	"context"
	"runtime"
)

// Start 在不支持的平台上 fail-closed。
func Start(_ context.Context, p Policy, _ StartSpec) (Proc, error) {
	if !p.Isolates() {
		return nil, ErrNotIsolated
	}
	return nil, unavailable("unsupported OS %s", runtime.GOOS)
}

// Resize 实现 Proc（此平台永不会构造 process）。
func (p *process) Resize(PTYSize) error { return ErrPTYUnsupported }
