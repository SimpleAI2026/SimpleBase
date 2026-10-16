//go:build !linux

// gosandbox-inner 只在 Linux 上有意义（macOS 走 sandbox-exec，不需要 helper）。
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "gosandbox-inner is only supported on linux")
	os.Exit(1)
}
