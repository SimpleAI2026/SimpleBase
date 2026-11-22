package agent

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// proc 是 agent 管的一个进程。
//
// 生命周期：start 建好 cmd 并启动 → 两个输出泵把 stdout/stderr 切片发给宿主
// → 进程结束后发 Exited 并从表里删掉。
type proc struct {
	id  string
	cmd *exec.Cmd
	// pty 非空表示 PTY 模式：输出只有一路，stdin 与 stdout 是同一个 fd。
	pty *os.File
	// stdin 是父进程侧的写端；PTY 模式下就是 pty 本身。
	stdin *os.File
	// slave 是 PTY 模式的从端；Start 之后关闭，只留主端。
	slave *os.File
	// readers 是管道模式的输出读端；PTY 模式为空，输出从 pty 读。
	readers []pipeReader
	// consumed 是已写出但宿主还不知道的字节数（见 InputConsumed）。
	consumed atomic.Uint32
	// input 是待写入的 stdin 数据；单独一个 goroutine 写，避免阻塞读循环。
	input chan []byte
	// closeOnce 保证 stdin 只关一次。
	closeOnce sync.Once
	done      chan struct{}
}

// start 启动一个进程。argv[0] 按 PATH 解析，失败回 127（与 shell 一致）。
func (s *Server) start(m proto.Msg) error {
	if len(m.Argv) == 0 {
		return errors.New("no command to run")
	}
	if m.ID == "" {
		return errors.New("process id is required")
	}
	if s.lookup(m.ID) != nil {
		return fmt.Errorf("process %s already exists", m.ID)
	}

	// 先自己解析可执行文件：「找不到命令」在 shell 语义里是 127，
	// 不是 agent 的致命错误，也不该让连接断开。
	exe, lookErr := exec.LookPath(m.Argv[0])
	if lookErr != nil {
		_ = s.send(&proto.Msg{Kind: proto.KindExited, ID: m.ID, Reason: proto.ExitCode, Code: 127})
		return nil
	}
	// 用户切换：先解析，再确认沙盒里真的能切过去。
	//
	// 单段 uid 映射下只有 0 可用。之前这里完全没读 m.User，等于
	// 请求「以 999 运行」实际跑成了 0——静默的权限差异，先修掉。
	cred, uerr := credentialFor(m.User)
	if uerr != nil {
		_ = s.send(&proto.Msg{Kind: proto.KindExited, ID: m.ID, Reason: proto.ExitCode, Code: 126,
			Err: uerr.Error()})
		return nil
	}
	return s.startWith(m, exe, cred)
}

// startWith 在解析完 exe 与凭证之后启动进程。
func (s *Server) startWith(m proto.Msg, exe string, cred *syscall.Credential) error {
	cmd := exec.Command(exe, m.Argv[1:]...)
	cmd.Env = m.Env
	if cmd.Env == nil {
		cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	}
	if m.Cwd != "" {
		cmd.Dir = m.Cwd
	}
	// 每个进程独立会话与进程组：信号打到整组，宿主的 Ctrl-C 不会误传到 agent。
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Credential: cred}

	p := &proc{id: m.ID, cmd: cmd, input: make(chan []byte, 64), done: make(chan struct{})}

	if m.TTY {
		master, slave, err := openPTY(m.Rows, m.Cols)
		if err != nil {
			return fmt.Errorf("open pty: %w", err)
		}
		p.pty, p.stdin = master, master
		p.slave = slave
		cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0, Credential: cred}
	} else {
		stdinR, stdinW, err := os.Pipe()
		if err != nil {
			return fmt.Errorf("stdin pipe: %w", err)
		}
		p.stdin = stdinW
		cmd.Stdin = stdinR
		// stdout / stderr 各自一根管道，输出才能标清来源。
		outR, outW, err := os.Pipe()
		if err != nil {
			stdinR.Close()
			stdinW.Close()
			return fmt.Errorf("stdout pipe: %w", err)
		}
		errR, errW, err := os.Pipe()
		if err != nil {
			stdinR.Close()
			stdinW.Close()
			outR.Close()
			outW.Close()
			return fmt.Errorf("stderr pipe: %w", err)
		}
		cmd.Stdout, cmd.Stderr = outW, errW
		// 父进程侧要关掉写端，读端才会在子进程退出时收到 EOF。
		defer func() { outW.Close(); errW.Close() }()
		p.readers = []pipeReader{
			{f: outR, stream: proto.StreamStdout},
			{f: errR, stream: proto.StreamStderr},
		}
		defer func() { stdinR.Close() }()
	}

	// 上限先设在 agent 上（fork 会继承），Start 之后立刻恢复：
	// 不这样做，agent 自己也会被这条命令的 nofile / nproc 限制住。
	restoreRlimits, err := applyRlimits(m.Rlimits)
	if err != nil {
		p.cleanup()
		return err
	}
	serr := cmd.Start()
	restoreRlimits()
	// 从端已交给子进程，父进程侧必须关掉，否则读到 EOF 要等子进程的孙进程退出。
	if p.slave != nil {
		p.slave.Close()
		p.slave = nil
	}
	if serr != nil {
		p.cleanup()
		// exec 失败发生在隔离已生效之后，是命令本身的问题：用 127 与 shell 对齐。
		var ee *exec.Error
		if errors.As(serr, &ee) {
			_ = s.send(&proto.Msg{Kind: proto.KindExited, ID: m.ID, Reason: proto.ExitCode, Code: 127})
			return nil
		}
		return fmt.Errorf("start %s: %w", m.Argv[0], serr)
	}

	s.add(p)
	_ = s.send(&proto.Msg{Kind: proto.KindStarted, ID: m.ID, PID: int32(cmd.Process.Pid)})

	// 输出泵：先读完残留输出，再发 Exited，保证宿主按顺序收到。
	var wg sync.WaitGroup
	if p.pty != nil {
		wg.Add(1)
		go func() { defer wg.Done(); s.pump(p, p.pty, proto.StreamStdout) }()
	} else {
		for _, r := range p.readers {
			wg.Add(1)
			go func(r pipeReader) { defer wg.Done(); s.pump(p, r.f, r.stream) }(r)
		}
	}
	go s.stdinLoop(p)
	go func() {
		werr := cmd.Wait()
		wg.Wait()
		reason, code, sig := exitStatus(cmd.ProcessState, werr)
		s.forget(m.ID)
		p.cleanup()
		_ = s.send(&proto.Msg{Kind: proto.KindExited, ID: m.ID, Reason: reason, Code: code, Sig: sig})
		close(p.done)
	}()
	return nil
}

// pipeReader 是一根输出管道与它的流标记。
type pipeReader struct {
	f      *os.File
	stream proto.Stream
}

// pump 读一根输出管道，按 MaxOutputChunk 切片发给宿主。
//
// 切片而不是整块转发：单帧上限 1 MiB，而宿主的读缓冲按 64 KiB 复用，
// 固定大小让两端都能用同一块缓冲。
func (s *Server) pump(p *proc, r io.Reader, stream proto.Stream) {
	buf := make([]byte, proto.MaxOutputChunk)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			if serr := s.send(&proto.Msg{Kind: proto.KindOutput, ID: p.id, Stream: stream, Data: data}); serr != nil {
				return // 宿主走了，输出没人要
			}
		}
		if err != nil {
			return
		}
	}
}

// stdinLoop 串行写入 stdin，并如实上报已消费的字节数（宿主据此补窗口）。
//
// 写成功才计消费：进程不读时写会阻塞，宿主的窗口随之耗尽并停发，
// 这正是我们要的背压。
func (s *Server) stdinLoop(p *proc) {
	for data := range p.input {
		if _, err := p.stdin.Write(data); err != nil {
			return
		}
		// 攒够一批再上报，减少帧数。
		total := p.consumed.Add(uint32(len(data)))
		if total >= proto.MaxOutputChunk/2 {
			p.consumed.Store(0)
			_ = s.send(&proto.Msg{Kind: proto.KindInputConsumed, ID: p.id, Bytes: total})
		}
	}
}

// drainConsumed 把剩下的 InputConsumed 发出去（进程结束时调用）。
func (s *Server) drainConsumed(p *proc) {
	if n := p.consumed.Swap(0); n > 0 {
		_ = s.send(&proto.Msg{Kind: proto.KindInputConsumed, ID: p.id, Bytes: n})
	}
}

// input 把一块 stdin 数据交给进程。超过窗口由宿主控制，这里只做缓冲。
func (s *Server) input(m proto.Msg) {
	p := s.lookup(m.ID)
	if p == nil {
		return
	}
	if len(m.Data) == 0 {
		return
	}
	// 缓冲满了说明进程长期不读；丢掉这一块并如实回一个错误，
	// 而不是阻塞 agent 的读循环。
	select {
	case p.input <- m.Data:
	default:
		_ = s.send(&proto.Msg{Kind: proto.KindError, Err: "stdin buffer full for " + m.ID})
	}
}

// closeStdin 关掉写端，进程读到 EOF。
func (s *Server) closeStdin(m proto.Msg) {
	p := s.lookup(m.ID)
	if p == nil {
		return
	}
	p.closeOnce.Do(func() {
		close(p.input)
		// PTY 模式关掉主端会终止输出，所以只关管道模式的写端。
		if p.pty == nil {
			s.drainConsumed(p)
			p.stdin.Close()
		}
	})
}

// signal 给进程组发信号。任意信号都允许：agent 在沙盒内，不会误伤宿主。
func (s *Server) signal(m proto.Msg) {
	p := s.lookup(m.ID)
	if p == nil || p.cmd.Process == nil {
		return
	}
	pid := p.cmd.Process.Pid
	sig := syscall.Signal(m.Signal)
	// 负 pid 打到整个进程组；组不存在时退回单进程。
	if err := syscall.Kill(-pid, sig); err != nil {
		_ = syscall.Kill(pid, sig)
	}
}

// resize 改 PTY 窗口大小；管道模式忽略。
func (s *Server) resize(m proto.Msg) {
	p := s.lookup(m.ID)
	if p == nil || p.pty == nil {
		return
	}
	_ = setWinsize(p.pty, m.Rows, m.Cols)
}

// killAll 结束所有进程（agent 关闭时调用）。
func (s *Server) killAll() {
	for _, p := range s.snapshot() {
		if p.cmd.Process != nil {
			pid := p.cmd.Process.Pid
			if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		p.cleanup()
	}
}

// cleanup 释放 fd。可重复调用。
func (p *proc) cleanup() {
	if p.slave != nil {
		p.slave.Close()
	}
	if p.pty != nil {
		p.pty.Close()
		return
	}
	if p.stdin != nil {
		p.stdin.Close()
	}
	for _, r := range p.readers {
		r.f.Close()
	}
}

// lookup 按 id 取进程；不存在返回 nil。
func (s *Server) lookup(id string) *proc {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.procs[id]
}

// add 把进程放进表里。
func (s *Server) add(p *proc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.procs[p.id] = p
}

// snapshot 返回当前进程表的一份副本，供 killAll 遍历（避免持锁调用可能阻塞的操作）。
func (s *Server) snapshot() []*proc {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*proc, 0, len(s.procs))
	for _, p := range s.procs {
		out = append(out, p)
	}
	return out
}

// forget 从进程表里删掉。
func (s *Server) forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.procs, id)
}

// exitStatus 把 ProcessState 翻译成协议里的退出原因。
func exitStatus(st *os.ProcessState, werr error) (proto.ExitReason, int32, int32) {
	if st == nil {
		if werr != nil {
			return proto.ExitCode, -1, 0
		}
		return proto.ExitCode, 0, 0
	}
	if ws, ok := st.Sys().(syscall.WaitStatus); ok {
		if ws.Signaled() {
			return proto.ExitSignaled, 0, int32(ws.Signal())
		}
		return proto.ExitCode, int32(ws.ExitStatus()), 0
	}
	return proto.ExitCode, int32(st.ExitCode()), 0
}
