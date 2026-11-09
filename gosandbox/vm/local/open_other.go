//go:build !unix

// 文件说明：open_other.go — gosandbox/vm/local 的内部实现（nonBlock 等）
//
// 用法
//
//	同包调用：nonBlock → local.go, open_unix.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/local/open_other.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] local.go, open_unix.go
//	[关键引用] nonBlock ← local.go, open_unix.go
//
// 主要符号
//
//	const nonBlock

package local

const nonBlock = 0
