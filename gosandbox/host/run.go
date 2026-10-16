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
	Output   []byte
	ExitCode int
	TimedOut bool
	// Limits 如实报告资源限额是否生效。
	Limits Limits
	// Duration 是从启动 bwrap 到结束的耗时。
	Duration time.Duration
}
