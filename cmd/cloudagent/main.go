// Command cloudagent 是 cloudagent 的独立命令行版本（planv6.0
// cloudagent-cli-and-cronjob-unification-plan §5）。
//
// 它与 cmd/simplebase 是两个二进制，互不影响：simplebase 是服务端进程，
// 同时复用为「人类操作者的资源 CLI」（internal/sbcli）；cloudagent 是本地
// 编码助手，会话落在 ~/.cloudagent/，模型出口走 SimpleBase HTTP API。
//
// 入口只依赖 internal/cloudagent 一个包：能力都挂在该包子目录下
// （cli 外壳 / tools 本机工具 / agentcore 模型协议），由父包门面统一转出。
package main

import (
	"os"

	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
)

// version 由 build.sh 通过 -ldflags -X 注入。
var version = "dev"

func main() {
	cloudagent.SetCLIVersion(version)
	os.Exit(cloudagent.RunCLI(os.Environ(), os.Args[1:], os.Stdout, os.Stderr))
}
