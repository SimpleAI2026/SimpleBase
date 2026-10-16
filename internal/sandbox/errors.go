// Package sandbox 是云沙盒的业务层（planv4.0 cloud-sandbox-plan §4）。
//
// 执行后端（Driver、cloud、fake）已独立到 gosandbox/vm（planv6.2）；
// 本包只保留 Store 元数据接口与 Manager（资源、限额、路径、锁、状态机、reaper）。
// fake 驱动仅供 e2e / 单测使用（须 SIMPLEBASE_SANDBOX_ALLOW_FAKE=1）。
package sandbox

import "errors"

// 领域错误；api/error.go 按 errors.Is 映射 HTTP 状态（§7.3）。
// 驱动侧错误（ErrUnavailable / ErrFileNotFound / ErrBackend）定义在 gosandbox/vm，
// api 层按 errors.Is 映射时直接引用那些值。
var (
	ErrNotFound      = errors.New("sandbox: not found")
	ErrNameConflict  = errors.New("sandbox: name already exists")
	ErrLimitExceeded = errors.New("sandbox: project sandbox limit exceeded")
	ErrInvalidSpec   = errors.New("sandbox: invalid spec")
	ErrInvalidPath   = errors.New("sandbox: invalid path")
	ErrFileTooLarge  = errors.New("sandbox: file too large")
	ErrBusy          = errors.New("sandbox: busy")
	ErrGone          = errors.New("sandbox: gone")
)
