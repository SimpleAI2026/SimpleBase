// 文件说明：hashx.go — 无密钥 SHA-256 摘要的十六进制封装
//
// 用法
//
//	Sum(data []byte) string  // Sum 返回 data 的 SHA-256 全量十六进制（64 字符）
//	SumString(s string) string  // SumString 与 Sum 相同，入参为字符串
//
// 调用架构
//
//	[上游] internal/api, internal/auth
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/utils/hashx/hashx.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//
// 主要符号
//
//	Sum(data []byte) string
//	SumString(s string) string
//	Short(data []byte, n int) string

package hashx

import (
	"crypto/sha256"
	"encoding/hex"
)

// Sum 返回 data 的 SHA-256 全量十六进制（64 字符）。
func Sum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// SumString 与 Sum 相同，入参为字符串。
func SumString(s string) string {
	return Sum([]byte(s))
}

// Short 返回 SHA-256 前 n 个字节的十六进制（2n 字符）。
// n <= 0 返回空串；n 超过 32 时按 32 处理。
func Short(data []byte, n int) string {
	if n <= 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	if n > len(sum) {
		n = len(sum)
	}
	return hex.EncodeToString(sum[:n])
}
