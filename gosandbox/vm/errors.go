// 文件说明：errors.go — gosandbox/vm 中 ErrUnavailable、ErrFileNotFound 等符号的实现
//
// 用法
//
//	包内实现细节，由上层包经包级 API 间接触达。
//
// 调用架构
//
//	[上游] gosandbox/vm/fake, gosandbox/vm/local, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/errors.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] driver.go, logged.go
//
// 主要符号
//
//	var ErrUnavailable
//	var ErrFileNotFound
//	var ErrBackend
//	var ErrNotSupported

package vm

import "errors"

// 驱动侧错误。internal/sandbox 把同一批值再导出，api 层按 errors.Is 映射 HTTP。
var (
	ErrUnavailable  = errors.New("sandbox: sandbox is not available")
	ErrFileNotFound = errors.New("sandbox: file not found")
	ErrBackend      = errors.New("sandbox: backend error")
)

// ErrNotSupported 表示后端不支持请求的能力（例如不能提供 PTY）。
var ErrNotSupported = errors.New("sandbox: not supported by this backend")
