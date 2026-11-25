// 文件说明：local.go — gosandbox/vm/local 中 Options、Driver 等符号的实现
//
// 用法
//
//	type Options struct{…}  // Options 是本机驱动的显式配置，不依赖调用方的配置结构
//	type Driver struct{…}  // Driver 是本机沙盒驱动
//	New(ctx context.Context, opts Options) (*Driver, error)  // New 校验配置并做一次真实的隔离冒烟测试；任何失败都返回 vm.ErrUnavailable，绝不退回裸执行
//	同包调用：rel() → exec.go, files.go；denyAround() → local_test.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/local/local.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] exec.go, export_test.go, files.go, local_test.go, metrics.go 等 8 个
//	[关键引用] Options ← local_test.go；Driver ← exec.go, export_test.go；New ← local_test.go
//
// 主要符号
//
//	type Options struct{…}
//	type Driver struct{…}
//	const defaultWorkdir / stoppedMarker / wsDirName
//	var vmNameRe
//	New(ctx context.Context, opts Options) (*Driver, error)
//	(d *Driver) logf(level, msg string, kv ...any)
//	(d *Driver) fail(op, name string, err error, kv ...any) error
//	(d *Driver) vmDir(name string) (string, error)
//	... 另有 12 个顶层符号

// Package local 提供 vm.Driver 的本机目录隔离实现（每个 VM 对应 Root 下的工作区，经 host 沙盒执行）。
package local

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/linkxzhou/SimpleBase/gosandbox/host"
	"github.com/linkxzhou/SimpleBase/gosandbox/internal/fsx"
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
	// ShmBytes 是沙盒内 /dev/shm 的大小；0 取 bwrap 默认。编译类任务需要放大。
	ShmBytes int64
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
	// freezeOnce / freezeOK 缓存 CanFreeze 的探测结果。
	freezeOnce sync.Once
	freezeOK   bool
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
	if caps.Cgroup != "delegated" && caps.Cgroup != "monitor" {
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

func (d *Driver) vmDir(name string) (string, error) {
	if !vmNameRe.MatchString(name) {
		return "", fmt.Errorf("invalid vm name %q", name)
	}
	return filepath.Join(d.opts.Root, name), nil
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

// rel 把逻辑路径映射为相对工作区的路径。非法路径与越界由 vm.CleanPath 统一拒绝，
// 与调用方和其它驱动共用同一套规则。
func rel(workdir, p string) (string, error) {
	clean, err := vm.CleanPath(workdir, p, true)
	if err != nil {
		return "", err
	}
	if clean == workdir {
		return ".", nil
	}
	prefix := workdir
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return strings.TrimPrefix(clean, prefix), nil
}

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
		ShmBytes:       d.opts.ShmBytes,
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

func (d *Driver) commandEnv(ws string, extra map[string]string) []string {
	env := d.baseEnv(ws)
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// Ensure 创建工作区（幂等）。
func (d *Driver) Ensure(_ context.Context, t vm.Target) error {
	if _, err := d.ensure(t); err != nil {
		return d.fail("ensure", t.Name, err)
	}
	return nil
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
	// 固定 cgroup 子组随沙盒一起删；组内仍有进程时删除失败，忽略即可
	// （进程退出后目录会被内核回收，或下次同名创建时清掉）。
	_ = host.RemoveCgroup(cgroupName(name))
	if err := fsx.RemoveAll(dir); err != nil {
		return d.fail("remove", name, err)
	}
	return nil
}
