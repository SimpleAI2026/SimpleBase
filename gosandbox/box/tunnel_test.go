package box

// tunnel_test.go：中继的隧道语义（半关闭、全双工、错误传播）。
//
// 中继本身是纯内存管道 + 一个 send 回调，不含平台相关代码，
// 所以这个文件不加 build tag：在 macOS 上也能跑。

import (
	"bytes"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// newRelayForTest 建一个能发帧的 Session 与一条中继。
//
// Session 需要 out 通道与 done 通道才能 send 成功，这里按最小集构造。
func newRelayForTest(t *testing.T, id uint32) (*relay, *Session) {
	t.Helper()
	s := &Session{
		cfg:     Config{},
		out:     make(chan outFrame, 1024),
		done:    make(chan struct{}),
		handles: map[string]*Handle{},
		pending: map[uint32]chan proto.Msg{},
		relays:  map[uint32]*relay{},
	}
	r := newRelay(s, id)
	s.mu.Lock()
	s.relays[id] = r
	s.mu.Unlock()
	return r, s
}

// sentFrames 取出发出去的帧（非阻塞）。
func sentFrames(t *testing.T, s *Session) []*proto.Msg {
	t.Helper()
	var out []*proto.Msg
	for {
		select {
		case f := <-s.out:
			out = append(out, f.msg)
		default:
			return out
		}
	}
}

// 半关闭：写方向关掉后，仍能读到 agent 发来的数据。
func TestRelayHalfCloseKeepsReading(t *testing.T) {
	r, s := newRelayForTest(t, 1)

	go func() {
		time.Sleep(10 * time.Millisecond)
		r.deliver([]byte("response-body"))
		time.Sleep(10 * time.Millisecond)
		r.deliverEOF()
	}()

	if err := r.CloseWrite(); err != nil {
		t.Fatalf("half close: %v", err)
	}
	// 半关闭之后不该允许再写。
	if _, err := r.Write([]byte("x")); err == nil {
		t.Fatal("write after CloseWrite must fail")
	}
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != "response-body" {
		t.Fatalf("body = %q", body)
	}
	// 发出的帧里应当有一个 HalfClose。
	var sawHalf bool
	for _, m := range sentFrames(t, s) {
		if m.Kind == proto.KindRelayHalfClose && m.Flags&halfCloseWrite != 0 {
			sawHalf = true
		}
	}
	if !sawHalf {
		t.Fatal("CloseWrite must emit a half-close frame")
	}
}

// 全双工：1 MiB 数据不被阻塞地流过。
func TestRelayFullDuplex(t *testing.T) {
	r, _ := newRelayForTest(t, 2)

	const chunk = 32 << 10
	total := 1 << 20
	go func() {
		for sent := 0; sent < total; sent += chunk {
			r.deliver(make([]byte, chunk))
		}
		r.deliverEOF()
	}()

	read := 0
	buf := make([]byte, chunk)
	for {
		n, err := r.Read(buf)
		read += n
		if err != nil {
			break
		}
	}
	if read != total {
		t.Fatalf("read %d bytes, want %d", read, total)
	}
}

// abort：agent 连不上端口时，读端拿到错误而不是永久阻塞。
func TestRelayAbort(t *testing.T) {
	r, _ := newRelayForTest(t, 3)
	want := errors.New("dial failed")
	go func() {
		time.Sleep(10 * time.Millisecond)
		r.abort(want)
	}()
	_, err := io.ReadAll(r)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

// 关闭之后读返回 EOF，写返回 ErrClosed。
func TestRelayClose(t *testing.T) {
	r, _ := newRelayForTest(t, 4)
	if err := r.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := r.Read(make([]byte, 8)); err != io.EOF {
		t.Fatalf("read after close = %v", err)
	}
	if _, err := r.Write([]byte("x")); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("write after close = %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

// 半关闭回来的帧：agent 说「我发完了」，宿主读到 EOF 但还能写。
func TestRelayDeliverEOFIsReadOnly(t *testing.T) {
	r, _ := newRelayForTest(t, 5)
	r.deliverEOF()
	if _, err := r.Read(make([]byte, 8)); err != io.EOF {
		t.Fatalf("read = %v", err)
	}
	// 写方向仍然可用：agent 只是发完了，没说不收。
	if _, err := r.Write([]byte("still-open")); err != nil {
		t.Fatalf("write after peer EOF = %v", err)
	}
}

// 端到端形状：请求体发完（半关闭）后服务端仍能回写。
//
// 用真实 TCP 回环而不是 net.Pipe：半关闭是 TCP 的语义
// （FIN 只关一个方向），net.Pipe 在部分平台上没有 CloseWrite。
func TestTunnelKeepAliveShape(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// 「沙盒内的服务端」：读到 EOF 才回写，模拟 HTTP/1.1 服务端。
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		c, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer c.Close()
		var req bytes.Buffer
		_, _ = io.Copy(&req, c) // 直到对端 FIN
		_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\n\r\n" + req.String()))
	}()

	hostSide, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer hostSide.Close()

	if _, err := hostSide.Write([]byte("GET / HTTP/1.1\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	// 半关闭写方向：服务端据此读到 EOF，但还能回写。
	if err := hostSide.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	resp, err := io.ReadAll(hostSide)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(resp, []byte("200 OK")) || !bytes.Contains(resp, []byte("GET /")) {
		t.Fatalf("resp = %q", resp)
	}
	<-serverDone
}

var _ = sync.Mutex{}

// agent 报「连不上」时，中继必须带错误，而不是静默 EOF。
//
// 之前 RelayClose 只有「关掉了」一种写法，DialPort 于是把「端口没人 listen」
// 返回成一条连上了但立刻 EOF 的连接，调用方无法重试。
func TestRelayConnectFailed(t *testing.T) {
	r, s := newRelayForTest(t, 6)
	r.port = 8080
	s.dispatch(proto.Msg{Kind: proto.KindRelayClose, ConnID: 6, Flags: proto.RelayConnectFailed})
	if err := r.err(); err == nil {
		t.Fatal("connect failure must be recorded on the relay")
	}
	if _, err := r.Read(make([]byte, 8)); err == nil {
		t.Fatal("read must report the connect failure")
	}
	if msg := r.err().Error(); !strings.Contains(msg, "8080") {
		t.Fatalf("error must name the port: %q", msg)
	}
}

// 正常的 RelayClose（对端发完就关）仍然是 EOF，不是错误。
func TestRelayCloseWithoutFlagsIsEOF(t *testing.T) {
	r, s := newRelayForTest(t, 7)
	s.dispatch(proto.Msg{Kind: proto.KindRelayClose, ConnID: 7})
	if err := r.err(); err != nil {
		t.Fatalf("normal close must not be an error: %v", err)
	}
	if _, err := r.Read(make([]byte, 8)); err != io.EOF {
		t.Fatalf("read = %v, want EOF", err)
	}
}
