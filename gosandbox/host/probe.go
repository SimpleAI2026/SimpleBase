// 文件说明：probe.go — gosandbox/host 中 Caps、MinLandlockABI 等符号的实现
//
// 用法
//
//	ResetProbeCache()  // ResetProbeCache 清掉进程内的探测缓存（测试用）
//	Probe(ctx context.Context, p Policy) (Caps, error)  // Probe 探测本机能否按 p 建立隔离
//	unavailable() → policy.go, probe_linux.go 等；Probe() → policy.go, run_darwin.go 等
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/probe.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 13 个
//
// 主要符号
//
//	type Caps struct {…}
//	const MinLandlockABI
//	ResetProbeCache()
//	Probe(ctx context.Context, p Policy) (Caps, error)
//	... 另有 14 个顶层符号

package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Caps 是本机隔离能力的探测结果。
type Caps struct {
	OS           string
	BwrapPath    string
	BwrapVersion string // `bwrap --version` 原文
	// DisableUserNS 为真表示 bwrap 的 --help 含 --disable-userns（>= 0.8.0）。
	DisableUserNS bool
	// UserNS 为真表示 `bwrap --unshare-user …` 烟雾测试成功。
	UserNS      bool
	LandlockABI int // 0 = 不可用
	Seccomp     bool
	// Cgroup 是 cgroup v2 限额的可用方式："delegated" | "systemd" | "none"。
	Cgroup string
	// SandboxExec 是 macOS 上 sandbox-exec 的路径。
	SandboxExec string
}

// MinLandlockABI 是受限模式要求的最低 Landlock ABI：V3 起含 truncate 权利，
// 低于它会留下截断文件的洞（go-landlock 作者的 BestEffort 说明）。
const MinLandlockABI = 3

// 为测试留的可替换点。
var (
	lookPath       = exec.LookPath
	landlockABIFn  = landlockABI // 平台实现
	seccompFn      = seccompSupported
	cgroupDetectFn = detectCgroup
	runProbe       = func(ctx context.Context, name string, args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
		return string(out), err
	}
)

type probeKey struct{ bwrap string }

var (
	probeMu    sync.Mutex
	probeCache = map[probeKey]probeEntry{}
)

type probeEntry struct {
	caps Caps
	err  error
}

// ResetProbeCache 清掉进程内的探测缓存（测试用）。
func ResetProbeCache() {
	probeMu.Lock()
	probeCache = map[probeKey]probeEntry{}
	probeMu.Unlock()
}

// Probe 探测本机能否按 p 建立隔离。结果在进程内缓存。
// 任何不满足都返回包装了 ErrIsolationUnavailable 的错误，不降级。
func Probe(ctx context.Context, p Policy) (Caps, error) {
	if !p.Isolates() {
		return Caps{OS: runtime.GOOS}, ErrNotIsolated
	}
	if err := Validate(p); err != nil {
		return Caps{}, err
	}
	switch runtime.GOOS {
	case "linux":
		key := probeKey{bwrap: p.BwrapPath}
		probeMu.Lock()
		if e, ok := probeCache[key]; ok {
			probeMu.Unlock()
			return e.caps, e.err
		}
		probeMu.Unlock()
		caps, err := probeLinux(ctx, p)
		probeMu.Lock()
		probeCache[key] = probeEntry{caps, err}
		probeMu.Unlock()
		return caps, err
	case "darwin":
		return probeDarwin()
	default:
		return Caps{OS: runtime.GOOS}, fmt.Errorf("%w: unsupported OS %s", ErrIsolationUnavailable, runtime.GOOS)
	}
}

func unavailable(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrIsolationUnavailable, fmt.Sprintf(format, a...))
}

func probeLinux(ctx context.Context, p Policy) (Caps, error) {
	caps := Caps{OS: "linux"}
	bw, err := resolveBwrap(p.BwrapPath)
	if err != nil {
		return caps, err
	}
	caps.BwrapPath = bw
	if st, err := os.Stat(bw); err != nil {
		return caps, unavailable("stat bwrap: %v", err)
	} else if st.Mode()&os.ModeSetuid != 0 {
		// CVE-2020-5291：setuid 安装的 bwrap 在特定内核组合下可被利用。
		return caps, unavailable("bwrap at %s is setuid; install a non-setuid distribution package", bw)
	}
	if out, err := runProbe(ctx, bw, "--version"); err == nil {
		caps.BwrapVersion = strings.TrimSpace(out)
	}
	if out, err := runProbe(ctx, bw, "--help"); err == nil || out != "" {
		caps.DisableUserNS = strings.Contains(out, "--disable-userns")
	}
	if !caps.DisableUserNS {
		return caps, unavailable("bwrap lacks --disable-userns (need bubblewrap >= 0.8.0)")
	}
	if out, err := runProbe(ctx, bw, "--unshare-user", "--ro-bind", "/", "/", "--dev", "/dev",
		"--proc", "/proc", "--die-with-parent", "--", "/bin/true"); err != nil {
		return caps, unavailable("user namespaces are not usable (%s); see https://bugs.launchpad.net/bugs/2046477",
			firstLine(out, err))
	}
	caps.UserNS = true
	caps.LandlockABI = landlockABIFn()
	if caps.LandlockABI < MinLandlockABI {
		return caps, unavailable("landlock ABI %d is below the required %d", caps.LandlockABI, MinLandlockABI)
	}
	caps.Seccomp = seccompFn()
	if !caps.Seccomp {
		return caps, unavailable("seccomp is not supported by this kernel")
	}
	caps.Cgroup = cgroupDetectFn()
	return caps, nil
}

// resolveBwrap 返回 bwrap 的绝对真实路径；拒绝相对路径与 cwd 里的同名文件。
func resolveBwrap(configured string) (string, error) {
	path := configured
	if path == "" {
		found, err := lookPath("bwrap")
		if err != nil {
			if errors.Is(err, exec.ErrDot) {
				return "", unavailable("refusing bwrap resolved relative to the current directory")
			}
			return "", unavailable("bwrap not found in PATH")
		}
		path = found
	}
	if !filepath.IsAbs(path) {
		return "", unavailable("bwrap path %q is not absolute", path)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", unavailable("resolve bwrap: %v", err)
	}
	return real, nil
}

func firstLine(out string, err error) string {
	out = strings.TrimSpace(out)
	if i := strings.IndexByte(out, '\n'); i >= 0 {
		out = out[:i]
	}
	if out == "" && err != nil {
		return err.Error()
	}
	return out
}
