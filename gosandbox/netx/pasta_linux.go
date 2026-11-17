//go:build linux

// 文件说明：pasta_linux.go — 用 pasta 给 netns 提供用户态出网
//
// 为什么是 pasta 而不是 veth + NAT：veth 要 root（或至少 CAP_NET_ADMIN）
// 与 nftables 规则，而我们的沙盒是 rootless 的。pasta 在用户态把 netns 的
// 流量接到宿主网络，不需要任何特权，podman 5 也用它作默认网络栈。
//
// 生命周期：
//
//	pasta --config-net --netns /proc/<pid>/ns/net   // 沙盒起来后把 netns 交给 pasta
//
// pasta 自己会 fork 到后台并持有 netns；沙盒退出后 netns 消失，pasta 随之退出。
// 我们不显式 kill 它：杀掉可能留下半配置的 netns，反而更难清理。
//
// 用法
//
//	r, err := netx.Start(ctx, netx.StartOptions{NetnsPID: pid, Mode: netx.ModeEgress})
//	r.Stop()
//
// 调用架构
//
//	[上游] gosandbox/box（launch）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/netx/pasta_linux.go
//	         │
//	         ▼
//	[下游] os/exec、netx.go
//	[同包] netx.go
//
// 主要符号
//
//	type StartOptions struct {…}
//	type Runner struct {…}
//	Start(ctx context.Context, opt StartOptions) (*Runner, error)
//	(r *Runner) Stop() error
//	Available() bool

package netx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// DefaultPastaPath 是 pasta 的常见安装位置（第一个存在的会被用）。
var pastaCandidates = []string{
	"/usr/bin/pasta",
	"/usr/local/bin/pasta",
	"/bin/pasta",
}

// pastaOnce 缓存「本机有没有 pasta」。
var (
	pastaOnce sync.Once
	pastaPath string
)

// Available 报告本机是否装了 pasta。
func Available() bool {
	pastaOnce.Do(func() {
		if p := os.Getenv("GOSANDBOX_PASTA"); p != "" {
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				pastaPath = p
				return
			}
		}
		for _, c := range pastaCandidates {
			if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
				pastaPath = c
				return
			}
		}
	})
	return pastaPath != ""
}

// PastaPath 返回要用的 pasta 路径（空表示不可用）。
func PastaPath() string {
	if !Available() {
		return ""
	}
	return pastaPath
}

// nsenterCandidates 是 nsenter 的常见安装位置（util-linux）。
var nsenterCandidates = []string{"/usr/bin/nsenter", "/bin/nsenter", "/usr/sbin/nsenter"}

// nsenterPath 返回可用的 nsenter 路径（空表示没有）。
func nsenterPath() (string, bool) {
	for _, c := range nsenterCandidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, true
		}
	}
	return "", false
}

// StartOptions 是出网配置。
type StartOptions struct {
	// NetnsPID 是沙盒 agent 的 pid：pasta 通过 /proc/<pid>/ns/net 找到 netns。
	NetnsPID int
	// Mode 是档位；ModeNone 直接返回 nil。
	Mode Mode
	// MTU 覆盖默认 MTU；0 取 65520（pasta 的默认，比宿主常见的 1500 大，
	// 但 pasta 自己做分片，所以不会出错）。
	MTU int
	// Log 接收 pasta 的 stderr；nil 表示丢弃。
	Log func(line string)
}

// Runner 是一个正在运行的 pasta 进程。
type Runner struct {
	cmd *exec.Cmd
	// done 在进程退出后关闭。
	done chan struct{}
	err  error
}

// Start 给一个 netns 接上出网。
//
// ModeNone 返回 (nil, nil)：没有出网就是没有，不需要进程。
func Start(ctx context.Context, opt StartOptions) (*Runner, error) {
	if opt.Mode == ModeNone {
		return nil, nil
	}
	bin := PastaPath()
	if bin == "" {
		return nil, fmt.Errorf("%w: pasta not found (install passt, or set GOSANDBOX_PASTA)", ErrUnsupported)
	}
	if opt.NetnsPID <= 0 {
		return nil, errors.New("netx: NetnsPID is required")
	}
	mtu := opt.MTU
	if mtu <= 0 {
		mtu = 65520
	}
	// --config-net：让 pasta 在 netns 里配好地址与默认路由。
	// --no-map-gw：不要把宿主的 127.0.0.1 映射进沙盒——那等于把宿主的
	//               本地服务直接暴露给沙盒，是最容易忽略的一处越权。
	// --runas 0：pasta 默认以 root 启动后降权到 nobody，降权后既打不开
	//               netns 也没有 CAP_NET_ADMIN 建 tap。
	pastaArgs := []string{
		"--config-net", "--no-map-gw", "--quiet",
		"--mtu", fmt.Sprint(mtu),
		"--runas", "0",
	}

	cmdPath, args := bin, pastaArgs
	if ns, ok := nsenterPath(); ok && os.Geteuid() == 0 {
		// 以 root 运行时优先用 nsenter：pasta 从**外部**打开沙盒 userns
		// 拥有的 netns 会被内核拒绝（"Couldn't open network namespace
		// /proc/<pid>/ns/net: Permission denied"），而先进入目标 netns
		// 再让它配 /proc/self/ns/net 一切正常。rootless 下同 userns 的
		// nsenter 也成立，拿不到 nsenter 时回退到直连。
		args = append([]string{"-t", fmt.Sprint(opt.NetnsPID), "-n", "--", bin},
			append(pastaArgs, "--netns", "/proc/self/ns/net")...)
		cmdPath = ns
	} else {
		// --netns 只接受**路径**或 /run/netns 下的名字；传纯数字会被当成
		// 名字拼成 /run/netns/<pid> 并报 "No such file or directory"。
		args = append(pastaArgs, "--netns", fmt.Sprintf("/proc/%d/ns/net", opt.NetnsPID))
	}
	cmd := exec.CommandContext(ctx, cmdPath, args...)
	if opt.Log != nil {
		cmd.Stderr = lineWriter(opt.Log)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w: start pasta: %v", ErrUnsupported, err)
	}
	r := &Runner{cmd: cmd, done: make(chan struct{})}
	go func() {
		defer close(r.done)
		r.err = cmd.Wait()
	}()
	return r, nil
}

// Done 在 pasta 退出后关闭。
func (r *Runner) Done() <-chan struct{} { return r.done }

// Err 返回 pasta 的退出原因（正常退出为 nil）。
func (r *Runner) Err() error {
	select {
	case <-r.done:
		return r.err
	default:
		return nil
	}
}

// Stop 停掉 pasta。netns 已被销毁时进程自己就退了，这里只做兜底。
func (r *Runner) Stop() error {
	if r == nil || r.cmd == nil || r.cmd.Process == nil {
		return nil
	}
	select {
	case <-r.done:
		return nil
	default:
	}
	_ = r.cmd.Process.Kill()
	<-r.done
	return nil
}

// lineWriter 把子进程输出按行回调。
func lineWriter(fn func(string)) *lineBuf {
	return &lineBuf{fn: fn}
}

// lineBuf 是按行切分的 io.Writer。
type lineBuf struct {
	fn  func(string)
	buf []byte
}

// Write 实现 io.Writer。
func (w *lineBuf) Write(p []byte) (int, error) {
	n := len(p)
	w.buf = append(w.buf, p...)
	for {
		i := indexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		w.fn(line)
	}
	// 单行过长时截断，避免无限累积。
	if len(w.buf) > 64<<10 {
		w.fn(string(w.buf))
		w.buf = nil
	}
	return n, nil
}

// indexByte 是 bytes.IndexByte 的短名，避免为一行代码引入 bytes。
func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}
