// Package cloud 是 microsandbox Cloud 驱动；SDK 调用只集中在本包。
package cloud

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	microsandbox "github.com/superradcompany/microsandbox/sdk/go"
)

// Options 是 Cloud 驱动的显式配置，不依赖调用方的配置结构。
type Options struct {
	APIKey string
	// APIURL 为空表示官方默认地址。
	APIURL string
	// Home 非空时作为 MSB_HOME（SDK 运行时解压目录）。
	Home string
	// MaxFileBytes 是 ReadFile 的读取上限；<=0 取 1 MiB。
	MaxFileBytes int
}

// Driver 是 microsandbox Cloud 驱动。
type Driver struct{ opts Options }

// envMu 串行化对进程环境的写入：microsandbox SDK v0.7.4 只读取 MSB_* 环境变量，
// 没有等价的 option 入口，所以配置只能经进程环境传递（进程全局，同进程不能并存两套 Cloud 配置）。
var envMu sync.Mutex

// New 显式配置 Cloud backend，校验失败返回 vm.ErrUnavailable，绝不回退到本地 runtime。
func New(opts Options) (*Driver, error) {
	if opts.APIKey == "" {
		return nil, vm.ErrUnavailable
	}
	envMu.Lock()
	defer envMu.Unlock()
	env := [][2]string{{"MSB_BACKEND", "cloud"}, {"MSB_API_KEY", opts.APIKey}}
	if opts.APIURL != "" {
		env = append(env, [2]string{"MSB_API_URL", opts.APIURL})
	}
	if opts.Home != "" {
		env = append(env, [2]string{"MSB_HOME", filepath.Join(opts.Home, "microsandbox")})
	}
	for _, kv := range env {
		if err := os.Setenv(kv[0], kv[1]); err != nil {
			return nil, vm.ErrUnavailable
		}
	}
	info, err := microsandbox.DefaultBackendInfo()
	if err != nil || info.Kind != microsandbox.BackendCloud {
		return nil, vm.ErrUnavailable
	}
	return &Driver{opts: opts}, nil
}

func (d *Driver) network(s vm.Spec) *microsandbox.NetworkConfig {
	if s.Network == "public" {
		return microsandbox.NetworkPolicy.FromProfiles(microsandbox.NetworkProfilePublic)
	}
	return microsandbox.NetworkPolicy.None()
}
func (d *Driver) open(ctx context.Context, t vm.Target) (*microsandbox.Sandbox, error) {
	s := t.Spec
	box, err := microsandbox.ConnectOrCreateSandbox(ctx, t.Name,
		microsandbox.WithImage(s.Image), microsandbox.WithCPUs(uint8(s.CPUs)),
		microsandbox.WithMemory(uint32(s.MemoryMiB)), microsandbox.WithWorkdir(s.Workdir),
		microsandbox.WithMaxDuration(s.MaxDuration), microsandbox.WithIdleTimeout(s.IdleTimeout),
		microsandbox.WithNetwork(d.network(s)), microsandbox.WithLabels(s.Labels))
	if err != nil {
		return nil, fmt.Errorf("%w: open", vm.ErrBackend)
	}
	return box, nil
}
func (d *Driver) Ensure(ctx context.Context, t vm.Target) error {
	box, err := d.open(ctx, t)
	if err != nil {
		return err
	}
	return box.Close()
}
func (d *Driver) Exec(ctx context.Context, t vm.Target, r vm.ExecSpec) (vm.ExecResult, error) {
	box, err := d.open(ctx, t)
	if err != nil {
		return vm.ExecResult{}, err
	}
	defer box.Close()
	opts := []microsandbox.ExecOption{microsandbox.WithExecCwd(r.Cwd), microsandbox.WithExecTimeout(r.Timeout)}
	if len(r.Env) > 0 {
		opts = append(opts, microsandbox.WithExecEnv(r.Env))
	}
	var out *microsandbox.ExecOutput
	if r.Shell != "" {
		out, err = box.Shell(ctx, r.Shell, opts...)
	} else {
		out, err = box.Exec(ctx, r.Cmd, r.Args, opts...)
	}
	if microsandbox.IsKind(err, microsandbox.ErrExecTimeout) {
		return vm.ExecResult{ExitCode: -1, TimedOut: true}, nil
	}
	if err != nil {
		return vm.ExecResult{}, fmt.Errorf("%w: exec", vm.ErrBackend)
	}
	return vm.ExecResult{Stdout: out.StdoutBytes(), Stderr: out.StderrBytes(), ExitCode: out.ExitCode()}, nil
}
func (d *Driver) ReadFile(ctx context.Context, t vm.Target, p string) ([]byte, error) {
	box, err := d.open(ctx, t)
	if err != nil {
		return nil, err
	}
	defer box.Close()
	stream, err := box.FS().ReadStream(ctx, p)
	if microsandbox.IsKind(err, microsandbox.ErrPathNotFound) {
		return nil, vm.ErrFileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%w: read", vm.ErrBackend)
	}
	defer stream.Close()
	limit := d.opts.MaxFileBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	b, err := io.ReadAll(io.LimitReader(&streamReader{ctx: ctx, stream: stream}, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read stream", vm.ErrBackend)
	}
	return b, nil
}

// streamReader 将 SDK Recv 适配为受 ctx 约束的 io.Reader，避免整文件载入内存。
type streamReader struct {
	ctx     context.Context
	stream  *microsandbox.FsReadStream
	pending []byte
}

func (r *streamReader) Read(p []byte) (int, error) {
	for len(r.pending) == 0 {
		chunk, err := r.stream.Recv(r.ctx)
		if err != nil {
			return 0, err
		}
		if chunk == nil {
			return 0, io.EOF
		}
		r.pending = chunk
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (d *Driver) WriteFile(ctx context.Context, t vm.Target, p string, b []byte) error {
	box, err := d.open(ctx, t)
	if err != nil {
		return err
	}
	defer box.Close()
	if err = box.FS().Write(ctx, p, b); err != nil {
		return fmt.Errorf("%w: write", vm.ErrBackend)
	}
	return nil
}
func (d *Driver) RemoveFile(ctx context.Context, t vm.Target, p string) error {
	box, err := d.open(ctx, t)
	if err != nil {
		return err
	}
	defer box.Close()
	if err = box.FS().Remove(ctx, p); err != nil {
		return fmt.Errorf("%w: remove file", vm.ErrBackend)
	}
	return nil
}
func (d *Driver) ListDir(ctx context.Context, t vm.Target, p string) ([]vm.FileEntry, error) {
	box, err := d.open(ctx, t)
	if err != nil {
		return nil, err
	}
	defer box.Close()
	entries, err := box.FS().List(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("%w: list", vm.ErrBackend)
	}
	out := make([]vm.FileEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, vm.FileEntry{Name: filepath.Base(e.Path), Path: e.Path, Kind: string(e.Kind), Size: e.Size})
	}
	return out, nil
}
func (d *Driver) Status(ctx context.Context, name string) (string, error) {
	h, err := microsandbox.GetSandbox(ctx, name)
	if microsandbox.IsKind(err, microsandbox.ErrSandboxNotFound) {
		return vm.VMAbsent, nil
	}
	if err != nil {
		return "", fmt.Errorf("%w: status", vm.ErrBackend)
	}
	if h.Status() == microsandbox.SandboxStatusRunning {
		return vm.VMRunning, nil
	}
	return vm.VMStopped, nil
}
func (d *Driver) Stop(ctx context.Context, name string) error {
	h, err := microsandbox.GetSandbox(ctx, name)
	if microsandbox.IsKind(err, microsandbox.ErrSandboxNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: stop", vm.ErrBackend)
	}
	if h.Status() != microsandbox.SandboxStatusRunning {
		return nil
	}
	if err = h.Stop(ctx); err != nil {
		return fmt.Errorf("%w: stop", vm.ErrBackend)
	}
	return nil
}
func (d *Driver) Remove(ctx context.Context, name string) error {
	h, err := microsandbox.GetSandbox(ctx, name)
	if microsandbox.IsKind(err, microsandbox.ErrSandboxNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: remove", vm.ErrBackend)
	}
	if h.Status() == microsandbox.SandboxStatusRunning {
		if err = h.Stop(ctx); err != nil {
			return fmt.Errorf("%w: stop", vm.ErrBackend)
		}
		// 等待 stopped 后才能 Remove；ctx 截止由调用方控制。
		if _, err = h.WaitForStatus(ctx, microsandbox.SandboxStatusStopped); err != nil {
			return fmt.Errorf("%w: wait stopped", vm.ErrBackend)
		}
	}
	if err = h.Remove(ctx); err != nil {
		return fmt.Errorf("%w: remove", vm.ErrBackend)
	}
	return nil
}

var _ vm.Driver = (*Driver)(nil)
