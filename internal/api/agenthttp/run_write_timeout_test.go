package agenthttp

// run_write_timeout_test.go：Agent run 的 SSE 长连接不受写超时影响（自 internal/api/sse_write_timeout_test.go 随 harness 迁入）。

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

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
			ctx := httpkit.WithProject(c.Request().Context(), httpkit.ProjectContext{ID: "testprj1", TenantID: catalog.ReservedTenantID})
			ctx = httpkit.WithPrincipal(ctx, auth.Principal{APIKeyID: "k1", TenantID: catalog.ReservedTenantID})
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
