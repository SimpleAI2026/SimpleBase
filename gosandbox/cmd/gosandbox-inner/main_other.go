//go:build !linux

// 文件说明：main_other.go — gosandbox-inner 只在 Linux 上有意义（macOS 走 sandbox-exec，不需要 helper…
//
// 用法
//
//	同包调用：main() → main_linux.go
//
// 调用架构
//
//	[上游] （无，本包为入口）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/cmd/gosandbox-inner/main_other.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[关键引用] main() ← main_linux.go
//
// 主要符号
//
//	main()
//	gosandbox-inner 只在 Linux 上有意义（macOS 走 sandbox-exec，不需要 helper）。

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "gosandbox-inner is only supported on linux")
	os.Exit(1)
}
