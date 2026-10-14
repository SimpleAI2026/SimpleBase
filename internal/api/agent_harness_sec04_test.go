package api

import (
	"context"
	"encoding/json"
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

// SEC-04 辅助：按指定 Principal 发请求（绕过认证中间件，直接注入上下文）。
func (hs *agentHarness) reqAs(method, path string, body any, p auth.Principal) *httptest.ResponseRecorder {
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, strings.NewReader(string(b)))
		r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	ctx := WithProject(r.Context(), ProjectContext{ID: "testprj1", TenantID: catalog.ReservedTenantID})
	ctx = WithPrincipal(ctx, p)
	r = r.WithContext(ctx)
	rec := httptest.NewRecorder()
	hs.echo.ServeHTTP(rec, r)
	return rec
}

// fakellmScriptLong 构造慢脚本：第一段内容后延时，保证 run 处于 running
// 状态时测试能发出取消/确认请求。
func fakellmScriptLong() fakellm.Script {
	return fakellm.Script{Steps: []fakellm.Step{
		{Content: "正在生成"},
		{Content: "还在生成", Delay: 8 * time.Second},
		{FinishReason: "stop"},
	}}
}

// startRunAsync 后台发起长 run 并返回 runID（不等待完成）。
func (hs *agentHarness) startRunAsync(t *testing.T, threadID string) string {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		rec := hs.req(http.MethodPost, "/agent-threads/"+threadID+"/runs",
			map[string]any{"content": "长回答", "stream": true})
		head := rec.Body.String()
		events, _ := parseSSE(head)
		done <- runIDOf(events)
	}()
	// 轮询 store 直到 run 出现（全量并行时调度偏慢，窗口放宽）。
	deadline := time.Now().Add(20 * time.Second)
	for {
		runs, err := hs.store.ListAgentRuns(context.Background(), "testprj1", threadID, 10)
		if err == nil && len(runs) > 0 {
			return runs[0].ID
		}
		select {
		case id := <-done:
			if id != "" {
				return id
			}
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("run never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// cleanupRun 停掉后台 run goroutine（断开 fake 上游即可结束）。
func (hs *agentHarness) cleanupRun() {
	if hs.fake != nil {
		hs.fake.Close()
	}
}
