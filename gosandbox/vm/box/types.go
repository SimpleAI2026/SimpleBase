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
