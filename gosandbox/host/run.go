// 文件说明：run.go — gosandbox/host 中 Command、Result 等符号的实现
//
// 用法
//
//	type Command（导出类型，同包与上层均可使用）
//	type Result（导出类型，同包与上层均可使用）
//	同包调用：Command → probe_linux.go, run_darwin.go 等；Result → policy.go, run_darwin.go 等；Write() → run_linux.go, start_darwin.go 等
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/run.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 13 个
//	[关键引用] Command ← probe_linux.go, run_darwin.go, run_linux.go
//
// 主要符号
//
//	type Command struct {…}
//	type Result struct {…}
//	... 另有 2 个顶层符号

package host

import (
	"bytes"
	"time"
)

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
	// Warnings 是安全相关的提示（如强制 deny 表被关闭），调用方应原样上报，不要吞掉。
	Warnings []string
	// Duration 是从启动 bwrap 到结束的耗时。
	Duration time.Duration
}

// cappedBuffer 是有上限的输出缓冲：超出上限的部分被丢弃但仍报告写入成功，
// 这样子进程不会因管道写失败而提前终止，也不会撑大宿主内存。max<=0 表示不限。
type cappedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.max <= 0 {
		return c.buf.Write(p)
	}
	room := c.max - c.buf.Len()
	if room <= 0 {
		c.truncated = true
		return len(p), nil
	}
	if len(p) > room {
		c.buf.Write(p[:room])
		c.truncated = true
		return len(p), nil
	}
	return c.buf.Write(p)
}
