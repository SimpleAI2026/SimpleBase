// Package host 在本机用操作系统原语隔离一条命令（planv6.2）。
//
// Linux：bubblewrap 负责挂载与 user/pid/ipc/net 命名空间，沙盒内再依次加载
// Landlock 与 seccomp（见 inner 子包与 cmd/gosandbox-inner）。
// macOS：系统自带 /usr/bin/sandbox-exec。
//
// 这是共享内核的进程沙盒，不是 VM：它不承诺挡住内核提权漏洞，
// 多租户不可信代码应使用 vm 包的 microVM 后端。
//
// 失败语义：隔离开着但不可用，一律 fail-closed（ErrIsolationUnavailable），
// 不退回裸执行；只有 Enabled=false 或 danger-full-access 才返回 ErrNotIsolated，
// 由调用方自行决定裸执行。
package host

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Mode 是权限档位，与 Codex 的 sandbox_mode 对齐。
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
)

// DefaultReadOnlySubpaths 是工作区内默认再盖成只读的子路径。
var DefaultReadOnlySubpaths = []string{".git", ".cloudagent"}

// Policy 描述一次隔离执行的约束。网络恒为关闭：放开网络只有 danger-full-access。
type Policy struct {
	Enabled bool
	Mode    Mode
	// Workspace 是工作区绝对路径；workspace-write 下可写。
	Workspace string
	// WritableRoots 是额外可写的绝对路径，必须已存在。
	WritableRoots []string
	// ReadOnlySubpaths 是工作区内再盖成只读的路径（相对 Workspace 或绝对）。
	ReadOnlySubpaths []string
	// DenyRead 是不可读的绝对路径（文件盖成空文件，目录盖成空 tmpfs）。
	DenyRead []string
	// MemoryMaxBytes / CPUMax / PidsMax 对应 cgroup v2 的 memory.max / cpu.max / pids.max。
	// 零值表示不限；cgroup 不可用时不会生效，Result.Limits 会如实标注。
	MemoryMaxBytes int64
	CPUMax         string
	PidsMax        int64
	// Timeout 是单条命令的超时；0 表示由调用方的 ctx 决定。
	Timeout time.Duration
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

// EffectiveReadOnlySubpaths 返回只读子路径；nil 取默认值，空切片表示显式不盖。
func (p Policy) EffectiveReadOnlySubpaths() []string {
	if p.ReadOnlySubpaths == nil {
		return DefaultReadOnlySubpaths
	}
	return p.ReadOnlySubpaths
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
		if !filepath.IsAbs(r) {
			return fmt.Errorf("host: deny_read path %q must be absolute", r)
		}
	}
	if p.MemoryMaxBytes < 0 || p.PidsMax < 0 || p.Timeout < 0 {
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
	if !p.Isolates() || len(p.DenyRead) == 0 {
		return false
	}
	target := filepath.Clean(absPath)
	for _, d := range p.DenyRead {
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
