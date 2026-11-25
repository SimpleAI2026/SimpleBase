// 文件说明：executil.go — gosandbox/internal/executil 中 CappedBuffer、NewCappedBuffer 等符号的实现
//
// 用法
//
//	type CappedBuffer struct{…}  // CappedBuffer 保留输出的前 max 字节，丢弃其余内容但始终接受写入
//	NewCappedBuffer(max int) *CappedBuffer
//	HasEnv(env []string, key string) bool
//
// 调用架构
//
//	[上游] gosandbox/host, gosandbox/vm/box, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/internal/executil/executil.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] executil_test.go
//	[关键引用] NewCappedBuffer ← executil_test.go；HasEnv ← executil_test.go
//
// 主要符号
//
//	type CappedBuffer struct{…}
//	NewCappedBuffer(max int) *CappedBuffer
//	(b *CappedBuffer) Write(p []byte) (int, error)
//	(b *CappedBuffer) Bytes() []byte
//	(b *CappedBuffer) Truncated() bool
//	HasEnv(env []string, key string) bool

package executil

import "strings"

// CappedBuffer 保留输出的前 max 字节，丢弃其余内容但始终接受写入。
// max <= 0 表示不限，避免调用方为无限输出设置额外分支。
type CappedBuffer struct {
	buf       []byte
	max       int
	truncated bool
}

func NewCappedBuffer(max int) *CappedBuffer { return &CappedBuffer{max: max} }

func (b *CappedBuffer) Write(p []byte) (int, error) {
	written := len(p)
	if b.max > 0 {
		room := b.max - len(b.buf)
		if room <= 0 {
			b.truncated = true
			return len(p), nil
		}
		if len(p) > room {
			p = p[:room]
			b.truncated = true
		}
	}
	b.buf = append(b.buf, p...)
	return written, nil
}

func (b *CappedBuffer) Bytes() []byte   { return b.buf }
func (b *CappedBuffer) Truncated() bool { return b.truncated }

func HasEnv(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}
