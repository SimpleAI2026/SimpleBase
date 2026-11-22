package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/linkxzhou/SimpleBase/gosandbox/host"
	"github.com/linkxzhou/SimpleBase/gosandbox/internal/executil"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// resolveCommandArgv 解析 Shell 或 Cmd+Args；裸命令未在 PATH 中找到时返回 ok=false。
func resolveCommandArgv(shell, cmd string, args []string) ([]string, bool) {
	if shell != "" {
		return []string{"/bin/sh", "-c", shell}, true
	}
	bin := cmd
	if !strings.Contains(bin, "/") {
		found, err := exec.LookPath(bin)
		if err != nil {
			return nil, false
		}
		bin = found
	}
	return append([]string{bin}, args...), true
}

// Exec 在隔离中执行一条命令。超时不是 error：TimedOut=true、ExitCode=-1。
func (d *Driver) Exec(ctx context.Context, t vm.Target, r vm.ExecSpec) (vm.ExecResult, error) {
	ws, err := d.ensure(t)
	if err != nil {
		return vm.ExecResult{}, d.fail("exec", t.Name, err)
	}
	dir, errRes := d.resolveCwd(ws, d.workdir(t), r.Cwd)
	if errRes != nil {
		return *errRes, nil
	}
	argv, ok := resolveCommandArgv(r.Shell, r.Cmd, r.Args)
	if !ok {
		return vm.ExecResult{Stderr: []byte(r.Cmd + ": command not found\n"), ExitCode: 127}, nil
	}
	env := d.commandEnv(ws, r.Env)
	policy := d.policy(ws, t.Spec)
	policy.Timeout = r.Timeout
	// 固定子组：命令结束后保留，供 Metrics / Pause 按名字找到它。
	policy.CgroupName = t.Name
	res, err := d.run(ctx, policy, host.Command{Argv: argv, Dir: dir, Env: env})
	if err != nil {
		if ctx.Err() != nil {
			return vm.ExecResult{}, ctx.Err()
		}
		return vm.ExecResult{}, d.fail("exec", t.Name, err)
	}
	if res.Limits.Configured && !res.Limits.Enforced {
		d.logf("debug", "sandbox exec ran without resource limits", "vm", t.Name, "backend", res.Limits.Backend)
	}
	return vm.ExecResult{Stdout: res.Output, ExitCode: res.ExitCode, TimedOut: res.TimedOut}, nil
}

// resolveCwd 把逻辑 cwd 映射为真实目录，并确认解析符号链接后仍在工作区内。
func (d *Driver) resolveCwd(ws, workdir, cwd string) (string, *vm.ExecResult) {
	if cwd == "" {
		return ws, nil
	}
	r, err := rel(workdir, cwd)
	if err != nil {
		return "", &vm.ExecResult{Stderr: []byte("sandbox: " + err.Error() + "\n"), ExitCode: 1}
	}
	real, err := filepath.EvalSymlinks(filepath.Join(ws, r))
	if err != nil || (real != ws && !strings.HasPrefix(real, ws+string(filepath.Separator))) {
		return "", &vm.ExecResult{Stderr: []byte("sandbox: cwd " + cwd + " does not exist\n"), ExitCode: 1}
	}
	if st, err := os.Stat(real); err != nil || !st.IsDir() {
		return "", &vm.ExecResult{Stderr: []byte("sandbox: cwd " + cwd + " is not a directory\n"), ExitCode: 1}
	}
	return real, nil
}

// Start 在隔离中启动一个保持句柄的进程（终端、后台进程）。
// 隔离不可用时返回 ErrBackend，不退回裸执行。超时由调用方通过 ctx 与 Proc.Close 控制。
func (d *Driver) Start(ctx context.Context, t vm.Target, r vm.StartSpec) (vm.Proc, error) {
	ws, err := d.ensure(t)
	if err != nil {
		return nil, d.fail("start", t.Name, err)
	}
	dir, errRes := d.resolveCwd(ws, d.workdir(t), r.Cwd)
	if errRes != nil {
		return nil, d.fail("start", t.Name, errors.New(strings.TrimSpace(string(errRes.Stderr))))
	}
	var argv []string
	if r.Shell == "" && r.Cmd == "" {
		argv = []string{"/bin/sh", "-i"}
	} else {
		var ok bool
		argv, ok = resolveCommandArgv(r.Shell, r.Cmd, r.Args)
		if !ok {
			return nil, d.fail("start", t.Name, fmt.Errorf("%s: command not found", r.Cmd))
		}
	}
	env := d.commandEnv(ws, r.Env)
	if r.PTY != nil && !executil.HasEnv(env, "TERM") {
		env = append(env, "TERM=xterm-256color")
	}
	policy := d.policy(ws, t.Spec)
	// 长会话不按字节截断输出（走背压）；时长由 ctx 控制。
	policy.MaxOutputBytes = 0
	policy.CgroupName = t.Name
	spec := host.StartSpec{Command: host.Command{Argv: argv, Dir: dir, Env: env}}
	if r.PTY != nil {
		spec.PTY = &host.PTYSize{Cols: r.PTY.Cols, Rows: r.PTY.Rows}
	}
	p, err := d.start(ctx, policy, spec)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, d.fail("start", t.Name, err)
	}
	return procAdapter{p}, nil
}

// procAdapter 把 host.Proc 适配为 vm.Proc。
type procAdapter struct{ p host.Proc }

func (a procAdapter) PID() int                      { return a.p.PID() }
func (a procAdapter) Stdin() io.WriteCloser         { return a.p.Stdin() }
func (a procAdapter) Output() io.Reader             { return a.p.Output() }
func (a procAdapter) Done() <-chan struct{}         { return a.p.Done() }
func (a procAdapter) Close() error                  { return a.p.Close() }
func (a procAdapter) Signal(s syscall.Signal) error { return a.p.Signal(s) }
func (a procAdapter) Resize(s vm.PTYSize) error {
	return a.p.Resize(host.PTYSize{Cols: s.Cols, Rows: s.Rows})
}
func (a procAdapter) Wait() vm.ProcExit {
	e := a.p.Wait()
	return vm.ProcExit{ExitCode: e.ExitCode, Signaled: e.Signaled}
}
