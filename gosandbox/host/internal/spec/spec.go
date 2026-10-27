// 文件说明：spec.go — 父进程（host）与沙盒内 helper（cmd/gosandbox-inner）之间的协议
//
// 用法
//
//	type Spec（导出类型，同包与上层均可使用）
//
// 调用架构
//
//	[上游] （无，本包为入口）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/internal/spec/spec.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//
// 主要符号
//
//	const ExitSetupFailed
//	const FailPrefix
//	type Spec struct {…}

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
}
