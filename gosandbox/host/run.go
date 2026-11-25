// 文件说明：run.go — gosandbox/host 中 Command、Result 等符号的实现
//
// 用法
//
//	type Command struct{…}  // Command 是一次要在沙盒里执行的命令
//	type Result struct{…}  // Result 是一次隔离执行的结果
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/box, gosandbox/vm/box 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/run.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, cgroup.go, cgroup_linux.go, cgroup_linux_test.go 等 36 个
//	[关键引用] Command ← probe_test.go, run_darwin.go；Result ← run_darwin.go, run_linux.go
//
// 主要符号
//
//	type Command struct{…}
//	type Result struct{…}

package host

import "time"

// Command 是一次要在沙盒里执行的命令。
type Command struct {
	// Argv 是完整命令，例如 {"/bin/sh","-c",script}。
	Argv []string
	// Dir 是工作目录（绝对路径）。
	Dir string
	// Env 是传给命令的环境（调用方负责先脱敏）。
	Env []string
}

// Result 是一次隔离执行的结果。
type Result struct {
	// Output 是 stdout 与 stderr 合并后的原始输出（不截断）。
	Output []byte
	// OutputTruncated 为真表示输出超过 Policy.MaxOutputBytes，尾部已丢弃。
	OutputTruncated bool
	ExitCode        int
	TimedOut        bool
	// Limits 如实报告资源限额是否生效。
	Limits Limits
	// LimitExceeded 非空表示命令因超出限额被终止（"memory" | "pids"），目前仅 macOS 的监控后端填写；
	// 此时 ExitCode 为 128+9。
	LimitExceeded string
	// Warnings 是执行过程中的安全提示（例如 macOS 资源限额终止），调用方应原样上报。
	Warnings []string
	// Duration 是从启动 bwrap 到结束的耗时。
	Duration time.Duration
}
