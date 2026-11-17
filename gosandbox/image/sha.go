// 文件说明：sha.go — 摘要计算的小工具
//
// 用法
//
//	sha256Sum(s string) string  // 返回十六进制摘要（不含 "sha256:" 前缀）
//
// 调用架构
//
//	[上游] gosandbox/image（unpack 的 ChainID）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/image/sha.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] store.go, unpack.go
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
