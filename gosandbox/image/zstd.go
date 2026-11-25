// 文件说明：zstd.go — gosandbox/image 中 ZstdReaderFunc、SetZstdReader 等符号的实现
//
// 用法
//
//	type ZstdReaderFunc func(r io.Reader) (io.ReadCloser, error)  // ZstdReaderFunc 是一个 zstd 解压器工厂
//	SetZstdReader(f ZstdReaderFunc)  // SetZstdReader 注册 zstd 解压器
//	同包调用：newZstdReader() → unpack.go
//
// 调用架构
//
//	[上游] gosandbox/vm/box, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/image/zstd.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] arch.go, import.go, manifest.go, manifest_test.go, pull.go 等 16 个
//	[关键引用] newZstdReader ← unpack.go
//
// 主要符号
//
//	var ErrNoZstd
//	type ZstdReaderFunc func(r io.Reader) (io.ReadCloser, error)
//	var zstdMu / zstdReader
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
