//go:build linux

// 文件说明：launch_linux.go — gosandbox/box 的内部实现（policyFD、socketPair 等）
//
// 用法
//
//	同包调用：socketPair() → supervise_linux.go；buildArgs() → netx_e2e_test.go；rootArgs() → launch_args_test.go
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/launch_linux.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backpressure_test.go, copytree.go, deadline.go, egress_e2e_test.go, files.go 等 26 个
//	[关键引用] socketPair ← supervise_linux.go；buildArgs ← netx_e2e_test.go；rootArgs ← launch_args_test.go
//
// 主要符号
//
//	const policyFD / ctrlFD
//	socketPair() (net.Conn, *os.File, error)
//	networkModeOf(cfg Config) string
//	buildArgs(cfg Config) ([]string, string, error)
//	rootArgs(cfg Config) ([]string, error)
//	overlaySupported(bwrap string) bool
//	var overlayMu / overlayOK
//	writePolicy(path string, p Policy, ws string) error
//	... 另有 7 个顶层符号

package box

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/host"
	"github.com/linkxzhou/SimpleBase/gosandbox/host/spec"
	"github.com/linkxzhou/SimpleBase/gosandbox/netx"
)

// ctrlFD 是控制通道在子进程里的 fd 号。
// ExtraFiles 从 3 开始：3 = 策略文件，4 = 控制通道。
const (
	policyFD = 3
	ctrlFD   = 4
)

// socketPair 建一对 unix socket（全双工，可传 fd）。
func socketPair() (net.Conn, *os.File, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, nil, err
	}
	hostFile := os.NewFile(uintptr(fds[0]), "box-ctrl-host")
	agentFile := os.NewFile(uintptr(fds[1]), "box-ctrl-agent")
	// 宿主侧包成 net.Conn：需要 SetReadDeadline。
	hostConn, err := net.FileConn(hostFile)
	hostFile.Close()
	if err != nil {
		agentFile.Close()
		return nil, nil, err
	}
	return hostConn, agentFile, nil
}

// networkModeOf 把网络策略归一成传给 agent 的档位字符串。
//
// 空值按 none 处理（最安全的一档，与 Session 的默认一致）。agent 只在
// egress 档装黑洞路由：none 档没有 default route，私网本来就不可达。
func networkModeOf(cfg Config) string {
	if cfg.Network.Mode == netx.ModeEgress {
		return "egress"
	}
	return "none"
}

// buildArgs 组装 bwrap 参数并写出策略文件，返回完整 argv 与策略文件路径。
func buildArgs(cfg Config) ([]string, string, error) {
	if cfg.Helper == "" || !filepath.IsAbs(cfg.Helper) {
		return nil, "", fmt.Errorf("box: helper must be an absolute path")
	}
	ws := filepath.Join(cfg.Root, "ws")
	runDir := filepath.Join(cfg.Root, "run")
	for _, d := range []string{ws, runDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, "", err
		}
	}
	policyPath := filepath.Join(runDir, "policy.json")
	if err := writePolicy(policyPath, cfg.Policy, ws); err != nil {
		return nil, "", err
	}

	agentArgv := []string{cfg.Helper, "--agent", "--ctrl", fmt.Sprint(ctrlFD), "--network", networkModeOf(cfg)}
	// 把多段映射的可用 id 区间传给 agent：它据此判断「能不能 setuid 到某个
	// uid」，而不是试一次失败一次。格式：uidmap=1000-1999,2000-2999
	if spec := subIDSpec(ProbeSubIDs()); spec != "" {
		agentArgv = append(agentArgv, "--uidmap", spec)
	}

	// Bwrap 为空：开发/测试路径，直接跑 agent（没有隔离，只验证协议与调度）。
	if cfg.Bwrap == "" {
		return agentArgv, policyPath, nil
	}

	args := []string{cfg.Bwrap,
		"--new-session", "--die-with-parent",
		// agent 要当 PID 1 收尸，所以不能交给 bwrap --as-pid-1。
		"--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup-try",
		// 网络：**任何档位都 unshare**。
		//
		// none 档就是没有网络；egress 档由 pasta 用 `--config-net --netns`
		// 在**沙盒自己的 netns** 里配地址与默认路由。早先的版本只在 none
		// 档 unshare、让 egress 共享宿主 netns 给 pasta，那等于沙盒直接拿到
		// 宿主的网络栈（含云元数据与私网），是本模块最严重的一处越权。
		//
		// 只隔离 netns 还不够：pasta 是透明 NAT，会照常转发到私网与元数据。
		// 地址层拦截由 agent 在自己的 netns 里加黑洞路由（--network egress）。
		"--unshare-net",
	}
	// 多段 uid 映射：让镜像里的服务用户（999、1000 等）在沙盒内真实存在。
	//
	// 单段映射下只有 0 可用，setuid 到别的 id 会 EINVAL，
	// 以非 root 用户运行的服务起不来。缺 subuid/newuidmap 时退回单段，
	// 并通过 SubIDReason 把原因传出去（不静默）。
	subIDs := ProbeSubIDs()
	if idArgs := subIDs.IDMapArgs(); len(idArgs) > 0 {
		args = append(args, idArgs...)
	}
	root, err := rootArgs(cfg)
	if err != nil {
		return nil, "", err
	}
	args = append(args, root...)
	args = append(args,
		"--bind", ws, cfg.Workspace,
		"--tmpfs", "/tmp",
		"--proc", "/proc",
		"--dev", "/dev",
		"--cap-drop", "ALL",
		// agent 要在自己的 netns 里拉起 lo（none 档的入站中继依赖它）并加
		// 黑洞路由（egress 档）。这两件事都要 CAP_NET_ADMIN；bwrap 的
		// --cap-drop ALL 连 userns 内的这份也一起摘掉，所以显式加回来。
		//
		// 权限只在沙盒 userns 内有效，碰不到宿主。agent 在配完之后会主动
		// 丢掉它（dropNetAdmin），用户命令拿不到。
		"--cap-add", "CAP_NET_ADMIN",
		// 目录必须在可写层：沙盒根是 --ro-bind / /，bwrap 建不了 /run/…
		"--dir", filepath.Dir(host.InnerPolicyPath),
		"--perms", "0400", "--file", fmt.Sprint(policyFD), host.InnerPolicyPath,
		"--",
	)
	return append(args, agentArgv...), policyPath, nil
}

// rootArgs 返回沙盒根的挂载参数。
//
// 三种情况，按能力从高到低：
//
//	cfg.Lower 非空且 bwrap 支持 --overlay  → overlay（lower 只读共享）
//	cfg.Lower 非空但不支持 --overlay      → 把 lower 拷进沙盒再 bind
//	cfg.Lower 为空                        → 宿主根只读（image=host）
//
// 拷贝路径是必要的退路：Ubuntu 24.04 自带的 bwrap 0.9 没有 --overlay，
// 而镜像沙盒是 M2 的核心能力，不能因为 bwrap 版本就整块不可用。
//
// 构造镜像根失败时**返回错误**，而不是退回宿主根：用户请求的是镜像，
// 悄悄给他宿主文件系统，等于他以为在隔离环境里跑、实际读到的是宿主
// （与 vm/box 的 resolveLower 同一条原则）。
func rootArgs(cfg Config) ([]string, error) {
	if cfg.Lower == "" {
		return []string{"--ro-bind", "/", "/"}, nil
	}
	if overlaySupported(cfg.Bwrap) {
		upper := filepath.Join(cfg.Root, "upper")
		work := filepath.Join(cfg.Root, "work")
		for _, d := range []string{upper, work} {
			if err := os.MkdirAll(d, 0o700); err != nil {
				return nil, fmt.Errorf("box: prepare overlay dir %s: %w", d, err)
			}
		}
		return []string{"--overlay-src", cfg.Lower, "--overlay", upper, work, "/"}, nil
	}
	// 拷贝退路：把 lower 复制一份，以 bind 作根。
	root := filepath.Join(cfg.Root, "rootfs")
	if err := copyTree(cfg.Lower, root); err != nil {
		return nil, fmt.Errorf("box: materialize image root %s: %w", cfg.Lower, err)
	}
	return []string{"--bind", root, "/"}, nil
}

// overlaySupported 报告 bwrap 是否支持 --overlay。
//
// 只在启动时探一次并缓存：探测要真起一个 bwrap，每个沙盒都探一次太贵。
func overlaySupported(bwrap string) bool {
	overlayMu.Lock()
	defer overlayMu.Unlock()
	if overlayOK != nil {
		return *overlayOK
	}
	ok := false
	if bwrap != "" {
		out, err := exec.Command(bwrap, "--help").Output()
		ok = err == nil && bytes.Contains(out, []byte("--overlay"))
	}
	overlayOK = &ok
	return ok
}

// overlayMu / overlayOK 缓存 --overlay 探测结果。
var (
	overlayMu sync.Mutex
	overlayOK *bool
)

// writePolicy 把 Landlock 策略与 seccomp 档位写成 JSON，交给 helper 读。
func writePolicy(path string, p Policy, ws string) error {
	if p.Mode == "" {
		p.Mode = "workspace-write"
	}
	if p.Workspace == "" {
		p.Workspace = ws
	}
	body, err := json.Marshal(spec.Spec{
		Mode:          p.Mode,
		Workspace:     p.Workspace,
		WritableRoots: p.WritableRoots,
		ScratchDirs:   append(defaultScratch(p.ScratchDirs), "/workspace"),
		Seccomp:       p.Seccomp,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o400)
}

// defaultScratch 保证 /tmp 在 Landlock 的可写列表里（bwrap 已挂 tmpfs）。
func defaultScratch(given []string) []string {
	has := false
	for _, d := range given {
		if d == "/tmp" {
			has = true
			break
		}
	}
	if has {
		return given
	}
	return append(append([]string{}, given...), "/tmp")
}

// controlConn 是控制通道的宿主侧：进程退出时关闭它，让 Session 感知。
type controlConn struct {
	net.Conn
	cmd  *exec.Cmd
	name string
}

// closeSession 在 Session.Close 之后收尾：关连接、等进程退出、清理临时文件。
func closeSession(s *Session) {
	c, ok := s.conn.(*controlConn)
	if !ok {
		return
	}
	_ = c.Conn.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = c.cmd.Wait()
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		// 宽限期到：按进程组杀掉（bwrap 的孙进程一并结束）。
		_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
		<-done
	}
	_ = os.RemoveAll(filepath.Join(s.cfg.Root, "run"))
}

// Pid 返回沙盒进程（bwrap，也就是 agent 的父进程）的 pid。
//
// 出网要用它：pasta 通过 /proc/<pid>/ns/net 找到沙盒的 netns。
// 返回 0 表示还没有进程（未启动或被关掉）。
func (c *controlConn) Pid() int {
	if c.cmd == nil || c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

// launchSupervised 起一个由 supervisor 托管的沙盒。
//
// 沙盒自己（bwrap + agent）由 supervisor 拉起，服务进程只连它的控制 socket。
// 这样服务进程重启不影响沙盒。
func launchSupervised(ctx context.Context, cfg Config) (net.Conn, error) {
	socket := supervisorSocket(cfg.Root)
	// 已有 supervisor 在跑（服务重启后的重连）：直接接管。
	//
	// 必须在 buildArgs 之前判断：buildArgs 会重写 run/policy.json，
	// 而正在运行的沙盒不需要（也不该被）重新生成策略。
	if conn, derr := dialSupervisor(socket); derr == nil {
		return conn, nil
	}
	argv, policyPath, err := buildArgs(cfg)
	if err != nil {
		return nil, err
	}

	// 用同一个 helper 二进制跑 supervisor：不引入第二个部署产物。
	//
	// 形状：--supervisor --socket <path> --policy <path> -- <bwrap argv...>
	// supervisor 按与非托管路径相同的 fd 约定拉起子进程：
	// 3 = 策略文件（bwrap 的 --file 3），4 = 控制通道（agent 的 --ctrl 4）。
	args := append([]string{"--supervisor", "--socket", socket, "--policy", policyPath, "--"}, argv...)
	cmd := exec.CommandContext(ctx, cfg.Helper, args...)
	cmd.Env = cfg.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if cfg.Log != nil {
		cfg.Log("debug", "starting supervised sandbox", "name", cfg.Name, "socket", socket)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("box: start supervisor: %w", err)
	}
	// 父进程侧不持有 supervisor 的进程句柄：它要活过本进程。
	_ = cmd.Process.Release()

	// 等 supervisor 把 socket 建好。
	deadline := time.Now().Add(DialTimeout)
	for time.Now().Before(deadline) {
		if conn, derr := dialSupervisor(socket); derr == nil {
			return conn, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, fmt.Errorf("box: supervisor did not come up at %s", socket)
}

// supervisorSocket 返回一个沙盒的控制 socket 路径。
func supervisorSocket(boxRoot string) string {
	return filepath.Join(boxRoot, "ctl.sock")
}

// launch 起一个沙盒：bwrap 里跑 agent，控制通道用 socketpair 传给 agent。
//
// 控制通道走 socketpair 而不是 TCP / unix socket：后者要在沙盒内开监听，
// 也就意味着沙盒里的任何进程都能连上控制面。socketpair 只有 agent 持有另一端。
func launch(ctx context.Context, cfg Config) (net.Conn, error) {
	// supervisor 模式：沙盒由独立进程托管，控制面走 unix socket。
	if cfg.Supervise {
		return launchSupervised(ctx, cfg)
	}
	hostConn, agentFile, err := socketPair()
	if err != nil {
		return nil, fmt.Errorf("box: control socket: %w", err)
	}

	argv, policyPath, err := buildArgs(cfg)
	if err != nil {
		hostConn.Close()
		agentFile.Close()
		return nil, err
	}

	policyFile, err := os.Open(policyPath)
	if err != nil {
		hostConn.Close()
		agentFile.Close()
		_ = os.RemoveAll(filepath.Join(cfg.Root, "run"))
		return nil, fmt.Errorf("box: open policy: %w", err)
	}
	defer policyFile.Close()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = cfg.Env
	// ExtraFiles 从 fd 3 开始：3 = 策略文件，4 = 控制通道。
	cmd.ExtraFiles = []*os.File{policyFile, agentFile}
	// 独立进程组：Close 时按组杀，bwrap 的孙进程一并结束。
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if cfg.Log != nil {
		cfg.Log("debug", "starting sandbox", "name", cfg.Name, "argv", argv)
	}
	if err := cmd.Start(); err != nil {
		hostConn.Close()
		agentFile.Close()
		_ = os.RemoveAll(filepath.Join(cfg.Root, "run"))
		return nil, fmt.Errorf("box: start bwrap: %w", err)
	}
	// 父进程侧必须关掉 agent 端，否则读不到 EOF。
	agentFile.Close()

	conn := &controlConn{Conn: hostConn, cmd: cmd, name: cfg.Name}
	// 进程退出后关连接，Session 的读循环随之结束。
	go func() {
		_ = cmd.Wait()
		_ = conn.Close()
	}()
	return conn, nil
}
