// 文件说明：sse.go — internal/api 的内部实现（beginSSE 等）
//
// 用法
//
//	同包调用：beginSSE() → agent_handler.go, e2b_process_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sse.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] beginSSE() ← agent_handler.go, e2b_process_handler.go, e2b_terminal.go
//
// 主要符号
//
//	beginSSE(c echo.Context)

package api

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
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
