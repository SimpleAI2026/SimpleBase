// 文件说明：interactive.go — gosandbox/vm/fake 中 Start 等符号的实现
//
// 用法
//
//	(d *Driver) Start(ctx context.Context, t sb.Target, req sb.StartSpec) (sb.Proc ...  // Start 实现 vm.Interactive：不启动任何真进程，用一个极小的行解释器模拟交互 shell
//	type fakeProc（包内使用）
//	type byteQueue（包内使用）
//	同包调用：Done() → fake.go；Size() → fake.go；run() → fake.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/fake/interactive.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/vm
//	[同包] fake.go
//	[关键引用] Done() ← fake.go
//
// 主要符号
//
//	(d *Driver) Start(ctx context.Context, t sb.Target, req sb.StartSpec) (sb.Proc ...
//	... 另有 20 个顶层符号

package fake

import (
	"bufio"
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"syscall"

	sb "github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// Start 实现 vm.Interactive：不启动任何真进程，用一个极小的行解释器模拟交互 shell。
//
// 每读到一行输入：把它交给与 Exec 相同的解释器，输出写回；"exit N" 结束会话。
// PTY 模式下输入会被回显（与真实终端一致）。仅供 e2e / 单测。
func (d *Driver) Start(ctx context.Context, t sb.Target, req sb.StartSpec) (sb.Proc, error) {
	d.mu.Lock()
	vm := d.ensureLocked(t)
	d.mu.Unlock()

	inR, inW := io.Pipe()
	q := newByteQueue()
	p := &fakeProc{
		stdin: inW, out: q, outW: q, inR: inR, done: make(chan struct{}),
		pty: req.PTY != nil,
	}
	if req.PTY != nil {
		p.size = *req.PTY
	}
	cctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	line := req.Shell
	if line == "" && req.Cmd != "" && !isShellBinary(req.Cmd) {
		line = strings.Join(append([]string{req.Cmd}, req.Args...), " ")
	}
	go p.run(cctx, d, vm, line, req)
	return p, nil
}

type fakeProc struct {
	stdin io.WriteCloser
	inR   *io.PipeReader
	out   io.Reader
	outW  *byteQueue
	pty   bool

	mu     sync.Mutex
	size   sb.PTYSize
	exit   sb.ProcExit
	done   chan struct{}
	cancel context.CancelFunc
	once   sync.Once
}

func (p *fakeProc) PID() int              { return 4242 }
func (p *fakeProc) Stdin() io.WriteCloser { return p.stdin }
func (p *fakeProc) Output() io.Reader     { return p.out }
func (p *fakeProc) Done() <-chan struct{} { return p.done }
func (p *fakeProc) Wait() sb.ProcExit     { <-p.done; return p.exit }

// Size 返回最近一次 Resize（测试用）。
func (p *fakeProc) Size() sb.PTYSize { p.mu.Lock(); defer p.mu.Unlock(); return p.size }

func (p *fakeProc) Resize(s sb.PTYSize) error {
	if !p.pty {
		return sb.ErrNotSupported
	}
	p.mu.Lock()
	p.size = s
	p.mu.Unlock()
	return nil
}

func (p *fakeProc) Signal(sig syscall.Signal) error {
	if sig != syscall.SIGTERM && sig != syscall.SIGKILL {
		return sb.ErrNotSupported
	}
	p.finish(sb.ProcExit{ExitCode: 128 + int(sig), Signaled: true})
	return nil
}

func (p *fakeProc) Close() error {
	p.finish(sb.ProcExit{ExitCode: 137, Signaled: true})
	<-p.done
	return nil
}

func (p *fakeProc) finish(e sb.ProcExit) {
	p.once.Do(func() {
		p.exit = e
		p.cancel()
		p.inR.CloseWithError(io.EOF)
		// 唤醒可能因无人读取而阻塞在 Write 上的解释器协程。
		p.outW.Close()
		close(p.done)
	})
}

func (p *fakeProc) run(ctx context.Context, d *Driver, vm *fakeVM, first string, req sb.StartSpec) {
	exec := func(line string) (string, int, bool) {
		line = strings.TrimSpace(line)
		if line == "" {
			return "", 0, false
		}
		if f := strings.Fields(line); f[0] == "exit" {
			code := 0
			if len(f) > 1 {
				code, _ = strconv.Atoi(f[1])
			}
			return "", code, true
		}
		res, err := d.run(ctx, vm, line, sb.ExecSpec{Cwd: req.Cwd, Env: req.Env})
		if err != nil {
			return "", 1, false
		}
		return string(res.Stdout) + string(res.Stderr), res.ExitCode, false
	}
	// 非交互：只执行一次命令。
	if first != "" {
		out, code, _ := exec(first)
		if out != "" {
			p.write(out)
		}
		p.finish(sb.ProcExit{ExitCode: code})
		return
	}
	sc := bufio.NewScanner(p.inR)
	for sc.Scan() {
		line := sc.Text()
		if p.pty {
			p.write(line + "\r\n")
		}
		out, code, quit := exec(line)
		if out != "" {
			p.write(strings.ReplaceAll(out, "\n", "\r\n"))
		}
		if quit {
			p.finish(sb.ProcExit{ExitCode: code})
			return
		}
	}
	select {
	case <-p.done:
	default:
		p.finish(sb.ProcExit{})
	}
}

func (p *fakeProc) write(s string) {
	_, _ = p.outW.Write([]byte(s))
}

// byteQueue 是无界缓冲的字节流：写永不阻塞，读在无数据时阻塞到有数据或关闭。
// 真实 PTY / 管道有内核缓冲，这里用它避免测试里的读写互相等待。
type byteQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []byte
	closed bool
}

func newByteQueue() *byteQueue {
	q := &byteQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *byteQueue) Write(b []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return 0, io.ErrClosedPipe
	}
	q.buf = append(q.buf, b...)
	q.cond.Broadcast()
	return len(b), nil
}

func (q *byteQueue) Read(b []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.buf) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.buf) == 0 {
		return 0, io.EOF
	}
	n := copy(b, q.buf)
	q.buf = q.buf[n:]
	return n, nil
}

func (q *byteQueue) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.cond.Broadcast()
}

var _ sb.Interactive = (*Driver)(nil)

// isShellBinary 判断 cmd 是不是一个 shell：此时进入交互循环，而不是把它当一次性命令。
// 真实后端里 /bin/sh 的 stdin 就是交互输入，fake 用行解释器模拟同样的行为。
func isShellBinary(cmd string) bool {
	switch cmd {
	case "sh", "bash", "/bin/sh", "/bin/bash", "/usr/bin/sh", "/usr/bin/bash":
		return true
	}
	return false
}
