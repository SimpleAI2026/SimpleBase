// 文件说明：sse_write_timeout_test.go — internal/api 的单元测试（TestSSECutOffByWriteTimeoutWithoutFix、TestSSESurvivesWriteTimeoutWithFix 等）
//
// 用法
//
//	go test ./internal/api -run 'TestSSECutOffByWriteTimeoutWithoutFix|TestSSESurvivesWriteTimeoutWithFix|TestBeginSSEIsSafeOnResponseRecorder|TestLLMStreamSurvivesWriteTimeout'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sse_write_timeout_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	sseFrameWriter(clearDeadline bool, blocks int, delay time.Duration) echo.HandlerFunc
//	countFrames(t *testing.T, writeTimeout time.Duration, h echo.HandlerFunc) int
//	TestSSECutOffByWriteTimeoutWithoutFix(t *testing.T)
//	TestSSESurvivesWriteTimeoutWithFix(t *testing.T)
//	TestBeginSSEIsSafeOnResponseRecorder(t *testing.T)
//	type slowStreamReader struct{…}
//	(s *slowStreamReader) Next() (*LLMStreamChunk, error)
//	(s *slowStreamReader) Close() error
//	... 另有 3 个顶层符号

package api

import (
	"bufio"
	"fmt"
	"github.com/linkxzhou/SimpleBase/internal/api/llmhttp"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// planv6.0 v2 §11：http.Server.WriteTimeout 是「从读完请求头起算的绝对写 deadline」，
// 不是两次写之间的空闲上限。SSE 若不解除它，连接会在 write_timeout（默认 30s）
// 到点时被服务端关闭、请求 context 随之取消 —— 线上表现就是「云助手 run 恰好
// 30 秒后 reason=canceled、0 token、界面看不到任何输出」。
//
// 本文件用真实 httptest.Server + 很小的 WriteTimeout 复现该机制，
// 并断言 beginSSE 之后长连接可以完整跑完。

// sseFrameWriter 模拟一个持续输出的 SSE 端点：每 delay 写一帧，共 blocks 帧。
// clearDeadline 为真时先调用 beginSSE（即生产代码的做法）。
//
// 返回 (已发出的帧数, context 是否在中途被取消)。
func sseFrameWriter(clearDeadline bool, blocks int, delay time.Duration) echo.HandlerFunc {
	return func(c echo.Context) error {
		if clearDeadline {
			beginSSE(c)
		}
		c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
		c.Response().WriteHeader(http.StatusOK)
		flusher, _ := c.Response().Writer.(http.Flusher)
		ctx := c.Request().Context()
		for i := 0; i < blocks; i++ {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(delay):
			}
			if _, err := fmt.Fprintf(c.Response(), "id: %d\ndata: {\"i\":%d}\n\n", i, i); err != nil {
				return nil
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		return nil
	}
}

// countFrames 请求 SSE 端点并返回收到的帧数。
func countFrames(t *testing.T, writeTimeout time.Duration, h echo.HandlerFunc) int {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	e.GET("/sse", h)
	srv := httptest.NewUnstartedServer(e)
	srv.Config.WriteTimeout = writeTimeout
	srv.Start()
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/sse")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	frames := 0
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "id:") {
			frames++
		}
	}
	return frames
}

// 机制对照：不解除写 deadline 时，连接会在 WriteTimeout 到点被切断。
// 这正是线上「恰好 30s、reason=canceled、无 token」的成因。
func TestSSECutOffByWriteTimeoutWithoutFix(t *testing.T) {
	// 每帧 40ms × 25 帧 ≈ 1s；WriteTimeout 设 250ms。
	frames := countFrames(t, 250*time.Millisecond, sseFrameWriter(false, 25, 40*time.Millisecond))
	if frames >= 25 {
		t.Fatalf("expected the stream to be cut off well before completion, got %d/25 frames", frames)
	}
}

// beginSSE 解除写 deadline 后，同一条长连接应完整输出所有帧。
func TestSSESurvivesWriteTimeoutWithFix(t *testing.T) {
	frames := countFrames(t, 250*time.Millisecond, sseFrameWriter(true, 25, 40*time.Millisecond))
	if frames != 25 {
		t.Fatalf("expected all 25 frames despite WriteTimeout, got %d", frames)
	}
}

// beginSSE 对不支持 deadline 的写入器必须安全无副作用（httptest.ResponseRecorder）。
func TestBeginSSEIsSafeOnResponseRecorder(t *testing.T) {
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/x", nil), rec)
	beginSSE(c)
	if _, err := c.Response().Write([]byte("data: {}\n\n")); err != nil {
		t.Fatalf("write after beginSSE: %v", err)
	}
	if rec.Body.String() != "data: {}\n\n" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

// slowStreamReader 按固定间隔产出 chunk，总时长超过 WriteTimeout。
type slowStreamReader struct {
	blocks int
	delay  time.Duration
	i      int
}

func (s *slowStreamReader) Next() (*LLMStreamChunk, error) {
	if s.i >= s.blocks {
		return nil, io.EOF
	}
	time.Sleep(s.delay)
	s.i++
	return &LLMStreamChunk{Type: "content", Content: "x"}, nil
}

func (s *slowStreamReader) Close() error { return nil }

// 端到端：OpenAI 兼容流式端点同样必须解除写超时（另一个 SSE 长连接端点）。
func TestLLMStreamSurvivesWriteTimeout(t *testing.T) {
	h := llmhttp.NewLLMHandler(&fakeLLMSvc{reader: &slowStreamReader{blocks: 25, delay: 40 * time.Millisecond}}, nil, nil, nil)
	e := echo.New()
	e.HideBanner = true
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithProject(c.Request().Context(), ProjectContext{ID: "proj-1", TenantID: "t"})
			ctx = WithPrincipal(ctx, auth.Principal{APIKeyID: "key-llm"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.POST("/v1/chat/completions", h.ChatCompletions)

	srv := httptest.NewUnstartedServer(e)
	srv.Config.WriteTimeout = 250 * time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	chunks := 0
	done := false
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == oaiSSEDone {
			done = true
			continue
		}
		// 只数正文帧：首帧 role 与结束帧 finish_reason 不计入。
		if strings.Contains(data, `"content":"x"`) {
			chunks++
		}
	}
	if !done {
		t.Fatalf("stream cut off before [DONE] (chunks=%d): beginSSE is not applied to ChatCompletions stream", chunks)
	}
	if chunks != 25 {
		t.Fatalf("chunks=%d want 25: stream was truncated", chunks)
	}
}
