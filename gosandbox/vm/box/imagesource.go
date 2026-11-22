//go:build linux

package box

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/linkxzhou/SimpleBase/gosandbox/image"
)

// ImageStore 把 image.Puller 适配成驱动的 ImageSource。
//
// 解析只做「已在 store 里且已解包」的镜像：拉取是显式动作（CLI 或 API），
// 不应该在用户启动沙盒时悄悄下几百 MB。
type ImageStore struct {
	puller *image.Puller
	// mu 保护 resolved：同一引用的并发解析只解包一次。
	mu       sync.Mutex
	resolved map[string]Lower
}

// NewImageSource 建一个 ImageStore。
func NewImageSource(p *image.Puller) *ImageStore {
	return &ImageStore{puller: p, resolved: map[string]Lower{}}
}

// Resolve 返回一个镜像的只读根。
//
// 本地没有该镜像时返回明确错误，提示先拉取，而不是自动去 registry 下载。
func (s *ImageStore) Resolve(ctx context.Context, ref string) (Lower, error) {
	if ref == "" {
		return Lower{}, errors.New("box: empty image reference")
	}
	s.mu.Lock()
	if l, ok := s.resolved[ref]; ok {
		s.mu.Unlock()
		return l, nil
	}
	s.mu.Unlock()

	img, err := s.puller.Store().Find(ref)
	if err != nil {
		if errors.Is(err, image.ErrImageNotFound) {
			return Lower{}, fmt.Errorf("image %q is not pulled yet: pull it first (image: ErrImageNotFound)", ref)
		}
		return Lower{}, err
	}
	chainID, err := s.puller.Unpack(ctx, img)
	if err != nil {
		return Lower{}, err
	}
	lower := Lower{Path: s.puller.LowerPath(chainID), ChainID: chainID}
	s.mu.Lock()
	s.resolved[ref] = lower
	s.mu.Unlock()
	return lower, nil
}

// Puller 暴露底层 Puller（供 CLI 拉取与 GC）。
func (s *ImageStore) Puller() *image.Puller { return s.puller }
