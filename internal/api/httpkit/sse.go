// 文件说明：sse.go — SSE 连接的公共部分：解除写 deadline、统一帧格式、带自增 id 的写入器
//
// 用法
//
//	NewSSEWriter(c, idHeader, idValue) *SSEWriter   // 建立流并把标识放进响应头（连接在首帧前断开也能续传）
//	(w *SSEWriter) Write(ev cloudagent.Event)        // 分配事件 ID 并写帧
//	WriteSSEFrame(dst io.Writer, flusher http.Flusher, id int64, payload any)
//	BeginSSE(c)                                      // 解除写 deadline（长连接）
//
// 调用架构
//
//	[上游] internal/api（run 流、轮次流、实时流、OpenAI 兼容流、E2B 事件流）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/httpkit/sse.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/cloudagent（事件类型）
//	[同包] context.go, errors.go
//
// 主要符号
//
//	type SSEWriter struct {…}
//	NewSSEWriter(c echo.Context, idHeader, idValue string) *SSEWriter
//	BeginSSE(c echo.Context)
//	WriteSSEFrame(dst io.Writer, flusher http.Flusher, id int64, payload any)
//	... 另有 3 个顶层符号

package httpkit

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

// BeginSSE 为长连接响应解除 http.Server.WriteTimeout 的写 deadline。
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
func BeginSSE(c echo.Context) {
	rc := http.NewResponseController(c.Response())
	_ = rc.SetWriteDeadline(time.Time{})
}

// WriteSSEFrame 写出一帧 `id: N\ndata: <json>\n\n` 并立刻 flush；Flusher 为 nil 时只写不刷。
func WriteSSEFrame(dst io.Writer, Flusher http.Flusher, id int64, payload any) {
	data, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(dst, "id: %d\ndata: %s\n\n", id, data)
	if Flusher != nil {
		Flusher.Flush()
	}
}

// SSEWriter 是带自增帧 id 的 SSE 写入器：一条连接上的所有帧串行写出（mu），
// run 流与轮次流共用，保证响应头与帧格式只有这一份实现。
type SSEWriter struct {
	c       echo.Context
	Flusher http.Flusher
	mu      sync.Mutex
	seq     int64
}

// NewSSEWriter 解除写 deadline、写出 SSE 响应头（附带一个标识头，如 X-Run-Id / X-Turn-Id，
// 让连接在首帧之前断开时客户端也能拿到 id 做续传），并返回写入器。
func NewSSEWriter(c echo.Context, idHeader, idValue string) *SSEWriter {
	BeginSSE(c)
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(idHeader, idValue)
	c.Response().WriteHeader(http.StatusOK)
	Flusher, _ := c.Response().Writer.(http.Flusher)
	return &SSEWriter{c: c, Flusher: Flusher}
}

func (w *SSEWriter) write(ev cloudagent.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seq++
	WriteSSEFrame(w.c.Response().Writer, w.Flusher, w.seq, ev)
}
