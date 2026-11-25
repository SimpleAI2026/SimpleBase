// 文件说明：sha.go — gosandbox/image 的内部实现（sha256Sum 等）
//
// 用法
//
//	同包调用：sha256Sum() → unpack.go
//
// 调用架构
//
//	[上游] gosandbox/vm/box, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/image/sha.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] arch.go, import.go, manifest.go, manifest_test.go, pull.go 等 16 个
//	[关键引用] sha256Sum ← unpack.go
//
// 主要符号
//
//	sha256Sum(s string) string

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
