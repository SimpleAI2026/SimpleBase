package cloudagent

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"

	"github.com/linkxzhou/SimpleBase/internal/sbcli"
)

const cliOutputLimit = 64 << 10

// CLIRunner executes one simplebase invocation.
type CLIRunner interface {
	Run(ctx context.Context, argv []string, env []string) (stdout, stderr string, exitCode int, err error)
}

// InProcessCLI calls sbcli.RunIO in this process. Tests use it so the CLI still speaks HTTP.
type InProcessCLI struct{}

func (InProcessCLI) Run(ctx context.Context, argv []string, env []string) (string, string, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var out, errb bytes.Buffer
	code := sbcli.RunIO(ctx, argv, env, &out, &errb, strings.NewReader(""))
	return limitOut(out.String()), limitOut(errb.String()), code, nil
}

// SubprocessCLI execs the simplebase binary. The environment is replaced, not inherited.
type SubprocessCLI struct {
	Path string
}

func (s SubprocessCLI) Run(ctx context.Context, argv []string, env []string) (string, string, int, error) {
	path := strings.TrimSpace(s.Path)
	if path == "" {
		path = "simplebase"
	}
	cmd := exec.CommandContext(ctx, path, argv...)
	cmd.Env = env
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
