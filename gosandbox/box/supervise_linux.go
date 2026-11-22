//go:build linux

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

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// DialTimeout 是服务进程连 supervisor 的超时。
const DialTimeout = 5 * time.Second

// SuperviseArgs 判断一组参数是否是 supervisor 模式，并拆出 socket、策略文件与子进程 argv。
//
// 形状：--supervisor --socket <path> --policy <path> -- <argv...>
func SuperviseArgs(args []string) (socket, policy string, argv []string, ok bool) {
	if len(args) == 0 || args[0] != "--supervisor" {
		return "", "", nil, false
	}
	i := 1
	for i < len(args) {
		if args[i] == "--" {
			return socket, policy, args[i+1:], true
		}
		if args[i] == "--socket" && i+1 < len(args) {
			socket = args[i+1]
			i += 2
			continue
		}
		if args[i] == "--policy" && i+1 < len(args) {
			policy = args[i+1]
			i += 2
			continue
		}
		i++
	}
	return socket, policy, nil, true
}

// RunSupervisor 是 supervisor 的主体：托管一个子进程，并把它的 socketpair
// 通过 unix socket 暴露给服务进程。返回进程退出码。
//
// 子进程的 fd 约定与非托管路径（launch）完全一致：3 = 策略文件，4 = 控制通道。
// argv 里的 `--file 3`（bwrap 把策略放进沙盒）与 `--ctrl 4`（agent 读控制通道）
// 都依赖它；只传控制通道会让两者同时错位。
//
// 子进程退出时 supervisor 也退出——沙盒没了，留着没有意义。
func RunSupervisor(socket, policy string, argv []string) int {
	if socket == "" || policy == "" || len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "gosandbox-inner: supervisor needs --socket, --policy and a command after --")
		return 2
	}
	policyFile, err := os.Open(policy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gosandbox-inner: supervisor policy: %v\n", err)
		return 1
	}
	defer policyFile.Close()
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
	// ExtraFiles 从 fd 3 开始：3 = 策略文件，4 = 控制通道（见 policyFD / ctrlFD）。
	cmd.ExtraFiles = []*os.File{policyFile, agentFile}
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

	s := &supervisor{ln: ln, agent: hostConn, broken: make(chan struct{})}
	go s.readAgent()
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
//
// agent → 服务进程方向只有**一个**读循环（readAgent），而不是每个连接各起一个
// io.Copy：后者在服务进程断开后仍会阻塞在 agent 的读上，下一批字节会被
// 这个「已死连接」的 goroutine 吞掉，新连接就丢帧。
type supervisor struct {
	ln    net.Listener
	agent net.Conn

	mu sync.Mutex
	// svc 是当前的服务进程连接；nil 表示没有人在接，此时 agent 发来的帧被丢弃。
	svc net.Conn
	// hello 是 agent 的 Hello 帧（含帧头）。agent 只在启动时发一次，
	// 之后接管的服务进程等不到它，所以要缓存并在每次接管时重放。
	hello  []byte
	closed bool
	// broken 在 agent 侧连接不可恢复（agent 死了）时关闭。
	broken chan struct{}
	once   sync.Once
}

// readAgent 读 agent 的帧并转给当前服务进程。
//
// agent → 宿主方向只有帧（没有裸字节：FsWrite 的裸数据只出现在宿主 → agent 方向），
// 所以可以按帧读写，不会把半帧交给新接管的连接。
func (s *supervisor) readAgent() {
	var kind proto.Kind
	var payload []byte
	for {
		if err := proto.ReadFrame(s.agent, &kind, &payload); err != nil {
			s.markBroken()
			return
		}
		frame := proto.Frame(kind, payload)
		s.mu.Lock()
		if kind == proto.KindHello && s.hello == nil {
			s.hello = frame
		}
		svc := s.svc
		s.mu.Unlock()
		if svc == nil {
			continue // 没人接：这一帧丢了，见文件头「代价」
		}
		if _, err := svc.Write(frame); err != nil {
			// 写失败说明这条连接已经死了；关掉让 pump 收尾，下一次接管再恢复。
			_ = svc.Close()
		}
	}
}

// markBroken 标记 agent 侧不可恢复。
func (s *supervisor) markBroken() {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.broken)
	})
}

// acceptLoop 接受服务进程的重连。
func (s *supervisor) acceptLoop() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		// 新服务进程接管：旧连接（如果有）先断掉。
		//
		// 重放 Hello 与登记 svc 必须在同一把锁里：readAgent 在同一把锁下
		// 缓存 Hello 并读取 svc，两者不论谁先到，Hello 都恰好被送出一次。
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			c.Close()
			return
		}
		if s.hello != nil {
			_ = c.SetWriteDeadline(time.Now().Add(DialTimeout))
			_, werr := c.Write(s.hello)
			_ = c.SetWriteDeadline(time.Time{})
			if werr != nil {
				s.mu.Unlock()
				c.Close()
				continue
			}
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

// pump 把一个服务进程连接的字节转给 agent，直到它断开。
//
// 服务进程 → agent 方向按字节流转发即可：协议是帧化的，且同一时刻只有
// 一个写者。注意这里**只关 svc、不关 agent**：agent 是沙盒的命脉，
// 断开时未读完的帧会丢——这是「断开的那一刻」的固有代价。
func (s *supervisor) pump(svc net.Conn) {
	_, _ = io.Copy(s.agent, svc)
	s.mu.Lock()
	if s.svc == svc {
		s.svc = nil
	}
	s.mu.Unlock()
	_ = svc.Close()
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
