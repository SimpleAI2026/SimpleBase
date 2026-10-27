//go:build linux

// 文件说明：inner_linux.go — 在 bwrap 沙盒内、exec 用户命令之前加载 Landlock 与 seccomp
//
// 用法
//
//	LoadSpec(path string) (spec.Spec, error)  // LoadSpec 读取父进程放入沙盒的策略文件
//	LandlockRules(s spec.Spec) []landlock.Rule  // LandlockRules 返回文件系统规则：根只读，可写目录读写
//
// 调用架构
//
//	[上游] （无，本包为入口）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/inner/inner_linux.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/host/internal/spec
//
// 主要符号
//
//	const MinABI
//	LoadSpec(path string) (spec.Spec, error)
//	LandlockRules(s spec.Spec) []landlock.Rule
//	SeccompFilter() seccomp.Filter
//	Apply(s spec.Spec) error
//	Main(args []string, env []string) int
//	... 另有 9 个顶层符号

package inner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"

	seccomp "github.com/elastic/go-seccomp-bpf"
	"github.com/landlock-lsm/go-landlock/landlock"
	"golang.org/x/sys/unix"

	"github.com/linkxzhou/SimpleBase/gosandbox/host/internal/spec"
)

// 可替换点，便于测试 ABI 门槛。
var (
	abiFn           = kernelLandlockABI
	restrictPathsFn = func(cfg landlock.Config, rules ...landlock.Rule) error { return cfg.RestrictPaths(rules...) }
	loadSeccompFn   = seccomp.LoadFilter
	execFn          = syscall.Exec
)

// MinABI 与 host.MinLandlockABI 相同：V3 起含 truncate 权利。
const MinABI = 3

func kernelLandlockABI() int {
	r, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0,
		uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION))
	if errno != 0 {
		return 0
	}
	return int(r)
}

// LoadSpec 读取父进程放入沙盒的策略文件。
func LoadSpec(path string) (spec.Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return spec.Spec{}, err
	}
	var s spec.Spec
	if err := json.Unmarshal(b, &s); err != nil {
		return spec.Spec{}, err
	}
	return s, nil
}

// LandlockRules 返回文件系统规则：根只读，可写目录读写。
// 与 bwrap 的挂载重复是有意的纵深防御：bwrap 漏绑了可写路径时 Landlock 仍拒绝写入。
func LandlockRules(s spec.Spec) []landlock.Rule {
	rules := []landlock.Rule{landlock.RODirs("/")}
	var rw []string
	if s.Mode == "workspace-write" && s.Workspace != "" {
		rw = append(rw, s.Workspace)
	}
	rw = append(rw, s.WritableRoots...)
	rw = append(rw, s.ScratchDirs...)
	if len(rw) > 0 {
		rules = append(rules, landlock.RWDirs(rw...))
	}
	// /dev 下的设备（null、tty、urandom 等）需要写，且 bwrap 的 --dev 是独立 tmpfs。
	rules = append(rules, landlock.RWFiles("/dev/null", "/dev/tty").IgnoreIfMissing())
	return rules
}

// SeccompFilter 返回默认允许 + 拒绝清单的过滤器。
//
//   - io_uring_*：可绕过 socket() 创建 AF_VSOCK。
//   - ptrace / process_vm_*：沙盒内进程之间互相窥探没有必要。
//   - socket(AF_UNIX|AF_VSOCK)：不让命令连宿主上的 Unix socket（docker.sock、ssh-agent 等）。
//   - connect/accept/bind/listen/sendto/sendmmsg：网络恒为关闭。
//
// 不拒绝 socketpair 与 recvfrom：cargo/clippy 等的子进程管理要用。
func SeccompFilter() seccomp.Filter {
	deny := seccomp.ActionErrno
	return seccomp.Filter{
		NoNewPrivs: true,
		Flag:       seccomp.FilterFlagTSync,
		Policy: seccomp.Policy{
			DefaultAction: seccomp.ActionAllow,
			Syscalls: []seccomp.SyscallGroup{
				{Names: []string{"io_uring_setup", "io_uring_enter", "io_uring_register",
					"ptrace", "process_vm_readv", "process_vm_writev"}, Action: deny},
				{NamesWithCondtions: []seccomp.NameWithConditions{
					{Name: "socket", Conditions: eq(0, unix.AF_UNIX)},
					{Name: "socket", Conditions: eq(0, unix.AF_VSOCK)},
				}, Action: deny},
				{Names: []string{"connect", "accept", "accept4", "bind", "listen", "sendto", "sendmmsg"}, Action: deny},
			},
		},
	}
}

func eq(arg uint32, v uint64) seccomp.ArgumentConditions {
	return seccomp.ArgumentConditions{{Argument: arg, Operation: seccomp.Equal, Value: v}}
}

// Apply 加载 Landlock 与 seccomp。任何一步失败都返回错误，不降级。
func Apply(s spec.Spec) error {
	if s.Mode != "read-only" && s.Mode != "workspace-write" {
		return fmt.Errorf("unsupported mode %q", s.Mode)
	}
	abi := abiFn()
	if abi < MinABI {
		return fmt.Errorf("landlock ABI %d is below the required %d", abi, MinABI)
	}
	// 严格 V3，不用 BestEffort：BestEffort 在内核不支持时会「成功且不限制」。
	if err := restrictPathsFn(landlock.V3, LandlockRules(s)...); err != nil {
		return fmt.Errorf("landlock: %w", err)
	}
	if err := loadSeccompFn(SeccompFilter()); err != nil {
		return fmt.Errorf("seccomp: %w", err)
	}
	return nil
}

// Main 是 helper 的入口：args 形如 ["--policy", path, "--", argv...]。
// 成功时不返回（exec 用户命令）；失败时写固定前缀并以 spec.ExitSetupFailed 退出。
func Main(args []string, env []string) int {
	policyPath, ctty, argv, err := parseArgs(args)
	if err != nil {
		return fail(err)
	}
	s, err := LoadSpec(policyPath)
	if err != nil {
		return fail(fmt.Errorf("read policy: %w", err))
	}
	// 控制终端必须在 seccomp 之前设置：bwrap 的 --new-session 已让本进程成为会话首领，
	// 这里把 stdin（PTY 从端）设为控制终端，shell 才有作业控制。
	if ctty {
		if err := setCtty(); err != nil {
			return fail(fmt.Errorf("controlling tty: %w", err))
		}
	}
	if err := Apply(s); err != nil {
		return fail(err)
	}
	bin := argv[0]
	if err := execFn(bin, argv, env); err != nil {
		// exec 失败发生在隔离已生效之后，是命令本身的问题（如 not found），用 127 与 shell 对齐。
		fmt.Fprintf(os.Stderr, "gosandbox-inner: exec %s: %v\n", bin, err)
		return 127
	}
	return 0
}

func parseArgs(args []string) (policy string, ctty bool, argv []string, err error) {
	const usage = "usage: gosandbox-inner --policy FILE [--ctty] -- cmd..."
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--policy":
			if i+1 >= len(args) {
				return "", false, nil, errors.New("--policy needs a value")
			}
			i++
			policy = args[i]
		case "--ctty":
			ctty = true
		case "--":
			argv = args[i+1:]
			if policy == "" || len(argv) == 0 {
				return "", false, nil, errors.New(usage)
			}
			return policy, ctty, argv, nil
		default:
			return "", false, nil, fmt.Errorf("unexpected argument %q", args[i])
		}
	}
	return "", false, nil, errors.New(usage)
}

// setCtty 把 fd 0 设为本进程（会话首领）的控制终端。
func setCtty() error {
	if err := unix.IoctlSetInt(0, unix.TIOCSCTTY, 0); err != nil {
		return err
	}
	return nil
}

func fail(err error) int {
	fmt.Fprintf(os.Stderr, "%s%v\n", spec.FailPrefix, err)
	return spec.ExitSetupFailed
}
