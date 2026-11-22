package image

import (
	"crypto/sha256"
	"encoding/hex"
)

// sha256Sum 返回字符串的 sha256 十六进制摘要（不含前缀）。
func sha256Sum(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
