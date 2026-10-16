package vm

import "errors"

// 驱动侧错误。internal/sandbox 把同一批值再导出，api 层按 errors.Is 映射 HTTP。
var (
	ErrUnavailable  = errors.New("sandbox: cloud sandbox is not available")
	ErrFileNotFound = errors.New("sandbox: file not found")
	ErrBackend      = errors.New("sandbox: backend error")
)
