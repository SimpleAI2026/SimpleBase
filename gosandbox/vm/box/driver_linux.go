//go:build linux

// Package box 是 vm.Driver 的常驻沙盒实现：每个沙盒一个 bwrap + 一个 agent。
//
// 与 vm/local 的区别：local 每条命令新起一个 bwrap，命令结束沙盒就不存在；
// 这里 bwrap 常驻、agent 是沙盒的 PID 1，因此
//   - 后台进程跨请求存活；
//   - 热 exec 只是一帧往返（不再有 bwrap 的挂载开销）；
//   - 文件操作在沙盒视角执行；
//   - 暂停能保留进程（cgroup.freeze，由 host 包提供）。
//
// 用法
//
//	d, err := box.New(ctx, box.Options{Root: "/var/lib/sb", Helper: "...", Bwrap: "/usr/bin/bwrap"})
//	d.Ensure(ctx, t); d.Exec(ctx, t, spec); d.Metrics(ctx, t.Name)
//
// 调用架构
//
//	[上游] internal/app（装配）、internal/sandbox（Manager）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/driver.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/{box, host, vm}
//	[同包] session.go
//
// 主要符号
//
//	type Options struct {…}
//	type Driver struct {…}
//	New(ctx context.Context, opts Options) (*Driver, error)
//	(d *Driver) Ensure / Exec / Start / ReadFile / WriteFile / RemoveFile / ListDir
//	(d *Driver) Status / Stop / Remove / Metrics / Pause / Resume / CanFreeze

package box

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	gosandboxbox "github.com/linkxzhou/SimpleBase/gosandbox/box"
	"github.com/linkxzhou/SimpleBase/gosandbox/host"
	"github.com/linkxzhou/SimpleBase/gosandbox/netx"
	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
	"github.com/linkxzhou/SimpleBase/gosandbox/snapshot"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// Options 是驱动配置。
type Options struct {
	// Root 是所有沙盒目录的父目录，必须是绝对路径。
	Root string
	// Workdir 是沙盒内的工作目录；空 = /workspace。
	Workdir string
	// Helper 是 gosandbox-inner 的绝对路径。
	Helper string
	// Bwrap 是 bwrap 的绝对路径；空表示不套隔离（只用于测试）。
	Bwrap string
	// MaxOutputBytes 是单次 Exec 的输出上限；0 = 不限。
	MaxOutputBytes int
	// MaxFileBytes 是单次 ReadFile 的上限；0 = 1 MiB。
	MaxFileBytes int
	// PidsMax / MemoryMiB 交给 cgroup；0 = 不限。
	PidsMax   int64
	MemoryMiB int
	// Seccomp 是 seccomp 档位："host"（默认）或 "default"。
	// 常驻沙盒里宿主文件系统不可见，所以默认应该用 "default"。
	Seccomp string
	// Policy 是额外的 Landlock 规则（保护目录等）。
	Policy Policy
	// Log 接收驱动日志。
	Log vm.LogFunc
	// Network 是出网策略；零值（Mode 为空）按 none 处理。
	Network netx.Policy
	// DiskLimitBytes 是每个沙盒可写部分（upper + ws）的上限；0 表示不限。
	//
	// rootless 下没有文件系统配额可用，只能靠宿主侧 du 统计后冻结。
	DiskLimitBytes int64
	// Supervise 为真时把沙盒交给独立的 supervisor 进程托管：服务进程重启后
	// 能重新接管，沙盒与其中的进程不丢。代价见 box.Config.Supervise。
	Supervise bool
	// Snapshots 是快照 store；nil 表示本驱动不支持快照与 fork。
	Snapshots *snapshot.Store
	// Images 是镜像来源；nil 表示只支持 image=host（宿主根只读）。
	//
	// 沙盒的 Spec.Image 为 "host"（或空）时用宿主根；否则按引用找镜像、
	// 解包出只读根（lower），再以它作沙盒根。
	Images ImageSource
}

// ImageSource 是驱动需要的镜像能力。由 gosandbox/image 的 Puller 实现；
// 用接口而不是直接依赖，是为了让不需要镜像的部署（image=host）不引入
// registry 代码路径。
type ImageSource interface {
	// Resolve 返回一个镜像的只读根目录。
	//
	// 实现应当按引用缓存：同一个引用反复调用必须很快（不能每次都拉取）。
	Resolve(ctx context.Context, ref string) (Lower, error)
}

// Lower 是一个解包好的只读根。
type Lower struct {
	// Path 是 lower 目录（overlay 的 lowerdir 或 copy 的源）。
	Path string
	// ChainID 是压平根的标识，用于 GC 与释放。
	ChainID string
}

// Policy 是交给沙盒内 Landlock 的附加规则。
type Policy struct {
	// DenyRead 是沙盒内不可读的宿主路径。
	DenyRead []string
}

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

	cfg := gosandboxbox.Config{
		Name:      t.Name,
		Root:      root,
		Workspace: d.workdir(),
		Helper:    d.opts.Helper,
		Bwrap:     d.opts.Bwrap,
		// Lower 非空时以它作只读根（overlay 或 copy）；空表示宿主根只读。
		Lower: lower.Path,
		Policy: gosandboxbox.Policy{
			Mode:          "workspace-write",
			Workspace:     d.opts.Workdir,
			WritableRoots: []string{d.opts.Workdir},
			ScratchDirs:   []string{"/tmp", d.opts.Workdir},
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

func (d *Driver) workdir() string { return d.opts.Workdir }

// commandEnv 是沙盒内命令的环境：最小集 + 调用方传入的。
func (d *Driver) commandEnv(extra map[string]string) []string {
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + d.opts.Workdir,
		"TMPDIR=/tmp",
		"LANG=C.UTF-8",
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// ---- vm.Driver ----

// Ensure 懒启动沙盒（幂等）。
func (d *Driver) Ensure(ctx context.Context, t vm.Target) error {
	_, err := d.ensure(ctx, t)
	return err
}

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
	cwd, err := d.resolveCwd(r.Cwd)
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
		Argv: argv, Env: d.commandEnv(r.Env), Cwd: cwd,
	})
	if err != nil {
		if ctx.Err() != nil {
			return vm.ExecResult{}, ctx.Err()
		}
		return vm.ExecResult{}, fmt.Errorf("%w: exec", vm.ErrBackend)
	}

	// 输出上限：超限部分丢弃但仍读完，避免 agent 的输出泵被卡住。
	out := &cappedBuffer{max: d.opts.MaxOutputBytes}
	_, _ = io.Copy(out, h.Output())
	ex := h.Wait()
	_ = h.Close()

	res := vm.ExecResult{Stdout: out.buf, ExitCode: int(ex.Code)}
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
func (d *Driver) resolveCwd(cwd string) (string, error) {
	if cwd == "" {
		return d.workdir(), nil
	}
	want := d.workdir()
	clean := filepath.Clean(cwd)
	if clean == want {
		return want, nil
	}
	if !strings.HasPrefix(clean, want+"/") {
		return "", fmt.Errorf("path %q is outside %s", cwd, want)
	}
	return clean, nil
}

// ReadFile 读沙盒内的文件。
func (d *Driver) ReadFile(ctx context.Context, t vm.Target, p string) ([]byte, error) {
	s, err := d.ensure(ctx, t)
	if err != nil {
		return nil, err
	}
	limit := d.opts.MaxFileBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	data, err := s.ReadFile(p, limit+1)
	if err != nil {
		if isNotExist(err) {
			return nil, vm.ErrFileNotFound
		}
		return nil, fmt.Errorf("%w: read", vm.ErrBackend)
	}
	return data, nil
}

// WriteFile 写沙盒内的文件。
func (d *Driver) WriteFile(ctx context.Context, t vm.Target, p string, data []byte) error {
	s, err := d.ensure(ctx, t)
	if err != nil {
		return err
	}
	if err := s.WriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("%w: write", vm.ErrBackend)
	}
	return nil
}

// RemoveFile 删除文件；不存在视为成功。
func (d *Driver) RemoveFile(ctx context.Context, t vm.Target, p string) error {
	s, err := d.ensure(ctx, t)
	if err != nil {
		return err
	}
	if err := s.Remove(p); err != nil && !isNotExist(err) {
		return fmt.Errorf("%w: remove", vm.ErrBackend)
	}
	return nil
}

// ListDir 列一层目录。
func (d *Driver) ListDir(ctx context.Context, t vm.Target, p string) ([]vm.FileEntry, error) {
	s, err := d.ensure(ctx, t)
	if err != nil {
		return nil, err
	}
	if p == "" {
		p = d.workdir()
	}
	entries, err := s.ListDir(p)
	if err != nil {
		return nil, fmt.Errorf("%w: list", vm.ErrBackend)
	}
	base := strings.TrimSuffix(p, "/")
	out := make([]vm.FileEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, vm.FileEntry{
			Name: e.Name, Path: base + "/" + e.Name, Kind: kindOf(e.Kind),
			Size: e.Size, Mode: e.Mode, ModTime: time.Unix(0, e.ModNS),
		})
	}
	return out, nil
}

// kindOf 把协议的 FileKind 映射成 vm 的字符串。
func kindOf(k proto.FileKind) string {
	switch k {
	case proto.FileRegular:
		return "file"
	case proto.FileDir:
		return "directory"
	case proto.FileSymlink:
		return "symlink"
	}
	return "other"
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
	if err := forceRemoveAll(filepath.Join(d.opts.Root, name)); err != nil {
		return fmt.Errorf("%w: remove", vm.ErrBackend)
	}
	d.mu.Lock()
	delete(d.locks, name)
	d.mu.Unlock()
	return nil
}

// ---- 可选能力 ----

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
	cwd, cerr := d.resolveCwd(r.Cwd)
	if cerr != nil {
		return nil, fmt.Errorf("%w: %v", vm.ErrBackend, cerr)
	}
	env := d.commandEnv(r.Env)
	req := gosandboxbox.ExecRequest{Argv: argv, Env: env, Cwd: cwd, Stdin: true}
	if r.PTY != nil {
		req.TTY, req.Rows, req.Cols = true, r.PTY.Rows, r.PTY.Cols
		if !hasEnv(env, "TERM") {
			req.Env = append(env, "TERM=xterm-256color")
		}
	}
	h, err := s.Exec(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%w: start", vm.ErrBackend)
	}
	return &procAdapter{h: h}, nil
}

// Metrics 读沙盒用量。
func (d *Driver) Metrics(ctx context.Context, name string) (vm.Metrics, error) {
	s := d.session(name)
	if s == nil {
		return vm.Metrics{}, fmt.Errorf("%w: sandbox is not running", vm.ErrNotSupported)
	}
	st, err := s.Stats()
	if err != nil {
		return vm.Metrics{}, fmt.Errorf("%w: metrics", vm.ErrBackend)
	}
	// 磁盘用量在宿主侧算（沙盒里看到的是 overlay 合并视图，分不出用户写入）。
	used := diskUsed(filepath.Join(d.opts.Root, name))
	// 超限就地冻结：等到 reaper 下一轮再处理，用户可能已经把盘写满。
	d.checkDiskLimit(name, used)
	return vm.Metrics{
		MemoryBytes:     st.MemoryBytes,
		MemoryPeakBytes: st.MemoryPeak,
		CPUUsec:         st.CPUUsec,
		Pids:            st.Pids,
		OOMKills:        st.OOMKills,
		DiskReadBytes:   st.DiskReadBytes,
		DiskWriteBytes:  st.DiskWriteBytes,
		DiskUsedBytes:   used,
		Backend:         "cgroup",
	}, nil
}

// Pause 冻结沙盒内全部进程（cgroup.freeze）。
func (d *Driver) Pause(_ context.Context, name string) error {
	if err := host.FreezeCgroup(host.CgroupNamePrefix+name, true); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("%w: pause", vm.ErrBackend)
	}
	return nil
}

// Resume 解冻。
func (d *Driver) Resume(_ context.Context, t vm.Target) error {
	if err := host.FreezeCgroup(host.CgroupNamePrefix+t.Name, false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("%w: resume", vm.ErrBackend)
	}
	return nil
}

// CanFreeze 报告 cgroup 是否已委派（冻结需要 cgroup v2）。
func (d *Driver) CanFreeze() bool {
	d.freezeOnce.Do(func() {
		caps, err := host.Probe(context.Background(), host.Policy{
			Enabled: true, Mode: host.ModeWorkspaceWrite, Workspace: d.opts.Root,
		})
		d.freezeOK = err == nil && caps.Cgroup == "delegated"
	})
	return d.freezeOK
}

// ---- 辅助 ----

// cappedBuffer 是有上限的输出缓冲：超出部分丢弃但仍报告写入成功。
type cappedBuffer struct {
	buf       []byte
	max       int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.max <= 0 {
		c.buf = append(c.buf, p...)
		return len(p), nil
	}
	room := c.max - len(c.buf)
	if room <= 0 {
		c.truncated = true
		return len(p), nil
	}
	if len(p) > room {
		c.buf = append(c.buf, p[:room]...)
		c.truncated = true
		return len(p), nil
	}
	c.buf = append(c.buf, p...)
	return len(p), nil
}

func isNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "no such file")
}

func hasEnv(env []string, key string) bool {
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			return true
		}
	}
	return false
}

// forceRemoveAll 删除目录树；沙盒里的命令可能留下只读目录。
func forceRemoveAll(dir string) error {
	if err := os.RemoveAll(dir); err == nil {
		return nil
	}
	_ = filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if err == nil && e.IsDir() {
			_ = os.Chmod(p, 0o700)
		}
		return nil
	})
	return os.RemoveAll(dir)
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

var (
	_ vm.Driver      = (*Driver)(nil)
	_ vm.Interactive = (*Driver)(nil)
	_ vm.Meter       = (*Driver)(nil)
	_ vm.Pauser      = (*Driver)(nil)
	_ vm.Freezer     = (*Driver)(nil)
)
