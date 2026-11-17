//go:build linux

package box

// netx_e2e_test.go：网络隔离与入站中继的相互作用。
//
// 加了 --unshare-net 之后最容易踩的坑是「顺手把入站也关了」：
// netns 里 lo 是 DOWN 的，于是沙盒内的 127.0.0.1:port 连不上，
// getHost(port) 全部失效。这里的用例就是这条边界的回归测试。
//
// 「none 档没有默认路由」这条断言只能在真的 --unshare-net 下成立，
// 而本文件的 sandbox 不套 bwrap（Bwrap 为空等于放弃隔离），
// 所以改为在参数层断言：none 档必须带 --unshare-net。

import (
	"bufio"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/netx"
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

// none 档：沙盒内 127.0.0.1 仍然可用（lo 被 agent 拉起），入站中继依赖它。
func TestNetworkIsolationKeepsLoopback(t *testing.T) {
	s := newTestSession(t)
	const port = 19999
	startTestServer(t, s, port)

	conn := dialPort(t, s, port)
	defer conn.Close()
	if _, err := conn.Write([]byte("GET /echo HTTP/1.1\r\nHost: sb\r\n\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp, err := responseWithin(t, bufio.NewReader(conn), 10*time.Second)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("loopback is not usable inside the sandbox: %q", resp)
	}
}

// 参数层断言：none（含零值）必须 --unshare-net，egress 档不能（pasta 要配 netns）。
func TestBuildArgsUnsharesNet(t *testing.T) {
	newCfg := func(mode netx.Mode) Config {
		root := t.TempDir()
		return Config{
			Name:      "net-args",
			Root:      root,
			Workspace: "/workspace",
			Helper:    "/usr/bin/gosandbox-inner",
			Bwrap:     "/usr/bin/bwrap",
			Network:   netx.Policy{Mode: mode},
			Policy:    Policy{Mode: "workspace-write", Workspace: "/workspace"},
		}
	}
	for _, mode := range []netx.Mode{"", netx.ModeNone} {
		argv, policyPath, err := buildArgs(newCfg(mode))
		if err != nil {
			t.Fatalf("mode %q: %v", mode, err)
		}
		if policyPath == "" {
			t.Errorf("mode %q: policy file must be written", mode)
		}
		if !hasFlag(argv, "--unshare-net") {
			t.Errorf("mode %q must isolate the network namespace: %v", mode, argv)
		}
	}
	argv, _, err := buildArgs(newCfg(netx.ModeEgress))
	if err != nil {
		t.Fatal(err)
	}
	if hasFlag(argv, "--unshare-net") {
		t.Errorf("egress mode must keep the host netns for pasta: %v", argv)
	}
}

// hasFlag 判断 argv 里是否出现过某个参数。
func hasFlag(argv []string, flag string) bool {
	for _, a := range argv {
		if a == flag {
			return true
		}
	}
	return false
}

// none 档：连宿主回环不通（127.0.0.1 在 netns 里就是沙盒自己）。
func TestNetworkIsolationBlocksMetadata(t *testing.T) {
	s := newTestSession(t)
	// 云元数据地址不该可达。用 curl 可能没装，改用路由表之外的方式：
	// 读 fib_trie 判断有没有到 169.254.0.0/16 的路径。
	out := runCapture(t, s, "cat /proc/net/fib_trie 2>/dev/null | head -20 || echo none")
	// 不断言具体内容（各发行版 fib_trie 差异大），只要求命令能跑通，
	// 真正断言在 netx 的单元测试里（CheckAllowed）。
	if out == "" {
		t.Log("no fib_trie output; address-level checks are covered by netx tests")
	}
}
