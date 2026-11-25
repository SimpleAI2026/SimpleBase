// 文件说明：errors.go — internal/sandbox 中 ErrNotFound、ErrInvalidPath 等符号的实现
//
// 用法
//
//	var ErrNotFound / ErrNameConflict / ErrLimitExceeded / ErrInvalidSpec / ErrFileTooLarge / ErrBusy / ErrGone / ErrAlreadyPaused  // 领域错误；api/error.go 按 errors.Is 映射 HTTP 状态（§7.3）
//	var ErrInvalidPath  // ErrInvalidPath 是 vm.ErrInvalidPath 的别名：「路径必须落在工作目录内」属于沙盒 运行时规则（实现在 gosandbox/vm）...
//
// 调用架构
//
//	[上游] internal/api, internal/app, internal/sandbox/sshd
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sandbox/errors.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] create_test.go, logs.go, manager.go, manager_exec.go, manager_lifecycle.go 等 13 个
//
// 主要符号
//
//	var ErrNotFound / ErrNameConflict / ErrLimitExceeded / ErrInvalidSpec / ErrFileTooLarge / ErrBusy / ErrGone / ErrAlreadyPaused
//	var ErrInvalidPath

package sandbox

import (
	"errors"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// 领域错误；api/error.go 按 errors.Is 映射 HTTP 状态（§7.3）。
// 运行时错误（ErrUnavailable / ErrFileNotFound / ErrInvalidPath / ErrInvalid /
// ErrNotFound / ErrBackend）定义在 gosandbox/vm，api 层按 errors.Is 映射。
var (
	ErrNotFound      = vm.ErrNotFound
	ErrNameConflict  = errors.New("sandbox: name already exists")
	ErrLimitExceeded = errors.New("sandbox: project sandbox limit exceeded")
	ErrInvalidSpec   = vm.ErrInvalid
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
