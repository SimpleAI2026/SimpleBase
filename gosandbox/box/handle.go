// 文件说明：handle.go — 一个由 agent 托管的进程：输出、stdin、信号、等待
//
// 用法
//
//	h, err := sess.Exec(ctx, box.ExecRequest{Argv: []string{"sh", "-c", "make"}})
//	io.Copy(dst, h.Output())  // 必须持续读：输出走背压，不堆内存
//	h.Write([]byte("y\n"))  // stdin；超过 InputWindow 会阻塞
//	h.Signal(15)  // 打到整个进程组
//	ex := h.Wait()  // ExitCode / Signaled / Reason
//
// 调用架构
//
//	[上游] gosandbox/vm/box、gosandbox/box（Session.Exec / Start）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/handle.go
//	         │
//	         ▼
//	[下游] gosandbox/proto
//	[同包] session.go
//
// 主要符号
//
//	type ExecRequest struct {…}
//	type Handle struct {…}
//	type Exit struct {…}
//	(s *Session) Exec(ctx context.Context, r ExecRequest) (*Handle, error)
//	(h *Handle) Output() io.Reader
//	(h *Handle) Write(p []byte) (int, error)
//	(h *Handle) Resize(rows, cols uint16) error
//	(h *Handle) Signal(sig int32) error
//	(h *Handle) Wait() Exit
//	(h *Handle) Close() error

package box

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// ExecRequest 是一次进程启动请求。
type ExecRequest struct {
	// ID 是进程 id；空表示由 Session 生成。
	ID string
	// Argv 是完整命令。
	Argv []string
	// Env 是环境变量（KEY=VALUE）。nil 表示用 agent 的默认 PATH。
	Env []string
	// Cwd 是工作目录。
	Cwd string
	// User 是 "uid:gid"；空表示继承 agent。
	User string
	// TTY 为真时分配伪终端。
	TTY bool
	// Rows / Cols 是 TTY 的初始大小。
	Rows, Cols uint16
	// Stdin 为真时 agent 保留 stdin 管道。
	Stdin bool
	// Rlimits 是资源上限。
	Rlimits []proto.Rlimit
}

// Exit 是进程结束信息。
type Exit struct {
	Reason   proto.ExitReason
	Code     int32
	Signal   int32
	Signaled bool
	// Err 是 agent 给的说明（可选）：例如「请求的 uid 在沙盒里无法映射」。
	//
	// 与 Reason 分开：Reason 是「怎么结束的」（正常/信号/超时），
	// Err 是「为什么没法开始」。后者不该被当成普通退出码吞掉。
	Err string
}

// Handle 是一个由 agent 托管的进程。
type Handle struct {
	sess *Session
	id   string

	// out 是合并后的输出（PTY 只有一路，管道模式 stdout 与 stderr 合在一起）。
	// 用管道而不是缓冲：调用方读得慢时 agent 与进程自然被背压挡住。
	outR *io.PipeReader
	outW *io.PipeWriter
	// log 是这个进程的日志；nil 表示不落盘。
	log *Log

	started chan struct{}
	exited  chan struct{}

	mu   sync.Mutex
	exit Exit
	// pendingStdin 是已发出但未被 agent 确认消费的字节数，用于窗口流控。
	pendingStdin int
	windowCond   *sync.Cond
	// stdinClosed 表示已发 CloseStdin。
	stdinClosed bool
	// closed 表示 Close 已调用。
	closed bool
}

// Exec 在沙盒里启动一个进程并返回句柄；Started 已经到达才返回。
func (s *Session) Exec(ctx context.Context, r ExecRequest) (*Handle, error) {
	if len(r.Argv) == 0 {
		return nil, errors.New("box: empty argv")
	}
	id := r.ID
	if id == "" {
		id = fmt.Sprintf("p%d", s.nextSeq())
	}
	h := newHandle(s, id, r.TTY)
	s.mu.Lock()
	s.handles[id] = h
	s.mu.Unlock()

	m := &proto.Msg{Kind: proto.KindStart, ID: id, Argv: r.Argv, Env: r.Env, Cwd: r.Cwd,
		User: r.User, TTY: r.TTY, Rows: r.Rows, Cols: r.Cols, Stdin: r.Stdin, Rlimits: r.Rlimits}
	if err := s.send(m); err != nil {
		s.dropHandle(id)
		h.closePipes()
		return nil, err
	}
	// context 取消时要能把已经起来的进程收掉，所以用一个 goroutine 看着 ctx。
	if ctx.Done() != nil {
		go func() {
			select {
			case <-ctx.Done():
				_ = h.Signal(15)
			case <-h.exited:
			case <-s.done:
			}
		}()
	}
	select {
	case <-h.started:
		return h, nil
	case <-h.exited:
		// 启动失败（找不到命令）：Exited 已到，如实返回句柄让调用方读退出码。
		return h, nil
	case <-ctx.Done():
		s.dropHandle(id)
		h.closePipes()
		return nil, ctx.Err()
	case <-s.done:
		s.dropHandle(id)
		h.closePipes()
		return nil, io.ErrClosedPipe
	}
}

// newHandle 建一个句柄并接好输出管道。
func newHandle(s *Session, id string, tty bool) *Handle {
	r, w := io.Pipe()
	h := &Handle{sess: s, id: id, outR: r, outW: w,
		started: make(chan struct{}), exited: make(chan struct{})}
	h.windowCond = sync.NewCond(&h.mu)
	// 日志与句柄一起建：进程一启动输出就该能落盘。
	if s.sinks != nil {
		h.log = s.sinks.open(id)
	}
	return h
}

// streamOf 把协议的流标记转成日志的流标记。
func streamOf(s proto.Stream) Stream {
	if s == proto.StreamStderr {
		return StreamStderr
	}
	return StreamStdout
}

// dropHandle 从 Session 里删掉这个句柄。
func (s *Session) dropHandle(id string) {
	s.mu.Lock()
	delete(s.handles, id)
	s.mu.Unlock()
}

// closePipes 关掉输出管道，让仍在读的调用方收到 EOF。
func (h *Handle) closePipes() {
	_ = h.outW.Close()
}

// deliver 收一帧（由 Session 的读循环调用）。
func (h *Handle) deliver(m proto.Msg) {
	switch m.Kind {
	case proto.KindStarted:
		select {
		case <-h.started:
		default:
			close(h.started)
		}
	case proto.KindOutput:
		// 先落盘再转发：日志是回放的唯一来源，不能因为读端慢而丢。
		if h.log != nil && h.sess.sinks != nil {
			h.sess.sinks.write(h.log, streamOf(m.Stream), m.Data)
		}
		// 写管道是阻塞的：读端不读时 agent 的输出泵会等，
		// 背压因此一路传到沙盒内进程的 write。
		_, _ = h.outW.Write(m.Data)
	case proto.KindInputConsumed:
		h.mu.Lock()
		h.pendingStdin -= int(m.Bytes)
		if h.pendingStdin < 0 {
			h.pendingStdin = 0
		}
		h.windowCond.Broadcast()
		h.mu.Unlock()
	case proto.KindExited:
		// 进程结束：关掉它的日志（落盘并释放 fd）。
		if h.sess.sinks != nil {
			h.sess.sinks.close(h.id)
		}
		h.mu.Lock()
		h.exit = Exit{Reason: m.Reason, Code: m.Code, Signal: m.Sig,
			Signaled: m.Reason == proto.ExitSignaled, Err: m.Err}
		h.mu.Unlock()
		// 输出先于 Exited 到齐（agent 保证顺序），所以这里可以直接关。
		_ = h.outW.Close()
		select {
		case <-h.exited:
		default:
			close(h.exited)
		}
		h.sess.dropHandle(h.id)
	}
}

// Output 是合并后的输出流。调用方必须持续读；Close 之后读会得到 EOF。
func (h *Handle) Output() io.Reader { return h.outR }

// ID 是进程 id。
func (h *Handle) ID() string { return h.id }

// Write 向 stdin 写。超过 InputWindow 未消费的部分会阻塞：
// 这是有意的背压，而不是把内存堆满。
func (h *Handle) Write(p []byte) (int, error) {
	written := 0
	for written < len(p) {
		h.mu.Lock()
		for h.pendingStdin >= proto.InputWindow && !h.stdinClosed {
			h.windowCond.Wait()
		}
		if h.stdinClosed {
			h.mu.Unlock()
			return written, io.ErrClosedPipe
		}
		h.mu.Unlock()

		// 一次最多发 MaxOutputChunk，帧不会超限。
		n := len(p) - written
		if n > proto.MaxOutputChunk {
			n = proto.MaxOutputChunk
		}
		chunk := make([]byte, n)
		copy(chunk, p[written:written+n])

		h.mu.Lock()
		h.pendingStdin += n
		h.mu.Unlock()
		if err := h.sess.send(&proto.Msg{Kind: proto.KindInput, ID: h.id, Data: chunk}); err != nil {
			return written, err
		}
		written += n
	}
	return written, nil
}

// CloseStdin 让进程读到 EOF。
func (h *Handle) CloseStdin() error {
	h.mu.Lock()
	if h.stdinClosed {
		h.mu.Unlock()
		return nil
	}
	h.stdinClosed = true
	h.windowCond.Broadcast()
	h.mu.Unlock()
	return h.sess.send(&proto.Msg{Kind: proto.KindCloseStdin, ID: h.id})
}

// Resize 改 PTY 大小；非 TTY 进程忽略。
func (h *Handle) Resize(rows, cols uint16) error {
	return h.sess.send(&proto.Msg{Kind: proto.KindResize, ID: h.id, Rows: rows, Cols: cols})
}

// Signal 给整个进程组发信号。任意信号都允许。
func (h *Handle) Signal(sig int32) error {
	return h.sess.send(&proto.Msg{Kind: proto.KindSignal, ID: h.id, Signal: proto.Signal(sig)})
}

// Wait 阻塞到进程结束。
func (h *Handle) Wait() Exit {
	<-h.exited
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.exit
}

// Done 在进程结束后关闭。
func (h *Handle) Done() <-chan struct{} { return h.exited }

// Close 结束进程并释放管道；进程已结束时只是释放。
func (h *Handle) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	h.windowCond.Broadcast()
	h.mu.Unlock()

	// 先礼后兵：SIGTERM 给进程一个清理的机会，宽限期后 SIGKILL。
	select {
	case <-h.exited:
	default:
		_ = h.Signal(15)
		select {
		case <-h.exited:
		case <-h.sess.done:
		}
	}
	_ = h.outR.Close()
	h.sess.dropHandle(h.id)
	return nil
}
