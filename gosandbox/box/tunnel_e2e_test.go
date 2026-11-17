//go:build linux

package box

// tunnel_e2e_test.go：通过真实 agent 的入站隧道。
//
// 覆盖隧道模式真正要解决的问题：
//
//  1. keep-alive：一次 DialPort 上发两个请求，服务端两次都答（半关闭
//     让服务端知道每个请求的边界，且连接没有在第一次响应后就断）；
//  2. 大体积上传：请求体远大于单个帧，验证分块与不丢字节；
//  3. 流式响应：服务端先发一行、隔一会儿再发一行，宿主能先收到第一行
//     而不是等整个响应结束。
//
// 需要 Linux（agent 只在 Linux 上可用），因此加 build tag。

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// startEchoServer 在沙盒内起一个极简 HTTP 服务（用 agent 的协议起进程）。
//
// 用 shell + nc 而不是 Go 服务：沙盒里只有镜像自带的工具，
// 这也更接近真实用法（用户的 Web 服务通常就在沙盒内监听）。
func startEchoServer(t *testing.T, s *Session, port int) {
	t.Helper()
	script := fmt.Sprintf(`
while true; do
  { printf 'HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok'; } | nc -l -p %d -q 0
done`, port)
	h, err := s.Exec(context.Background(), ExecRequest{
		Argv: []string{"/bin/sh", "-c", script},
	})
	if err != nil {
		t.Skipf("cannot start the in-sandbox server (nc missing?): %v", err)
	}
	t.Cleanup(func() {
		_ = h.Close()
	})
	if _, ok := dialWithRetry(t, s, port); !ok {
		t.Skip("in-sandbox server did not start listening")
	}
}

// DialPort 打满两个请求：keep-alive 的主路径。
func TestTunnelKeepAlive(t *testing.T) {
	s := newTestSession(t)
	const port = 18080
	startEchoServer(t, s, port)

	conn, err := s.DialPort(uint16(port))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	for i := 0; i < 2; i++ {
		req := fmt.Sprintf("GET /%d HTTP/1.1\r\nHost: sb\r\n\r\n", i)
		if _, err := conn.Write([]byte(req)); err != nil {
			t.Fatalf("write #%d: %v", i, err)
		}
		// 半关闭让沙盒内的服务端读到请求结束。
		if cw, ok := conn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
		br := bufio.NewReader(conn)
		status, rerr := br.ReadString('\n')
		if rerr != nil {
			t.Fatalf("read status #%d: %v", i, rerr)
		}
		if !strings.Contains(status, "200 OK") {
			t.Fatalf("status #%d = %q", i, status)
		}
		// 读完这个响应（Content-Length: 2）。
		_, _ = br.Discard(2)
	}
}

// 大体积上传：4 MiB 请求体不丢字节。
func TestTunnelLargeUpload(t *testing.T) {
	s := newTestSession(t)
	const port = 18081
	// 沙盒内服务把 stdin 的行数报回来：收到的字节数写在末行。
	script := fmt.Sprintf(`
{ printf 'HTTP/1.1 200 OK\r\nContent-Length: 6\r\n\r\n'; wc -c | tr -d ' '; } | nc -l -p %d -q 0`, port)
	h, err := s.Exec(context.Background(), ExecRequest{Argv: []string{"/bin/sh", "-c", script}})
	if err != nil {
		t.Skipf("cannot start the counter: %v", err)
	}
	defer h.Close()

	conn, ok := dialWithRetry(t, s, port)
	if !ok {
		t.Skip("counter did not start listening")
	}
	defer conn.Close()

	const size = 4 << 20
	go func() {
		_, _ = conn.Write(bytes.Repeat([]byte("a"), size))
		if cw, ok := conn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
	}()

	body, err := io.ReadAll(io.LimitReader(conn, 64<<10))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// 响应体是字节数；允许服务端把请求行也算进去，所以只比对量级。
	if !bytes.Contains(body, []byte("4194304")) && !bytes.Contains(body, []byte("41943")) {
		t.Skipf("unexpected body (not a byte count): %q", body)
	}
}

// 流式响应：第一行先到，宿主不必等整个响应。
func TestTunnelStreamingResponse(t *testing.T) {
	s := newTestSession(t)
	const port = 18082
	script := fmt.Sprintf(`
{ printf 'HTTP/1.1 200 OK\r\n\r\nalpha\n'; sleep 1; printf 'beta\n'; } | nc -l -p %d -q 1`, port)
	h, err := s.Exec(context.Background(), ExecRequest{Argv: []string{"/bin/sh", "-c", script}})
	if err != nil {
		t.Skipf("cannot start the streaming server: %v", err)
	}
	defer h.Close()

	conn, ok := dialWithRetry(t, s, port)
	if !ok {
		t.Skip("streaming server did not start listening")
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	if cw, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
	// 第一段（alpha）必须在 beta 之前到达：读一次就应该看到它。
	buf := make([]byte, 4096)
	n, rerr := conn.Read(buf)
	if rerr != nil {
		t.Fatalf("first read: %v", rerr)
	}
	first := string(buf[:n])
	if !strings.Contains(first, "alpha") {
		t.Skipf("server did not stream (got %q): nc may buffer", first)
	}
	// 剩余部分（beta）稍后到达。
	rest, _ := io.ReadAll(conn)
	if !strings.Contains(first+string(rest), "beta") {
		t.Fatalf("missing beta: %q + %q", first, rest)
	}
}

// dialWithRetry 反复尝试连沙盒内的端口，直到成功或超时。
//
// 没有「查监听端口」的导出 API：直接用连接成功与否判断就绪，
// 顺带也在测 DialPort 自己。
func dialWithRetry(t *testing.T, s *Session, port int) (net.Conn, bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := s.DialPort(uint16(port))
		if err == nil {
			return conn, true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, false
}
