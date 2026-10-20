package cloudagent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
)

const cliOutputLimit = 64 << 10

// CLIRunner executes one simplebase invocation.
// stdin 由宿主内联传入（planv6.0 D3：助手模式不读宿主机文件）。
type CLIRunner interface {
	Run(ctx context.Context, argv []string, env []string, stdin string) (stdout, stderr string, exitCode int, err error)
}

// ValidateCLIPath reports only a safe failure category; it never exposes the path.
func ValidateCLIPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "not_configured"
	}
	info, err := os.Stat(path)
	if err != nil {
		return CLIStartFailure(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return "not_executable"
	}
	return ""
}

// CLIStartFailure classifies startup failures without including paths or child output.
func CLIStartFailure(err error) string {
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

// SubprocessCLI execs the simplebase binary. The environment is replaced, not inherited.
type SubprocessCLI struct {
	Path string
}

func (s SubprocessCLI) Run(ctx context.Context, argv []string, env []string, stdin string) (string, string, int, error) {
	path := strings.TrimSpace(s.Path)
	if path == "" {
		path = "simplebase"
	}
	cmd := exec.CommandContext(ctx, path, argv...)
	cmd.Env = env
	// planv6.0 D3 防御纵深：工作目录指向空临时目录，缩小相对路径的攻击面。
	if dir, err := os.MkdirTemp("", "sbcli-run-"); err == nil {
		cmd.Dir = dir
		defer os.RemoveAll(dir)
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &limitWriter{w: &out, n: cliOutputLimit}
	cmd.Stderr = &limitWriter{w: &errb, n: cliOutputLimit}
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
			err = nil
		}
	}
	return limitOut(out.String()), limitOut(errb.String()), code, err
}

func limitOut(s string) string {
	if len(s) <= cliOutputLimit {
		return s
	}
	return s[:cliOutputLimit]
}

type limitWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	remain := l.n - l.w.Len()
	if remain <= 0 {
		return len(p), nil
	}
	if len(p) > remain {
		p = p[:remain]
	}
	_, err := l.w.Write(p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// compile-time check that limitWriter is an io.Writer
var _ io.Writer = (*limitWriter)(nil)
