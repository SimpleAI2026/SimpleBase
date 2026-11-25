// 文件说明：types.go — gosandbox/vm/box 中 Options、ImageSource 等符号的实现
//
// 用法
//
//	type Options struct{…}  // Options 是常驻 agent 驱动的显式配置
//	type ImageSource interface{…}  // ImageSource 把镜像引用解析为只读文件系统根
//	type Lower struct{…}  // Lower 标识一个已解包的只读根
//	type Policy struct{…}  // Policy 是沙盒内 Landlock 的附加规则
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/box/types.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] capabilities.go, contracts_linux_test.go, diskuse_linux.go, driver_linux.go, driver_other.go 等 15 个
//	[关键引用] Options ← contracts_linux_test.go, driver_linux.go；Lower ← driver_linux.go, driver_test.go；Policy ← driver_linux.go, metrics_linux.go
//
// 主要符号
//
//	type Options struct{…}
//	type ImageSource interface{…}
//	type Lower struct{…}
//	type Policy struct{…}

package box

import (
	"context"

	"github.com/linkxzhou/SimpleBase/gosandbox/netx"
	"github.com/linkxzhou/SimpleBase/gosandbox/snapshot"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// Options 是常驻 agent 驱动的显式配置。
type Options struct {
	// Root 是所有沙盒目录的父目录，必须是绝对路径。
	Root string
	// Workdir 是沙盒内工作目录；空为 /workspace。
	Workdir string
	// Helper 是 gosandbox-inner 的绝对路径。
	Helper string
	// Bwrap 是 bwrap 的绝对路径；空表示不隔离，仅供测试。
	Bwrap          string
	MaxOutputBytes int
	// MaxFileBytes 是单次 ReadFile 上限；0 为 1 MiB。
	MaxFileBytes int
	PidsMax      int64
	MemoryMiB    int
	// Seccomp 是 seccomp 档位；空为 default。
	Seccomp string
	Policy  Policy
	Log     vm.LogFunc
	// Network 的零值不出网。
	Network        netx.Policy
	DiskLimitBytes int64
	Supervise      bool
	// Snapshots 为 nil 时不支持快照与 fork。
	Snapshots *snapshot.Store
	// Images 为 nil 时只支持 image=host。
	Images ImageSource
}

// ImageSource 把镜像引用解析为只读文件系统根。
type ImageSource interface {
	Resolve(ctx context.Context, ref string) (Lower, error)
}

// Lower 标识一个已解包的只读根。
type Lower struct {
	Path    string
	ChainID string
}

// Policy 是沙盒内 Landlock 的附加规则。
type Policy struct {
	DenyRead []string
}
