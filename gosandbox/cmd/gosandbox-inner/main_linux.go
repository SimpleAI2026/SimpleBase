//go:build linux

// gosandbox-inner 在 bwrap 沙盒内加载 Landlock 与 seccomp，再 exec 用户命令。
// 它只 import host/inner，不链接 DuckDB 等重依赖。
package main

import (
	"os"

	"github.com/linkxzhou/SimpleBase/gosandbox/host/inner"
)

func main() {
	os.Exit(inner.Main(os.Args[1:], os.Environ()))
}
