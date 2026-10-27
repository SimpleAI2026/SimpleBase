// 文件说明：driver.go — gosandbox/vm 中 VMRunning、VMStopped 等符号的实现
//
// 用法
//
//	type Spec（导出类型，同包与上层均可使用）
//	type Target（导出类型，同包与上层均可使用）
//	同包调用：Spec → logged.go；Target → logged.go；ExecSpec → logged.go
//
// 调用架构
//
//	[上游] gosandbox/vm/fake, gosandbox/vm/local, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/driver.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] errors.go, logged.go
//	[关键引用] Spec ← logged.go
//
// 主要符号
//
//	const VMRunning
//	const VMStopped
//	const VMAbsent
//	type Spec struct {…}
//	type Target struct {…}
//	type ExecSpec struct {…}
//	... 另有 8 个顶层符号

package vm

import (
	"context"
	"io"
	"syscall"
	"time"
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
	// Mode 是 Unix 权限位（含类型位之外的低 12 位）；ModTime 是最后修改时间。零值表示驱动没有提供。
	Mode    uint32    `json:"-"`
	ModTime time.Time `json:"-"`
}

// Driver 是执行后端。所有需要 VM 的操作都按 Target 懒创建 / 接回，
// 操作完成即释放句柄，不保活（§3.3）。
type Driver interface {
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

// PTYSize 是伪终端大小。
type PTYSize struct{ Cols, Rows uint16 }

// StartSpec 描述一个需要保持句柄的进程。Cmd+Args 与 Shell 二选一，语义同 ExecSpec；
// 不含 Timeout：生命周期由 ctx 与 Proc.Close 控制。
type StartSpec struct {
	Cmd   string
	Args  []string
	Shell string
	Env   map[string]string
	Cwd   string
	// PTY 为 nil 时用管道（stdout 与 stderr 合并为一路）；非 nil 时分配伪终端。
	PTY *PTYSize
}

// ProcExit 是进程结束信息。
type ProcExit struct {
	ExitCode int
	Signaled bool
}

// Proc 是正在运行的进程。Output 是单一合并流，调用方必须持续读取（写端有背压，不在内存里堆积）。
type Proc interface {
	PID() int
	Stdin() io.WriteCloser
	Output() io.Reader
	Resize(PTYSize) error
	// Signal 只支持 SIGTERM 与 SIGKILL。
	Signal(sig syscall.Signal) error
	Wait() ProcExit
	Done() <-chan struct{}
	// Close 终止进程（若仍在运行）并释放资源，可重复调用。
	Close() error
}

// Interactive 是 Driver 的可选能力：启动后保持句柄的进程（终端、后台进程、流式输出）。
// 不实现它的后端，上层必须关闭终端与 process.Start 能力，而不是退化成同步执行。
type Interactive interface {
	Start(ctx context.Context, t Target, req StartSpec) (Proc, error)
}
