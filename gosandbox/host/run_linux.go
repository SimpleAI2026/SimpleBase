//go:build linux

// 文件说明：run_linux.go — gosandbox/host 中 HelperName、Run 等符号的实现
//
// 用法
//
//	Run(ctx context.Context, p Policy, cmd Command) (Result, error)  // Run 在沙盒里执行 cmd
//	同包调用：Run() → run_darwin.go, run_other.go 等；innerSpec() → start_linux.go；resolveHelper() → start_linux.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/run_linux.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/host/internal/spec
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 14 个
//	[关键引用] Run() ← run_darwin.go, run_other.go, start_linux.go
//
// 主要符号
//
//	const HelperName
//	Run(ctx context.Context, p Policy, cmd Command) (Result, error)
//	... 另有 3 个顶层符号

package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/host/internal/spec"
)

// HelperName 是沙盒内 helper 的二进制名。
const HelperName = "gosandbox-inner"

// Run 在沙盒里执行 cmd。
//
// Enabled=false 或 danger-full-access 返回 ErrNotIsolated（调用方走裸执行）；
// 其他任何无法建立隔离的情况返回包装了 ErrIsolationUnavailable 的错误，命令不会被执行。
func Run(ctx context.Context, p Policy, cmd Command) (Result, error) {
	if !p.Isolates() {
		return Result{}, ErrNotIsolated
	}
	caps, err := Probe(ctx, p)
	if err != nil {
		return Result{}, err
	}
	helper, err := resolveHelper(p.HelperPath)
	if err != nil {
		return Result{}, err
	}
	argv := cmd.Argv
	if len(argv) == 0 {
		return Result{}, errors.New("host: empty argv")
	}
	dir := cmd.Dir
	if dir == "" {
		dir = p.Workspace
	}

	args, err := BwrapArgs(BwrapInput{Policy: p, Dir: dir, Helper: helper, Argv: argv,
		PolicyFD: 3, DisableUserNS: caps.DisableUserNS})
	if err != nil {
		return Result{}, err
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return Result{}, unavailable("pipe: %v", err)
	}
	defer pr.Close()
	body, _ := json.Marshal(innerSpec(p))
	// 管道容量足够容纳策略 JSON，先写后关，bwrap 读到 EOF 即可。
	if _, err := pw.Write(body); err != nil {
		pw.Close()
		return Result{}, unavailable("write policy: %v", err)
	}
	pw.Close()

	runCtx := ctx
	if p.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}

	c := exec.Command(caps.BwrapPath, args...)
	c.Env = cmd.Env
	c.ExtraFiles = []*os.File{pr}
	// 独立进程组：超时时向整组发 SIGKILL，不依赖 CommandContext 只杀直接子进程。
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out := &cappedBuffer{max: p.MaxOutputBytes}
	c.Stdout, c.Stderr = out, out

	limits := Limits{Configured: limitsConfigured(p), Backend: cgroupNone}
	var grp *cgroupGroup
	if limits.Configured && caps.Cgroup == cgroupDelegated {
		if g, gerr := newCgroupGroup(); gerr == nil {
			if aerr := g.apply(p); aerr != nil {
				g.remove()
				return Result{}, unavailable("apply cgroup limits: %v", aerr)
			}
			grp = g
		}
	}

	start := time.Now()
	if err := c.Start(); err != nil {
		if grp != nil {
			grp.remove()
		}
		return Result{}, unavailable("start bwrap: %v", err)
	}
	if grp != nil {
		// 有一个极短的窗口：进程被加入 cgroup 之前已经 fork。内核 cgroup 无法在 exec 前原子加入，
		// 这里如实记录为 Enforced，仅当写入成功。
		if aerr := grp.addPID(c.Process.Pid); aerr == nil {
			limits.Enforced, limits.Backend = true, "cgroup"
		}
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
	if grp != nil {
		defer grp.remove()
	}

	res := Result{Output: out.buf.Bytes(), OutputTruncated: out.truncated, TimedOut: timedOut, Limits: limits,
		Warnings: Warnings(p), Duration: time.Since(start)}
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
			return res, unavailable("wait bwrap: %v", werr)
		}
		res.ExitCode = ee.ExitCode()
	}
	// 退出码 71 且输出含固定前缀，才是 helper 装载失败；否则只是用户命令的退出码。
	if res.ExitCode == spec.ExitSetupFailed && bytes.Contains(res.Output, []byte(spec.FailPrefix)) {
		return Result{}, unavailable("%s", strings.TrimSpace(afterPrefix(res.Output)))
	}
	// bwrap 自身启动失败（挂载、命名空间）时输出以 "bwrap:" 开头且退出码非零。
	if res.ExitCode != 0 && bytes.HasPrefix(res.Output, []byte("bwrap: ")) {
		return Result{}, unavailable("%s", firstLine(string(res.Output), nil))
	}
	return res, nil
}

func afterPrefix(out []byte) string {
	i := bytes.Index(out, []byte(spec.FailPrefix))
	if i < 0 {
		return string(out)
	}
	rest := out[i+len(spec.FailPrefix):]
	if j := bytes.IndexByte(rest, '\n'); j >= 0 {
		rest = rest[:j]
	}
	return string(rest)
}

func innerSpec(p Policy) spec.Spec {
	s := spec.Spec{Mode: string(p.EffectiveMode()), Workspace: filepath.Clean(p.Workspace),
		ScratchDirs: ScratchDirs(p)}
	for _, r := range p.WritableRoots {
		s.WritableRoots = append(s.WritableRoots, filepath.Clean(r))
	}
	return s
}

// resolveHelper 找到 gosandbox-inner 的绝对路径：显式配置 → 可执行文件同目录。
// 不在 PATH 上找，避免被同名文件劫持。
func resolveHelper(configured string) (string, error) {
	path := configured
	if path == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", unavailable("locate executable: %v", err)
		}
		path = filepath.Join(filepath.Dir(exe), HelperName)
	}
	if !filepath.IsAbs(path) {
		return "", unavailable("helper path %q is not absolute", path)
	}
	st, err := os.Stat(path)
	if err != nil {
		return "", unavailable("helper %s not found (build cmd/gosandbox-inner)", path)
	}
	if st.IsDir() || st.Mode()&0o111 == 0 {
		return "", unavailable("helper %s is not executable", path)
	}
	return path, nil
}
