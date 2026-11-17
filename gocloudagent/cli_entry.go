// 文件说明：cli_entry.go — 模块根包仅剩的生产代码：本地命令行门面
//
// 云端 Runtime 在 cloudagent/，内嵌技能在 skills/，本地命令行外壳在 cli/。
// 根包只让 cmd 依赖「一个包」，不含任何逻辑。
//
//	RunCLI(env []string, args []string, stdout, stderr *os.File) int
//	SetCLIVersion(v string)

package gocloudagent

import (
	"os"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cli"
)

// cli_entry.go 是 gocloudagent 对外的命令行门面（planv6.0 §5）。
//
// 为什么需要这层薄门面：CLI 的实现拆在子包里（cli 外壳 / tools 本机工具 /
// agentcore 协议），但 cmd 只应该认识 gocloudagent 这一个包——
// 否则「入口依赖哪个子包」就成了实现细节，将来子包重组会牵动 cmd。
//
// 门面只做转发，不含逻辑；所有导出符号都在子包，门面不复制类型
// （Event → cli.CLIEvent 之类在子包里叫法的差异，不应泄漏到入口）。

// RunCLI 执行 gocloudagent 命令行，返回进程退出码。
func RunCLI(env []string, args []string, stdout, stderr *os.File) int {
	return cli.RunCommand(env, args, stdout, stderr)
}

// SetCLIVersion 注入版本号（由 cmd 通过 -ldflags 提供）。
func SetCLIVersion(v string) { cli.SetVersion(v) }
