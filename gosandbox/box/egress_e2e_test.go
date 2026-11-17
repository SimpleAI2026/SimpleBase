//go:build linux

package box

// egress_e2e_test.go：出网隔离的真机验收。
//
// 为什么必须在真机上跑：none 档用 --unshare-net 就够了，egress 档的出站要
// 真的经过 pasta，而「元数据与私网被挡住」只能由 agent 在自己 netns 里加的
// 黑洞路由保证。参数层断言证明不了这两件事。
//
// 缺 bwrap 或 pasta 时 Skip，并在原因里写明缺哪个——不要让「没装依赖」被
// 读成「已验证通过」。

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gosandbox/netx"
)

// newEgressSession 起一个真的套了 bwrap、配了 pasta 的 egress 沙盒。
func newEgressSession(t *testing.T) *Session {
	t.Helper()
	if _, err := os.Stat("/proc/self/cgroup"); err != nil {
		t.Skip("needs a Linux host")
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		t.Skip("needs bubblewrap (bwrap) on PATH")
	}
	if !netx.Available() {
		t.Skip("needs pasta (passt)")
	}
	root := t.TempDir()
	// Workspace 是**沙盒内**的挂载点，必须已在底层 rootfs 里存在：
	// image=host 时底层是 `--ro-bind / /`，bwrap 无法在只读根上新建目录。
	//
	// 不能用 /tmp：args 里后面的 `--tmpfs /tmp` 会把它盖掉（bwrap 按参数
	// 顺序应用挂载）。/var/tmp 在 Linux 上稳定存在，也不被覆盖。
	ws, err := os.MkdirTemp("/var/tmp", "gosandbox-egress-")
	if err != nil {
		t.Skipf("cannot create a workspace under /var/tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(ws) })
	s, err := Start(context.Background(), Config{
		Name:      "egress",
		Root:      root,
		Workspace: ws,
		Helper:    helperBin(t),
		Bwrap:     bwrap,
		Network:   netx.Policy{Mode: netx.ModeEgress},
		Policy:    Policy{Mode: "workspace-write", Workspace: ws},
		Env:       []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
		Log:       func(level, msg string, kv ...any) { t.Logf("%s %s %v", level, msg, kv) },
	})
	if err != nil {
		t.Fatalf("start egress sandbox: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// 黑洞路由必须挡在 pasta 配的 default 之前（前缀更长，优先命中）。
//
// 用 `ip route show` 而不是 `ip route get`：命中 blackhole 时 route get 只
// 回一行 "RTNETLINK answers: Invalid argument"，看不出是哪条规则。
func TestEgressBlocksMetadataAndPrivate(t *testing.T) {
	s := newEgressSession(t)
	out := runCapture(t, s, "/usr/sbin/ip -o route show table main 2>&1")
	for _, want := range []string{
		"169.254.0.0/16", // 云元数据（含 169.254.169.254）
		"10.0.0.0/8",     // 私网
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10", // CGNAT
		"127.0.0.0/8",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing blackhole route for %s in: %q", want, out)
		}
	}
	if !strings.Contains(out, "blackhole") {
		t.Errorf("no blackhole routes at all: %q", out)
	}
}

// requireEgressConfigured 区分两种"出网不通"：pasta 没配好 netns（环境/依赖
// 问题，Skip）与 pasta 配好了但隔离本身错（FAIL）。不做这个区分，任何 pasta
// 启动失败都会被读成"隔离实现坏了"。
func requireEgressConfigured(t *testing.T, s *Session) {
	t.Helper()
	if s.egress == nil {
		t.Skipf("pasta was not started (see warnings above); egress stays closed")
	}
	links := runCapture(t, s, "/usr/sbin/ip -o link show 2>&1")
	if !strings.Contains(links, "tap") {
		t.Skipf("pasta did not configure the sandbox netns (no tap interface): %q", links)
	}
}

// 公网必须仍然可达：隔离不能顺手把出网关掉。
//
// pasta 自身起不来时（例如内核不允许从外部 setns 到沙盒子 userns 的
// netns）不应把这条判成"隔离实现错了"：那种情况下沙盒没有任何 default
// route，出网是**关着**的（fail-closed），只是可用性受损。这种场景 Skip
// 并在原因里写明，让「没验证」和「验证失败」区分得开。
func TestEgressAllowsPublic(t *testing.T) {
	s := newEgressSession(t)
	requireEgressConfigured(t, s)
	out := strings.TrimSpace(runCapture(t, s,
		"curl -sS -m 10 -o /dev/null -w '%{http_code}' http://1.1.1.1/ 2>&1 || echo curl_failed"))
	if out == "" || strings.Contains(out, "curl_failed") {
		t.Fatalf("public egress does not work in egress mode: %q", out)
	}
}

// 宿主回环不能被 pasta 映射进沙盒：否则沙盒能直接打宿主上的服务。
func TestEgressHidesHostLoopback(t *testing.T) {
	s := newEgressSession(t)
	requireEgressConfigured(t, s)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	out := runCapture(t, s, fmt.Sprintf(
		"curl -sS -m 3 -o /dev/null http://127.0.0.1:%d/ 2>&1; echo exit=$?", port))
	if strings.Contains(out, "exit=0") {
		t.Fatalf("sandbox reached the host loopback service on port %d: %q", port, out)
	}
}

// 沙盒内的 lo 仍然要能用（入站中继依赖它），与本文件的出网拦截不冲突。
func TestEgressKeepsSandboxLoopback(t *testing.T) {
	s := newEgressSession(t)
	out := runCapture(t, s, "/usr/sbin/ip -o link show lo 2>&1 | head -1")
	if !strings.Contains(out, "LOOPBACK") && !strings.Contains(out, "UNKNOWN") {
		t.Fatalf("loopback is not up inside the sandbox: %q", out)
	}
	// 顺便确认工作区可写：Landlock 的 workspace-write 生效时它必须可写。
	if err := os.WriteFile(filepath.Join(s.cfg.Root, "ws", ".probe"), []byte("x"), 0o600); err != nil {
		t.Fatalf("workspace is not writable: %v", err)
	}
}
