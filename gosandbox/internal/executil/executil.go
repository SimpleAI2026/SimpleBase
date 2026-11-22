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
