// Package hashx 提供无密钥 SHA-256 摘要的十六进制封装。
//
// 只允许 import 标准库。带密钥的 HMAC（auth.HashKey）与多字段拼接指纹
// （llmgateway.credFingerprint）语义不同，不在此包内。
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
