package agent

// agent_test.go：agent 的端到端行为（不需要 bwrap、不需要 root）。
//
// 用 socketpair 直接连 agent：协议、进程生命周期、文件服务、观测都在这里验证。
// 这些用例在 macOS 与 Linux 上都能跑——agent 的 PTY 与 rlimit 是平台相关实现，
// 本文件只测与平台无关的协议与进程语义。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// harness 是一个跑在 socketpair 上的 agent 与宿主编。
type harness struct {
	t *testing.T
	// host 是宿主 → agent 的写端。
	host *os.File
	// in 是 agent → 宿主的读端。
	in   *os.File
	stop func()
}

// newHarness 起一个 agent 并用 socketpair 连上。
func newHarness(t *testing.T) *harness {
	t.Helper()
	// 两条单向管道：hostOut → agent 读；agent 写 → hostIn。
	toAgentR, toAgentW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	fromAgentR, fromAgentW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, host: toAgentW, in: fromAgentR}

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer toAgentR.Close()
		defer fromAgentW.Close()
		_ = Serve(toAgentR, fromAgentW, nil)
	}()
	h.stop = func() {
		toAgentW.Close()
		fromAgentR.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("agent did not stop")
		}
	}
	t.Cleanup(h.stop)

	// agent 起手就发 Hello，先读掉。
	if m := h.next(); m.Kind != proto.KindHello {
		t.Fatalf("first frame = %d, want Hello", m.Kind)
	}
	return h
}

// send 向 agent 发一帧。
func (h *harness) send(m *proto.Msg) {
	h.t.Helper()
	if err := proto.Send(h.host, m); err != nil {
		h.t.Fatalf("send %d: %v", m.Kind, err)
	}
}

// next 读一帧（带超时）。
func (h *harness) next() proto.Msg {
	h.t.Helper()
	type result struct {
		m   proto.Msg
		err error
	}
	ch := make(chan result, 1)
	go func() {
		var kind proto.Kind
		var payload []byte
		if err := proto.ReadFrame(h.in, &kind, &payload); err != nil {
			ch <- result{err: err}
			return
		}
		m, err := proto.Decode(kind, payload)
		ch <- result{m, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			h.t.Fatalf("read: %v", r.err)
		}
		return r.m
	case <-time.After(10 * time.Second):
		h.t.Fatal("timed out waiting for a frame")
		return proto.Msg{}
	}
}

// until 读到满足条件的帧，把中间的输出帧跳过。
func (h *harness) until(kind proto.Kind) proto.Msg {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		m := h.next()
		if m.Kind == kind {
			return m
		}
		if m.Kind == proto.KindError {
			h.t.Fatalf("agent error: %s", m.Err)
		}
	}
	h.t.Fatalf("never saw kind %d", kind)
	return proto.Msg{}
}

// collect 收进程的 Output 直到 Exited，返回合并输出与退出帧。
func (h *harness) collect(id string) (string, proto.Msg) {
	h.t.Helper()
	var out strings.Builder
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		m := h.next()
		switch m.Kind {
		case proto.KindStarted, proto.KindInputConsumed:
		case proto.KindOutput:
			if m.ID == id {
				out.Write(m.Data)
			}
		case proto.KindExited:
			if m.ID == id {
				return out.String(), m
			}
		case proto.KindError:
			h.t.Fatalf("agent error: %s", m.Err)
		}
	}
	h.t.Fatal("process did not exit")
	return "", proto.Msg{}
}

// 一条命令的完整生命周期：Started → Output → Exited(0)。
func TestAgentRunCommand(t *testing.T) {
	h := newHarness(t)
	h.send(&proto.Msg{Kind: proto.KindStart, ID: "p1", Argv: []string{"/bin/sh", "-c", "echo hi; echo err >&2"}})
	h.until(proto.KindStarted)
	out, ex := h.collect("p1")
	if !strings.Contains(out, "hi") || !strings.Contains(out, "err") {
		t.Fatalf("output = %q", out)
	}
	if ex.Reason != proto.ExitCode || ex.Code != 0 {
		t.Fatalf("exit = %+v", ex)
	}
}

// 非零退出码如实回报。
func TestAgentExitCode(t *testing.T) {
	h := newHarness(t)
	h.send(&proto.Msg{Kind: proto.KindStart, ID: "p1", Argv: []string{"/bin/sh", "-c", "exit 42"}})
	if _, ex := h.collect("p1"); ex.Code != 42 {
		t.Fatalf("code = %d", ex.Code)
	}
}

// stdin：写入的数据被进程读到；CloseStdin 让进程收到 EOF。
func TestAgentStdin(t *testing.T) {
	h := newHarness(t)
	h.send(&proto.Msg{Kind: proto.KindStart, ID: "p1", Argv: []string{"/bin/sh", "-c", "read line; echo got:$line"}, Stdin: true})
	h.until(proto.KindStarted)
	h.send(&proto.Msg{Kind: proto.KindInput, ID: "p1", Data: []byte("hello\n")})
	out, ex := h.collect("p1")
	if !strings.Contains(out, "got:hello") {
		t.Fatalf("output = %q", out)
	}
	if ex.Code != 0 {
		t.Fatalf("code = %d", ex.Code)
	}
}

// 信号：SIGTERM 打到整个进程组，长命令随之结束。
func TestAgentSignal(t *testing.T) {
	h := newHarness(t)
	h.send(&proto.Msg{Kind: proto.KindStart, ID: "p1", Argv: []string{"/bin/sh", "-c", "sleep 30"}})
	h.until(proto.KindStarted)
	h.send(&proto.Msg{Kind: proto.KindSignal, ID: "p1", Signal: 15})
	if _, ex := h.collect("p1"); ex.Reason != proto.ExitSignaled || ex.Sig != 15 {
		t.Fatalf("exit = %+v", ex)
	}
}

// 不存在的命令：回 127，而不是让 agent 断开。
func TestAgentCommandNotFound(t *testing.T) {
	h := newHarness(t)
	h.send(&proto.Msg{Kind: proto.KindStart, ID: "p1", Argv: []string{"/definitely/not/here"}})
	if _, ex := h.collect("p1"); ex.Code != 127 {
		t.Fatalf("code = %d", ex.Code)
	}
}

// Ping / Pong 保持连接可用。
func TestAgentPing(t *testing.T) {
	h := newHarness(t)
	h.send(&proto.Msg{Kind: proto.KindPing, Nonce: 7})
	if m := h.next(); m.Kind != proto.KindPong || m.Nonce != 7 {
		t.Fatalf("pong = %+v", m)
	}
}

// 文件服务：写 → 读 → stat → list → rename → remove。
func TestAgentFiles(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "a.txt")

	// 写：帧后跟裸字节。
	body := []byte("hello fs")
	h.send(&proto.Msg{Kind: proto.KindFsWrite, ID: "fs", Path: path, Size: int64(len(body)), Mode: 0o644})
	if _, err := h.host.Write(body); err != nil {
		t.Fatal(err)
	}
	if m := h.next(); m.Kind != proto.KindFsWriteAck {
		t.Fatalf("write ack = %+v", m)
	}

	// 读。
	h.send(&proto.Msg{Kind: proto.KindFsRead, ID: "fs", Path: path, Size: 4096})
	rd := h.until(proto.KindFsReadReply)
	if string(rd.Data) != "hello fs" {
		t.Fatalf("read = %q", rd.Data)
	}

	// stat。
	h.send(&proto.Msg{Kind: proto.KindFsStat, ID: "fs", Path: path})
	st := h.until(proto.KindFsStatReply)
	if st.FileKnd != proto.FileRegular || st.Size != int64(len(body)) {
		t.Fatalf("stat = %+v", st)
	}

	// list。
	h.send(&proto.Msg{Kind: proto.KindFsList, ID: "fs", Path: filepath.Dir(path)})
	ls := h.until(proto.KindFsListReply)
	if len(ls.Entries) != 1 || ls.Entries[0].Name != "a.txt" {
		t.Fatalf("list = %+v", ls.Entries)
	}

	// rename。
	to := filepath.Join(dir, "b.txt")
	h.send(&proto.Msg{Kind: proto.KindFsRename, ID: "fs", Path: path, To: to})
	if m := h.next(); m.Kind != proto.KindAck {
		t.Fatalf("rename = %+v", m)
	}
	if _, err := os.Stat(to); err != nil {
		t.Fatal(err)
	}

	// remove。
	h.send(&proto.Msg{Kind: proto.KindFsRemove, ID: "fs", Path: to})
	if m := h.next(); m.Kind != proto.KindAck {
		t.Fatalf("remove = %+v", m)
	}
	if _, err := os.Stat(to); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("file must be gone")
	}
}

// 读不存在的文件：回 Error，连接继续可用。
func TestAgentFileErrors(t *testing.T) {
	h := newHarness(t)
	h.send(&proto.Msg{Kind: proto.KindFsRead, ID: "fs", Path: "/definitely/not/here", Size: 16})
	if m := h.next(); m.Kind != proto.KindError || m.Err == "" {
		t.Fatalf("read error = %+v", m)
	}
	// 连接仍然可用。
	h.send(&proto.Msg{Kind: proto.KindPing, Nonce: 1})
	if m := h.next(); m.Kind != proto.KindPong {
		t.Fatalf("connection must stay usable: %+v", m)
	}
}

// 用量与端口查询总能回答（读不到 cgroup 时报 0，不报错）。
func TestAgentStatsAndListening(t *testing.T) {
	h := newHarness(t)
	h.send(&proto.Msg{Kind: proto.KindStatsQuery})
	if m := h.next(); m.Kind != proto.KindStats {
		t.Fatalf("stats = %+v", m)
	}
	h.send(&proto.Msg{Kind: proto.KindListenQuery, ID: "p1"})
	if m := h.next(); m.Kind != proto.KindListening {
		t.Fatalf("listening = %+v", m)
	}
}

// Shutdown 让 agent 关掉连接。
func TestAgentShutdown(t *testing.T) {
	h := newHarness(t)
	h.send(&proto.Msg{Kind: proto.KindShutdown})
	// 读会拿到 EOF 或错误，两种都算正常关闭。
	var kind proto.Kind
	var payload []byte
	for i := 0; i < 5; i++ {
		if err := proto.ReadFrame(h.in, &kind, &payload); err != nil {
			return
		}
	}
}

// 未知帧类型：回 Error，连接继续可用。
func TestAgentUnknownFrame(t *testing.T) {
	h := newHarness(t)
	// 直接写一个 agent 不处理的 kind（方向反了的那种）。
	if err := proto.Send(h.host, &proto.Msg{Kind: proto.KindStarted, ID: "x", PID: 1}); err != nil {
		t.Fatal(err)
	}
	if m := h.next(); m.Kind != proto.KindError {
		t.Fatalf("want Error, got %+v", m)
	}
	h.send(&proto.Msg{Kind: proto.KindPing, Nonce: 3})
	if m := h.next(); m.Kind != proto.KindPong {
		t.Fatalf("connection must stay usable: %+v", m)
	}
}
