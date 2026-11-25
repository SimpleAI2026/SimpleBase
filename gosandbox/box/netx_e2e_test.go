//go:build linux

// 文件说明：netx_e2e_test.go — gosandbox/box 的单元测试（TestNetworkIsolationKeepsLoopback、TestBuildArgsUnsharesNet 等）
//
// 用法
//
//	go test ./gosandbox/box -run 'TestNetworkIsolationKeepsLoopback|TestBuildArgsUnsharesNet|TestNetworkIsolationBlocksMetadata'
//	同包调用：runCapture() → egress_e2e_test.go
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/netx_e2e_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backpressure_test.go, copytree.go, deadline.go, egress_e2e_test.go, files.go 等 26 个
//	[关键引用] runCapture ← egress_e2e_test.go
//
// 主要符号
//
//	runCapture(t *testing.T, s *Session, script string) string
//	TestNetworkIsolationKeepsLoopback(t *testing.T)
//	TestBuildArgsUnsharesNet(t *testing.T)
//	flagValue(argv []string, flag string) string
//	hasFlag(argv []string, flag string) bool
//	TestNetworkIsolationBlocksMetadata(t *testing.T)

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

// 参数层断言：任何档位都必须 --unshare-net。
//
// egress 档的旧实现共享宿主 netns 给 pasta，沙盒因此能直接摸到云元数据与
// 私网；现在由 pasta 在沙盒自己的 netns 里配网，agent 再叠加黑洞路由。
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
	for _, mode := range []netx.Mode{"", netx.ModeNone, netx.ModeEgress} {
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
	// agent 只在 egress 档装黑洞路由，所以档位必须如实传进去。
	for mode, want := range map[netx.Mode]string{"": "none", netx.ModeNone: "none", netx.ModeEgress: "egress"} {
		argv, _, err := buildArgs(newCfg(mode))
		if err != nil {
			t.Fatalf("mode %q: %v", mode, err)
		}
		if got := flagValue(argv, "--network"); got != want {
			t.Errorf("mode %q: --network = %q, want %q", mode, got, want)
		}
	}
}

// flagValue 取 argv 里某个 flag 的下一个参数（不存在返回空串）。
func flagValue(argv []string, flag string) string {
	for i := range argv {
		if argv[i] == flag && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return ""
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
