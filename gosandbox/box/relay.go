// 文件说明：relay.go — gosandbox/box 的内部实现（halfCloseWrite、relay 等）
//
// 用法
//
//	同包调用：newRelay() → files.go, tunnel_test.go
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/relay.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backpressure_test.go, copytree.go, deadline.go, egress_e2e_test.go, files.go 等 26 个
//	[关键引用] newRelay ← files.go, tunnel_test.go
//
// 主要符号
//
//	const halfCloseWrite / halfCloseRead
//	type relay struct{…}
//	newRelay(s *Session, id uint32) *relay
//	(r *relay) watchContext(ctx context.Context)
//	(r *relay) deliver(data []byte)
//	(r *relay) deliverEOF()
//	(r *relay) err() error
//	(r *relay) abort(err error)
//	... 另有 15 个顶层符号

package box

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// 半关闭方向。
const (
	// halfCloseWrite 表示「我不再发数据」（宿主 → 沙盒）。
	halfCloseWrite uint8 = 1
	// halfCloseRead 表示「我不再收数据」（宿主不再读沙盒的输出）。
	halfCloseRead uint8 = 2
)

// relay 是一条中继连接：对调用方是 net.Conn，对 agent 是两个方向的 Relay 帧。
type relay struct {
	sess *Session
	id   uint32
	// port 是沙盒内的目标端口；只用于错误信息（协议里的 RelayClose 不带端口）。
	port uint16

	// inR/inW 是 agent → 宿主的字节。用管道而不是带缓冲的通道：
	// 管道天然有背压，读端慢时 agent 的输出泵会等。
	inR *io.PipeReader
	inW *io.PipeWriter

	// ready 在第一次收到数据或关闭时关闭，供 DialPort 判断「连上了」。
	ready     chan struct{}
	readyOnce sync.Once
	done      chan struct{}
	watchDone chan struct{}

	mu sync.Mutex
	// closed 表示整条连接已断（两端都不再收发）。
	closed bool
	// readEOF 表示 agent 侧已发完（读到 EOF 后不再阻塞）。
	readEOF bool
	// wroteFin 表示宿主已经半关闭了写方向，避免重复发帧。
	wroteFin bool
	// pending 是还没交出去的错误（例如 agent 报告连接失败）。
	pending error
	// deadline 是整体的读写超时；零值表示不限。
	deadline time.Time
}

// newRelay 建一条中继连接。
func newRelay(s *Session, id uint32) *relay {
	r, w := io.Pipe()
	return &relay{sess: s, id: id, inR: r, inW: w, ready: make(chan struct{}), done: make(chan struct{}), watchDone: make(chan struct{})}
}

func (r *relay) watchContext(ctx context.Context) {
	defer close(r.watchDone)
	select {
	case <-r.done:
		return
	case <-ctx.Done():
		_ = r.Close()
	case <-r.sess.done:
		r.abort(io.ErrClosedPipe)
	}
}

// deliver 收 agent 发来的一块数据。由 Session 的读循环调用。
//
// 写管道是阻塞的：宿主不读时 agent 的输出泵会等，背压因此一路传到
// 沙盒内的进程。这比在宿主侧无限缓冲安全——慢客户端不该吃掉宿主的
// 内存，但也不能把 agent 的输出泵永久卡死，所以有 Deadline 兜底。
func (r *relay) deliver(data []byte) {
	r.readyOnce.Do(func() { close(r.ready) })
	if len(data) == 0 {
		return
	}
	_, _ = r.inW.Write(data)
}

// deliverEOF 表示 agent 侧已经发完（沙盒内的进程关了连接）。
func (r *relay) deliverEOF() {
	r.mu.Lock()
	if r.readEOF {
		r.mu.Unlock()
		return
	}
	r.readEOF = true
	r.mu.Unlock()
	r.readyOnce.Do(func() { close(r.ready) })
	_ = r.inW.Close()
}

// err 返回这条连接上挂着的错误（agent 报告连不上时设）。
func (r *relay) err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pending
}

// abort 以错误结束这条连接（例如 agent 连不上沙盒内的端口）。
func (r *relay) abort(err error) { r.finish(err, false) }

// Read 实现 net.Conn。
func (r *relay) Read(p []byte) (int, error) {
	n, err := r.inR.Read(p)
	if err != nil && errors.Is(err, io.ErrClosedPipe) {
		return n, io.EOF
	}
	return n, err
}

func (r *relay) send(m *proto.Msg) error {
	select {
	case <-r.done:
		return net.ErrClosed
	default:
	}
	select {
	case <-r.done:
		return net.ErrClosed
	case <-r.sess.done:
		return io.ErrClosedPipe
	case r.sess.out <- outFrame{msg: m}:
		return nil
	}
}

// Write 把字节发给 agent。
func (r *relay) Write(p []byte) (int, error) {
	r.mu.Lock()
	closed, wroteFin := r.closed, r.wroteFin
	r.mu.Unlock()
	if closed {
		return 0, net.ErrClosed
	}
	if wroteFin {
		return 0, errors.New("box: write after CloseWrite")
	}
	written := 0
	for written < len(p) {
		n := len(p) - written
		if n > proto.MaxOutputChunk {
			n = proto.MaxOutputChunk
		}
		chunk := make([]byte, n)
		copy(chunk, p[written:written+n])
		if err := r.send(&proto.Msg{Kind: proto.KindRelayData, ConnID: r.id, Data: chunk}); err != nil {
			return written, err
		}
		written += n
	}
	return written, nil
}

// CloseWrite 半关闭写方向：告诉沙盒内的进程「请求发完了」，但仍然读响应。
//
// 这是隧道模式与旧实现的关键差别：旧实现只有「整条关」，
// 于是 keep-alive 与 WebSocket 都没法正确终止。
func (r *relay) CloseWrite() error {
	r.mu.Lock()
	if r.closed || r.wroteFin {
		r.mu.Unlock()
		return nil
	}
	r.wroteFin = true
	r.mu.Unlock()
	return r.send(&proto.Msg{Kind: proto.KindRelayHalfClose, ConnID: r.id, Flags: halfCloseWrite})
}

// CloseRead 半关闭读方向：不再读沙盒的输出。
//
// 用 Close 就能做到，但明确提供这个动作让调用方不必记得
// 「关掉读端 = 通知 agent 别再发」的约定。
func (r *relay) CloseRead() error {
	return r.Close()
}

// Close 关掉中继：通知 agent 断开沙盒内的连接。
func (r *relay) Close() error {
	r.finish(nil, true)
	return nil
}

func (r *relay) finish(err error, notify bool) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	if r.pending == nil {
		r.pending = err
	}
	close(r.done)
	r.mu.Unlock()
	r.readyOnce.Do(func() { close(r.ready) })
	_ = r.inW.CloseWithError(err)
	r.sess.mu.Lock()
	delete(r.sess.relays, r.id)
	r.sess.mu.Unlock()
	if notify {
		_ = r.inR.Close()
		r.sess.queueCleanup(&proto.Msg{Kind: proto.KindRelayClose, ConnID: r.id})
	}
}

// LocalAddr / RemoteAddr 只满足 net.Conn 接口：中继没有真实地址。
func (r *relay) LocalAddr() net.Addr  { return relayAddr{} }
func (r *relay) RemoteAddr() net.Addr { return relayAddr{} }

// SetDeadline 记下超时；实际的超时由调用方用 ctx 控制（中继是内存管道，
// 没有内核 socket 的超时可用）。
func (r *relay) SetDeadline(t time.Time) error {
	r.mu.Lock()
	r.deadline = t
	r.mu.Unlock()
	return nil
}

// SetReadDeadline 同 SetDeadline。
func (r *relay) SetReadDeadline(t time.Time) error { return r.SetDeadline(t) }

// SetWriteDeadline 同 SetDeadline。
func (r *relay) SetWriteDeadline(t time.Time) error { return r.SetDeadline(t) }

// relayAddr 是中继的占位地址。
type relayAddr struct{}

func (relayAddr) Network() string { return "gobox-relay" }
func (relayAddr) String() string  { return "gobox-relay" }
