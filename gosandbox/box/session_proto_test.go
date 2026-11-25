// 文件说明：session_proto_test.go — gosandbox/box 的单元测试（TestSessionFileOpsOverAgent、TestSessionListeningOverAgent 等）
//
// 用法
//
//	go test ./gosandbox/box -run 'TestSessionFileOpsOverAgent|TestSessionListeningOverAgent|TestDialPortRelayOverAgent|TestDialPortReportsConnectFailure'
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/session_proto_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backpressure_test.go, copytree.go, deadline.go, egress_e2e_test.go, files.go 等 26 个
//
// 主要符号
//
//	newAgentSession(t *testing.T) *Session
//	TestSessionFileOpsOverAgent(t *testing.T)
//	TestSessionListeningOverAgent(t *testing.T)
//	TestDialPortRelayOverAgent(t *testing.T)
//	TestDialPortReportsConnectFailure(t *testing.T)

package box

// session_proto_test.go：宿主 Session 与进程内 agent 的协议闭环。
//
// 不套 bwrap、不 fork：直接用 net.Pipe 把 Session 与 agent.Serve 接起来。
// 这样「文件服务要靠 Seq 关联回复」「入站中继要能双向传字节」这些
// 与平台无关的契约在 macOS 上也能跑，不必等 Linux 的 e2e 用例。

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/agent"
	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// newAgentSession 起一个进程内 agent，并返回连上它的 Session。
func newAgentSession(t *testing.T) *Session {
	t.Helper()
	hostConn, agentConn := net.Pipe()
	go func() { _ = agent.Serve(agentConn, agentConn, io.Discard) }()
	s, err := newSessionOn(context.Background(), Config{
		Name:      "local",
		Root:      t.TempDir(),
		Workspace: "/workspace",
	}, hostConn)
	if err != nil {
		_ = agentConn.Close()
		t.Fatalf("handshake: %v", err)
	}
	t.Cleanup(func() {
		// 先关 agent 侧：Session.Close 要等读侧看到 EOF 才返回。
		_ = agentConn.Close()
		_ = s.Close()
	})
	return s
}

// 文件服务：写 → 读 → stat → list → rename → remove。
//
// 每个回复都靠 Seq 找回等待中的调用方；Seq 没带回来时这里会一路等到超时。
func TestSessionFileOpsOverAgent(t *testing.T) {
	s := newAgentSession(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "a.txt")

	if err := s.WriteFile(path, []byte("hello box"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := s.ReadFile(path, 4096)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello box" {
		t.Fatalf("read = %q", data)
	}
	fi, err := s.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Kind != proto.FileRegular || fi.Size != 9 {
		t.Fatalf("stat = %+v", fi)
	}
	entries, err := s.ListDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "a.txt" {
		t.Fatalf("list = %+v", entries)
	}
	to := filepath.Join(dir, "b.txt")
	if err := s.Rename(path, to); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := s.Remove(to); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := s.Stat(to); err == nil {
		t.Fatal("stat of a removed file must fail")
	}
}

// 监听端口查询同样靠 Seq 关联：回复没带 Seq 会一直等到超时。
func TestSessionListeningOverAgent(t *testing.T) {
	s := newAgentSession(t)
	ports, err := s.Listening()
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	// 只断言调用能返回（沙盒里有没有监听是环境相关的）。
	_ = ports
}

// 入站中继：宿主经 agent 连到本机端口，请求与响应双向流动，半关闭只结束写方向。
func TestDialPortRelayOverAgent(t *testing.T) {
	s := newAgentSession(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer c.Close()
		// 读到 EOF（宿主半关闭）才回写，模拟 HTTP/1.1 服务端。
		body, _ := io.ReadAll(c)
		_, _ = c.Write([]byte("pong:" + string(body)))
	}()

	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	conn, err := s.DialPort(context.Background(), port)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if cw, ok := conn.(interface{ CloseWrite() error }); ok {
		if err := cw.CloseWrite(); err != nil {
			t.Fatalf("half close: %v", err)
		}
	}
	resp, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(resp) != "pong:ping" {
		t.Fatalf("response = %q", resp)
	}
}

// 端口没人 listen：DialPort 必须报错，而不是返回一条「立刻 EOF」的连接。
func TestDialPortReportsConnectFailure(t *testing.T) {
	s := newAgentSession(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close() // 关掉之后这个端口就没人监听了

	done := make(chan error, 1)
	go func() {
		conn, derr := s.DialPort(context.Background(), port)
		if derr == nil {
			_ = conn.Close()
		}
		done <- derr
	}()
	select {
	case derr := <-done:
		if derr == nil {
			t.Fatal("dialing a closed port must fail")
		}
		if !strings.Contains(derr.Error(), "listening") {
			t.Errorf("error should say nothing is listening: %v", derr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("DialPort hung on a closed port")
	}
}
