// 文件说明：errors.go — gosandbox/vm 中 ErrUnavailable、ErrNotSupported 等符号的实现
//
// 用法
//
//	var ErrUnavailable / ErrFileNotFound / ErrBackend / ErrNotFound / ErrInvalid  // 驱动侧错误，调用方按 errors.Is 分类并映射到自己的错误协议
//	var ErrNotSupported  // ErrNotSupported 表示后端不支持请求的能力（例如不能提供 PTY）
//
// 调用架构
//
//	[上游] gosandbox/vm/box, gosandbox/vm/fake, gosandbox/vm/local, internal/api 等 7 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/errors.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] driver.go, env.go, env_test.go, output.go, output_test.go 等 9 个
//
// 主要符号
//
//	var ErrUnavailable / ErrFileNotFound / ErrBackend / ErrNotFound / ErrInvalid
//	var ErrNotSupported

package vm

import "errors"

// 驱动侧错误，调用方按 errors.Is 分类并映射到自己的错误协议。
var (
	ErrUnavailable  = errors.New("sandbox: sandbox is not available")
	ErrFileNotFound = errors.New("sandbox: file not found")
	ErrBackend      = errors.New("sandbox: backend error")
	// ErrNotFound 表示沙盒或快照不存在。
	ErrNotFound = errors.New("sandbox: not found")
	// ErrInvalid 表示请求在当前状态下不合法（例如往运行中的沙盒 fork）。
	// 与 ErrBackend 分开：这类是调用方的错（4xx），不是后端故障（5xx）。
	ErrInvalid = errors.New("sandbox: invalid request")
)

// ErrNotSupported 表示后端不支持请求的能力（例如不能提供 PTY）。
var ErrNotSupported = errors.New("sandbox: not supported by this backend")
