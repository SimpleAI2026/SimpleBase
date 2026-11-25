// 文件说明：limit.go — godatabase/limit 中 Normalize、Clamp 等符号的实现
//
// 用法
//
//	Normalize(n, def, max int) int  // Normalize returns def when n is non-positive or exceeds max. A non-positive max ...
//	Clamp(n, def, max int) int  // Clamp replaces a non-positive n with def and caps n at max. A non-positive max d...
//
// 调用架构
//
//	[上游] godatabase/blob, internal/audit, internal/auth, internal/catalog 等 7 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/limit/limit.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] limit_test.go
//	[关键引用] Normalize ← limit_test.go；Clamp ← limit_test.go
//
// 主要符号
//
//	Normalize(n, def, max int) int
//	Clamp(n, def, max int) int

// Package limit normalizes pagination limits.
package limit

// Normalize returns def when n is non-positive or exceeds max.
// A non-positive max disables the upper bound.
func Normalize(n, def, max int) int {
	if n <= 0 || (max > 0 && n > max) {
		return def
	}
	return n
}

// Clamp replaces a non-positive n with def and caps n at max.
// A non-positive max disables the upper bound.
func Clamp(n, def, max int) int {
	if n <= 0 {
		n = def
	}
	if max > 0 && n > max {
		return max
	}
	return n
}
