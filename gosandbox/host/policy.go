// 文件说明：policy.go — gosandbox/host 中 Mode、Policy 等符号的实现
//
// 用法
//
//	type Mode string  // Mode 是文件系统与网络权限档位
//	type Policy struct{…}  // Policy 描述一次隔离执行的约束
//	Validate(p Policy) error  // Validate 校验策略自身，不检查本机能力（见 Probe）
//	DeniesRead(p Policy, absPath string) bool  // DeniesRead 报告 absPath 是否落在 DenyRead 之内
//	同包调用：resolveSub() → mandatory.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/box, gosandbox/vm/box 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/policy.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, cgroup.go, cgroup_linux.go, cgroup_linux_test.go 等 36 个
//	[关键引用] Mode ← run_darwin_test.go, run_linux_test.go；Policy ← bwrap_args.go, cgroup.go；Validate ← bwrap_args.go, policy_test.go
//
// 主要符号
//
//	type Mode string
//	const ModeReadOnly / ModeWorkspaceWrite / ModeDangerFullAccess
//	var ErrNotIsolated / ErrIsolationUnavailable / ErrNotPTY / ErrUnsupportedSignal / ErrPTYUnsupported
//	type Policy struct{…}
//	(p Policy) Isolates() bool
//	(p Policy) EffectiveMode() Mode
//	Validate(p Policy) error
//	validateCPUMax(s string) error
//	... 另有 2 个顶层符号

package host

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Mode 是文件系统与网络权限档位。
type Mode string

const (
	ModeReadOnly         Mode = "read-only"
	ModeWorkspaceWrite   Mode = "workspace-write"
	ModeDangerFullAccess Mode = "danger-full-access"
)

// 哨兵错误。
var (
	// ErrNotIsolated：Enabled=false 或 danger-full-access，调用方走原来的裸执行。
	ErrNotIsolated = errors.New("host: isolation not requested")
	// ErrIsolationUnavailable：要求隔离但无法建立，命令没有被执行。
	ErrIsolationUnavailable = errors.New("host: isolation unavailable")
	// ErrNotPTY：对非 PTY 进程调用了 Resize。
	ErrNotPTY = errors.New("host: process has no pty")
	// ErrUnsupportedSignal：只允许 SIGTERM 与 SIGKILL。
	ErrUnsupportedSignal = errors.New("host: unsupported signal")
	// ErrPTYUnsupported：当前平台没有可用的 PTY 实现。
	ErrPTYUnsupported = errors.New("host: pty unsupported on this platform")
)

// Policy 描述一次隔离执行的约束。网络恒为关闭：放开网络只有 danger-full-access。
type Policy struct {
	Enabled bool
	Mode    Mode
	// Workspace 是工作区绝对路径；workspace-write 下可写。
	Workspace string
	// WritableRoots 是额外可写的绝对路径，必须已存在。
	WritableRoots []string
	// ReadOnlySubpaths 是工作区内再盖成只读的路径（相对 Workspace 或绝对）。
	// 只对生成命令时已存在的路径生效；nil 与空切片都不额外保护子路径。
	ReadOnlySubpaths []string
	// DenyRead 是不可读的绝对路径（文件盖成空文件，目录盖成空 tmpfs）。
	// 支持 glob（*、**、?、[abc]，没有转义字符；拼不出集合的方括号按字面量，如 [user-id]），
	// 不以 / 结尾。"literal:" 前缀强制字面量。
	// Linux 在生成命令时展开为当下存在的路径（快照语义，之后新建的匹配文件不受掩蔽，
	// 单个 glob 最多 MaxGlobMatches 条）；macOS 转成 SBPL 正则，覆盖未来新建的文件。
	DenyRead []string
	// MemoryMaxBytes / CPUMax / PidsMax 对应 cgroup v2 的 memory.max / cpu.max / pids.max。
	// 零值表示不限。Linux 仅在 cgroup 已委派时生效；macOS 由父进程采样监控执行
	// （内存、进程数超限即杀整个进程组，CPU 超额即暂停），Result.Limits 会如实标注。
	MemoryMaxBytes int64
	CPUMax         string
	PidsMax        int64
	// ShmBytes 是 /dev/shm 的大小；0 取 bwrap 的默认（与宿主 /dev/shm 一致）。
	// 编译类任务（Go、Rust、Chromium）默认 64MiB 会炸，需要显式放大。
	// 仅在 bwrap 支持 --size（>= 0.9）时生效，否则忽略。
	ShmBytes int64
	// TmpBytes 是 /tmp tmpfs 的大小；0 表示不限（受内存限额约束）。
	TmpBytes int64
	// CgroupName 非空时把命令放进固定的 cgroup 子组 gosandbox-<CgroupName>，
	// 而不是每次命令一个随机子组。固定子组在命令结束后保留，供 MetricsFor /
	// FreezeCgroup 按名字查询与冻结；调用方负责在沙盒删除时 RemoveCgroup。
	// 只在 cgroup v2 已委派时生效。
	CgroupName string
	// Timeout 是单条命令的超时；0 表示由调用方的 ctx 决定。
	Timeout time.Duration
	// MaxOutputBytes 是合并输出的内存上限，超出部分被丢弃并置 Result.OutputTruncated；
	// 0 表示不限。服务端必须设置，否则命令可以刷满进程内存。
	MaxOutputBytes int
	// BwrapPath 为空表示在 PATH 上查找，结果必须解析为绝对路径。
	BwrapPath string
	// HelperPath 为空表示在可执行文件同目录查找 gosandbox-inner。
	HelperPath string
}

// Isolates 报告该策略是否要求启动沙盒。
func (p Policy) Isolates() bool {
	return p.Enabled && p.Mode != ModeDangerFullAccess
}

// EffectiveMode 返回生效的档位；未指定时取 workspace-write。
func (p Policy) EffectiveMode() Mode {
	if p.Mode == "" {
		return ModeWorkspaceWrite
	}
	return p.Mode
}

// Validate 校验策略自身，不检查本机能力（见 Probe）。
func Validate(p Policy) error {
	switch p.EffectiveMode() {
	case ModeReadOnly, ModeWorkspaceWrite, ModeDangerFullAccess:
	default:
		return fmt.Errorf("host: unknown mode %q", p.Mode)
	}
	if !p.Isolates() {
		return nil
	}
	if p.Workspace == "" || !filepath.IsAbs(p.Workspace) {
		return errors.New("host: workspace must be an absolute path")
	}
	for _, r := range p.WritableRoots {
		if !filepath.IsAbs(r) {
			return fmt.Errorf("host: writable root %q must be absolute", r)
		}
	}
	for _, r := range p.DenyRead {
		if err := validateDenyEntry(r); err != nil {
			return err
		}
	}
	if p.MemoryMaxBytes < 0 || p.PidsMax < 0 || p.Timeout < 0 || p.MaxOutputBytes < 0 || p.ShmBytes < 0 || p.TmpBytes < 0 {
		return errors.New("host: limits and timeout must not be negative")
	}
	if p.CPUMax != "" {
		if err := validateCPUMax(p.CPUMax); err != nil {
			return err
		}
	}
	if p.BwrapPath != "" && !filepath.IsAbs(p.BwrapPath) {
		return errors.New("host: bwrap_path must be absolute")
	}
	return nil
}

// validateCPUMax 校验 cgroup v2 cpu.max 的原文："<quota|max> <period>"。
func validateCPUMax(s string) error {
	f := strings.Fields(s)
	if len(f) < 1 || len(f) > 2 {
		return fmt.Errorf("host: cpu_max %q must be \"<quota|max> [period]\"", s)
	}
	isNum := func(v string) bool {
		if v == "" {
			return false
		}
		for _, r := range v {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	if f[0] != "max" && !isNum(f[0]) {
		return fmt.Errorf("host: cpu_max %q has an invalid quota", s)
	}
	if len(f) == 2 && !isNum(f[1]) {
		return fmt.Errorf("host: cpu_max %q has an invalid period", s)
	}
	return nil
}

// DeniesRead 报告 absPath 是否落在 DenyRead 之内。
// 文件工具（read_file 等）在父进程里运行，必须与 shell 里被盖住的路径保持一致。
func DeniesRead(p Policy, absPath string) bool {
	if !p.Isolates() {
		return false
	}
	entries := p.DenyRead
	if defaultDenyReadOS {
		entries = effectiveDenyRead(p, userHome())
	}
	if len(entries) == 0 {
		return false
	}
	target := filepath.Clean(absPath)
	for _, d := range entries {
		if isGlobEntry(d) {
			if globMatchesPath(d, target) {
				return true
			}
			continue
		}
		d, _ = splitDenyEntry(d)
		d = filepath.Clean(d)
		if target == d || strings.HasPrefix(target, d+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// resolveSub 把只读子路径解析为绝对路径。
func resolveSub(workspace, sub string) string {
	if filepath.IsAbs(sub) {
		return filepath.Clean(sub)
	}
	return filepath.Join(workspace, sub)
}
