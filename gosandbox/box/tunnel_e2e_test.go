//go:build linux

// 文件说明：tunnel_e2e_test.go — gosandbox/box 的单元测试（TestMain、TestTunnelKeepAlive 等）
//
// 用法
//
//	go test ./gosandbox/box -run 'TestMain|TestTunnelKeepAlive|TestTunnelLargeUpload|TestTunnelStreamingResponse'
//	同包调用：startTestServer() → netx_e2e_test.go；dialPort() → netx_e2e_test.go；responseWithin() → netx_e2e_test.go
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/tunnel_e2e_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] backpressure_test.go, copytree.go, deadline.go, egress_e2e_test.go, files.go 等 26 个
//	[关键引用] startTestServer ← netx_e2e_test.go；dialPort ← netx_e2e_test.go；responseWithin ← netx_e2e_test.go
//
// 主要符号
//
//	const tunnelServerEnv / tunnelPortEnv
//	TestMain(m *testing.M)
//	startTestServer(t *testing.T, s *Session, port int)
//	dialPort(t *testing.T, s *Session, port int) net.Conn
//	readHTTPResponse(br *bufio.Reader) (string, error)
//	responseWithin(t *testing.T, br *bufio.Reader, d time.Duration) (string, error)
//	TestTunnelKeepAlive(t *testing.T)
//	TestTunnelLargeUpload(t *testing.T)
//	... 另有 3 个顶层符号

package box

// tunnel_e2e_test.go：通过真实 agent 的入站隧道。
//
// 覆盖隧道模式真正要解决的问题：
//
//  1. keep-alive：同一条连接上两个请求都被应答（连接没有在第一次响应后就断）；
//  2. 大体积上传：请求体远大于单个帧，验证分块与不丢字节；
//  3. 流式响应：服务端先发一段、隔一会儿再发一段，宿主能先收到第一段。
//
// 沙盒内的服务端直接复用测试二进制本身（TestMain 拦截，见 runTunnelServer）：
// nc 在各发行版上的参数与 EOF 语义都不一样（openbsd 的 -q、nmap-ncat 的 -p），
// 测隧道时不该把结果交给这些差异。
//
// 需要 Linux（agent 只在 Linux 上可用），因此加 build tag。

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 沙盒内服务的启动约定：同一个测试二进制 + 这两个环境变量。
const (
	tunnelServerEnv = "GOBOX_TUNNEL_SERVER"
	tunnelPortEnv   = "GOBOX_TUNNEL_PORT"
)

// TestMain 让同一个测试二进制既能跑测试，也能当沙盒内的服务端。
func TestMain(m *testing.M) {
	if os.Getenv(tunnelServerEnv) == "1" {
		os.Exit(runTunnelServer())
	}
	os.Exit(m.Run())
}

// startTestServer 在沙盒里起一个 HTTP 服务，返回时它已经在监听。
func startTestServer(t *testing.T, s *Session, port int) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("cannot locate the test binary: %v", err)
	}
	h, err := s.Exec(context.Background(), ExecRequest{
		Argv: []string{exe},
		Env:  []string{tunnelServerEnv + "=1", fmt.Sprintf("%s=%d", tunnelPortEnv, port)},
	})
	if err != nil {
		t.Skipf("cannot start the in-sandbox server: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	// 等服务端自己说 ready：比「反复 DialPort 直到成功」确定得多。
	line, lerr := readLine(h.Output())
	if lerr != nil || !strings.Contains(line, "ready") {
		t.Skipf("in-sandbox server did not come up (%q): %v", line, lerr)
	}
}

// dialPort 反复尝试连沙盒内的端口，直到成功或超时。
func dialPort(t *testing.T, s *Session, port int) net.Conn {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		conn, err := s.DialPort(context.Background(), uint16(port))
		if err == nil {
			return conn
		}
		last = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("cannot dial 127.0.0.1:%d inside the sandbox: %v", port, last)
	return nil
}

// readHTTPResponse 读一条完整响应（按 Content-Length 读 body）。
func readHTTPResponse(br *bufio.Reader) (string, error) {
	var sb strings.Builder
	length := 0
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return sb.String(), err
		}
		sb.WriteString(line)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		if k, v, ok := strings.Cut(trimmed, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "content-length") {
			length, _ = strconv.Atoi(strings.TrimSpace(v))
		}
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(br, body); err != nil {
		return sb.String(), err
	}
	sb.Write(body)
	return sb.String(), nil
}

// responseWithin 在超时内读一条响应，避免中继挂住把整包测试拖到 -timeout。
func responseWithin(t *testing.T, br *bufio.Reader, d time.Duration) (string, error) {
	t.Helper()
	type result struct {
		body string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		body, err := readHTTPResponse(br)
		ch <- result{body, err}
	}()
	select {
	case r := <-ch:
		return r.body, r.err
	case <-time.After(d):
		t.Fatalf("no response within %s", d)
		return "", nil
	}
}

// keep-alive：一条连接上两个请求都得到应答。
func TestTunnelKeepAlive(t *testing.T) {
	s := newTestSession(t)
	const port = 18080
	startTestServer(t, s, port)

	conn := dialPort(t, s, port)
	defer conn.Close()

	br := bufio.NewReader(conn)
	for i := 0; i < 2; i++ {
		req := fmt.Sprintf("GET /echo/%d HTTP/1.1\r\nHost: sb\r\n\r\n", i)
		if _, err := conn.Write([]byte(req)); err != nil {
			t.Fatalf("write #%d: %v", i, err)
		}
		resp, rerr := responseWithin(t, br, 10*time.Second)
		if rerr != nil {
			t.Fatalf("response #%d: %v", i, rerr)
		}
		if !strings.Contains(resp, "200 OK") || !strings.HasSuffix(resp, "ok") {
			t.Fatalf("response #%d = %q", i, resp)
		}
	}

	// 半关闭只结束写方向：服务端读到 EOF 后关连接，宿主读到 EOF 而不是报错。
	if cw, ok := conn.(interface{ CloseWrite() error }); ok {
		if err := cw.CloseWrite(); err != nil {
			t.Fatalf("half close: %v", err)
		}
	}
	rest, err := io.ReadAll(io.LimitReader(conn, 1<<10))
	if err != nil {
		t.Fatalf("read after half close: %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("unexpected trailing bytes: %q", rest)
	}
}

// 大体积上传：4 MiB 请求体不丢字节。
func TestTunnelLargeUpload(t *testing.T) {
	s := newTestSession(t)
	const port = 18081
	const size = 4 << 20
	startTestServer(t, s, port)

	conn := dialPort(t, s, port)
	defer conn.Close()

	head := fmt.Sprintf("POST /count HTTP/1.1\r\nHost: sb\r\nContent-Length: %d\r\n\r\n", size)
	go func() {
		if _, err := conn.Write([]byte(head)); err != nil {
			return
		}
		_, _ = conn.Write(bytes.Repeat([]byte("a"), size))
	}()

	br := bufio.NewReader(conn)
	resp, err := responseWithin(t, br, 60*time.Second)
	if err != nil {
		t.Fatalf("response: %v", err)
	}
	if !strings.Contains(resp, "200 OK") || !strings.HasSuffix(resp, strconv.Itoa(size)) {
		t.Fatalf("server did not see the whole body: %q", resp)
	}
}

// 流式响应：第一段先到，宿主不必等整个响应。
func TestTunnelStreamingResponse(t *testing.T) {
	s := newTestSession(t)
	const port = 18082
	startTestServer(t, s, port)

	conn := dialPort(t, s, port)
	defer conn.Close()

	if _, err := conn.Write([]byte("GET /stream HTTP/1.1\r\nHost: sb\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	// 服务端写完第一段要睡 1 秒才写第二段：第一段必须在 800ms 内到达，
	// 否则说明整条隧道在攒够整个响应才交给宿主。
	//
	// 每次读到一块就发一个累积快照：网络分段与帧分段都可能把响应切开，
	// 断言必须打在「累积内容」上。
	snaps := make(chan string, 16)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
				snaps <- sb.String()
			}
			if err != nil {
				close(snaps)
				return
			}
		}
	}()

	var last string
	deadline := time.After(800 * time.Millisecond)
	for !strings.Contains(last, "alpha") {
		select {
		case snap, ok := <-snaps:
			if !ok {
				t.Fatalf("connection closed before alpha arrived: %q", last)
			}
			last = snap
		case <-deadline:
			t.Fatalf("alpha did not arrive while the server was still writing: %q", last)
		}
	}
	// 剩下的 beta 随后到；连接关闭时通道关闭。
	for snap := range snaps {
		last = snap
	}
	if !strings.Contains(last, "beta") {
		t.Fatalf("missing beta: %q", last)
	}
}

// runTunnelServer 是沙盒内服务端的主体：极简 HTTP/1.1，支持 keep-alive。
//
//	GET  /echo*    200 ok
//	POST /count    读 Content-Length 字节，回字节数
//	GET  /stream   先写 alpha，睡 1 秒再写 beta，然后关连接
func runTunnelServer() int {
	port := os.Getenv(tunnelPortEnv)
	if port == "" {
		fmt.Fprintln(os.Stderr, "tunnel server: missing "+tunnelPortEnv)
		return 2
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "tunnel server: %v\n", err)
		return 1
	}
	defer ln.Close()
	fmt.Println("ready")
	for {
		conn, aerr := ln.Accept()
		if aerr != nil {
			return 0
		}
		go serveTunnelConn(conn)
	}
}

// serveTunnelConn 服务一条连接上的多个请求（keep-alive）。
func serveTunnelConn(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			return
		}
		target := fields[1]
		length := 0
		for {
			header, herr := br.ReadString('\n')
			if herr != nil {
				return
			}
			trimmed := strings.TrimSpace(header)
			if trimmed == "" {
				break
			}
			if k, v, ok := strings.Cut(trimmed, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "content-length") {
				length, _ = strconv.Atoi(strings.TrimSpace(v))
			}
		}
		switch {
		case strings.HasPrefix(target, "/count"):
			body := make([]byte, length)
			if _, rerr := io.ReadFull(br, body); rerr != nil {
				return
			}
			text := strconv.Itoa(len(body))
			if _, werr := fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(text), text); werr != nil {
				return
			}
		case strings.HasPrefix(target, "/stream"):
			// Content-Length 是两段之和（"alpha\n" + "beta\n"）。
			if _, werr := io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 11\r\n\r\nalpha\n"); werr != nil {
				return
			}
			time.Sleep(time.Second)
			_, _ = io.WriteString(conn, "beta\n")
			return
		default:
			if _, werr := io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"); werr != nil {
				return
			}
		}
	}
}
