// Package spec 是父进程（host）与沙盒内 helper（cmd/gosandbox-inner）之间的协议。
// 单独成包，是为了让 host 不必链接 go-landlock / go-seccomp-bpf。
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
