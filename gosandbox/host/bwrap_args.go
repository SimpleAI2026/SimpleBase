// 文件说明：bwrap_args.go — gosandbox/host 中 InnerPolicyPath、BwrapInput 等符号的实现
//
// 用法
//
//	BwrapArgs(in BwrapInput) ([]string, error)  // BwrapArgs 生成 bwrap 参数（不含 bwrap 本身）
//	ScratchDirs(p Policy) []string  // ScratchDirs 返回沙盒内需要 tmpfs 的目录
//	同包调用：BwrapInput → run_linux.go, start_linux.go；BwrapArgs() → run_linux.go, start_linux.go；ScratchDirs() → run_linux.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/bwrap_args.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] cgroup.go, glob.go, mandatory.go, policy.go, probe.go 等 13 个
//	[关键引用] BwrapInput ← run_linux.go, start_linux.go
//
// 主要符号
//
//	const InnerPolicyPath
//	type BwrapInput struct {…}
//	BwrapArgs(in BwrapInput) ([]string, error)
//	ScratchDirs(p Policy) []string

package host

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// InnerPolicyPath 是 helper 在沙盒内读取策略的位置（父进程经 --file 放入）。
const InnerPolicyPath = "/run/gosandbox/policy.json"

// BwrapInput 是生成 bwrap 参数所需的全部输入。
type BwrapInput struct {
	Policy Policy
	// Dir 是命令的工作目录（已解析的绝对路径）。
	Dir string
	// Helper 是 gosandbox-inner 的绝对路径（沙盒内同路径可见，因为根是只读绑定）。
	Helper string
	// Argv 是沙盒内最终执行的命令，例如 {"/bin/sh","-c",cmd}。
	Argv []string
	// PolicyFD 是承载 policy.json 的 fd 号（ExtraFiles[0] 即 3）。
	PolicyFD int
	// DisableUserNS 为真时追加 --disable-userns（bwrap >= 0.8.0 且非 setuid）。
	DisableUserNS bool
	// Exists 判断路径是否存在；nil 取 os.Lstat。仅用于只读子路径、强制 deny 表与 deny_read，便于测试。
	Exists func(string) bool
	// IsDir 判断路径是否为目录；nil 取 os.Stat。
	IsDir func(string) bool
	// Home 是用户 HOME，用于判断强制 deny 表是否要追加 HOME 下的 shell/git 配置；
	// 空串表示取 os.UserHomeDir()。测试可传入固定值。
	Home string
	// CTTY 为真时让 helper 把 stdin 设为控制终端（交互 PTY 会话）。
	CTTY bool
}

// BwrapArgs 生成 bwrap 参数（不含 bwrap 本身）。
//
// 顺序固定，后挂的更具体路径覆盖先挂的：
// 会话/退出 → 只读根 → /tmp tmpfs → 可写绑定 → 只读路径（含强制 deny 表）→ deny_read → /dev /proc →
// 命名空间 → cap-drop → 策略文件 → chdir → helper。
func BwrapArgs(in BwrapInput) ([]string, error) {
	p := in.Policy
	if !p.Isolates() {
		return nil, ErrNotIsolated
	}
	if err := Validate(p); err != nil {
		return nil, err
	}
	if in.Helper == "" || !filepath.IsAbs(in.Helper) {
		return nil, errors.New("host: helper path must be absolute")
	}
	if len(in.Argv) == 0 {
		return nil, errors.New("host: empty argv")
	}
	exists := in.Exists
	if exists == nil {
		exists = func(s string) bool { _, err := os.Lstat(s); return err == nil }
	}
	isDir := in.IsDir
	if isDir == nil {
		isDir = func(s string) bool { st, err := os.Stat(s); return err == nil && st.IsDir() }
	}
	mode := p.EffectiveMode()
	fd := in.PolicyFD
	if fd == 0 {
		fd = 3
	}

	args := []string{"--new-session", "--die-with-parent", "--ro-bind", "/", "/"}

	// /tmp 用 tmpfs，否则编译器等写不了临时文件。必须在所有绑定之前挂：
	// 工作区或可写根可能位于 /tmp 之下（如 /tmp/ws），后挂的 tmpfs 会把它们整个盖掉。
	// /tmp 本身已被可写根覆盖时不再挂。
	for _, s := range ScratchDirs(p) {
		args = append(args, "--tmpfs", s)
	}

	// 可写绑定。read-only 档不绑工作区。
	var writable []string
	if mode == ModeWorkspaceWrite {
		writable = append(writable, filepath.Clean(p.Workspace))
	}
	for _, r := range p.WritableRoots {
		writable = append(writable, filepath.Clean(r))
	}
	for _, w := range writable {
		args = append(args, "--bind", w, w)
	}

	// 工作区内的只读路径：用户配置的只读子路径加强制 deny 表。只在可写时才有意义，
	// 且仅覆盖存在的路径（--ro-bind 到不存在的路径会让 bwrap 启动失败）。
	// 文件与目录都用 --ro-bind 自身，不需要区分。
	home := in.Home
	if home == "" {
		home = userHome()
	}
	if mode == ModeWorkspaceWrite {
		for _, abs := range readOnlyPaths(p, home, exists) {
			args = append(args, "--ro-bind", abs, abs)
		}
	}

	// 不可读路径：目录盖空 tmpfs，文件盖 /dev/null。
	// glob 条目在这里展开为当下存在的路径（快照语义）。
	var denied []string
	for _, d := range p.DenyRead {
		if isGlobEntry(d) {
			path, _ := splitDenyEntry(d)
			matches, err := ExpandGlob(path)
			if err != nil {
				return nil, err
			}
			denied = append(denied, matches...)
			continue
		}
		path, _ := splitDenyEntry(d)
		denied = append(denied, path)
	}
	for _, d := range denied {
		d = filepath.Clean(d)
		if !exists(d) {
			continue
		}
		if isDir(d) {
			args = append(args, "--tmpfs", d)
		} else {
			args = append(args, "--ro-bind", "/dev/null", d)
		}
	}

	args = append(args, "--dev", "/dev", "--proc", "/proc",
		"--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-net")
	if in.DisableUserNS {
		args = append(args, "--disable-userns")
	}
	args = append(args, "--cap-drop", "ALL",
		"--dir", "/run/gosandbox",
		"--perms", "0400", "--file", strconv.Itoa(fd), InnerPolicyPath)
	if in.Dir != "" {
		args = append(args, "--chdir", in.Dir)
	}
	args = append(args, "--", in.Helper, "--policy", InnerPolicyPath)
	if in.CTTY {
		args = append(args, "--ctty")
	}
	args = append(args, "--")
	args = append(args, in.Argv...)
	return args, nil
}

// ScratchDirs 返回沙盒内需要 tmpfs 的目录。工作区或可写根已经覆盖 /tmp 时不重复挂。
func ScratchDirs(p Policy) []string {
	covered := func(path string) bool {
		roots := append([]string{}, p.WritableRoots...)
		if p.EffectiveMode() == ModeWorkspaceWrite {
			roots = append(roots, p.Workspace)
		}
		for _, r := range roots {
			r = filepath.Clean(r)
			if path == r || strings.HasPrefix(path, r+string(filepath.Separator)) {
				return true
			}
		}
		return false
	}
	if covered("/tmp") {
		return nil
	}
	return []string{"/tmp"}
}
