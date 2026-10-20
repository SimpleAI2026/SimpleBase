package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/host"
)

// ShellIsolation 是 ~/.cloudagent/config.json 的 shell_isolation 段（planv6.2）。
// CLI 不读服务器的 config.yaml；这段与 agent.shell_isolation 同形，由操作者自行抄写。
//
// 指针字段区分「没写」与「写了零值」：缺省字段用内置默认（enabled=false）。
type ShellIsolation struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Mode    string `json:"mode,omitempty"` // read-only | workspace-write | danger-full-access
	// WritableRoots 是额外可写的绝对路径。
	WritableRoots []string `json:"writable_roots,omitempty"`
	// ReadOnlySubpaths 为 nil 取默认（.git、.cloudagent）；写成空数组表示都不盖。
	ReadOnlySubpaths []string `json:"read_only_subpaths,omitempty"`
	DenyRead         []string `json:"deny_read,omitempty"`
	MemoryMaxBytes   int64    `json:"memory_max_bytes,omitempty"`
	CPUMax           string   `json:"cpu_max,omitempty"`
	PidsMax          int64    `json:"pids_max,omitempty"`
	Timeout          string   `json:"timeout,omitempty"` // 如 "120s"；空沿用 shell 默认
	BwrapPath        string   `json:"bwrap_path,omitempty"`
	HelperPath       string   `json:"helper_path,omitempty"`
}

// overlayShellIsolation 用调用方（命令行）给出的字段覆盖文件配置；零值视为没传。
func overlayShellIsolation(base, override ShellIsolation) ShellIsolation {
	if override.Enabled != nil {
		base.Enabled = override.Enabled
	}
	if override.Mode != "" {
		base.Mode = override.Mode
	}
	if override.WritableRoots != nil {
		base.WritableRoots = override.WritableRoots
	}
	if override.ReadOnlySubpaths != nil {
		base.ReadOnlySubpaths = override.ReadOnlySubpaths
	}
	if override.DenyRead != nil {
		base.DenyRead = override.DenyRead
	}
	if override.MemoryMaxBytes != 0 {
		base.MemoryMaxBytes = override.MemoryMaxBytes
	}
	if override.CPUMax != "" {
		base.CPUMax = override.CPUMax
	}
	if override.PidsMax != 0 {
		base.PidsMax = override.PidsMax
	}
	if override.Timeout != "" {
		base.Timeout = override.Timeout
	}
	if override.BwrapPath != "" {
		base.BwrapPath = override.BwrapPath
	}
	if override.HelperPath != "" {
		base.HelperPath = override.HelperPath
	}
	return base
}

// ShellIsolationFromFlag 把 --shell-isolation <mode> 转成覆盖项。
// 传入任何档位都视为显式开启隔离；danger-full-access 则是显式关闭。
func ShellIsolationFromFlag(mode string) (ShellIsolation, error) {
	switch host.Mode(mode) {
	case host.ModeReadOnly, host.ModeWorkspaceWrite, host.ModeDangerFullAccess:
	default:
		return ShellIsolation{}, fmt.Errorf("invalid --shell-isolation %q: want read-only, workspace-write or danger-full-access", mode)
	}
	on := true
	return ShellIsolation{Enabled: &on, Mode: mode}, nil
}

// Policy 把配置转成 host.Policy。workspace 是 CLI 的工作区根；home 用于展开 "~"。
// 配置非法（未知模式、相对路径、负数等）直接返回错误，不静默当成关闭。
func (s ShellIsolation) Policy(workspace, home string) (host.Policy, error) {
	p := host.Policy{
		Enabled:          s.Enabled != nil && *s.Enabled,
		Mode:             host.Mode(s.Mode),
		MemoryMaxBytes:   s.MemoryMaxBytes,
		CPUMax:           s.CPUMax,
		PidsMax:          s.PidsMax,
		BwrapPath:        s.BwrapPath,
		HelperPath:       s.HelperPath,
		ReadOnlySubpaths: s.ReadOnlySubpaths,
	}
	if s.Timeout != "" {
		d, err := time.ParseDuration(s.Timeout)
		if err != nil || d < 0 {
			return host.Policy{}, fmt.Errorf("shell_isolation.timeout %q is not a valid duration", s.Timeout)
		}
		p.Timeout = d
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return host.Policy{}, err
	}
	// 解析符号链接：bwrap 与 Landlock 按真实路径匹配。
	if real, rerr := filepath.EvalSymlinks(abs); rerr == nil {
		abs = real
	}
	p.Workspace = abs
	for _, r := range s.WritableRoots {
		p.WritableRoots = append(p.WritableRoots, expandHome(r, home))
	}
	for _, r := range s.DenyRead {
		p.DenyRead = append(p.DenyRead, expandHome(r, home))
	}
	if err := host.Validate(p); err != nil {
		return host.Policy{}, err
	}
	for _, r := range p.WritableRoots {
		if _, serr := os.Stat(r); serr != nil && p.Isolates() {
			return host.Policy{}, fmt.Errorf("shell_isolation.writable_roots: %s does not exist", r)
		}
	}
	return p, nil
}

func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok && home != "" {
		return filepath.Join(home, rest)
	}
	return path
}
