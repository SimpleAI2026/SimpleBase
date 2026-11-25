// 文件说明：driver.go — gosandbox/vm 中 LogFunc、Spec 等符号的实现
//
// 用法
//
//	type LogFunc func(level, msg string, kv ...any)  // LogFunc 接收驱动日志，避免模块依赖具体日志库
//	type Spec struct{…}  // Spec 是创建 VM 时的规格；已存在的 VM 以云端持久配置为准
//	type Target struct{…}  // Target 定位一个 VM：云端名 + 懒创建所需规格
//	type ExecSpec struct{…}  // ExecSpec 是一次命令执行请求
//	type ExecResult struct{…}  // ExecResult 是驱动返回的执行结果；输出可能受后端配置的字节限额约束
//	type FileEntry struct{…}  // FileEntry 是单层目录项
//
// 调用架构
//
//	[上游] gosandbox/vm/box, gosandbox/vm/fake, gosandbox/vm/local, internal/api 等 7 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/driver.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] env.go, env_test.go, errors.go, output.go, output_test.go 等 9 个
//
// 主要符号
//
//	type LogFunc func(level, msg string, kv ...any)
//	const VMRunning / VMStopped / VMAbsent
//	type Spec struct{…}
//	type Target struct{…}
//	type ExecSpec struct{…}
//	type ExecResult struct{…}
//	type FileEntry struct{…}
//	type Driver interface{…}
//	... 另有 19 个顶层符号

package vm

import (
	"context"
	"io"
	"syscall"
	"time"
)

// LogFunc 接收驱动日志，避免模块依赖具体日志库。
// level 为 debug / info / warn / error；kv 是交替的键与值。
type LogFunc func(level, msg string, kv ...any)

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

// ExecResult 是驱动返回的执行结果；输出可能受后端配置的字节限额约束。
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
// 会话生命周期由后端管理：local 按命令隔离，box 保持常驻 agent。
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

// Metrics 是一个沙盒当前的资源用量快照。
//
// 只有 Linux 且 cgroup v2 已委派时才有值；其他后端返回 ErrNotSupported，
// 上层据此在 capabilities 里标注「无指标」，而不是报 0。
type Metrics struct {
	MemoryBytes     int64 `json:"memory_bytes"`
	MemoryPeakBytes int64 `json:"memory_peak_bytes"`
	CPUUsec         int64 `json:"cpu_usec"`
	Pids            int64 `json:"pids"`
	OOMKills        int64 `json:"oom_kills"`
	DiskReadBytes   int64 `json:"disk_read_bytes"`
	DiskWriteBytes  int64 `json:"disk_write_bytes"`
	// DiskUsedBytes 是沙盒可写部分（upper + ws）的实际占用。
	//
	// 与 DiskWriteBytes（io 带宽）不同：这是「现在占了多少盘」。
	// 后端算不出来时留 0——别用 io 累计值冒充，那只会误导配额判断。
	DiskUsedBytes int64 `json:"disk_used_bytes"`
	// Backend 是数据来源："cgroup" | "none"。
	Backend string `json:"backend"`
}

// Meter 是 Driver 的可选能力：读沙盒的资源用量。
type Meter interface {
	Metrics(ctx context.Context, name string) (Metrics, error)
}

// Pauser 是 Driver 的可选能力：冻结 / 解冻沙盒内的全部进程。
//
// 冻结保留内存与进程状态，恢复是毫秒级；不实现它的后端，上层只能把 pause
// 降级为 stop（进程丢失），并在 capabilities 里如实标注。
type Pauser interface {
	Pause(ctx context.Context, name string) error
	Resume(ctx context.Context, t Target) error
}

// LogQuery 是一次日志查询。
type LogQuery struct {
	// Lines 是最后多少条记录；0 = 全部。
	Lines int
	// Since 是起始时间；零值 = 不限。
	Since time.Time
	// Streams 是只保留的流："stdout" / "stderr"；空 = 全部。
	Streams []string
}

// LogRecord 是一条日志记录。
type LogRecord struct {
	// Stream 是 "stdout" 或 "stderr"。
	Stream string
	Time   time.Time
	Data   []byte
}

// LogSource 是 Driver 的可选能力：读沙盒的历史输出。
//
// 「历史」是相对于「实时流」而言：进程已经退出、或调用方当时没在读，
// 输出仍然能回放。不实现它的后端，上层必须关掉 logs 能力而不是
// 返回空列表（空列表会被误解为「没有输出」）。
type LogSource interface {
	Logs(ctx context.Context, name string, q LogQuery) ([]LogRecord, error)
}

// WatchOptions 是目录监听的参数。
type WatchOptions struct {
	// Recursive 为真时连子目录一起看。
	Recursive bool
}

// FileEvent 是一次文件变化。
type FileEvent struct {
	// Path 是沙盒内路径。
	Path string
	// Kind 是 "file" / "directory" / "symlink" / "other"。
	Kind string
}

// FileWatch 是一个进行中的目录监听。
//
// Events 关闭表示监听结束（Close 或沙盒退出）。通道满时实现可以丢事件，
// 调用方应定期全量重扫；不实现 Watcher 的后端，上层必须关掉 WatchDir。
type FileWatch interface {
	Events() <-chan FileEvent
	// Err 返回监听结束的原因；正常关闭返回 nil。
	Err() error
	Close() error
}

// Watcher 是 Driver 的可选能力：监听沙盒内的目录变化。
type Watcher interface {
	Watch(ctx context.Context, t Target, path string, opt WatchOptions) (FileWatch, error)
}

// SnapshotInfo 是一个文件系统快照。
type SnapshotInfo struct {
	// ID 是快照 id。
	ID string
	// Name 是拍快照时的沙盒名。
	Name string
	// Label 是调用方给的说明。
	Label string
	// Parent 是父快照 id（fork 出来的沙盒再拍快照时填）。
	Parent string
	// Size 是归档大小。
	Size int64
	// Digest 是归档内容的摘要。
	Digest string
	// Created 是创建时间。
	Created time.Time
}

// Snapshotter 是 Driver 的可选能力：文件系统快照与 fork。
//
// 语义边界（必须让上层与用户都知道）：
//
//   - 只保存**文件系统**：可写层与工作区。不保存内存与进程；
//   - 因此 Fork 出来的沙盒**没有**原沙盒里正在跑的进程，需要重新启动；
//   - 运行中的沙盒拍快照不是原子的：磁盘可能在打包期间变化，
//     快照是「某个瞬间的近似」，不是一致性快照。
//
// 这些都是共享内核的容器做不到内存快照的直接后果。不实现
// Snapshotter 的后端，上层必须把 fork / snapshots 回答 501。
type Snapshotter interface {
	// Snapshot 给一个沙盒拍快照。
	Snapshot(ctx context.Context, t Target, label string) (SnapshotInfo, error)
	// Snapshots 列出全部快照（新→旧）。
	Snapshots(ctx context.Context) ([]SnapshotInfo, error)
	// RemoveSnapshot 删一个快照。
	RemoveSnapshot(ctx context.Context, id string) error
	// Fork 从快照建一个新沙盒（新名字），并返回它的快照来源。
	//
	// 新沙盒的磁盘内容等于快照，进程未启动。
	Fork(ctx context.Context, snapshotID string, t Target) (SnapshotInfo, error)
}

// PortDialer 是 Driver 的可选能力：连到沙盒内 127.0.0.1 的某个端口。
//
// 这是 E2B getHost(port) 的实现方式：沙盒不需要网络栈，平台反向代理
// 收到请求后经控制通道开一条中继。
type PortDialer interface {
	DialPort(ctx context.Context, name string, port uint16) (io.ReadWriteCloser, error)
}

// Freezer 是 Pauser 的可选补充：报告冻结在当前环境里是否真的可用。
//
// 实现了 Pauser 但当前环境冻结不了的后端（例如 cgroup 未委派、或 macOS 没有
// cgroup）必须实现本接口并返回 false，上层据此把 pause 降级为 stop，
// 而不是让调用方收到一个 backend error。
type Freezer interface {
	CanFreeze() bool
}
