//go:build linux

// Package box 是 vm.Driver 的常驻沙盒实现：每个沙盒一个 bwrap + 一个 agent。
//
// 与 vm/local 的区别：local 每条命令新起一个 bwrap，命令结束沙盒就不存在；
// 这里 bwrap 常驻、agent 是沙盒的 PID 1，因此
//   - 后台进程跨请求存活；
//   - 热 exec 只是一帧往返（不再有 bwrap 的挂载开销）；
//   - 文件操作在沙盒视角执行；
//   - 暂停能保留进程（cgroup.freeze，由 host 包提供）。
package box

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	gosandboxbox "github.com/linkxzhou/SimpleBase/gosandbox/box"
	"github.com/linkxzhou/SimpleBase/gosandbox/host"
	"github.com/linkxzhou/SimpleBase/gosandbox/internal/fsx"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// Driver 是常驻沙盒驱动。
type Driver struct {
	opts Options

	mu       sync.Mutex
	sessions map[string]*gosandboxbox.Session
	locks    map[string]*sync.Mutex

	freezeOnce sync.Once
	freezeOK   bool
}

// New 校验配置并做一次真实的冒烟测试：起一个沙盒跑 true，不通过返回 vm.ErrUnavailable。
func New(ctx context.Context, opts Options) (*Driver, error) {
	if opts.Root == "" || !filepath.IsAbs(opts.Root) {
		return nil, fmt.Errorf("%w: root must be absolute", vm.ErrUnavailable)
	}
	if err := os.MkdirAll(opts.Root, 0o700); err != nil {
		return nil, fmt.Errorf("%w: %v", vm.ErrUnavailable, err)
	}
	if real, err := filepath.EvalSymlinks(opts.Root); err == nil {
		opts.Root = real
	}
	if opts.Workdir == "" {
		opts.Workdir = "/workspace"
	}
	if opts.Seccomp == "" {
		opts.Seccomp = "default"
	}
	d := &Driver{opts: opts,
		sessions: map[string]*gosandboxbox.Session{},
		locks:    map[string]*sync.Mutex{}}

	// 冒烟测试：起一个沙盒跑 true。失败就整体不可用，不退回裸执行。
	t := vm.Target{Name: "smoke", Spec: vm.Spec{Workdir: opts.Workdir}}
	if err := d.Ensure(ctx, t); err != nil {
		d.logf("error", "sandbox box smoke test: ensure failed", "error", err.Error())
		return nil, vm.ErrUnavailable
	}
	res, err := d.Exec(ctx, t, vm.ExecSpec{Cmd: "true"})
	if err != nil || res.ExitCode != 0 {
		reason := "non-zero exit"
		if err != nil {
			reason = err.Error()
		}
		d.logf("error", "sandbox box smoke test failed", "error", reason, "output", res.Stdout)
		_ = d.Remove(ctx, t.Name)
		return nil, vm.ErrUnavailable
	}
	_ = d.Remove(ctx, t.Name)
	return d, nil
}

func (d *Driver) logf(level, msg string, kv ...any) {
	if d.opts.Log != nil {
		d.opts.Log(level, msg, kv...)
	}
}

// lock 取某个沙盒的锁：同一沙盒的操作串行。
func (d *Driver) lock(name string) *sync.Mutex {
	d.mu.Lock()
	defer d.mu.Unlock()
	l := d.locks[name]
	if l == nil {
		l = &sync.Mutex{}
		d.locks[name] = l
	}
	return l
}

// session 返回已存在的会话；不存在返回 nil。
func (d *Driver) session(name string) *gosandboxbox.Session {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sessions[name]
}

// ensure 拿到会话，必要时拉起。
func (d *Driver) ensure(ctx context.Context, t vm.Target) (*gosandboxbox.Session, error) {
	l := d.lock(t.Name)
	l.Lock()
	defer l.Unlock()
	if s := d.session(t.Name); s != nil {
		return s, nil
	}
	root := filepath.Join(d.opts.Root, t.Name)
	ws := filepath.Join(root, "ws")
	if err := os.MkdirAll(ws, 0o700); err != nil {
		return nil, fmt.Errorf("%w: %v", vm.ErrBackend, err)
	}
	// 停止标记：有它说明上次是 Stop，Ensure 会清掉。
	_ = os.Remove(filepath.Join(root, "stopped"))

	// 镜像根：Spec.Image 为 host 或空时用宿主根，否则解包出 lower。
	lower, err := d.resolveLower(ctx, t)
	if err != nil {
		return nil, err
	}

	workdir := d.workdir(t)
	if _, err := vm.CleanPath(workdir, workdir, true); err != nil {
		return nil, err
	}
	cfg := gosandboxbox.Config{
		Name:      t.Name,
		Root:      root,
		Workspace: workdir,
		Helper:    d.opts.Helper,
		Bwrap:     d.opts.Bwrap,
		// Lower 非空时以它作只读根（overlay 或 copy）；空表示宿主根只读。
		Lower: lower.Path,
		Policy: gosandboxbox.Policy{
			Mode:          "workspace-write",
			Workspace:     workdir,
			WritableRoots: []string{workdir},
			ScratchDirs:   []string{"/tmp", workdir},
			Seccomp:       d.opts.Seccomp,
		},
		Env: d.agentEnv(),
		Log: func(level, msg string, kv ...any) { d.logf(level, msg, kv...) },
		// 进程输出落盘：logs 能力与「进程退出后仍能回放」都依赖它。
		LogDir: filepath.Join(root, "logs"),
		// 出网：默认 none（netns 只有 lo）。这是安全默认值，
		// 加这个字段之前沙盒直接共享宿主网络栈。
		Network: d.opts.Network,
		// 让沙盒活过服务重启（升级、崩溃）。
		Supervise: d.opts.Supervise,
	}
	ses, err := gosandboxbox.Start(ctx, cfg)
	if err != nil {
		if errors.Is(err, gosandboxbox.ErrUnavailable) {
			return nil, fmt.Errorf("%w: %v", vm.ErrUnavailable, err)
		}
		return nil, fmt.Errorf("%w: %v", vm.ErrBackend, err)
	}
	d.mu.Lock()
	d.sessions[t.Name] = ses
	d.mu.Unlock()
	return ses, nil
}

// resolveLower 解析沙盒的只读根。
//
// image=host（或空）返回空 Lower：沙盒以宿主根只读运行（现状）。
// 其他引用交给 ImageSource；没配置 ImageSource 时明确报错，而不是
// 悄悄退化成宿主根（那等于用户以为在跑镜像，实际在跑宿主）。
func (d *Driver) resolveLower(ctx context.Context, t vm.Target) (Lower, error) {
	ref := strings.TrimSpace(t.Spec.Image)
	if ref == "" || ref == "host" {
		return Lower{}, nil
	}
	if d.opts.Images == nil {
		return Lower{}, fmt.Errorf("%w: image %q needs an image source; this deployment only supports image=host",
			vm.ErrUnavailable, ref)
	}
	lower, err := d.opts.Images.Resolve(ctx, ref)
	if err != nil {
		return Lower{}, fmt.Errorf("%w: resolve image %q: %v", vm.ErrBackend, ref, err)
	}
	if lower.Path == "" {
		return Lower{}, fmt.Errorf("%w: image %q has no root", vm.ErrBackend, ref)
	}
	return lower, nil
}

// agentEnv 是 agent 自己的环境：不继承服务进程，只给最小值。
func (d *Driver) agentEnv() []string {
	return []string{"PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin", "HOME=/root", "LANG=C.UTF-8"}
}

func (d *Driver) workdir(t vm.Target) string {
	if t.Spec.Workdir != "" {
		return filepath.Clean(t.Spec.Workdir)
	}
	return d.opts.Workdir
}

// commandEnv 是沙盒内命令的环境：最小集 + 调用方传入的。
func (d *Driver) commandEnv(workdir string, extra map[string]string) []string {
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + workdir,
		"TMPDIR=/tmp",
		"LANG=C.UTF-8",
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// Ensure 懒启动沙盒（幂等）。
func (d *Driver) Ensure(ctx context.Context, t vm.Target) error {
	_, err := d.ensure(ctx, t)
	return err
}

// Status 工作区不存在 = absent；有 stopped 标记 = stopped；否则 running。
func (d *Driver) Status(_ context.Context, name string) (string, error) {
	root := filepath.Join(d.opts.Root, name)
	if _, err := os.Stat(filepath.Join(root, "ws")); errors.Is(err, os.ErrNotExist) {
		return vm.VMAbsent, nil
	} else if err != nil {
		return "", fmt.Errorf("%w: status", vm.ErrBackend)
	}
	if _, err := os.Stat(filepath.Join(root, "stopped")); err == nil {
		return vm.VMStopped, nil
	}
	return vm.VMRunning, nil
}

// Stop 关掉沙盒并写停止标记；工作区保留。
func (d *Driver) Stop(ctx context.Context, name string) error {
	d.mu.Lock()
	s := d.sessions[name]
	delete(d.sessions, name)
	d.mu.Unlock()
	if s != nil {
		_ = s.Close()
	}
	root := filepath.Join(d.opts.Root, name)
	if _, err := os.Stat(filepath.Join(root, "ws")); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(root, "stopped"), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("%w: stop", vm.ErrBackend)
	}
	return f.Close()
}

// Remove 停止并删除沙盒。
func (d *Driver) Remove(ctx context.Context, name string) error {
	_ = d.Stop(ctx, name)
	_ = host.RemoveCgroup(host.CgroupNamePrefix + name)
	forgetDiskFrozen(name)
	if err := fsx.RemoveAll(filepath.Join(d.opts.Root, name)); err != nil {
		return fmt.Errorf("%w: remove", vm.ErrBackend)
	}
	d.mu.Lock()
	delete(d.locks, name)
	d.mu.Unlock()
	return nil
}

var (
	_ vm.Driver      = (*Driver)(nil)
	_ vm.Interactive = (*Driver)(nil)
	_ vm.Meter       = (*Driver)(nil)
	_ vm.Pauser      = (*Driver)(nil)
	_ vm.Freezer     = (*Driver)(nil)
)
