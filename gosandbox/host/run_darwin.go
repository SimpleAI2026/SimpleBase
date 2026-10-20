//go:build darwin

package host

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func probeDarwin() (Caps, error) {
	caps := Caps{OS: "darwin", Cgroup: cgroupNone}
	if _, err := os.Stat(SeatbeltExecutable); err != nil {
		return caps, unavailable("%s not found", SeatbeltExecutable)
	}
	caps.SandboxExec = SeatbeltExecutable
	return caps, nil
}

// Run 在 Seatbelt 沙盒里执行 cmd。语义与 Linux 实现一致。
// macOS 没有 cgroup，配置了限额时 Limits 恒为未生效。
func Run(ctx context.Context, p Policy, cmd Command) (Result, error) {
	if !p.Isolates() {
		return Result{}, ErrNotIsolated
	}
	if _, err := Probe(ctx, p); err != nil {
		return Result{}, err
	}
	if len(cmd.Argv) == 0 {
		return Result{}, errors.New("host: empty argv")
	}
	args, err := SeatbeltArgs(p, cmd.Argv)
	if err != nil {
		return Result{}, err
	}
	runCtx := ctx
	if p.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}
	c := exec.Command(SeatbeltExecutable, args...)
	c.Dir = cmd.Dir
	if c.Dir == "" {
		c.Dir = p.Workspace
	}
	c.Env = cmd.Env
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out := &cappedBuffer{max: p.MaxOutputBytes}
	c.Stdout, c.Stderr = out, out

	start := time.Now()
	if err := c.Start(); err != nil {
		return Result{}, unavailable("start sandbox-exec: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	var werr error
	timedOut := false
	select {
	case werr = <-done:
	case <-runCtx.Done():
		timedOut = errors.Is(runCtx.Err(), context.DeadlineExceeded)
		_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
		werr = <-done
	}
	res := Result{Output: out.buf.Bytes(), OutputTruncated: out.truncated, TimedOut: timedOut, Duration: time.Since(start),
		Limits: Limits{Configured: limitsConfigured(p), Backend: cgroupNone}}
	if timedOut {
		res.ExitCode = -1
		return res, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		res.ExitCode = -1
		return res, ctxErr
	}
	if werr != nil {
		var ee *exec.ExitError
		if !errors.As(werr, &ee) {
			return res, unavailable("wait sandbox-exec: %v", werr)
		}
		res.ExitCode = ee.ExitCode()
	}
	// 策略本身无法加载时 sandbox-exec 以 65 退出并输出 "sandbox-exec:" 前缀。
	if res.ExitCode != 0 && bytes.HasPrefix(res.Output, []byte("sandbox-exec: ")) {
		return Result{}, unavailable("%s", firstLine(string(res.Output), nil))
	}
	return res, nil
}
