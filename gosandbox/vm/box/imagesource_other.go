//go:build !linux

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
