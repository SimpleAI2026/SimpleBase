// 文件说明：mandatory.go — gosandbox/host 中 ForceDenyWriteSubpaths、ForceDenyWriteHomeFiles 等符号的…
//
// 用法
//
//	ForceDenyWritePaths(p Policy, home string, exists func(string) bool) []string  // ForceDenyWritePaths 返回应当强制只读的绝对路径，已去重、排序，且只含 exists 判定存在的路径
//	Warnings(p Policy) []string  // Warnings 返回策略的安全相关提示，供调用方如实上报
//	同包调用：Warnings() → policy.go, run.go 等；readOnlyPaths() → bwrap_args.go, seatbelt.go；userHome() → bwrap_args.go, seatbelt.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/mandatory.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, policy.go, probe.go 等 13 个
//	[关键引用] Warnings() ← policy.go, run.go, run_darwin.go
//
// 主要符号
//
//	var ForceDenyWriteSubpaths
//	var ForceDenyWriteHomeFiles
//	ForceDenyWritePaths(p Policy, home string, exists func(string) bool) []string
//	Warnings(p Policy) []string
//	... 另有 4 个顶层符号

package host

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ForceDenyWriteSubpaths 是工作区内强制只读的路径（相对 Workspace），对齐
// sandbox-runtime 的 DANGEROUS_FILES / DANGEROUS_DIRECTORIES，按本项目场景裁剪。
//
// 这些路径会被沙盒之外的程序执行或读取：git（hooks、config、attributes 里的 filter）、
// 子模块（.gitmodules）、MCP 客户端（.mcp.json）、IDE（.vscode、.idea 的任务与运行配置）。
// 沙盒内写入它们不会立刻造成越界，但会在用户之后于宿主侧操作该目录时被触发。
//
// 只保护生成命令时已存在的路径：对不存在的 .git/hooks 加 deny 会让沙盒内 git init 直接失败
// （见 planv6.3 §3.1，D7）。代价是同一条命令内「git init 后写 hook」的窗口，init 之后的
// 下一条命令起即受保护。
var ForceDenyWriteSubpaths = []string{
	".git/hooks",
	".git/config",
	".git/info/attributes",
	".gitmodules",
	".mcp.json",
	".vscode",
	".idea",
}

// ForceDenyWriteHomeFiles 是 HOME 下强制只读的文件。
// 根本来就是只读的，所以只有某个可写根覆盖了 HOME 时才需要追加。
var ForceDenyWriteHomeFiles = []string{
	".bashrc", ".bash_profile", ".zshrc", ".zprofile", ".profile", ".gitconfig",
}

// pathExists 判断路径是否存在（不跟随符号链接，符号链接本身也算存在）。
func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// ForceDenyWritePaths 返回应当强制只读的绝对路径，已去重、排序，且只含 exists 判定存在的路径。
// DisableForceDeny 或不在 workspace-write 档时返回 nil：read-only 档没有可写区，无需再盖。
//
// home 为空表示不处理 HOME 下的文件；exists 为 nil 时取 os.Lstat。
func ForceDenyWritePaths(p Policy, home string, exists func(string) bool) []string {
	if p.DisableForceDeny || !p.Isolates() || p.EffectiveMode() != ModeWorkspaceWrite {
		return nil
	}
	if exists == nil {
		exists = pathExists
	}
	set := map[string]struct{}{}
	add := func(abs string) {
		abs = filepath.Clean(abs)
		if exists(abs) {
			set[abs] = struct{}{}
		}
	}
	for _, sub := range ForceDenyWriteSubpaths {
		add(filepath.Join(p.Workspace, sub))
	}
	// 其他可写根里同样可能有 git 仓库。
	for _, root := range p.WritableRoots {
		for _, sub := range ForceDenyWriteSubpaths {
			add(filepath.Join(root, sub))
		}
	}
	if home != "" && filepath.IsAbs(home) {
		roots := append([]string{p.Workspace}, p.WritableRoots...)
		for _, r := range roots {
			if covers(r, home) {
				for _, f := range ForceDenyWriteHomeFiles {
					add(filepath.Join(home, f))
				}
				break
			}
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// covers 报告 root 是否等于 path 或为其祖先目录。
func covers(root, path string) bool {
	root, path = filepath.Clean(root), filepath.Clean(path)
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

// readOnlyPaths 合并用户可配的只读子路径与强制 deny 表，已去重、保持稳定顺序：
// 先是 ReadOnlySubpaths（按配置顺序），再是强制表（排序）。仅含存在的路径。
// 去重按 Clean 后的绝对路径。
func readOnlyPaths(p Policy, home string, exists func(string) bool) []string {
	if exists == nil {
		exists = pathExists
	}
	seen := map[string]struct{}{}
	var out []string
	if p.EffectiveMode() == ModeWorkspaceWrite {
		for _, sub := range p.EffectiveReadOnlySubpaths() {
			abs := resolveSub(p.Workspace, sub)
			if !exists(abs) {
				continue
			}
			if _, dup := seen[abs]; dup {
				continue
			}
			seen[abs] = struct{}{}
			out = append(out, abs)
		}
	}
	for _, abs := range ForceDenyWritePaths(p, home, exists) {
		if _, dup := seen[abs]; dup {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	return out
}

// userHome 返回当前用户的 HOME；取不到时返回空串（此时不处理 HOME 下的文件）。
func userHome() string {
	h, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(h) {
		return ""
	}
	return h
}

// Warnings 返回策略的安全相关提示，供调用方如实上报。目前只有强制 deny 表被关闭这一项。
func Warnings(p Policy) []string {
	if p.Isolates() && p.DisableForceDeny {
		return []string{"force-deny table disabled: .git/hooks, .git/config, .vscode and similar paths are writable"}
	}
	return nil
}

// DefaultDenyReadHomePaths 是 macOS 上默认对沙盒不可读的 HOME 子路径（凭据、令牌、钥匙串、Cookie）。
//
// macOS 的 Seatbelt 是「根可读」模型，没有 Linux 那样的挂载命名空间，宿主 HOME 下的凭据默认对命令可见；
// 这张表把最常见的凭据位置盖掉。不存在的路径同样生效（覆盖之后新建的文件）。
// Policy.DisableDefaultDenyRead=true 可关闭。Linux 不使用这张表（行为不变）。
var DefaultDenyReadHomePaths = []string{
	".ssh",
	".aws",
	".azure",
	".gnupg",
	".kube",
	".netrc",
	".git-credentials",
	".docker/config.json",
	".config/gh",
	".config/gcloud",
	"Library/Keychains",
	"Library/Cookies",
}

// defaultDenyReadOS 为真表示当前平台启用 DefaultDenyReadHomePaths（仅 macOS），测试可替换。
var defaultDenyReadOS = runtime.GOOS == "darwin"

// defaultDenyRead 返回 DefaultDenyReadHomePaths 展开后的条目（带 literal: 前缀，HOME 里的 * ? [ 不被当作 glob）。
// 工作区或可写根落在某个条目之内时跳过该条目，否则命令连自己的工作区都读不了。
func defaultDenyRead(p Policy, home string) []string {
	if p.DisableDefaultDenyRead || !p.Isolates() || home == "" || !filepath.IsAbs(home) {
		return nil
	}
	roots := append([]string{p.Workspace}, p.WritableRoots...)
	var out []string
next:
	for _, sub := range DefaultDenyReadHomePaths {
		abs := filepath.Join(home, sub)
		for _, r := range roots {
			if r != "" && covers(abs, r) {
				continue next
			}
		}
		out = append(out, literalPrefix+abs)
	}
	return out
}

// effectiveDenyRead 合并用户的 DenyRead 与平台默认表；用户条目在前，序号稳定。
func effectiveDenyRead(p Policy, home string) []string {
	out := append([]string{}, p.DenyRead...)
	return append(out, defaultDenyRead(p, home)...)
}
