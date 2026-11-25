// 文件说明：hashx.go — godatabase/hashx 中 Sum、SumString 等符号的实现
//
// 用法
//
//	Sum(data []byte) string  // Sum returns the full SHA-256 digest of data.
//	SumString(s string) string  // SumString returns the full SHA-256 digest of s.
//	Short(data []byte, n int) string  // Short returns the hexadecimal encoding of the first n digest bytes. It returns a...
//
// 调用架构
//
//	[上游] godatabase/blob, godatabase/ducklake, internal/api, internal/auth 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/hashx/hashx.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] hashx_test.go
//	[关键引用] Sum ← hashx_test.go；SumString ← hashx_test.go；Short ← hashx_test.go
//
// 主要符号
//
//	Sum(data []byte) string
//	SumString(s string) string
//	Short(data []byte, n int) string

// Package hashx provides hexadecimal SHA-256 digests.
package hashx

import (
	"crypto/sha256"
	"encoding/hex"
)

// Sum returns the full SHA-256 digest of data.
func Sum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// SumString returns the full SHA-256 digest of s.
func SumString(s string) string {
	return Sum([]byte(s))
}

// Short returns the hexadecimal encoding of the first n digest bytes.
// It returns an empty string for n <= 0 and caps n at 32.
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
