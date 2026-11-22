//go:build !linux

package box

import (
	"context"
	"net"
)

// launch 在非 Linux 上直接失败。
func launch(context.Context, Config) (net.Conn, error) { return nil, ErrUnavailable }

// closeSession 在非 Linux 上无事可做。
func closeSession(*Session) {}
