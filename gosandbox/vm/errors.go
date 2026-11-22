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
