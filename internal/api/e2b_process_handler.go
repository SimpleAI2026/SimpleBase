// 文件说明：e2b_process_handler.go — internal/api 中 ProcStart、ProcConnect 等符号的实现
//
// 用法
//
//	(h *SandboxHandler) ProcStart(c echo.Context) error  // ProcStart 对应 process.Process/Start（服务端流）：启动进程并把事件流式返回
//	(h *SandboxHandler) ProcConnect(c echo.Context) error  // ProcConnect 对应 process.Process/Connect：连接到已有进程，继续接收事件
//	同包调用：event() → agent_handler.go, data_handler.go 等；end() → agent_handler.go, gocloudagent_access.go 等；stop() → kv_commands.go, oai_dto.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/e2b_process_handler.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/vm, internal/e2bproto, internal/sandbox
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] event() ← agent_handler.go, data_handler.go, event_fanout.go
//
// 主要符号
//
//	(h *SandboxHandler) ProcStart(c echo.Context) error
//	(h *SandboxHandler) ProcConnect(c echo.Context) error
//	(h *SandboxHandler) ProcList(c echo.Context) error
//	(h *SandboxHandler) ProcSendInput(c echo.Context) error
//	(h *SandboxHandler) ProcSendSignal(c echo.Context) error
//	(h *SandboxHandler) ProcCloseStdin(c echo.Context) error
//	... 另有 13 个顶层符号

package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/e2bproto"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
)

// keepaliveInterval 是 process 流在无输出时发 keepalive 的间隔，避免中间代理把长连接当空闲切断。
const keepaliveInterval = 15 * time.Second

// beginStream 为 Connect 服务端流写响应头并解除 WriteTimeout（同 beginSSE 的原因）。
func beginStream(c echo.Context) http.Flusher {
	beginSSE(c)
	h := c.Response().Header()
	h.Set("Content-Type", e2bproto.ContentTypeStream)
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)
	fl, _ := c.Response().Writer.(http.Flusher)
	return fl
}

// streamWriter 串行化对响应的写入（输出协程与 keepalive 协程会并发写）。
type streamWriter struct {
	mu      sync.Mutex
	w       io.Writer
	fl      http.Flusher
	stopped bool
}

// stop 之后所有写入都是空操作并返回 errStreamStopped。
// handler 返回后 Echo 会回收并重置 Response，此后任何协程再写都是数据竞争，
// 所以 handler 必须在返回前 stop，再等输出协程退出。
func (s *streamWriter) stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
}

var errStreamStopped = errors.New("stream stopped")

func (s *streamWriter) event(ev e2bproto.ProcessEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return errStreamStopped
	}
	if err := e2bproto.WriteFrame(s.w, e2bproto.StartResponse{Event: ev}); err != nil {
		return err
	}
	if s.fl != nil {
		s.fl.Flush()
	}
	return nil
}

func (s *streamWriter) end(e *e2bproto.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	_ = e2bproto.WriteEndStream(s.w, e)
	if s.fl != nil {
		s.fl.Flush()
	}
}

// ptyMode 指出事件里的数据该走哪个字段：PTY 进程用 pty，其余用 stdout。
func dataEvent(pty bool, b []byte) e2bproto.ProcessEvent {
	if pty {
		return e2bproto.ProcessEvent{Data: &e2bproto.DataEvent{PTY: append(e2bproto.Bytes(nil), b...)}}
	}
	return e2bproto.ProcessEvent{Data: &e2bproto.DataEvent{Stdout: append(e2bproto.Bytes(nil), b...)}}
}

// ProcStart 对应 process.Process/Start（服务端流）：启动进程并把事件流式返回。
// 客户端断开时：有 tag 的后台进程继续运行（之后可 Connect），否则终止进程。
func (h *SandboxHandler) ProcStart(c echo.Context) error {
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, e2bproto.MaxFrameBytes+5))
	if err != nil {
		return writeConnectErr(c, &connectErr{e2bproto.CodeInvalidArgument, "cannot read request body"})
	}
	var req e2bproto.StartRequest
	if err := decodeStreamRequest(c, body, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, true)
	if err != nil {
		return writeConnectErr(c, err)
	}
	if !h.svc.Interactive() {
		return writeConnectErr(c, &connectErr{e2bproto.CodeUnimplemented, "this backend cannot run interactive processes"})
	}
	if req.Process.Cmd == "" {
		return writeConnectErr(c, &connectErr{e2bproto.CodeInvalidArgument, "process.cmd is required"})
	}
	spec := sandbox.StartSpec{Kind: sandbox.ProcCommand, Cmd: req.Process.Cmd, Args: req.Process.Args, Env: req.Process.Envs}
	if req.Process.Cwd != nil {
		spec.Cwd = *req.Process.Cwd
	}
	if req.Tag != nil {
		spec.Tag = *req.Tag
	}
	if req.PTY != nil {
		spec.PTY = &vm.PTYSize{Cols: sandbox.ClampPTYDim(req.PTY.Size.Cols, 80), Rows: sandbox.ClampPTYDim(req.PTY.Size.Rows, 24)}
	}
	// E2B 的 cmd 通常是 /bin/bash 配合 -l -c 'script'：保持 argv 原样交给驱动，不经额外一层 shell。
	proc, err := h.svc.StartProc(c.Request().Context(), p, id, spec)
	if err != nil {
		return writeConnectErr(c, err)
	}
	h.recordExec(c, p, id, "process.start", req.Process.Cmd+" "+strings.Join(req.Process.Args, " "), 0, 0)
	return h.streamProc(c, p, id, proc, spec.PTY != nil, req.Tag != nil, true)
}

// ProcConnect 对应 process.Process/Connect：连接到已有进程，继续接收事件。
func (h *SandboxHandler) ProcConnect(c echo.Context) error {
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, e2bproto.MaxFrameBytes+5))
	if err != nil {
		return writeConnectErr(c, &connectErr{e2bproto.CodeInvalidArgument, "cannot read request body"})
	}
	var req e2bproto.ConnectRequest
	if err := decodeStreamRequest(c, body, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, true)
	if err != nil {
		return writeConnectErr(c, err)
	}
	proc, ferr := h.findProc(p, id, req.Process)
	if ferr != nil {
		return writeConnectErr(c, ferr)
	}
	return h.streamProc(c, p, id, proc, false, true, false)
}

func (h *SandboxHandler) findProc(projectID, sandboxID string, sel e2bproto.ProcessSelector) (*sandbox.ProcHandle, error) {
	pid, tag := 0, ""
	if sel.PID != nil {
		pid = int(*sel.PID)
	}
	if sel.Tag != nil {
		tag = *sel.Tag
	}
	if pid == 0 && tag == "" {
		return nil, &connectErr{e2bproto.CodeInvalidArgument, "process selector (pid or tag) is required"}
	}
	proc, ok := h.svc.FindProc(projectID, sandboxID, pid, tag)
	if !ok {
		return nil, &connectErr{e2bproto.CodeNotFound, "process not found"}
	}
	return proc, nil
}

// decodeStreamRequest 解服务端流 RPC 的请求。Connect 里服务端流的请求也是一个 envelope。
func decodeStreamRequest(c echo.Context, body []byte, dst any) error {
	ct := c.Request().Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/connect+proto") || strings.HasPrefix(ct, "application/proto") {
		return &connectErr{e2bproto.CodeUnimplemented, "binary protobuf is not supported; use the JSON encoding"}
	}
	if strings.HasPrefix(ct, e2bproto.ContentTypeStream) {
		f, err := e2bproto.ReadFrame(strings.NewReader(string(body)))
		if err != nil {
			return &connectErr{e2bproto.CodeInvalidArgument, "invalid request envelope"}
		}
		return e2bproto.DecodeUnary(f.Body, dst)
	}
	if err := e2bproto.DecodeUnary(body, dst); err != nil {
		return &connectErr{e2bproto.CodeInvalidArgument, "invalid JSON body"}
	}
	return nil
}

// streamProc 把进程输出作为事件流写给客户端，直到进程结束或客户端断开。
// emitStart 为真时先发 start 事件（Start 发，Connect 不发）。
// 客户端断开时：detach 为真（有 tag）则进程继续运行；否则终止进程。
func (h *SandboxHandler) streamProc(c echo.Context, projectID, sandboxID string, proc *sandbox.ProcHandle, pty, detach, emitStart bool) error {
	sw := &streamWriter{w: c.Response().Writer, fl: beginStream(c)}
	if emitStart {
		if err := sw.event(e2bproto.ProcessEvent{Start: &e2bproto.StartEvent{PID: uint32(proc.PID())}}); err != nil {
			return nil
		}
	}
	reqCtx := c.Request().Context()
	// Start 看全部输出；Connect 重新接上时也看缓冲里的历史（断开期间产生的输出不丢）。
	sub := proc.Subscribe(true)
	defer sub.Close()
	outDone := make(chan struct{})
	// 返回前必须让输出协程停手并退出：之后 Echo 会回收 Response，再写就是数据竞争。
	// sub.Close（上面的 defer）会唤醒阻塞在 Read 上的输出协程；sw.stop 保证它醒来后也不再碰响应。
	defer sw.stop()
	go func() {
		defer close(outDone)
		buf := make([]byte, 16*1024)
		for {
			n, err := sub.Read(buf)
			if n > 0 {
				proc.Touch()
				if werr := sw.event(dataEvent(pty, buf[:n])); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	tick := time.NewTicker(keepaliveInterval)
	defer tick.Stop()
	for {
		select {
		case <-proc.Done():
			// 等输出协程把残留数据读完再发 end，保证 end 之前所有输出已送达。
			select {
			case <-outDone:
			case <-time.After(2 * time.Second):
			}
			e := proc.Wait()
			status := "exited"
			if e.Signaled {
				status = "signal: killed"
			}
			sw.event(e2bproto.ProcessEvent{End: &e2bproto.EndEvent{ExitCode: int32(e.ExitCode), Exited: true, Status: status}})
			sw.end(nil)
			return nil
		case <-tick.C:
			if sw.event(e2bproto.ProcessEvent{KeepAlive: &e2bproto.KeepAlive{}}) != nil {
				return nil
			}
		case <-reqCtx.Done():
			if !detach {
				_ = proc.Close()
			}
			return nil
		}
	}
}

// ProcList 对应 process.Process/List。
func (h *SandboxHandler) ProcList(c echo.Context) error {
	if err := h.readConnectUnary(c, &struct{}{}); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, false)
	if err != nil {
		return writeConnectErr(c, err)
	}
	if _, err := h.svc.Get(c.Request().Context(), p, id, false); err != nil {
		return writeConnectErr(c, err)
	}
	list := h.svc.Procs(id)
	out := make([]e2bproto.ProcessInfo, 0, len(list))
	for _, pi := range list {
		info := e2bproto.ProcessInfo{PID: uint32(pi.PID)}
		if pi.Tag != "" {
			tag := pi.Tag
			info.Tag = &tag
		}
		out = append(out, info)
	}
	return c.JSON(http.StatusOK, e2bproto.ListResponse{Processes: out})
}

// ProcSendInput 对应 process.Process/SendInput。
func (h *SandboxHandler) ProcSendInput(c echo.Context) error {
	var req e2bproto.SendInputRequest
	if err := h.readConnectUnary(c, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, true)
	if err != nil {
		return writeConnectErr(c, err)
	}
	proc, ferr := h.findProc(p, id, req.Process)
	if ferr != nil {
		return writeConnectErr(c, ferr)
	}
	data := req.Input.Stdin
	if len(data) == 0 {
		data = req.Input.PTY
	}
	if len(data) > 0 {
		proc.Touch()
		if _, werr := proc.Stdin().Write(data); werr != nil {
			return writeConnectErr(c, &connectErr{e2bproto.CodeFailedPrecondition, "process is not accepting input"})
		}
	}
	return c.JSON(http.StatusOK, struct{}{})
}

// ProcSendSignal 对应 process.Process/SendSignal：只支持 SIGTERM 与 SIGKILL。
func (h *SandboxHandler) ProcSendSignal(c echo.Context) error {
	var req e2bproto.SendSignalRequest
	if err := h.readConnectUnary(c, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, true)
	if err != nil {
		return writeConnectErr(c, err)
	}
	proc, ferr := h.findProc(p, id, req.Process)
	if ferr != nil {
		return writeConnectErr(c, ferr)
	}
	var sig syscall.Signal
	switch req.Signal {
	case e2bproto.SignalSIGKILL:
		sig = syscall.SIGKILL
	case e2bproto.SignalSIGTERM:
		sig = syscall.SIGTERM
	default:
		return writeConnectErr(c, &connectErr{e2bproto.CodeInvalidArgument, "only SIGTERM and SIGKILL are supported"})
	}
	if err := proc.Signal(sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return writeConnectErr(c, &connectErr{e2bproto.CodeInternal, "cannot signal process"})
	}
	return c.JSON(http.StatusOK, struct{}{})
}

// ProcCloseStdin 对应 process.Process/CloseStdin：发送 EOF。PTY 进程需发 Ctrl+D，这里关闭无效。
func (h *SandboxHandler) ProcCloseStdin(c echo.Context) error {
	var req e2bproto.CloseStdinRequest
	if err := h.readConnectUnary(c, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, true)
	if err != nil {
		return writeConnectErr(c, err)
	}
	proc, ferr := h.findProc(p, id, req.Process)
	if ferr != nil {
		return writeConnectErr(c, ferr)
	}
	_ = proc.Stdin().Close()
	return c.JSON(http.StatusOK, struct{}{})
}

// ProcUpdate 对应 process.Process/Update：调整 PTY 大小。
func (h *SandboxHandler) ProcUpdate(c echo.Context) error {
	var req e2bproto.UpdateRequest
	if err := h.readConnectUnary(c, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, true)
	if err != nil {
		return writeConnectErr(c, err)
	}
	proc, ferr := h.findProc(p, id, req.Process)
	if ferr != nil {
		return writeConnectErr(c, ferr)
	}
	if req.PTY != nil {
		if rerr := proc.Resize(vm.PTYSize{Cols: sandbox.ClampPTYDim(req.PTY.Size.Cols, 80), Rows: sandbox.ClampPTYDim(req.PTY.Size.Rows, 24)}); rerr != nil {
			return writeConnectErr(c, &connectErr{e2bproto.CodeFailedPrecondition, "process has no pty"})
		}
	}
	return c.JSON(http.StatusOK, struct{}{})
}

// ProcStreamInput 对应 process.Process/StreamInput（客户端流）：HTTP/1.1 无法双向流，不实现。
func (h *SandboxHandler) ProcStreamInput(c echo.Context) error {
	return writeConnectErr(c, &connectErr{e2bproto.CodeUnimplemented, "StreamInput is not supported; use SendInput"})
}
