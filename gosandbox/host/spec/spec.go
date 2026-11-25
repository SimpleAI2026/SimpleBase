// 文件说明：spec.go — gosandbox/host/spec 中 Spec 等符号的实现
//
// 用法
//
//	type Spec struct{…}  // Spec 是 helper 的输入，由父进程经 bwrap 的 --file 放入沙盒内的只读文件， 不进 argv（避免出现在 /proc/<pid>/cmd...
//
// 调用架构
//
//	[上游] gosandbox/box, gosandbox/host, gosandbox/host/inner
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/spec/spec.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] 无
//
// 主要符号
//
//	const ExitSetupFailed
//	const FailPrefix
//	type Spec struct{…}
//	const SeccompHost / SeccompDefault

package spec

// ExitSetupFailed 是 helper 在 Landlock / seccomp 装载失败时的退出码。
const ExitSetupFailed = 71

// FailPrefix 是 helper 装载失败时写到 stderr 的固定前缀。
// 父进程要求「退出码 71 且输出含该前缀」才判定为装载失败，
// 单独的 71 可能只是用户命令自己的退出码。
const FailPrefix = "gosandbox-inner: setup failed: "

// Spec 是 helper 的输入，由父进程经 bwrap 的 --file 放入沙盒内的只读文件，
// 不进 argv（避免出现在 /proc/<pid>/cmdline），也不进环境变量。
type Spec struct {
	// Mode 是 read-only 或 workspace-write。
	Mode string `json:"mode"`
	// Workspace 与 WritableRoots 是 Landlock 的可写目录（read-only 档不含 Workspace）。
	Workspace     string   `json:"workspace"`
	WritableRoots []string `json:"writable_roots"`
	// ScratchDirs 是沙盒内的 tmpfs 目录（/tmp 等），Landlock 同样放行写。
	ScratchDirs []string `json:"scratch_dirs"`
	// Seccomp 选择过滤器："host"（默认，拒绝本地 socket 与网络）或
	// "default"（Docker 默认 profile 的形状，允许本地 socket，仍拒绝 vsock 与 io_uring）。
	// 镜像根下宿主文件系统不可见，拒绝本地 socket 的理由不成立，反而会让
	// postgres、redis、python multiprocessing 这类依赖本地 IPC 的程序失败。
	Seccomp string `json:"seccomp,omitempty"`
}

// Seccomp 档位取值。
const (
	// SeccompHost 是 host 模式：宿主根可见，因此拒绝 AF_UNIX / AF_VSOCK 与全部网络调用。
	SeccompHost = "host"
	// SeccompDefault 对齐 Docker 的默认 profile：允许本地 socket，拒绝越界 syscall。
	SeccompDefault = "default"
)
