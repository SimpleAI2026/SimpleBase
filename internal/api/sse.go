// 文件说明：sse.go — SSE 连接的公共部分：解除写 deadline、统一帧格式、带自增 id 的写入器
//
// 用法
//
//	beginSSE() → e2b_process_handler.go, e2b_terminal.go 等自己写帧的调用方
//	newSSEWriter() → agent_run_handler.go（run 流）、collab_group_stream.go（轮次流）
//	writeSSEFrame() → run_events.go, collab_query_handler.go（续传 / 回放）
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sse.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方（gocloudagent/cloudagent 的事件类型）
//	[同包] agent_run_handler.go, collab_group_stream.go, run_events.go, collab_query_handler.go 等
//	[关键引用] beginSSE() ← agent_handler.go, e2b_process_handler.go, e2b_terminal.go
//
// 主要符号
//
//	beginSSE(c echo.Context)
//	writeSSEFrame(dst io.Writer, flusher http.Flusher, id int64, payload any)
//	type sseWriter struct {…}
//	newSSEWriter(c echo.Context, idHeader, idValue string) *sseWriter
//	(*sseWriter).write(ev cloudagent.Event)

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
)

// beginSSE 为长连接响应解除 http.Server.WriteTimeout 的写 deadline。
//
// Go 的 WriteTimeout 是「从读完请求头起算的绝对写 deadline」，不是两次写之间的
// 空闲上限。SSE 在写完响应头后仍要持续输出，若不解除，连接会在 write_timeout
// （默认 30s）到点时被服务端关闭，请求 context 随之取消：云助手 run 记为
// canceled、0 token、用户只看到一个空气泡（planv6.0 v2 §11）。
//
// 解除后，流式连接由各自的上限管控：云助手 run 受 llm.agent_run_timeout
// （默认 180s）约束，并有 15s 注释心跳；客户端断开则 context 取消。
//
// 底层 ResponseWriter 不支持 deadline（如 httptest.ResponseRecorder）时静默忽略：
// 那类写入器本来就没有超时，不需要处理。
func beginSSE(c echo.Context) {
	rc := http.NewResponseController(c.Response())
	_ = rc.SetWriteDeadline(time.Time{})
}

// writeSSEFrame 写出一帧 `id: N\ndata: <json>\n\n` 并立刻 flush；flusher 为 nil 时只写不刷。
func writeSSEFrame(dst io.Writer, flusher http.Flusher, id int64, payload any) {
	data, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(dst, "id: %d\ndata: %s\n\n", id, data)
	if flusher != nil {
		flusher.Flush()
	}
}

// sseWriter 是带自增帧 id 的 SSE 写入器：一条连接上的所有帧串行写出（mu），
// run 流与轮次流共用，保证响应头与帧格式只有这一份实现。
type sseWriter struct {
	c       echo.Context
	flusher http.Flusher
	mu      sync.Mutex
	seq     int64
}

// newSSEWriter 解除写 deadline、写出 SSE 响应头（附带一个标识头，如 X-Run-Id / X-Turn-Id，
// 让连接在首帧之前断开时客户端也能拿到 id 做续传），并返回写入器。
func newSSEWriter(c echo.Context, idHeader, idValue string) *sseWriter {
	beginSSE(c)
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(idHeader, idValue)
	c.Response().WriteHeader(http.StatusOK)
	flusher, _ := c.Response().Writer.(http.Flusher)
	return &sseWriter{c: c, flusher: flusher}
}

func (w *sseWriter) write(ev cloudagent.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seq++
	writeSSEFrame(w.c.Response().Writer, w.flusher, w.seq, ev)
}
