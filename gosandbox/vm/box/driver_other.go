//go:build !linux

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
