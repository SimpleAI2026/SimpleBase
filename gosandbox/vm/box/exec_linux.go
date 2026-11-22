//go:build linux

package box

import (
	"context"
	"errors"
	"fmt"
	"io"
	"syscall"

	gosandboxbox "github.com/linkxzhou/SimpleBase/gosandbox/box"
	"github.com/linkxzhou/SimpleBase/gosandbox/internal/executil"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// Exec 在沙盒里执行一条命令。超时不是 error：TimedOut=true、ExitCode=-1。
func (d *Driver) Exec(ctx context.Context, t vm.Target, r vm.ExecSpec) (vm.ExecResult, error) {
	s, err := d.ensure(ctx, t)
	if err != nil {
		return vm.ExecResult{}, err
	}
	argv, ok := resolveArgv(r.Shell, r.Cmd, r.Args)
	if !ok {
		return vm.ExecResult{Stderr: []byte(r.Cmd + ": command not found\n"), ExitCode: 127}, nil
	}
	cwd, err := resolveCwd(d.workdir(t), r.Cwd)
	if err != nil {
		return vm.ExecResult{Stderr: []byte("sandbox: " + err.Error() + "\n"), ExitCode: 1}, nil
	}

	runCtx := ctx
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	h, err := s.Exec(runCtx, gosandboxbox.ExecRequest{
		Argv: argv, Env: d.commandEnv(d.workdir(t), r.Env), Cwd: cwd,
	})
	if err != nil {
		if ctx.Err() != nil {
			return vm.ExecResult{}, ctx.Err()
		}
		return vm.ExecResult{}, fmt.Errorf("%w: exec", vm.ErrBackend)
	}

	// 输出上限：超限部分丢弃但仍读完，避免 agent 的输出泵被卡住。
	out := executil.NewCappedBuffer(d.opts.MaxOutputBytes)
	_, _ = io.Copy(out, h.Output())
	ex := h.Wait()
	_ = h.Close()

	res := vm.ExecResult{Stdout: out.Bytes(), ExitCode: int(ex.Code)}
	if ex.Signaled {
		res.ExitCode = 128 + int(ex.Signal)
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		res.TimedOut, res.ExitCode = true, -1
	}
	return res, nil
}

// resolveArgv 解析 Shell 或 Cmd+Args。
func resolveArgv(shell, cmd string, args []string) ([]string, bool) {
	if shell != "" {
		return []string{"/bin/sh", "-c", shell}, true
	}
	return append([]string{cmd}, args...), cmd != ""
}

// resolveCwd 把逻辑路径映射为沙盒内路径。
func resolveCwd(workdir, cwd string) (string, error) {
	if cwd == "" {
		return workdir, nil
	}
	return vm.CleanPath(workdir, cwd, true)
}

// Start 在沙盒里启动一个保持句柄的进程。
func (d *Driver) Start(ctx context.Context, t vm.Target, r vm.StartSpec) (vm.Proc, error) {
	s, err := d.ensure(ctx, t)
	if err != nil {
		return nil, err
	}
	argv, ok := resolveArgv(r.Shell, r.Cmd, r.Args)
	if !ok {
		if r.Shell == "" && r.Cmd == "" {
			argv = []string{"/bin/sh", "-i"}
		} else {
			return nil, fmt.Errorf("%w: %s: command not found", vm.ErrBackend, r.Cmd)
		}
	}
	cwd, cerr := resolveCwd(d.workdir(t), r.Cwd)
	if cerr != nil {
		return nil, fmt.Errorf("%w: %v", vm.ErrBackend, cerr)
	}
	env := d.commandEnv(d.workdir(t), r.Env)
	req := gosandboxbox.ExecRequest{Argv: argv, Env: env, Cwd: cwd, Stdin: true}
	if r.PTY != nil {
		req.TTY, req.Rows, req.Cols = true, r.PTY.Rows, r.PTY.Cols
		if !executil.HasEnv(env, "TERM") {
			req.Env = append(env, "TERM=xterm-256color")
		}
	}
	h, err := s.Exec(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%w: start", vm.ErrBackend)
	}
	return &procAdapter{h: h}, nil
}

// procAdapter 把 box.Handle 适配为 vm.Proc。
type procAdapter struct{ h *gosandboxbox.Handle }

func (a *procAdapter) PID() int              { return 0 } // agent 里的 pid，宿主看不到
func (a *procAdapter) Stdin() io.WriteCloser { return a.h }
func (a *procAdapter) Output() io.Reader     { return a.h.Output() }
func (a *procAdapter) Done() <-chan struct{} { return a.h.Done() }
func (a *procAdapter) Close() error          { return a.h.Close() }
func (a *procAdapter) Resize(s vm.PTYSize) error {
	return a.h.Resize(s.Rows, s.Cols)
}
func (a *procAdapter) Signal(sig syscall.Signal) error { return a.h.Signal(int32(sig)) }
func (a *procAdapter) Wait() vm.ProcExit {
	ex := a.h.Wait()
	return vm.ProcExit{ExitCode: int(ex.Code), Signaled: ex.Signaled}
}
