//go:build linux

// 文件说明：files_linux.go — gosandbox/vm/box 的内部实现（ReadFile、WriteFile 等）
//
// 用法
//
//	同包调用：kindOf() → capabilities.go, contracts_linux_test.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/files_linux.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] capabilities.go, contracts_linux_test.go, diskuse_linux.go, driver_linux.go, driver_other.go 等 15 个
//	[关键引用] kindOf ← capabilities.go, contracts_linux_test.go
//
// 主要符号
//
//	(d *Driver) ReadFile(ctx context.Context, t vm.Target, p string) ([]byte, error)
//	(d *Driver) WriteFile(ctx context.Context, t vm.Target, p string, data []byte) error
//	(d *Driver) RemoveFile(ctx context.Context, t vm.Target, p string) error
//	(d *Driver) ListDir(ctx context.Context, t vm.Target, p string) ([]vm.FileEntry, error)
//	kindOf(k proto.FileKind) string
//	isNotExist(err error) bool

package box

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// ReadFile 读沙盒内的文件。
func (d *Driver) ReadFile(ctx context.Context, t vm.Target, p string) ([]byte, error) {
	p, err := vm.CleanPath(d.workdir(t), p, false)
	if err != nil {
		return nil, err
	}
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
	p, err := vm.CleanPath(d.workdir(t), p, false)
	if err != nil {
		return err
	}
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
	p, err := vm.CleanPath(d.workdir(t), p, false)
	if err != nil {
		return err
	}
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
	if p == "" {
		p = d.workdir(t)
	}
	p, err := vm.CleanPath(d.workdir(t), p, true)
	if err != nil {
		return nil, err
	}
	s, err := d.ensure(ctx, t)
	if err != nil {
		return nil, err
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

func isNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "no such file")
}
