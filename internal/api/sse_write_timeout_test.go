package api

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
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
	h := &LLMHandler{svc: &fakeLLMSvc{reader: &slowStreamReader{blocks: 25, delay: 40 * time.Millisecond}}}
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

// slowStreamScript 构造「按固定间隔持续输出」的上游剧本，总时长超过 WriteTimeout。
func slowStreamScript(blocks int, delay time.Duration) fakellm.Script {
	steps := make([]fakellm.Step, 0, blocks+2)
	for i := 0; i < blocks; i++ {
		steps = append(steps, fakellm.Step{Content: "x", Delay: delay})
	}
	steps = append(steps, fakellm.Step{FinishReason: "stop"}, fakellm.Step{PromptTokens: 1, CompletionTokens: 1})
	return fakellm.Script{Steps: steps}
}

// 端到端：真实云助手 handler 在 WriteTimeout 生效的服务器上也必须完整收尾。
// 覆盖 beginSSE 真的接在了 streamRun 上（而不只是辅助函数本身可用）。
func TestAgentRunSurvivesWriteTimeout(t *testing.T) {
	// 上游每 40ms 一块、共 25 块 ≈ 1s；服务端 WriteTimeout 只有 250ms。
	hs := newAgentHarness(t, nil, slowStreamScript(25, 40*time.Millisecond))
	threadID := hs.createThread(t)
	// 服务端注入项目/身份上下文：客户端 req.Context() 不传到服务端。
	hs.echo.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithProject(c.Request().Context(), ProjectContext{ID: "testprj1", TenantID: catalog.ReservedTenantID})
			ctx = WithPrincipal(ctx, auth.Principal{APIKeyID: "k1", TenantID: catalog.ReservedTenantID})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	srv := httptest.NewUnstartedServer(hs.echo)
	srv.Config.WriteTimeout = 250 * time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/agent-threads/"+threadID+"/runs",
		strings.NewReader(`{"content":"长回答","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("run status=%d body=%s", resp.StatusCode, string(raw))
	}

	var tokens int
	var sawEnd, sawUsage bool
	reason := ""
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		switch {
		case strings.Contains(data, `"type":"token"`):
			tokens++
		case strings.Contains(data, `"type":"usage"`):
			sawUsage = true
		case strings.Contains(data, `"type":"end"`):
			sawEnd = true
			if idx := strings.Index(data, `"reason":"`); idx >= 0 {
				rest := data[idx+len(`"reason":"`):]
				if j := strings.Index(rest, `"`); j >= 0 {
					reason = rest[:j]
				}
			}
		}
	}

	// 被 WriteTimeout 切断时：客户端收不到 end 帧（连接直接关闭），
	// 且 token 数远少于上游实际输出的 25 块。只断言「有 token」会被误判通过。
	if !sawEnd {
		t.Fatalf("stream ended without an end frame (tokens=%d): connection was cut off, beginSSE is not applied to streamRun", tokens)
	}
	if reason != "stop" {
		t.Fatalf("end reason=%q want stop (tokens=%d)", reason, tokens)
	}
	if !sawUsage {
		t.Fatalf("missing usage frame (tokens=%d)", tokens)
	}
	if tokens != 25 {
		t.Fatalf("tokens=%d want 25: stream was truncated", tokens)
	}
}
