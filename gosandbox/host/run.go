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
	TimedOut bool
	// Limits 如实报告资源限额是否生效。
	Limits Limits
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
