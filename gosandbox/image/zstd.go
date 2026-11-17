// 文件说明：zstd.go — zstd 层的解压钩子：默认为不可用，可由调用方注入
//
// 为什么是钩子而不是直接依赖：
// zstd 层（OCI 的 `+zstd` media type）在标准库里没有解压实现，引入
// klauspost/compress 会新增一个直接依赖。绝大多数镜像用 gzip，所以这里
// 留一个可注入点：需要 zstd 的部署自己 SetZstdReader，其余情况给出明确错误，
// 而不是静默跳过层（跳过会导致根目录内容缺失，症状极难排查）。
//
// 用法
//
//	image.SetZstdReader(func(r io.Reader) (io.ReadCloser, error) { … })
//
// 调用架构
//
//	[上游] gosandbox/image（unpack）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/image/zstd.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] unpack.go
//
// 主要符号
//
//	type ZstdReaderFunc func(r io.Reader) (io.ReadCloser, error)
//	SetZstdReader(f ZstdReaderFunc)
//	newZstdReader(r io.Reader) (io.ReadCloser, error)

package image

import (
	"errors"
	"fmt"
	"io"
	"sync"
)

// ErrNoZstd 表示 zstd 解压器没有注册。
var ErrNoZstd = errors.New("image: zstd layers are not supported in this build")

// ZstdReaderFunc 是一个 zstd 解压器工厂。
type ZstdReaderFunc func(r io.Reader) (io.ReadCloser, error)

var (
	zstdMu     sync.RWMutex
	zstdReader ZstdReaderFunc
)

// SetZstdReader 注册 zstd 解压器。传 nil 表示取消注册。
func SetZstdReader(f ZstdReaderFunc) {
	zstdMu.Lock()
	defer zstdMu.Unlock()
	zstdReader = f
}

// newZstdReader 用注册的解压器包一层；没注册就报错。
func newZstdReader(r io.Reader) (io.ReadCloser, error) {
	zstdMu.RLock()
	f := zstdReader
	zstdMu.RUnlock()
	if f == nil {
		return nil, fmt.Errorf("%w (register one with image.SetZstdReader)", ErrNoZstd)
	}
	return f(r)
}
