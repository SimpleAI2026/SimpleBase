package sandbox

import (
	"context"
	"time"
)

// 驱动类型。
const (
	DriverCloud = "cloud"
	DriverFake  = "fake"
)

// 驱动侧 VM 状态。
const (
	VMRunning = "running"
	VMStopped = "stopped"
	VMAbsent  = "absent"
)

// Spec 是创建 VM 时的规格；已存在的 VM 以云端持久配置为准。
type Spec struct {
	Image       string
	CPUs        int
	MemoryMiB   int
	Network     string // none | public
	Workdir     string
	IdleTimeout time.Duration
	MaxDuration time.Duration
	Labels      map[string]string
}

// Target 定位一个 VM：云端名 + 懒创建所需规格。
type Target struct {
	Name string
	Spec Spec
}

// ExecSpec 是一次命令执行请求。Shell 与 Cmd 二选一。
type ExecSpec struct {
	Cmd     string
	Args    []string
	Shell   string // 经 /bin/sh -c
	Env     map[string]string
	Cwd     string
	Timeout time.Duration
}

// ExecResult 是驱动返回的原始结果（未截断）。
// 超时不是 error：TimedOut=true、ExitCode=-1。
type ExecResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	TimedOut bool
}

// FileEntry 是单层目录项。
type FileEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"` // file | directory | symlink | other
	Size int64  `json:"size"`
}

// Driver 是执行后端。所有需要 VM 的操作都按 Target 懒创建 / 接回，
// 操作完成即释放句柄，不保活（§3.3）。
type Driver interface {
	Kind() string
	Ensure(ctx context.Context, t Target) error
	Exec(ctx context.Context, t Target, req ExecSpec) (ExecResult, error)
	ReadFile(ctx context.Context, t Target, path string) ([]byte, error)
	WriteFile(ctx context.Context, t Target, path string, data []byte) error
	RemoveFile(ctx context.Context, t Target, path string) error
	ListDir(ctx context.Context, t Target, path string) ([]FileEntry, error)
	Status(ctx context.Context, name string) (string, error)
	Stop(ctx context.Context, name string) error
	// Remove 停止并删除 VM；不存在视为成功。
	Remove(ctx context.Context, name string) error
}
