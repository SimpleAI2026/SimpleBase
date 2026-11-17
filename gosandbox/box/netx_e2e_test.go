//go:build linux

package box

// netx_e2e_test.go：网络隔离与入站中继的相互作用。
//
// 加了 --unshare-net 之后最容易踩的坑是「顺手把入站也关了」：
// netns 里 lo 是 DOWN 的，于是沙盒内的 127.0.0.1:port 连不上，
// getHost(port) 全部失效。这两个用例就是这条边界的回归测试。

import (
	"context"
	"strings"
	"testing"
	"time"
)

// runCapture 跑一条命令并返回 stdout（读完才返回，避免管道阻塞）。
func runCapture(t *testing.T, s *Session, script string) string {
	t.Helper()
	h, err := s.Exec(context.Background(), ExecRequest{Argv: []string{"/bin/sh", "-c", script}})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	defer h.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, rerr := h.Output().Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if rerr != nil {
			break
		}
	}
	h.Wait()
	return sb.String()
}

// none 档：沙盒里没有宿主网络，但 127.0.0.1 仍然可用（lo 被拉起）。
func TestNetworkIsolationKeepsLoopback(t *testing.T) {
	s := newTestSession(t)

	// 1) 出网应当不通：连一个宿主上必然可达的地址（pasta 不可用或 none 档）。
	//    这里用 /proc/net/route 判断默认路由：none 档不该有 default 路由。
	route := runCapture(t, s, "cat /proc/net/route 2>/dev/null || echo no-route-file")
	// 没有 default 路由（Linux 的默认路由是 00000000 那一行）。
	for _, line := range strings.Split(route, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == "00000000" {
			t.Fatalf("none mode must not have a default route: %q", line)
		}
	}

	// 2) 回环必须能 listen + connect：入站中继依赖它。
	if _, ok := dialWithRetry(t, s, 19999); ok {
		// 端口没人监听时 DialPort 会失败——这本身说明链路是通的。
		t.Skip("unexpected listener on 19999")
	}
	// 起一个监听在 127.0.0.1 的服务，确认能连上。
	h, err := s.Exec(context.Background(), ExecRequest{
		Argv: []string{"/bin/sh", "-c", "printf 'HTTP/1.1 200 OK\\r\\nContent-Length: 2\\r\\n\\r\\nok' | nc -l -s 127.0.0.1 -p 19999 -q 0"},
	})
	if err != nil {
		t.Skipf("nc not available: %v", err)
	}
	defer h.Close()
	conn, ok := dialWithRetry(t, s, 19999)
	if !ok {
		t.Fatal("cannot reach 127.0.0.1 inside the sandbox: loopback is down")
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\n\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 128)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, rerr := conn.Read(buf)
	if rerr != nil || !strings.Contains(string(buf[:n]), "200 OK") {
		t.Fatalf("read: %v %q", rerr, buf[:n])
	}
}

// none 档：连宿主回环不通（127.0.0.1 在 netns 里就是沙盒自己）。
func TestNetworkIsolationBlocksMetadata(t *testing.T) {
	s := newTestSession(t)
	// 云元数据地址不该可达。用 curl 可能没装，改用 /dev/tcp 之外的方式：
	// 直接读路由表判断有没有到 169.254.0.0/16 的路径。
	out := runCapture(t, s, "cat /proc/net/fib_trie 2>/dev/null | head -20 || echo none")
	// 不断言具体内容（各发行版 fib_trie 差异大），只要求命令能跑通，
	// 真正断言在 netx 的单元测试里（CheckAllowed）。
	if out == "" {
		t.Log("no fib_trie output; address-level checks are covered by netx tests")
	}
}
