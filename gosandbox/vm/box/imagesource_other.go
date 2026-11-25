//go:build !linux

// 文件说明：imagesource_other.go — gosandbox/vm/box 中 ImageStore、NewImageSource 等符号的实现
//
// 用法
//
//	type ImageStore struct{…}  // ImageStore 是非 Linux 上的占位：镜像沙盒需要 Linux 的 namespace 与 overlay
//	NewImageSource(any) *ImageStore  // NewImageSource 返回一个恒失败的镜像来源
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/imagesource_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] capabilities.go, contracts_linux_test.go, diskuse_linux.go, driver_linux.go, driver_other.go 等 15 个
//
// 主要符号
//
//	type ImageStore struct{…}
//	NewImageSource(any) *ImageStore
//	(s *ImageStore) Resolve(context.Context, string) (Lower, error)

package box

import (
	"context"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// ImageStore 是非 Linux 上的占位：镜像沙盒需要 Linux 的 namespace 与 overlay。
type ImageStore struct{}

// NewImageSource 返回一个恒失败的镜像来源。
func NewImageSource(any) *ImageStore { return &ImageStore{} }

// Resolve 恒不可用（fail-closed，不退回宿主根）。
func (s *ImageStore) Resolve(context.Context, string) (Lower, error) {
	return Lower{}, vm.ErrUnavailable
}
