// 文件说明：local.go — vm.Driver 的本机实现：每个「VM」是 Root 下的一个工作区目录，
//
// 用法
//
//	New(ctx context.Context, opts Options) (*Driver, error)
//	(d *Driver) Ensure(_ context.Context, t vm.Target) error  // Ensure 创建工作区（幂等）
//	(d *Driver) Exec(ctx context.Context, t vm.Target, r vm.ExecSpec) (vm.ExecResult ...  // Exec 在隔离中执行一条命令
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/local/local.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/host, gosandbox/vm
//	[同包] open_unix.go
//
// 主要符号
//
//	type Options struct {…}
//	type Driver struct {…}
//	New(ctx context.Context, opts Options) (*Driver, error)
//	(d *Driver) Ensure(_ context.Context, t vm.Target) error
//	(d *Driver) Exec(ctx context.Context, t vm.Target, r vm.ExecSpec) (vm.ExecResult ...
//	(d *Driver) ReadFile(_ context.Context, t vm.Target, p string) ([]byte, error)
//	... 另有 38 个顶层符号

package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/linkxzhou/SimpleBase/gosandbox/host"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// Options 是本机驱动的显式配置，不依赖调用方的配置结构。
type Options struct {
	// Root 是所有 VM 工作区的父目录，必须是绝对路径；不存在会被创建（0700）。
	Root string
	// Workdir 是 API 层看到的逻辑工作目录；Spec.Workdir 非空时以 Spec 为准。空 = /workspace。
	Workdir string
	// MaxFileBytes 是 ReadFile 的读取上限；<=0 取 1 MiB。
	MaxFileBytes int
	// MaxOutputBytes 是单次命令输出的内存上限；<=0 不限。服务端必须设置。
	MaxOutputBytes int
	// PidsMax 是 cgroup pids.max；0 = 不限。cgroup 不可用时不会生效。
	PidsMax int64
	// ProtectDirs / ProtectFiles 是对沙盒内命令隐藏的宿主路径（DataDir、缓存目录、配置与 .env 等）。
	// 目录若是工作区的祖先，只隐藏其中不通向工作区的部分。
	ProtectDirs  []string
	ProtectFiles []string
	// BwrapPath / HelperPath 见 host.Policy；macOS 忽略。
	BwrapPath  string
	HelperPath string
	// Log 接收驱动内部日志（含底层错误，对外返回的错误仍是脱敏的 ErrBackend）；nil 表示不输出。
	Log vm.LogFunc
}

// Driver 是本机沙盒驱动。
type Driver struct {
	opts Options
	// run 可在测试里替换，默认 host.Run。
	run func(context.Context, host.Policy, host.Command) (host.Result, error)
	// start 同上，默认 host.Start。
	start func(context.Context, host.Policy, host.StartSpec) (host.Proc, error)
}

const (
	defaultWorkdir = "/workspace"
	stoppedMarker  = "stopped"
	wsDirName      = "ws"
)

var vmNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,80}$`)

// New 校验配置并做一次真实的隔离冒烟测试；任何失败都返回 vm.ErrUnavailable，绝不退回裸执行。
func New(ctx context.Context, opts Options) (*Driver, error) {
	d := &Driver{opts: opts, run: host.Run, start: host.Start}
	if opts.Root == "" || !filepath.IsAbs(opts.Root) {
		d.logf("error", "sandbox local root must be an absolute path", "root", opts.Root)
		return nil, vm.ErrUnavailable
	}
	if err := os.MkdirAll(opts.Root, 0o700); err != nil {
		d.logf("error", "sandbox local root not creatable", "root", opts.Root, "error", err.Error())
		return nil, vm.ErrUnavailable
	}
	if real, err := filepath.EvalSymlinks(opts.Root); err == nil {
		d.opts.Root = real
	}
	policy := d.policy(d.opts.Root, vm.Spec{})
	caps, err := host.Probe(ctx, policy)
	if err != nil {
		d.logf("error", "sandbox local isolation unavailable", "error", err.Error())
		return nil, vm.ErrUnavailable
	}
	truePath, err := exec.LookPath("true")
	if err != nil {
		truePath = "/usr/bin/true"
	}
	res, err := d.run(ctx, policy, host.Command{Argv: []string{truePath}, Dir: d.opts.Root, Env: d.baseEnv(d.opts.Root)})
	if err != nil || res.ExitCode != 0 {
		reason := "non-zero exit"
		if err != nil {
			reason = err.Error()
		}
		d.logf("error", "sandbox local isolation smoke test failed", "error", reason, "output", string(res.Output))
		return nil, vm.ErrUnavailable
	}
	d.logf("info", "sandbox local driver initialized", "root", d.opts.Root, "os", caps.OS,
		"landlock_abi", caps.LandlockABI, "cgroup", caps.Cgroup)
	if caps.Cgroup != "delegated" {
		d.logf("warn", "sandbox cpu/memory/pids limits are not enforced on this host", "cgroup", caps.Cgroup)
	}
	return d, nil
}

func (d *Driver) logf(level, msg string, kv ...any) {
	if d.opts.Log != nil {
		d.opts.Log(level, msg, kv...)
	}
}

func (d *Driver) fail(op, name string, err error, kv ...any) error {
	d.logf("error", "sandbox local call failed", append([]any{"op", op, "vm", name, "error", err.Error()}, kv...)...)
	return fmt.Errorf("%w: %s", vm.ErrBackend, op)
}

// ---- 布局 ----

func (d *Driver) vmDir(name string) (string, error) {
	if !vmNameRe.MatchString(name) {
		return "", fmt.Errorf("invalid vm name %q", name)
	}
	return filepath.Join(d.opts.Root, name), nil
}

func (d *Driver) wsDir(name string) (string, error) {
	dir, err := d.vmDir(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, wsDirName), nil
}

// ensure 懒创建工作区并清除 stopped 标记，返回真实工作区路径。
func (d *Driver) ensure(t vm.Target) (string, error) {
	if t.Spec.Network == "public" {
		return "", errors.New("network=public is not supported by the local backend")
	}
	dir, err := d.vmDir(t.Name)
	if err != nil {
		return "", err
	}
	ws := filepath.Join(dir, wsDirName)
	if err := os.MkdirAll(ws, 0o700); err != nil {
		return "", err
	}
	if err := os.Remove(filepath.Join(dir, stoppedMarker)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	real, err := filepath.EvalSymlinks(ws)
	if err != nil {
		return "", err
	}
	return real, nil
}

func (d *Driver) workdir(t vm.Target) string {
	if t.Spec.Workdir != "" {
		return path.Clean(t.Spec.Workdir)
	}
	if d.opts.Workdir != "" {
		return path.Clean(d.opts.Workdir)
	}
	return defaultWorkdir
}

// rel 把逻辑路径映射为相对工作区的路径。
func rel(workdir, p string) (string, error) {
	clean := path.Clean(p)
	if clean == workdir {
		return ".", nil
	}
	if !strings.HasPrefix(clean, workdir+"/") {
		return "", fmt.Errorf("path %q is outside %s", p, workdir)
	}
	return strings.TrimPrefix(clean, workdir+"/"), nil
}

// ---- 隔离策略 ----

func (d *Driver) policy(ws string, spec vm.Spec) host.Policy {
	p := host.Policy{
		Enabled:        true,
		Mode:           host.ModeWorkspaceWrite,
		Workspace:      ws,
		MaxOutputBytes: d.opts.MaxOutputBytes,
		PidsMax:        d.opts.PidsMax,
		BwrapPath:      d.opts.BwrapPath,
		HelperPath:     d.opts.HelperPath,
		DenyRead:       d.denyRead(ws),
	}
	if d.opts.MaxOutputBytes > 0 {
		// 多留 1 字节，让上层能判断「超限被截断」。
		p.MaxOutputBytes = d.opts.MaxOutputBytes + 1
	}
	if spec.MemoryMiB > 0 {
		p.MemoryMaxBytes = int64(spec.MemoryMiB) << 20
	}
	if spec.CPUs > 0 {
		p.CPUMax = fmt.Sprintf("%d 100000", spec.CPUs*100000)
	}
	return p
}

// denyRead 返回沙盒内不可读的宿主路径：服务数据、缓存、配置，以及其他 VM 的工作区。
func (d *Driver) denyRead(ws string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(ps ...string) {
		for _, p := range ps {
			if p != "" && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	tops := append([]string{d.opts.Root}, d.opts.ProtectDirs...)
	for _, top := range tops {
		if abs, err := filepath.Abs(top); err == nil {
			if real, err := filepath.EvalSymlinks(abs); err == nil {
				abs = real
			}
			add(denyAround(abs, ws)...)
		}
	}
	for _, f := range d.opts.ProtectFiles {
		if abs, err := filepath.Abs(f); err == nil {
			if real, err := filepath.EvalSymlinks(abs); err == nil {
				abs = real
			}
			add(abs)
		}
	}
	return out
}

// denyAround 返回 top 之下需要遮住的路径：keep 不在 top 内则整个 top；
// 否则只遮住 top 里不通向 keep 的子项，并逐层下探。
func denyAround(top, keep string) []string {
	top, keep = filepath.Clean(top), filepath.Clean(keep)
	if keep != top && !strings.HasPrefix(keep, top+string(filepath.Separator)) {
		return []string{top}
	}
	if keep == top {
		return nil
	}
	entries, err := os.ReadDir(top)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		child := filepath.Join(top, e.Name())
		switch {
		case child == keep:
		case strings.HasPrefix(keep, child+string(filepath.Separator)):
			out = append(out, denyAround(child, keep)...)
		default:
			out = append(out, child)
		}
	}
	return out
}

// baseEnv 是沙盒进程的最小环境：不继承服务进程的任何变量。
func (d *Driver) baseEnv(ws string) []string {
	return []string{
		"PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + ws,
		"TMPDIR=/tmp",
		"LANG=C.UTF-8",
		"SANDBOX_WORKSPACE=" + ws,
	}
}

// ---- vm.Driver ----

// Ensure 创建工作区（幂等）。
func (d *Driver) Ensure(_ context.Context, t vm.Target) error {
	if _, err := d.ensure(t); err != nil {
		return d.fail("ensure", t.Name, err)
	}
	return nil
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
	var argv []string
	if r.Shell != "" {
		argv = []string{"/bin/sh", "-c", r.Shell}
	} else {
		bin := r.Cmd
		if !strings.Contains(bin, "/") {
			found, lerr := exec.LookPath(bin)
			if lerr != nil {
				return vm.ExecResult{Stderr: []byte(bin + ": command not found\n"), ExitCode: 127}, nil
			}
			bin = found
		}
		argv = append([]string{bin}, r.Args...)
	}
	env := d.baseEnv(ws)
	for k, v := range r.Env {
		env = append(env, k+"="+v)
	}
	policy := d.policy(ws, t.Spec)
	policy.Timeout = r.Timeout
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

func (d *Driver) openRoot(t vm.Target) (*os.Root, string, error) {
	ws, err := d.ensure(t)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(ws)
	if err != nil {
		return nil, "", err
	}
	return root, d.workdir(t), nil
}

// ReadFile 读取至多 MaxFileBytes+1 字节（上层据此判断截断）。
func (d *Driver) ReadFile(_ context.Context, t vm.Target, p string) ([]byte, error) {
	root, workdir, err := d.openRoot(t)
	if err != nil {
		return nil, d.fail("read", t.Name, err, "path", p)
	}
	defer root.Close()
	r, err := rel(workdir, p)
	if err != nil {
		return nil, d.fail("read", t.Name, err, "path", p)
	}
	f, err := root.OpenFile(r, os.O_RDONLY|nonBlock, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, vm.ErrFileNotFound
	}
	if err != nil {
		return nil, d.fail("read", t.Name, err, "path", p)
	}
	defer f.Close()
	// 沙盒内的命令可能留下 FIFO / 设备文件；只读普通文件。
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		if err == nil {
			err = errors.New("not a regular file")
		}
		return nil, d.fail("read", t.Name, err, "path", p)
	}
	limit := d.opts.MaxFileBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, d.fail("read", t.Name, err, "path", p)
	}
	return b, nil
}

// WriteFile 写入文件并按需创建父目录。
func (d *Driver) WriteFile(_ context.Context, t vm.Target, p string, data []byte) error {
	root, workdir, err := d.openRoot(t)
	if err != nil {
		return d.fail("write", t.Name, err, "path", p)
	}
	defer root.Close()
	r, err := rel(workdir, p)
	if err != nil || r == "." {
		if err == nil {
			err = errors.New("path is the workspace itself")
		}
		return d.fail("write", t.Name, err, "path", p)
	}
	// 目标若已被沙盒内命令替换成 FIFO / 设备，O_WRONLY 打开会阻塞，所以先确认它是普通文件。
	if st, lerr := root.Lstat(r); lerr == nil && !st.Mode().IsRegular() && st.Mode()&fs.ModeSymlink == 0 {
		return d.fail("write", t.Name, errors.New("target is not a regular file"), "path", p)
	}
	if dir := path.Dir(r); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return d.fail("write", t.Name, err, "path", p)
		}
	}
	if err := root.WriteFile(r, data, 0o644); err != nil {
		return d.fail("write", t.Name, err, "path", p, "bytes", len(data))
	}
	return nil
}

// RemoveFile 删除文件；不存在视为成功。
func (d *Driver) RemoveFile(_ context.Context, t vm.Target, p string) error {
	root, workdir, err := d.openRoot(t)
	if err != nil {
		return d.fail("remove file", t.Name, err, "path", p)
	}
	defer root.Close()
	r, err := rel(workdir, p)
	if err != nil || r == "." {
		if err == nil {
			err = errors.New("path is the workspace itself")
		}
		return d.fail("remove file", t.Name, err, "path", p)
	}
	if err := root.Remove(r); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return d.fail("remove file", t.Name, err, "path", p)
	}
	return nil
}

// ListDir 单层列目录，至多 501 项（上层截到 500）。
func (d *Driver) ListDir(_ context.Context, t vm.Target, p string) ([]vm.FileEntry, error) {
	root, workdir, err := d.openRoot(t)
	if err != nil {
		return nil, d.fail("list", t.Name, err, "path", p)
	}
	defer root.Close()
	r, err := rel(workdir, p)
	if err != nil {
		return nil, d.fail("list", t.Name, err, "path", p)
	}
	f, err := root.OpenFile(r, os.O_RDONLY|nonBlock, 0)
	if err != nil {
		return nil, d.fail("list", t.Name, err, "path", p)
	}
	defer f.Close()
	entries, err := f.ReadDir(501)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, d.fail("list", t.Name, err, "path", p)
	}
	base := path.Clean(p)
	out := make([]vm.FileEntry, 0, len(entries))
	for _, e := range entries {
		entry := vm.FileEntry{Name: e.Name(), Path: path.Join(base, e.Name()), Kind: kindOf(e.Type())}
		if info, ierr := e.Info(); ierr == nil {
			if info.Mode().IsRegular() {
				entry.Size = info.Size()
			}
			entry.Mode = uint32(info.Mode().Perm())
			entry.ModTime = info.ModTime()
		}
		out = append(out, entry)
	}
	return out, nil
}

func kindOf(m fs.FileMode) string {
	switch {
	case m.IsRegular():
		return "file"
	case m.IsDir():
		return "directory"
	case m&fs.ModeSymlink != 0:
		return "symlink"
	default:
		return "other"
	}
}

// Status 工作区不存在 = absent；有 stopped 标记 = stopped；否则 running。
func (d *Driver) Status(_ context.Context, name string) (string, error) {
	dir, err := d.vmDir(name)
	if err != nil {
		return "", d.fail("status", name, err)
	}
	if _, err := os.Stat(filepath.Join(dir, wsDirName)); errors.Is(err, fs.ErrNotExist) {
		return vm.VMAbsent, nil
	} else if err != nil {
		return "", d.fail("status", name, err)
	}
	if _, err := os.Stat(filepath.Join(dir, stoppedMarker)); err == nil {
		return vm.VMStopped, nil
	}
	return vm.VMRunning, nil
}

// Stop 只做标记，文件保留；本机驱动没有常驻进程。
func (d *Driver) Stop(_ context.Context, name string) error {
	dir, err := d.vmDir(name)
	if err != nil {
		return d.fail("stop", name, err)
	}
	if _, err := os.Stat(filepath.Join(dir, wsDirName)); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, stoppedMarker), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return d.fail("stop", name, err)
	}
	return f.Close()
}

// Remove 删除工作区；不存在视为成功。
func (d *Driver) Remove(_ context.Context, name string) error {
	dir, err := d.vmDir(name)
	if err != nil {
		return d.fail("remove", name, err)
	}
	if err := forceRemoveAll(dir); err != nil {
		return d.fail("remove", name, err)
	}
	return nil
}

// forceRemoveAll 删除目录树；沙盒内的命令可能留下只读目录，失败时先放开权限再重试一次。
func forceRemoveAll(dir string) error {
	if err := os.RemoveAll(dir); err == nil {
		return nil
	}
	_ = filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err == nil && e.IsDir() {
			_ = os.Chmod(p, 0o700)
		}
		return nil
	})
	return os.RemoveAll(dir)
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
	switch {
	case r.Shell != "":
		argv = []string{"/bin/sh", "-c", r.Shell}
	case r.Cmd != "":
		bin := r.Cmd
		if !strings.Contains(bin, "/") {
			found, lerr := exec.LookPath(bin)
			if lerr != nil {
				return nil, d.fail("start", t.Name, fmt.Errorf("%s: command not found", bin))
			}
			bin = found
		}
		argv = append([]string{bin}, r.Args...)
	default:
		argv = []string{"/bin/sh", "-i"}
	}
	env := d.baseEnv(ws)
	for k, v := range r.Env {
		env = append(env, k+"="+v)
	}
	if r.PTY != nil && !hasEnv(env, "TERM") {
		env = append(env, "TERM=xterm-256color")
	}
	policy := d.policy(ws, t.Spec)
	// 长会话不按字节截断输出（走背压）；时长由 ctx 控制。
	policy.MaxOutputBytes = 0
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

func hasEnv(env []string, key string) bool {
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			return true
		}
	}
	return false
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

var (
	_ vm.Driver      = (*Driver)(nil)
	_ vm.Interactive = (*Driver)(nil)
)
