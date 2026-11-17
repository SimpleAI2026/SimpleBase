//go:build !linux

// 文件说明：driver_other.go — 非 Linux 上的 box 后端：恒不可用（fail-closed）
//
// 用法
//
//	New(ctx, opts)  // 恒返回 vm.ErrUnavailable
//
// 调用架构
//
//	[上游] internal/app（装配）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/driver_other.go
//	         │
//	         ▼
//	[下游] gosandbox/vm
//	[同包] driver_linux.go
//
// 主要符号
//
//	type Options struct {…}
//	type Policy struct {…}
//	type Driver struct{…}
//	New(ctx context.Context, opts Options) (*Driver, error)

package box

import (
	"context"
	"io"
	"syscall"

	"github.com/linkxzhou/SimpleBase/gosandbox/netx"
	"github.com/linkxzhou/SimpleBase/gosandbox/snapshot"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// Options 与 Linux 实现同形，便于上层跨平台写配置。
type Options struct {
	Root           string
	Workdir        string
	Helper         string
	Bwrap          string
	MaxOutputBytes int
	MaxFileBytes   int
	PidsMax        int64
	MemoryMiB      int
	Seccomp        string
	Policy         Policy
	Log            vm.LogFunc
	// Images 是镜像来源；非 Linux 上恒不可用。
	Images ImageSource
	// Supervise 是 supervisor 托管开关；非 Linux 上恒不可用。
	Supervise bool
	// Snapshots 是快照 store；非 Linux 上恒不可用。
	Snapshots *snapshot.Store
	// Network 是出网策略；非 Linux 上恒不可用。
	Network netx.Policy
	// DiskLimitBytes 是磁盘上限；非 Linux 上恒不可用。
	DiskLimitBytes int64
}

// ImageSource 是驱动需要的镜像能力（非 Linux 桩）。
type ImageSource interface {
	Resolve(ctx context.Context, ref string) (Lower, error)
}

// Lower 是一个解包好的只读根。
type Lower struct {
	Path    string
	ChainID string
}

// Policy 是给沙盒内 Landlock 的附加规则。
type Policy struct {
	DenyRead []string
}

// Driver 在非 Linux 上不存在：agent 只跑在 Linux 沙盒里。
//
// 仍然实现 vm.Driver（所有方法返回 vm.ErrUnavailable），这样装配代码不必
// 按平台分叉，而运行时行为是 fail-closed：绝不退回裸执行。
type Driver struct{}

// New 在非 Linux 上直接失败。
func New(context.Context, Options) (*Driver, error) { return nil, vm.ErrUnavailable }

func (d *Driver) Ensure(context.Context, vm.Target) error { return vm.ErrUnavailable }
func (d *Driver) Exec(context.Context, vm.Target, vm.ExecSpec) (vm.ExecResult, error) {
	return vm.ExecResult{}, vm.ErrUnavailable
}
func (d *Driver) ReadFile(context.Context, vm.Target, string) ([]byte, error) {
	return nil, vm.ErrUnavailable
}
func (d *Driver) WriteFile(context.Context, vm.Target, string, []byte) error {
	return vm.ErrUnavailable
}
func (d *Driver) RemoveFile(context.Context, vm.Target, string) error { return vm.ErrUnavailable }
func (d *Driver) ListDir(context.Context, vm.Target, string) ([]vm.FileEntry, error) {
	return nil, vm.ErrUnavailable
}
func (d *Driver) Status(context.Context, string) (string, error) { return "", vm.ErrUnavailable }
func (d *Driver) Stop(context.Context, string) error             { return vm.ErrUnavailable }
func (d *Driver) Remove(context.Context, string) error           { return vm.ErrUnavailable }

// Start 实现 vm.Interactive：同样不可用。
func (d *Driver) Start(context.Context, vm.Target, vm.StartSpec) (vm.Proc, error) {
	return nil, vm.ErrUnavailable
}

// Metrics / Pause / Resume / CanFreeze 同样不可用。
func (d *Driver) Metrics(context.Context, string) (vm.Metrics, error) {
	return vm.Metrics{}, vm.ErrNotSupported
}
func (d *Driver) Pause(context.Context, string) error     { return vm.ErrUnavailable }
func (d *Driver) Resume(context.Context, vm.Target) error { return vm.ErrUnavailable }
func (d *Driver) CanFreeze() bool                         { return false }

var (
	_ vm.Driver      = (*Driver)(nil)
	_ vm.Interactive = (*Driver)(nil)
	_ vm.Meter       = (*Driver)(nil)
	_ vm.Pauser      = (*Driver)(nil)
	_ vm.Freezer     = (*Driver)(nil)
)

// 引用一下未用到的导入（保持与 Linux 实现一致的签名集合）。
var _ = io.Discard
var _ = syscall.SIGTERM
