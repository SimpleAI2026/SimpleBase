//go:build !linux

// 文件说明：main_other.go — gosandbox/cmd 的内部实现（main 等）
//
// 用法
//
//	包内实现，无导出与跨文件调用
//
// 调用架构
//
//	[上游] 无（可执行入口）
//	[本文件] gosandbox/cmd/main_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] main_linux.go
//
// 主要符号
//
//	main()

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "gosandbox-inner is only supported on linux")
	os.Exit(1)
}
