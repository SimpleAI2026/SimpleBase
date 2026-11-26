// 文件说明：oai_dto_test.go — internal/api 的单元测试（TestOpenAIRoutePaths、TestOpenAIRoutesRequireLLMInvoke 等）
//
// 用法
//
//	go test ./internal/api -run 'TestOpenAIRoutePaths|TestOpenAIRoutesRequireLLMInvoke|TestOpenAIRoutesRequireAuth|TestOAIErrorType'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/oai_dto_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestOpenAIRoutePaths(t *testing.T)
//	openAIRoutes(all map[string]bool) []string
//	TestOpenAIRoutesRequireLLMInvoke(t *testing.T)
//	TestOpenAIRoutesRequireAuth(t *testing.T)
//	TestOAIErrorType(t *testing.T)
//	TestNormalizeFinishReason(t *testing.T)
//	TestDecodeToolChoice(t *testing.T)
//	TestToGatewayRequestCarriesToolChoice(t *testing.T)
//	... 另有 2 个顶层符号

package llmhttp

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

func TestOAIErrorType(t *testing.T) {
	cases := map[int]string{
		http.StatusUnauthorized:          "authentication_error",
		http.StatusForbidden:             "permission_error",
		http.StatusTooManyRequests:       "rate_limit_error",
		http.StatusBadRequest:            "invalid_request_error",
		http.StatusNotFound:              "invalid_request_error",
		http.StatusUnprocessableEntity:   "invalid_request_error",
		http.StatusRequestEntityTooLarge: "invalid_request_error",
		http.StatusInternalServerError:   "api_error",
		http.StatusBadGateway:            "api_error",
		http.StatusServiceUnavailable:    "api_error",
	}
	for status, want := range cases {
		if got := oaiErrorType(status); got != want {
			t.Errorf("oaiErrorType(%d)=%q want %q", status, got, want)
		}
	}
}

func TestNormalizeFinishReason(t *testing.T) {
	cases := []struct {
		reason    string
		toolCalls int
		want      string
	}{
		{"", 0, "stop"},
		{"", 2, "tool_calls"},
		{"stop", 0, "stop"},
		{"length", 0, "length"},
		{"tool_calls", 1, "tool_calls"},
		{"tool_use", 1, "tool_calls"}, // Anthropic 语义归一
		{"  ", 1, "tool_calls"},
	}
	for _, tc := range cases {
		if got := normalizeFinishReason(tc.reason, tc.toolCalls); got != tc.want {
			t.Errorf("normalizeFinishReason(%q,%d)=%q want %q", tc.reason, tc.toolCalls, got, tc.want)
		}
	}
}

func TestDecodeToolChoice(t *testing.T) {
	if httpkit.DecodeToolChoice(nil) != nil || httpkit.DecodeToolChoice(json.RawMessage("null")) != nil {
		t.Fatal("empty/null must mean unset")
	}
	if got := httpkit.DecodeToolChoice(json.RawMessage(`"required"`)); got != "required" {
		t.Fatalf("string choice=%v", got)
	}
	obj, ok := httpkit.DecodeToolChoice(json.RawMessage(`{"type":"function","function":{"name":"t"}}`)).(map[string]any)
	if !ok || obj["type"] != "function" {
		t.Fatalf("object choice=%v", obj)
	}
	// 非法 JSON 兜底为「不指定」；入口校验负责以 400 拒绝，这里不能 panic。
	if httpkit.DecodeToolChoice(json.RawMessage(`{bad`)) != nil {
		t.Fatal("invalid JSON must degrade to unset")
	}
}

// toGatewayRequest 要把 tool_choice 带到网关请求上。
func TestToGatewayRequestCarriesToolChoice(t *testing.T) {
	req := httpkit.LLMRequest{
		Messages:   []httpkit.LLMMessage{{Role: "user", Content: "x"}},
		Tools:      []httpkit.LLMTool{{Name: "t"}},
		ToolChoice: json.RawMessage(`"required"`),
	}
	if got := httpkit.ToGatewayRequest(req).ToolChoice; got != "required" {
		t.Fatalf("tool_choice=%v", got)
	}
	// 云 Agent 的循环从不设置 ToolChoice：必须保持 nil，否则会强制每轮都调工具。
	if got := httpkit.ToGatewayRequest(httpkit.LLMRequest{Messages: req.Messages, Tools: req.Tools}).ToolChoice; got != nil {
		t.Fatalf("agent loop must not force a tool choice, got %v", got)
	}
}

// 兼容请求映射：工具声明里 Parameters 缺省时由 adapter 补空 schema，这里只保证原样搬运。
func TestOAIToLLMRequestMapping(t *testing.T) {
	maxTok := 9
	temp := 0.3
	req := oaiChatRequest{
		Model:       "m",
		MaxTokens:   &maxTok,
		Temperature: &temp,
		Messages: []oaiMessage{
			{Role: "user", Content: "q"},
			{Role: "assistant", ToolCalls: []oaiToolCall{{ID: "c1", Type: "function", Function: oaiCallFunction{Name: "t", Arguments: `{"a":1}`}}}},
			{Role: "tool", ToolCallID: "c1", Content: "r"},
		},
		Tools:      []oaiTool{{Type: "function", Function: oaiFunction{Name: "t", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}}},
		ToolChoice: json.RawMessage(`"auto"`),
	}
	got := oaiToLLMRequest(req)
	if got.Model != "m" || *got.MaxTokens != 9 || *got.Temperature != 0.3 {
		t.Fatalf("scalars=%+v", got)
	}
	if len(got.Messages) != 3 || got.Messages[1].ToolCalls[0].Arguments != `{"a":1}` || got.Messages[2].ToolCallID != "c1" {
		t.Fatalf("messages=%+v", got.Messages)
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "t" || got.Tools[0].Description != "d" || string(got.Tools[0].Parameters) != `{"type":"object"}` {
		t.Fatalf("tools=%+v", got.Tools)
	}
	if string(got.ToolChoice) != `"auto"` {
		t.Fatalf("tool_choice=%s", got.ToolChoice)
	}
}

// 模型列表：总数为 0 时求和补齐；总数已给时不覆盖。
func TestOAITokenUsageTotals(t *testing.T) {
	if u := oaiTokenUsage(httpkit.LLMTokenUsage{PromptTokens: 2, CompletionTokens: 3}); u.TotalTokens != 5 {
		t.Fatalf("total=%d", u.TotalTokens)
	}
	if u := oaiTokenUsage(httpkit.LLMTokenUsage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 9}); u.TotalTokens != 9 {
		t.Fatalf("total must not be overridden: %d", u.TotalTokens)
	}
}
