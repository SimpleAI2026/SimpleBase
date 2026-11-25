// 文件说明：process.go — 受控 argv 子进程：无 shell、环境替换、分流有界输出
//
// 只是执行机制，不含任何平台策略：可执行文件的默认名、环境变量、超时、退出码语义
// 和令牌注入都由调用方决定。与本机 shell 工具（/bin/sh -c、合并输出）、云沙盒
// （gosandbox/vm）都不是同一个东西，也不要互相替换。

package cloudagent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"unicode/utf8"
)

// DefaultProcessOutputLimit 是每路输出（stdout / stderr）的默认字节上限。
const DefaultProcessOutputLimit = 64 << 10

// ProcessRunner 以 argv 方式执行一个外部程序。
//
// 约定：
//   - 不经 shell；
//   - Env 整体替换子进程环境，nil 表示继承父进程环境（os/exec 语义），调用方通常应传完整环境；
//   - stdout 与 stderr 分别限流，超限部分丢弃但持续排空，子进程不会因管道写满而阻塞；
//   - 工作目录为一个空的临时目录（缩小相对路径的攻击面）；创建失败时拒绝执行，不退回当前目录；
//   - 子进程以非零码退出不是错误：返回退出码且 err 为 nil；只有无法启动或被信号终止之外的
//     执行错误才通过 err 返回。
type ProcessRunner struct {
	// Path 是可执行文件路径，不能为空。
	Path string
	// OutputLimit 是每路输出的字节上限；<=0 用 DefaultProcessOutputLimit。
	OutputLimit int
	// TempDirPrefix 是临时工作目录前缀；空用 "proc-"。
	TempDirPrefix string
}

// Run 执行 Path argv...，返回截断后的 stdout、stderr 与退出码。
func (p ProcessRunner) Run(ctx context.Context, argv []string, env []string, stdin string) (stdout, stderr string, exitCode int, err error) {
	path := strings.TrimSpace(p.Path)
	if path == "" {
		return "", "", 0, errors.New("process: executable path is not set")
	}
	limit := p.OutputLimit
	if limit <= 0 {
		limit = DefaultProcessOutputLimit
	}
	prefix := p.TempDirPrefix
	if prefix == "" {
		prefix = "proc-"
	}
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return "", "", 0, err
	}
	defer os.RemoveAll(dir)

	cmd := exec.CommandContext(ctx, path, argv...)
	cmd.Env = env
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &limitWriter{w: &out, n: limit}
	cmd.Stderr = &limitWriter{w: &errb, n: limit}
	err = cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
			err = nil
		}
	}
	return LimitProcessOutput(out.String(), limit), LimitProcessOutput(errb.String(), limit), exitCode, err
}

// ValidateExecutable 只返回安全的失败类别，从不暴露路径：
// not_configured / not_found / not_executable / invalid_binary / start_failed；空串表示可用。
//
// 只检查「是普通文件且带执行位」，不做 PATH 搜索，也不验证它真的能运行。
func ValidateExecutable(path string) string {
	if strings.TrimSpace(path) == "" {
		return "not_configured"
	}
	info, err := os.Stat(path)
	if err != nil {
		return ProcessStartFailure(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return "not_executable"
	}
	return ""
}

// ProcessStartFailure 把启动失败归类成不含路径与子进程输出的稳定类别。
func ProcessStartFailure(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "not_found"
	case errors.Is(err, os.ErrPermission):
		return "not_executable"
	case err != nil && (strings.Contains(strings.ToLower(err.Error()), "exec format") ||
		strings.Contains(strings.ToLower(err.Error()), "executable format")):
		return "invalid_binary"
	default:
		return "start_failed"
	}
}

// LimitProcessOutput 把 s 限制在 maxBytes 字节内，且不留下被截断的半个 UTF-8 字符。
//
// 单位是字节而不是字符，不追加截断标记；只修剪末尾不完整的合法字符，
// 不会规范化原本就无效的字节序列。
func LimitProcessOutput(s string, maxBytes int) string {
	if len(s) > maxBytes {
		s = s[:maxBytes]
	}
	if len(s) < maxBytes {
		return s
	}
	for k := 1; k <= utf8.UTFMax && k <= len(s); k++ {
		if utf8.RuneStart(s[len(s)-k]) {
			if !utf8.FullRuneInString(s[len(s)-k:]) {
				return s[:len(s)-k]
			}
			break
		}
	}
	return s
}

// limitWriter 最多保留 n 字节，其余丢弃；总是报告写入了全部输入。
//
// 必须报告完整长度：io.Writer 的短写要求返回错误，否则 os/exec 的复制协程会得到
// io.ErrShortWrite 并停止排空管道，使「输出被截断但命令成功」变成启动失败甚至阻塞。
type limitWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if remain := l.n - l.w.Len(); remain > 0 {
		keep := p
		if len(keep) > remain {
			keep = keep[:remain]
		}
		l.w.Write(keep)
	}
	return len(p), nil
}
