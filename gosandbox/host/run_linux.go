//go:build linux

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

	"github.com/linkxzhou/SimpleBase/gosandbox/host/spec"
)

// HelperName 是沙盒内 helper 的二进制名。
const HelperName = "gosandbox-inner"

func prepareBwrapCmd(ctx context.Context, p Policy, cmd Command, ctty bool) (*exec.Cmd, *os.File, Caps, error) {
	if !p.Isolates() {
		return nil, nil, Caps{}, ErrNotIsolated
	}
	caps, err := Probe(ctx, p)
	if err != nil {
		return nil, nil, Caps{}, err
	}
	helper, err := resolveHelper(p.HelperPath)
	if err != nil {
		return nil, nil, Caps{}, err
	}
	if len(cmd.Argv) == 0 {
		return nil, nil, Caps{}, errors.New("host: empty argv")
	}
	dir := cmd.Dir
	if dir == "" {
		dir = p.Workspace
	}
	args, err := BwrapArgs(BwrapInput{
		Policy: p, Dir: dir, Helper: helper, Argv: cmd.Argv,
		PolicyFD: 3, DisableUserNS: caps.DisableUserNS, SizesSupported: caps.BwrapSize, CTTY: ctty,
	})
	if err != nil {
		return nil, nil, Caps{}, err
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, nil, Caps{}, unavailable("pipe: %v", err)
	}
	body, _ := json.Marshal(innerSpec(p))
	// 管道容量足够容纳策略 JSON，先写后关，bwrap 读到 EOF 即可。
	if _, err := pw.Write(body); err != nil {
		pw.Close()
		pr.Close()
		return nil, nil, Caps{}, unavailable("write policy: %v", err)
	}
	pw.Close()

	c := exec.Command(caps.BwrapPath, args...)
	c.Env = cmd.Env
	c.ExtraFiles = []*os.File{pr}
	// 独立进程组：超时或取消时向整组发 SIGKILL，不依赖 CommandContext 只杀直接子进程。
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c, pr, caps, nil
}

func prepareCgroup(p Policy, caps Caps) (*cgroupGroup, bool, error) {
	configured := limitsConfigured(p)
	if caps.Cgroup != cgroupDelegated {
		return nil, configured, nil
	}
	// 固定子组：即使没有配置限额也要建，因为 Metrics / Freeze 按名字找它。
	if !configured && p.CgroupName == "" {
		return nil, configured, nil
	}
	g, gerr := newNamedCgroupGroup(p.CgroupName)
	if gerr != nil {
		if !configured {
			// 只要指标 / 冻结时建不出子组不算失败：命令照常跑，只是没有指标。
			return nil, configured, nil
		}
		return nil, configured, unavailable("create cgroup: %v", gerr)
	}
	if aerr := g.apply(p); aerr != nil {
		g.remove()
		return nil, configured, unavailable("apply cgroup limits: %v", aerr)
	}
	return g, configured, nil
}

// joinCgroup 让子进程在 fork 时原子地进入 cgroup（CLONE_INTO_CGROUP），
// 而不是启动后再写 cgroup.procs：后者有一个窗口，子进程在窗口内 fork 出的
// 孙进程会留在父 cgroup 里，限额就漏了。
//
// 返回的 *os.File 必须在 c.Start() 之后才能关闭。
func joinCgroup(c *exec.Cmd, g *cgroupGroup) (*os.File, bool) {
	if g == nil {
		return nil, false
	}
	f, err := g.open()
	if err != nil {
		return nil, false
	}
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.UseCgroupFD = true
	c.SysProcAttr.CgroupFD = int(f.Fd())
	return f, true
}

// Run 在沙盒里执行 cmd。
//
// Enabled=false 或 danger-full-access 返回 ErrNotIsolated（调用方走裸执行）；
// 其他任何无法建立隔离的情况返回包装了 ErrIsolationUnavailable 的错误，命令不会被执行。
func Run(ctx context.Context, p Policy, cmd Command) (Result, error) {
	c, pr, caps, err := prepareBwrapCmd(ctx, p, cmd, false)
	if err != nil {
		return Result{}, err
	}
	defer pr.Close()

	runCtx := ctx
	if p.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}

	out := &cappedBuffer{max: p.MaxOutputBytes}
	c.Stdout, c.Stderr = out, out

	grp, configured, err := prepareCgroup(p, caps)
	if err != nil {
		return Result{}, err
	}
	limits := Limits{Configured: configured, Backend: cgroupNone}
	// 原子入组：CLONE_INTO_CGROUP 让子进程一出生就在子组里，没有逃逸窗口。
	cgFile, intoCgroup := joinCgroup(c, grp)

	start := time.Now()
	if err := c.Start(); err != nil {
		if cgFile != nil {
			cgFile.Close()
		}
		if grp != nil {
			grp.remove()
		}
		return Result{}, unavailable("start bwrap: %v", err)
	}
	if cgFile != nil {
		cgFile.Close()
	}
	if grp != nil {
		if intoCgroup {
			limits.Enforced, limits.Backend = true, "cgroup"
		} else if aerr := grp.addPID(c.Process.Pid); aerr == nil {
			// 回退路径：内核不支持 CLONE_INTO_CGROUP 时仍有极短窗口，如实记录为 Enforced。
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
		return "", unavailable("helper %s not found (build ./cmd: go build -o gosandbox-inner ./cmd)", path)
	}
	if st.IsDir() || st.Mode()&0o111 == 0 {
		return "", unavailable("helper %s is not executable", path)
	}
	return path, nil
}
