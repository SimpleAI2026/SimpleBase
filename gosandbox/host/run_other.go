//go:build !linux && !darwin

package host

import (
	"context"
	"runtime"
)

func probeDarwin() (Caps, error) {
	return Caps{OS: runtime.GOOS}, unavailable("unsupported OS %s", runtime.GOOS)
}

// Run 在不支持的平台上 fail-closed。
func Run(_ context.Context, p Policy, _ Command) (Result, error) {
	if !p.Isolates() {
		return Result{}, ErrNotIsolated
	}
	return Result{}, unavailable("unsupported OS %s", runtime.GOOS)
}
