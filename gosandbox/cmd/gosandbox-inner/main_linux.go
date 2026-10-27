//go:build linux

// 文件说明：main_linux.go — gosandbox-inner 在 bwrap 沙盒内加载 Landlock 与 seccomp，再 exec 用户命令
//
// 用法
//
//	同包调用：main() → main_other.go
//
// 调用架构
//
//	[上游] （无，本包为入口）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/cmd/gosandbox-inner/main_linux.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/host/inner
//	[同包] main_other.go
//	[关键引用] main() ← main_other.go
//
// 主要符号
//
//	main()

package main

import (
	"os"

	"github.com/linkxzhou/SimpleBase/gosandbox/host/inner"
)

func main() {
	os.Exit(inner.Main(os.Args[1:], os.Environ()))
}
