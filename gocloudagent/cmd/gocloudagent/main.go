// 文件说明：main.go — Command gocloudagent 是 gocloudagent 的独立命令行版本（planv6.0
//
// 用法
//
//	包内实现细节，由上层包经包级 API 间接触达。
//
// 调用架构
//
//	[上游] （无，本包为入口）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cmd/gocloudagent/main.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent
//
// 主要符号
//
//	var version
//	main()

package main

import (
	"os"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
)

// version 由 build.sh 通过 -ldflags -X 注入。
var version = "dev"

func main() {
	gocloudagent.SetCLIVersion(version)
	os.Exit(gocloudagent.RunCLI(os.Environ(), os.Args[1:], os.Stdout, os.Stderr))
}
