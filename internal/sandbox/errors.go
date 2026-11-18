// 文件说明：errors.go — 云沙盒的业务层（planv4.0 cloud-sandbox-plan §4）
//
// 用法
//
//	同包调用：ErrInvalidSpec → manager.go, paths.go 等；ErrLimitExceeded → manager.go, proc.go；ErrBusy → manager.go, proc.go
//
// 调用架构
//
//	[上游] internal/api, internal/app, internal/sandbox/sshd
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sandbox/errors.go
//	         │
//	         ▼
//	[下游] gosandbox/vm（ErrInvalidPath 的别名来源）
//	[同包] manager.go, paths.go, proc.go
//	[关键引用] ErrInvalidSpec ← manager.go, paths.go, proc.go
//
// 主要符号
//
//	var ErrNotFound
//	var ErrNameConflict
//	var ErrLimitExceeded
//	var ErrInvalidSpec
//	var ErrInvalidPath
//	var ErrFileTooLarge
//	... 另有 2 个顶层符号

package sandbox

import (
	"errors"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// 领域错误；api/error.go 按 errors.Is 映射 HTTP 状态（§7.3）。
// 驱动侧错误（ErrUnavailable / ErrFileNotFound / ErrInvalidPath / ErrBackend）定义在
// gosandbox/vm，api 层按 errors.Is 映射时直接引用那些值。
var (
	ErrNotFound      = errors.New("sandbox: not found")
	ErrNameConflict  = errors.New("sandbox: name already exists")
	ErrLimitExceeded = errors.New("sandbox: project sandbox limit exceeded")
	ErrInvalidSpec   = errors.New("sandbox: invalid spec")
	ErrFileTooLarge  = errors.New("sandbox: file too large")
	ErrBusy          = errors.New("sandbox: busy")
	ErrGone          = errors.New("sandbox: gone")
	// ErrAlreadyPaused 表示对已冻结的沙盒再次 pause（E2B 返回 409）。
	ErrAlreadyPaused = errors.New("sandbox: already paused")
)

// ErrInvalidPath 是 vm.ErrInvalidPath 的别名：「路径必须落在工作目录内」属于沙盒
// 运行时规则（实现在 gosandbox/vm），业务层不再自己定义一份。别名让 api 层继续按
// errors.Is(err, sandbox.ErrInvalidPath) 映射 400。
var ErrInvalidPath = vm.ErrInvalidPath
