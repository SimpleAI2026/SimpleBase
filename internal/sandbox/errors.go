// Package sandbox 是云沙盒的业务层（planv4.0 cloud-sandbox-plan §4）。
//
// SDK 调用仅集中在 driver_cloud.go；保留 v3 client.go 兼容既有单测。
// 这里定义 Driver（执行后端）、Store（元数据）接口与 Manager（资源、限额、
// 路径、锁、状态机、reaper）。fake 驱动供 dev_mode / e2e / 单测使用。
package sandbox

import "errors"

// 领域错误；api/error.go 按 errors.Is 映射 HTTP 状态（§7.3）。
var (
	ErrNotFound      = errors.New("sandbox: not found")
	ErrNameConflict  = errors.New("sandbox: name already exists")
	ErrLimitExceeded = errors.New("sandbox: project sandbox limit exceeded")
	ErrInvalidSpec   = errors.New("sandbox: invalid spec")
	ErrInvalidPath   = errors.New("sandbox: invalid path")
	ErrFileTooLarge  = errors.New("sandbox: file too large")
	ErrFileNotFound  = errors.New("sandbox: file not found")
	ErrBusy          = errors.New("sandbox: busy")
	ErrGone          = errors.New("sandbox: gone")
	ErrBackend       = errors.New("sandbox: backend error")
)
