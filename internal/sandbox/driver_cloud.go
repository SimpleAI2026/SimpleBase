package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	microsandbox "github.com/superradcompany/microsandbox/sdk/go"
)

// CloudDriver 是 microsandbox Cloud 驱动。除本文件与兼容层 client.go 外，其余业务文件不使用 SDK。
type CloudDriver struct{ cfg config.SandboxConfig }

// NewCloudDriver 显式配置 Cloud backend，校验失败绝不回退到本地 runtime。
func NewCloudDriver(cfg config.SandboxConfig, cacheDir string, logger observability.Logger) (*CloudDriver, error) {
	if cfg.EffectiveBackend() != config.SandboxBackendCloud || !cfg.Enabled || cfg.APIKey == "" {
		return nil, ErrUnavailable
	}
	if err := os.Setenv("MSB_BACKEND", "cloud"); err != nil {
		return nil, ErrUnavailable
	}
	if err := os.Setenv("MSB_API_KEY", cfg.APIKey); err != nil {
		return nil, ErrUnavailable
	}
	if cfg.APIURL != "" {
		if err := os.Setenv("MSB_API_URL", cfg.APIURL); err != nil {
			return nil, ErrUnavailable
		}
	}
	if cacheDir != "" {
		if err := os.Setenv("MSB_HOME", filepath.Join(cacheDir, "microsandbox")); err != nil {
			return nil, ErrUnavailable
		}
	}
	info, err := microsandbox.DefaultBackendInfo()
	if err != nil || info.Kind != microsandbox.BackendCloud {
		return nil, ErrUnavailable
	}
	if logger != nil {
		logger.Info("sandbox cloud backend ready")
	}
	return &CloudDriver{cfg: cfg}, nil
}

func (d *CloudDriver) Kind() string { return DriverCloud }

func (d *CloudDriver) network(s Spec) *microsandbox.NetworkConfig {
	if s.Network == "public" {
		return microsandbox.NetworkPolicy.FromProfiles(microsandbox.NetworkProfilePublic)
	}
	return microsandbox.NetworkPolicy.None()
}
func (d *CloudDriver) open(ctx context.Context, t Target) (*microsandbox.Sandbox, error) {
	s := t.Spec
	vm, err := microsandbox.ConnectOrCreateSandbox(ctx, t.Name,
		microsandbox.WithImage(s.Image), microsandbox.WithCPUs(uint8(s.CPUs)),
		microsandbox.WithMemory(uint32(s.MemoryMiB)), microsandbox.WithWorkdir(s.Workdir),
		microsandbox.WithMaxDuration(s.MaxDuration), microsandbox.WithIdleTimeout(s.IdleTimeout),
		microsandbox.WithNetwork(d.network(s)), microsandbox.WithLabels(s.Labels))
	if err != nil {
		return nil, fmt.Errorf("%w: open", ErrBackend)
	}
	return vm, nil
}
func (d *CloudDriver) Ensure(ctx context.Context, t Target) error {
	vm, err := d.open(ctx, t)
	if err != nil {
		return err
	}
	return vm.Close()
}
func (d *CloudDriver) Exec(ctx context.Context, t Target, r ExecSpec) (ExecResult, error) {
	vm, err := d.open(ctx, t)
	if err != nil {
		return ExecResult{}, err
	}
	defer vm.Close()
	opts := []microsandbox.ExecOption{microsandbox.WithExecCwd(r.Cwd), microsandbox.WithExecTimeout(r.Timeout)}
	if len(r.Env) > 0 {
		opts = append(opts, microsandbox.WithExecEnv(r.Env))
	}
	var out *microsandbox.ExecOutput
	if r.Shell != "" {
		out, err = vm.Shell(ctx, r.Shell, opts...)
	} else {
		out, err = vm.Exec(ctx, r.Cmd, r.Args, opts...)
	}
	if microsandbox.IsKind(err, microsandbox.ErrExecTimeout) {
		return ExecResult{ExitCode: -1, TimedOut: true}, nil
	}
	if err != nil {
		return ExecResult{}, fmt.Errorf("%w: exec", ErrBackend)
	}
	return ExecResult{Stdout: out.StdoutBytes(), Stderr: out.StderrBytes(), ExitCode: out.ExitCode()}, nil
}
func (d *CloudDriver) ReadFile(ctx context.Context, t Target, p string) ([]byte, error) {
	vm, err := d.open(ctx, t)
	if err != nil {
		return nil, err
	}
	defer vm.Close()
	stream, err := vm.FS().ReadStream(ctx, p)
	if microsandbox.IsKind(err, microsandbox.ErrPathNotFound) {
		return nil, ErrFileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%w: read", ErrBackend)
	}
	defer stream.Close()
	limit := d.cfg.MaxFileBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	b, err := io.ReadAll(io.LimitReader(&streamReader{ctx: ctx, stream: stream}, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read stream", ErrBackend)
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

func (d *CloudDriver) WriteFile(ctx context.Context, t Target, p string, b []byte) error {
	vm, err := d.open(ctx, t)
	if err != nil {
		return err
	}
	defer vm.Close()
	if err = vm.FS().Write(ctx, p, b); err != nil {
		return fmt.Errorf("%w: write", ErrBackend)
	}
	return nil
}
func (d *CloudDriver) RemoveFile(ctx context.Context, t Target, p string) error {
	vm, err := d.open(ctx, t)
	if err != nil {
		return err
	}
	defer vm.Close()
	if err = vm.FS().Remove(ctx, p); err != nil {
		return fmt.Errorf("%w: remove file", ErrBackend)
	}
	return nil
}
func (d *CloudDriver) ListDir(ctx context.Context, t Target, p string) ([]FileEntry, error) {
	vm, err := d.open(ctx, t)
	if err != nil {
		return nil, err
	}
	defer vm.Close()
	entries, err := vm.FS().List(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("%w: list", ErrBackend)
	}
	out := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, FileEntry{Name: filepath.Base(e.Path), Path: e.Path, Kind: string(e.Kind), Size: e.Size})
	}
	return out, nil
}
func (d *CloudDriver) Status(ctx context.Context, name string) (string, error) {
	h, err := microsandbox.GetSandbox(ctx, name)
	if microsandbox.IsKind(err, microsandbox.ErrSandboxNotFound) {
		return VMAbsent, nil
	}
	if err != nil {
		return "", fmt.Errorf("%w: status", ErrBackend)
	}
	if h.Status() == microsandbox.SandboxStatusRunning {
		return VMRunning, nil
	}
	return VMStopped, nil
}
func (d *CloudDriver) Stop(ctx context.Context, name string) error {
	h, err := microsandbox.GetSandbox(ctx, name)
	if microsandbox.IsKind(err, microsandbox.ErrSandboxNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: stop", ErrBackend)
	}
	if h.Status() != microsandbox.SandboxStatusRunning {
		return nil
	}
	if err = h.Stop(ctx); err != nil {
		return fmt.Errorf("%w: stop", ErrBackend)
	}
	return nil
}
func (d *CloudDriver) Remove(ctx context.Context, name string) error {
	h, err := microsandbox.GetSandbox(ctx, name)
	if microsandbox.IsKind(err, microsandbox.ErrSandboxNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: remove", ErrBackend)
	}
	if h.Status() == microsandbox.SandboxStatusRunning {
		if err = h.Stop(ctx); err != nil {
			return fmt.Errorf("%w: stop", ErrBackend)
		}
		// 等待 stopped 后才能 Remove；ctx 截止由调用方控制。
		if _, err = h.WaitForStatus(ctx, microsandbox.SandboxStatusStopped); err != nil {
			return fmt.Errorf("%w: wait stopped", ErrBackend)
		}
	}
	if err = h.Remove(ctx); err != nil {
		return fmt.Errorf("%w: remove", ErrBackend)
	}
	return nil
}

var _ Driver = (*CloudDriver)(nil)
