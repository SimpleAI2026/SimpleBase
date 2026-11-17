//go:build !linux

// 文件说明：imagesource_other.go — 非 Linux 上没有镜像来源
//
// 用法
//
//	NewImageSource(p)  // 返回一个恒失败的实现
//
// 调用架构
//
//	[上游] internal/app（装配）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/imagesource_other.go
//	         │
//	         ▼
//	[下游] gosandbox/vm
//	[同包] driver_other.go
//
// 主要符号
//
//	NewImageSource(p any) *ImageStore

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
