// 文件说明：session.go — internal/sandbox/sshd 的内部实现（handleSession、pty 等）
//
// 用法
//
//	同包调用：mu → membuf.go, server.go；handleSession() → server.go；pty → server.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sandbox/sshd/session.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/vm, internal/sandbox
//	[同包] membuf.go, server.go
//	[关键引用] mu ← membuf.go, server.go
//
// 主要符号
//
//	(s *Server) handleSession(id Identity, ch ssh.Channel, reqs <-chan *ssh.Request)
//	var pty
//	var term
//	var started
//	var handle
//	var mu
//	... 另有 5 个顶层符号

package sshd

import (
	"context"
	"encoding/binary"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
)

// handleSession 处理一个 session 通道：按顺序消费 pty-req / env / window-change，
// 遇到 shell 或 exec 就启动沙盒进程并把输入输出接到通道上。
func (s *Server) handleSession(id Identity, ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()
	var (
		pty     *ptyReq
		term    string
		started bool
		handle  *sandbox.ProcHandle
		mu      sync.Mutex
	)
	reply := func(r *ssh.Request, ok bool) {
		if r.WantReply {
			_ = r.Reply(ok, nil)
		}
	}
	done := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(done) }) }

	for req := range reqs {
		switch req.Type {
		case "pty-req":
			var p ptyReq
			if started || ssh.Unmarshal(req.Payload, &p) != nil {
				reply(req, false)
				continue
			}
			pty = &p
			term = p.Term
			reply(req, true)
		case "env":
			var e envReq
			if started || ssh.Unmarshal(req.Payload, &e) != nil || !allowedEnv(e.Name) {
				reply(req, false)
				continue
			}
			if e.Name == "TERM" {
				term = e.Value
			}
			reply(req, true)
		case "window-change":
			var w windowChange
			mu.Lock()
			h := handle
			mu.Unlock()
			if ssh.Unmarshal(req.Payload, &w) != nil || h == nil {
				reply(req, false)
				continue
			}
			_ = h.Resize(vm.PTYSize{Cols: sandbox.ClampPTYDim(w.Cols, 80), Rows: sandbox.ClampPTYDim(w.Rows, 24)})
			h.Touch()
			reply(req, true)
		case "shell", "exec":
			if started {
				reply(req, false)
				continue
			}
			spec := sandbox.StartSpec{Kind: sandbox.ProcTerminal}
			if pty != nil {
				spec.PTY = &vm.PTYSize{Cols: sandbox.ClampPTYDim(pty.Cols, 80), Rows: sandbox.ClampPTYDim(pty.Rows, 24)}
				if term != "" && validTerm(term) {
					spec.Env = map[string]string{"TERM": term}
				}
			}
			if req.Type == "exec" {
				var e execReq
				if ssh.Unmarshal(req.Payload, &e) != nil || e.Command == "" {
					reply(req, false)
					continue
				}
				spec.Shell = e.Command
				// exec 不占「交互会话」配额：与 shell 不同，它是一次性命令。
				spec.Kind = sandbox.ProcCommand
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			h, err := s.sess.StartProc(ctx, id.ProjectID, id.SandboxID, spec)
			cancel()
			if err != nil {
				reply(req, false)
				// 把原因写给用户（固定文案，不含内部细节），然后结束通道。
				io.WriteString(ch.Stderr(), "sandbox: cannot start a session ("+reasonOf(err)+")\r\n")
				sendExit(ch, 1)
				finish()
				continue
			}
			started = true
			mu.Lock()
			handle = h
			mu.Unlock()
			reply(req, true)
			go s.pump(id, ch, h, spec, finish)
		case "signal":
			// 只转发 TERM / KILL；其余忽略。
			mu.Lock()
			h := handle
			mu.Unlock()
			reply(req, h != nil)
		default:
			// subsystem（sftp）、x11-req、auth-agent-req@openssh.com、tcpip-forward 等一律拒绝。
			reply(req, false)
		}
		select {
		case <-done:
			return
		default:
		}
	}
	// 通道的请求流结束（客户端关闭）：终止仍在运行的进程。
	mu.Lock()
	h := handle
	mu.Unlock()
	if h != nil {
		_ = h.Close()
	}
	<-waitOrTimeout(done, 100*time.Millisecond)
}

func waitOrTimeout(ch <-chan struct{}, d time.Duration) <-chan struct{} {
	out := make(chan struct{})
	go func() {
		select {
		case <-ch:
		case <-time.After(d):
		}
		close(out)
	}()
	return out
}

// pump 在进程与通道之间双向搬运数据，进程结束后发 exit-status 并结束会话。
func (s *Server) pump(id Identity, ch ssh.Channel, h *sandbox.ProcHandle, spec sandbox.StartSpec, finish func()) {
	started := s.now()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // 进程输出 → 通道
		defer wg.Done()
		_ = h.CopyOutput(ch)
	}()
	go func() { // 通道输入 → 进程
		buf := make([]byte, 32*1024)
		for {
			n, err := ch.Read(buf)
			if n > 0 {
				h.Touch()
				if _, werr := h.Stdin().Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				// 客户端关闭了输入：管道模式发 EOF；PTY 模式由连接关闭终止进程。
				if spec.PTY == nil {
					_ = h.Stdin().Close()
				}
				return
			}
		}
	}()
	exit := h.Wait()
	// 进程已退出：等输出协程把残留数据写完（最多 2 秒），再发 exit-status。
	waitDone := make(chan struct{})
	go func() { wg.Wait(); close(waitDone) }()
	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
	}
	sendExit(ch, uint32(exit.ExitCode))
	_ = h.Close()
	if s.audit != nil {
		kind := "terminal"
		if spec.Kind != sandbox.ProcTerminal {
			kind = "exec"
		}
		s.audit(context.Background(), id, kind, s.now().Sub(started), exit.ExitCode)
	}
	finish()
	_ = ch.Close()
}

func sendExit(ch ssh.Channel, code uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], code)
	_, _ = ch.SendRequest("exit-status", false, b[:])
}

// validTerm 只接受常见终端类型名，防止把任意字符串塞进环境变量。
func validTerm(t string) bool {
	if t == "" || len(t) > 32 {
		return false
	}
	for _, r := range t {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

// reasonOf 把 StartProc 的错误映射成固定文案，不透传内部细节。
func reasonOf(err error) string {
	switch {
	case isErr(err, sandbox.ErrLimitExceeded):
		return "too many sessions"
	case isErr(err, sandbox.ErrBusy):
		return "sandbox is busy"
	case isErr(err, sandbox.ErrGone):
		return "sandbox has expired"
	case isErr(err, vm.ErrNotSupported):
		return "terminal is not supported by this backend"
	}
	return "backend error"
}
