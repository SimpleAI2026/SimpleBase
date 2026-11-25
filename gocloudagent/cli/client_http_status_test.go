// 文件说明：client_http_status_test.go — gocloudagent/cli 的单元测试（TestStreamHTTPErrorDoesNotFallBackToChat、TestToolsNotSupportedStillFallsBackToTextProtocol 等）
//
// 用法
//
//	go test ./gocloudagent/cli -run 'TestStreamHTTPErrorDoesNotFallBackToChat|TestToolsNotSupportedStillFallsBackToTextProtocol'
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/client_http_status_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_loop.go, agent_loop_test.go, budget_test.go, client.go, client_conn_reuse_test.go 等 39 个
//
// 主要符号
//
//	TestStreamHTTPErrorDoesNotFallBackToChat(t *testing.T)
//	TestToolsNotSupportedStillFallsBackToTextProtocol(t *testing.T)

package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// BUG-05：401/429 等带状态的 HTTP 错误不能让 Model.Stream 退化为第二次 Chat 请求。
func TestStreamHTTPErrorDoesNotFallBackToChat(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500} {
		var hits int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"code":"denied","message":"nope"}}`))
		}))
		client := &HTTPClient{Endpoint: srv.URL + "/v1", Token: "t"}
		m := agentcore.NewModel(client, "", "m")
		m.Protocol = "auto"
		if _, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("hi")}); err == nil {
			t.Fatalf("status %d: expected an error", status)
		}
		srv.Close()
		if got := atomic.LoadInt32(&hits); got != 1 {
			t.Fatalf("status %d: server hit %d times, want exactly 1 (no stream→chat fallback)", status, got)
		}
	}
}

// 修复不能破坏既有回退：服务端用 tools_not_supported 声明上游不支持工具时，
// auto 协议仍要降级到文本工具协议（第二次请求不带 tools）。
func TestToolsNotSupportedStillFallsBackToTextProtocol(t *testing.T) {
	agentcore.ResetUnsupportedNativeTools()
	t.Cleanup(agentcore.ResetUnsupportedNativeTools)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":"tools_not_supported","message":"model does not support tools"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()
	client := &HTTPClient{Endpoint: srv.URL + "/v1", Token: "t"}
	m := agentcore.NewModel(client, "scope", "m")
	m.Protocol = "auto"
	tools := []*schema.ToolInfo{{Name: "t", Desc: "d"}}
	bound, err := m.WithTools(tools)
	if err != nil {
		t.Fatal(err)
	}
	sr, err := bound.Stream(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("fallback stream: %v", err)
	}
	defer sr.Close()
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("server hit %d times, want 2 (native attempt + text fallback)", got)
	}
}
