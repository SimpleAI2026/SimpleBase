// 文件说明：membuf.go — internal/sandbox/sshd 中 NewPipe、WithRemote 等符号的实现
//
// 用法
//
//	NewPipe() (net.Conn, net.Conn)
//	WithRemote(conn net.Conn, remote string) net.Conn  // WithRemote 返回一个报告指定远端地址的连接包装，供服务端按真实来源限速
//	同包调用：Write() → server.go, session.go；Close() → server.go, session.go；RemoteAddr() → server.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sandbox/sshd/membuf.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] server.go, session.go
//	[关键引用] Write() ← server.go, session.go
//
// 主要符号
//
//	NewPipe() (net.Conn, net.Conn)
//	WithRemote(conn net.Conn, remote string) net.Conn
//	... 另有 17 个顶层符号

package sshd

import (
	"io"
	"net"
	"sync"
	"time"
)

// NewPipe 返回一对带缓冲的全双工内存连接，用于把 SSH 客户端与 ServeConn 在进程内接起来
// （浏览器终端的 WebSocket 桥）。
//
// 不能用 net.Pipe：它是无缓冲的同步管道，SSH 握手时双方都先写版本串、互相等对方读，会死锁。
// 带缓冲后写永不阻塞，读在无数据时阻塞到有数据、关闭或超时。远端地址固定为 "mem:0"，
// 服务端据此做来源限速时，所有进程内连接共享同一个来源；桥应改用 WithRemote 给出真实客户端地址。
func NewPipe() (net.Conn, net.Conn) {
	a2b, b2a := newByteBuf(), newByteBuf()
	return &memConn{r: b2a, w: a2b, mu: new(sync.Mutex), rd: new(time.Time)},
		&memConn{r: a2b, w: b2a, mu: new(sync.Mutex), rd: new(time.Time)}
}

// byteBuf 是无界缓冲的单向字节流。
type byteBuf struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []byte
	closed bool
}

func newByteBuf() *byteBuf {
	b := &byteBuf{}
	b.cond = sync.NewCond(&b.mu)
	return b
}

func (b *byteBuf) write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, io.ErrClosedPipe
	}
	b.buf = append(b.buf, p...)
	b.cond.Broadcast()
	return len(p), nil
}

// read 在无数据时阻塞；deadline 到期返回超时错误（net.Conn 约定）。
func (b *byteBuf) read(p []byte, deadline func() time.Time) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for len(b.buf) == 0 && !b.closed {
		if d := deadline(); !d.IsZero() {
			wait := time.Until(d)
			if wait <= 0 {
				return 0, timeoutErr{}
			}
			t := time.AfterFunc(wait, func() { b.mu.Lock(); b.cond.Broadcast(); b.mu.Unlock() })
			b.cond.Wait()
			t.Stop()
			if len(b.buf) == 0 && !b.closed && !time.Now().Before(d) {
				return 0, timeoutErr{}
			}
			continue
		}
		b.cond.Wait()
	}
	if len(b.buf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, b.buf)
	b.buf = b.buf[n:]
	return n, nil
}

func (b *byteBuf) close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.cond.Broadcast()
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

type memConn struct {
	r, w   *byteBuf
	mu     *sync.Mutex
	rd     *time.Time
	remote string
}

func (c *memConn) Read(p []byte) (int, error) {
	return c.r.read(p, func() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return *c.rd })
}
func (c *memConn) Write(p []byte) (int, error) { return c.w.write(p) }
func (c *memConn) Close() error                { c.w.close(); c.r.close(); return nil }
func (c *memConn) LocalAddr() net.Addr         { return memAddr{} }
func (c *memConn) RemoteAddr() net.Addr {
	if c.remote != "" {
		return memAddr{s: c.remote}
	}
	return memAddr{}
}

// WithRemote 返回一个报告指定远端地址的连接包装，供服务端按真实来源限速。
// conn 必须是 NewPipe 返回的连接。
func WithRemote(conn net.Conn, remote string) net.Conn {
	if mc, ok := conn.(*memConn); ok {
		cp := *mc
		cp.remote = remote
		return &cp
	}
	return conn
}
func (c *memConn) SetDeadline(t time.Time) error {
	return c.SetReadDeadline(t)
}
func (c *memConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	*c.rd = t
	c.mu.Unlock()
	c.r.mu.Lock()
	c.r.cond.Broadcast()
	c.r.mu.Unlock()
	return nil
}
func (c *memConn) SetWriteDeadline(time.Time) error { return nil }

type memAddr struct{ s string }

func (memAddr) Network() string { return "mem" }
func (a memAddr) String() string {
	if a.s != "" {
		return a.s
	}
	return "mem:0"
}
