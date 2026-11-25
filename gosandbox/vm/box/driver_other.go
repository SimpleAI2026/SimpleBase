//go:build !linux

// 文件说明：driver_other.go — gosandbox/vm/box 中 Driver、New 等符号的实现
//
// 用法
//
//	type Driver struct{…}  // Driver 在非 Linux 宿主上不可用，因为 agent 依赖 Linux namespace
//	New(context.Context, Options) (*Driver, error)
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/driver_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] capabilities.go, contracts_linux_test.go, diskuse_linux.go, driver_linux.go, driver_test.go 等 15 个
//
// 主要符号
//
//	type Driver struct{…}
//	New(context.Context, Options) (*Driver, error)
//	(d *Driver) Ensure(context.Context, vm.Target) error
//	(d *Driver) Exec(context.Context, vm.Target, vm.ExecSpec) (vm.ExecResult, error)
//	(d *Driver) ReadFile(context.Context, vm.Target, string) ([]byte, error)
//	(d *Driver) WriteFile(context.Context, vm.Target, string, []byte) error
//	(d *Driver) RemoveFile(context.Context, vm.Target, string) error
//	(d *Driver) ListDir(context.Context, vm.Target, string) ([]vm.FileEntry, error)
//	... 另有 3 个顶层符号

package box

import (
	"context"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// Driver 在非 Linux 宿主上不可用，因为 agent 依赖 Linux namespace。
type Driver struct{}

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

var _ vm.Driver = (*Driver)(nil)
