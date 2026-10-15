package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// 锁定双 v1 路径：前一段是 SimpleBase 的 API 版本，后一段是 OpenAI SDK 约定的资源前缀。
// 防止后来者把 ".../projects/:id/v1/chat/completions" "顺手修正"成单段 v1——
// 那会让官方 SDK 在 base_url=".../projects/<id>/v1" 时拼出 404 路径。
func TestOpenAIRoutePaths(t *testing.T) {
	deps, key := fullRouterDeps(t)
	e := NewRouter(deps)

	registered := map[string]bool{}
	for _, r := range e.Routes() {
		registered[r.Method+" "+r.Path] = true
	}
	for _, want := range []string{
		"POST /v1/projects/:projectID/v1/chat/completions",
		"GET /v1/projects/:projectID/v1/models",
	} {
		if !registered[want] {
			t.Fatalf("route %q not registered; got %v", want, openAIRoutes(registered))
		}
	}
	// 旧端点已删除，不保留兼容期。
	for _, gone := range []string{
		"POST /v1/projects/:projectID/llm/chat",
		"POST /v1/projects/:projectID/llm/stream",
		"GET /v1/projects/:projectID/llm/providers",
	} {
		if registered[gone] {
			t.Fatalf("legacy route %q must be removed", gone)
		}
	}
	// /llm/settings 是模型配置读写，不在清理范围，必须仍在。
	for _, kept := range []string{
		"GET /v1/projects/:projectID/llm/settings",
		"PUT /v1/projects/:projectID/llm/settings",
	} {
		if !registered[kept] {
			t.Fatalf("route %q is settings, not the chat protocol, and must be kept", kept)
		}
	}

	// 端到端：带 Key 能通；旧路径不再可达（POST 落到 SPA fallback 的 405/404）。
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/v1/models", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("models status=%d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"object":"list"`) {
		t.Fatalf("models body=%s", rec.Body.String())
	}

	old := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/llm/chat", strings.NewReader(`{"messages":[{"role":"user","content":"x"}]}`))
	old.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	old.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	oldRec := httptest.NewRecorder()
	e.ServeHTTP(oldRec, old)
	if oldRec.Code == http.StatusOK {
		t.Fatalf("legacy /llm/chat must not be served any more: %s", oldRec.Body.String())
	}
}

func openAIRoutes(all map[string]bool) []string {
	var out []string
	for r := range all {
		if strings.Contains(r, "/v1/chat") || strings.Contains(r, "/v1/models") || strings.Contains(r, "/llm/") {
			out = append(out, r)
		}
	}
	return out
}

// 新端点要求 llm:invoke，而不是 database:read：只有读权限的 Key 不能调用模型。
func TestOpenAIRoutesRequireLLMInvoke(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Auth = auth.NewService(&memAuthRepo{rec: auth.APIKeyRecord{
		ID: "key-ro", KeyHash: deps.Auth.HashKey(key), TenantID: "tenant-1",
		ProjectIDs: []string{"proj-1"}, Permissions: []auth.Permission{auth.DatabaseRead},
	}}, "secret")
	e := NewRouter(deps)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/projects/proj-1/v1/models", ""},
		{http.MethodPost, "/v1/projects/proj-1/v1/chat/completions", `{"messages":[{"role":"user","content":"x"}]}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s with database:read only: status=%d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

// 无凭证必须 401，而不是落进 SPA fallback。
func TestOpenAIRoutesRequireAuth(t *testing.T) {
	deps, _ := fullRouterDeps(t)
	e := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/v1/models", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
}

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
	if decodeToolChoice(nil) != nil || decodeToolChoice(json.RawMessage("null")) != nil {
		t.Fatal("empty/null must mean unset")
	}
	if got := decodeToolChoice(json.RawMessage(`"required"`)); got != "required" {
		t.Fatalf("string choice=%v", got)
	}
	obj, ok := decodeToolChoice(json.RawMessage(`{"type":"function","function":{"name":"t"}}`)).(map[string]any)
	if !ok || obj["type"] != "function" {
		t.Fatalf("object choice=%v", obj)
	}
	// 非法 JSON 兜底为「不指定」；入口校验负责以 400 拒绝，这里不能 panic。
	if decodeToolChoice(json.RawMessage(`{bad`)) != nil {
		t.Fatal("invalid JSON must degrade to unset")
	}
}

// toGatewayRequest 要把 tool_choice 带到网关请求上。
func TestToGatewayRequestCarriesToolChoice(t *testing.T) {
	req := LLMRequest{
		Messages:   []LLMMessage{{Role: "user", Content: "x"}},
		Tools:      []LLMTool{{Name: "t"}},
		ToolChoice: json.RawMessage(`"required"`),
	}
	if got := toGatewayRequest(req).ToolChoice; got != "required" {
		t.Fatalf("tool_choice=%v", got)
	}
	// 云 Agent 的循环从不设置 ToolChoice：必须保持 nil，否则会强制每轮都调工具。
	if got := toGatewayRequest(LLMRequest{Messages: req.Messages, Tools: req.Tools}).ToolChoice; got != nil {
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
	if u := oaiTokenUsage(LLMTokenUsage{PromptTokens: 2, CompletionTokens: 3}); u.TotalTokens != 5 {
		t.Fatalf("total=%d", u.TotalTokens)
	}
	if u := oaiTokenUsage(LLMTokenUsage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 9}); u.TotalTokens != 9 {
		t.Fatalf("total must not be overridden: %d", u.TotalTokens)
	}
}
