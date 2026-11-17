//go:build linux

// 文件说明：supervise_linux.go — 一个独立进程托管沙盒，服务重启不丢沙盒
//
// 问题：现在控制通道是 socketpair，一端在服务进程里。服务进程一退出
// （升级、崩溃、被 supervisor 重启），socketpair 就断了，bwrap 与 agent
// 也随之死掉——所有沙盒消失。
//
// 解法：把「持有 socketpair」这件事挪到一个独立进程（supervisor）。
//
//	服务进程 ──unix socket──► supervisor ──socketpair──► bwrap + agent
//
//   - supervisor 是 `gosandbox-inner --supervisor` 自己，与沙盒共用同一个
//     二进制，不需要额外的部署产物；
//   - 服务进程连上是「一条字节流」，协议层完全不变（单写者、帧格式都不动）；
//   - supervisor 断开后仍持有 agent 侧的连接，新服务进程连上继续收发，
//     agent 甚至不知道自己被重新接管过；
//   - 控制面是宿主上的 unix socket，不在沙盒的 netns 里，
//     沙盒内进程依旧摸不到它（与原来 socketpair 的安全性质一致）。
//
// 代价（必须明确的）：
//
//   - 服务进程断开那一刻，agent 正在发但还没被读走的帧会丢。协议是帧级的，
//     丢了就是丢了——调用方看到的是「那条请求没回复」，需要自己重试。
//     不缓冲是为了不让 supervisor 变成内存黑洞；
//   - supervisor 自己不重启服务进程：它只负责「不丢沙盒」，
//     谁拉起服务进程由外部的 init 系统决定（systemd / launchd）。
//
// 用法
//
//	// supervisor 侧（由 launch 拉起）
//	gosandbox-inner --supervisor --socket /var/lib/sb/sb1/ctl.sock -- <bwrap argv>
//
//	// 服务进程侧
//	conn, err := dialSupervisor("/var/lib/sb/sb1/ctl.sock")
//
// 调用架构
//
//	[上游] gosandbox/box（launch / dialSupervisor）、gosandbox/cmd
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/supervise_linux.go
//	         │
//	         ▼
//	[下游] net、os/exec、仅标准库
//	[同包] launch_linux.go, session.go
//
// 主要符号
//
//	SuperviseArgs(args []string) (socket string, argv []string, ok bool)
//	RunSupervisor(socket string, argv []string) int
//	dialSupervisor(path string) (net.Conn, error)

package box

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// DialTimeout 是服务进程连 supervisor 的超时。
const DialTimeout = 5 * time.Second

// SuperviseArgs 判断一组参数是否是 supervisor 模式，并拆出 socket 与子进程 argv。
//
// 形状：--supervisor --socket <path> -- <argv...>
func SuperviseArgs(args []string) (socket string, argv []string, ok bool) {
	if len(args) == 0 || args[0] != "--supervisor" {
		return "", nil, false
	}
	i := 1
	for i < len(args) {
		if args[i] == "--" {
			return socket, args[i+1:], true
		}
		if args[i] == "--socket" && i+1 < len(args) {
			socket = args[i+1]
			i += 2
			continue
		}
		i++
	}
	return socket, nil, true
}

// RunSupervisor 是 supervisor 的主体：托管一个子进程，并把它的 socketpair
// 通过 unix socket 暴露给服务进程。返回进程退出码。
//
// 子进程退出时 supervisor 也退出——沙盒没了，留着没有意义。
func RunSupervisor(socket string, argv []string) int {
	if socket == "" || len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "gosandbox-inner: supervisor needs --socket and a command after --")
		return 2
	}
	// socket 目录要存在；文件权限 0600：只有同用户的服务进程能连。
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "gosandbox-inner: supervisor: %v\n", err)
		return 1
	}
	_ = os.Remove(socket) // 上一轮的残留
	ln, err := net.Listen("unix", socket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gosandbox-inner: supervisor listen: %v\n", err)
		return 1
	}
	defer func() {
		_ = ln.Close()
		_ = os.Remove(socket)
	}()
	if err := os.Chmod(socket, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "gosandbox-inner: supervisor chmod: %v\n", err)
		return 1
	}

	hostConn, agentFile, err := socketPair()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gosandbox-inner: supervisor socketpair: %v\n", err)
		return 1
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = os.Environ()
	// 控制通道作为 fd 3 交给子进程：策略文件在 supervisor 模式下由
	// 调用方自己处理，这里只搬控制通道。
	cmd.ExtraFiles = []*os.File{agentFile}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "gosandbox-inner: supervisor start: %v\n", err)
		return 1
	}
	agentFile.Close()
	defer hostConn.Close()

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	s := &supervisor{ln: ln, agent: hostConn}
	go s.acceptLoop()

	select {
	case werr := <-exited:
		_ = s.stop()
		if werr != nil {
			// 子进程非正常退出：把退出码带回给调用方，便于排查。
			if ee, ok := werr.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			return 1
		}
		return 0
	case <-s.broken:
		// agent 侧彻底断了（子进程还在但连接没了）：杀掉子进程收尾。
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-exited
		_ = s.stop()
		return 1
	}
}

// supervisor 持有 agent 侧连接与监听，负责把两者接起来。
type supervisor struct {
	ln    net.Listener
	agent net.Conn

	mu     sync.Mutex
	svc    net.Conn
	closed bool
	// broken 在 agent 侧连接不可恢复时关闭。
	broken chan struct{}
	once   sync.Once
}

// acceptLoop 接受服务进程的重连。
func (s *supervisor) acceptLoop() {
	if s.broken == nil {
		s.broken = make(chan struct{})
	}
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		// 新服务进程接管：旧连接（如果有）先断掉。
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			c.Close()
			return
		}
		old := s.svc
		s.svc = c
		s.mu.Unlock()
		if old != nil {
			_ = old.Close()
		}
		go s.pump(c)
	}
}

// pump 在一个服务进程连接与 agent 之间双向转发。
//
// 两个方向用一条拷贝即可：协议是帧化的，且同一时刻只有一个写者，
// 不需要按帧切分——按字节流转发对上层完全等价。
func (s *supervisor) pump(svc net.Conn) {
	done := make(chan struct{}, 2)
	// svc → agent
	go func() {
		_, _ = io.Copy(s.agent, svc)
		done <- struct{}{}
	}()
	// agent → svc
	go func() {
		_, _ = io.Copy(svc, s.agent)
		done <- struct{}{}
	}()
	<-done
	// 任一方向结束：关掉服务进程侧，保留 agent 侧等下一次接管。
	//
	// 注意这里只关 svc 不关 agent：agent 那边是沙盒的命脉。
	// 未读完的帧会丢——这是「断开的那一刻」的固有代价。
	s.mu.Lock()
	if s.svc == svc {
		s.svc = nil
	}
	s.mu.Unlock()
	_ = svc.Close()
	// 如果 agent 侧真的断了（读错），标记整体不可恢复。
	if err := probeClosed(s.agent); err != nil {
		s.once.Do(func() {
			s.mu.Lock()
			s.closed = true
			s.mu.Unlock()
			if s.broken != nil {
				close(s.broken)
			}
		})
	}
}

// stop 关闭监听与 agent 连接。
func (s *supervisor) stop() error {
	s.mu.Lock()
	s.closed = true
	svc := s.svc
	s.svc = nil
	s.mu.Unlock()
	if svc != nil {
		_ = svc.Close()
	}
	_ = s.ln.Close()
	return s.agent.Close()
}

// probeClosed 判断一个连接是不是已经断了。
//
// 用一次零字节的写探测：对端关闭时 write 返回 EPIPE。这不是百分之百可靠
// （缓冲区满时也可能返回别的），但足够区分「对端关了」与「连接还在」，
// 而且不会像读探测那样吃掉数据。
func probeClosed(c net.Conn) error {
	if c == nil {
		return errors.New("box: nil conn")
	}
	_ = c.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	_, err := c.Write(nil)
	_ = c.SetWriteDeadline(time.Time{})
	return err
}

// dialSupervisor 连上一个 supervisor 的控制 socket。
func dialSupervisor(path string) (net.Conn, error) {
	if path == "" {
		return nil, errors.New("box: empty supervisor socket")
	}
	conn, err := net.DialTimeout("unix", path, DialTimeout)
	if err != nil {
		return nil, fmt.Errorf("box: connect supervisor: %w", err)
	}
	return conn, nil
}
