// 文件说明：client.go — gocloudagent/cli 中 HTTPClient、Chat 等符号的实现
//
// 用法
//
//	(c *HTTPClient) Chat(ctx context.Context, _ string, req agentcore.ChatRequest) ...  // Chat 走 POST .../v1/chat/completions（非流式）
//	(c *HTTPClient) Stream(ctx context.Context, _ string, req agentcore.ChatRequest) ...  // Stream 走 POST .../v1/chat/completions，stream:true（SSE）
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/client.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/agentcore
//	[同包] command.go, compaction.go, config.go, prompt.go, repeat_detector.go 等 9 个
//
// 主要符号
//
//	type HTTPClient struct {…}
//	(c *HTTPClient) Chat(ctx context.Context, _ string, req agentcore.ChatRequest) ...
//	(c *HTTPClient) Stream(ctx context.Context, _ string, req agentcore.ChatRequest) ...
//	(c *HTTPClient) Probe(ctx context.Context) error
//	... 另有 23 个顶层符号

package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/voocel/litellm/providers"
)

// HTTPClient calls an OpenAI-compatible Chat Completions endpoint.
// Endpoint is the complete API base URL, including any version or tenant path.
//
// HTTP 为空时，首次使用惰性创建一个默认客户端并在后续请求间复用（保持连接、TLS 会话），
// 它是 HTTPClient 自己的资源：不再用时调用 CloseIdleConnections。调用方注入的 HTTP 优先，
// 且永远不会被本类型替换或关闭。HTTPClient 创建后不应被复制（含 sync.Once）。
type HTTPClient struct {
	Endpoint string
	Token    string
	HTTP     *http.Client

	defaultOnce   sync.Once
	defaultClient *http.Client
}

var _ agentcore.ChatClient = (*HTTPClient)(nil)

// clientTimeout bounds the entire model response, including streaming. A shorter
// header timeout can cut off reasoning models before their first token.
const clientTimeout = 5 * time.Minute

func (c *HTTPClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	c.defaultOnce.Do(func() {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.ResponseHeaderTimeout = clientTimeout
		c.defaultClient = &http.Client{Timeout: clientTimeout, Transport: transport}
	})
	return c.defaultClient
}

// CloseIdleConnections 关闭本类型自己创建的默认客户端上的空闲连接。
// 调用方注入的 HTTP 客户端不受影响。
func (c *HTTPClient) CloseIdleConnections() {
	if c == nil || c.HTTP != nil {
		return
	}
	if client := c.defaultClient; client != nil {
		client.CloseIdleConnections()
	}
}

// url appends an OpenAI resource path to the configured base URL.
func (c *HTTPClient) url(path string) string {
	return strings.TrimRight(c.Endpoint, "/") + path
}

// Chat 走 POST .../v1/chat/completions（非流式）。
// 重试由后端 llmgateway 的 Resilience 统一处理；本客户端只保留整体超时。
func (c *HTTPClient) Chat(ctx context.Context, _ string, req agentcore.ChatRequest) (agentcore.ChatResponse, error) {
	body, err := json.Marshal(toWire(req))
	if err != nil {
		return agentcore.ChatResponse{}, err
	}
	resp, err := c.do(ctx, http.MethodPost, "/chat/completions", body, "")
	if err != nil {
		return agentcore.ChatResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return agentcore.ChatResponse{}, apiError(resp.StatusCode, raw)
	}
	var parsed wireChatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return agentcore.ChatResponse{}, fmt.Errorf("decode llm response: %w", err)
	}
	out := agentcore.ChatResponse{Model: parsed.Model, Usage: parsed.Usage.toChatUsage()}
	if len(parsed.Choices) > 0 {
		ch := parsed.Choices[0]
		out.Content = ch.Message.Content
		out.FinishReason = ch.FinishReason
		for _, tc := range ch.Message.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, agentcore.ChatToolCall{
				ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
			})
		}
	}
	return out, nil
}

// Stream 走 POST .../v1/chat/completions，stream:true（SSE）。
func (c *HTTPClient) Stream(ctx context.Context, _ string, req agentcore.ChatRequest) (agentcore.TokenStream, error) {
	wire := toWire(req)
	wire.Stream = true
	wire.StreamOptions = &wireStreamOption{IncludeUsage: true}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, http.MethodPost, "/chat/completions", body, "text/event-stream")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		return nil, apiError(resp.StatusCode, raw)
	}
	return &httpTokenStream{body: resp.Body, reader: bufio.NewReader(resp.Body)}, nil
}

// do 发送一次请求并返回响应；响应体由调用方关闭。accept 非空时覆盖默认的 application/json。
// 传输层错误统一包成 "call <endpoint>: ..."；状态码检查与响应体读取上限由各调用方决定。
func (c *HTTPClient) do(ctx context.Context, method, path string, body []byte, accept string) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url(path), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", c.Endpoint, err)
	}
	return resp, nil
}

// apiError 把服务端错误信封转成可读错误（携带 code 便于脚本判断）。
//
// 新端点返回 OpenAI 错误形状 {"error":{"message","type","param","code"}}。
// 服务端其余路由仍是 {"error":{"code","message","request_id"}}，两者 message 与 code
// 字段同名，因此同一份解析即可兼容。
//
// 一律返回 *providers.LiteLLMError 并保留 StatusCode：agentcore 的回退判定只认这个类型，
//   - tools_not_supported（400/422 且文案含 tool）→ native→text 回退；
//   - 其余带状态的错误（401/403/429/5xx…）→ 不得再退化为第二次 Chat 请求。
//
// Error() 文本保持 "<code> (http <status>): <message>"，调用方与脚本看到的文案不变
// （见 statusError：不加 "[litellm:...]" 前缀）。
func apiError(status int, raw []byte) error {
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Error.Message != "" {
		code := envelope.Error.Code
		if code == "" {
			code = "api_error"
		}
		// 服务端用稳定错误码声明「上游模型不支持工具调用」。
		if code == "tools_not_supported" {
			return &providers.LiteLLMError{
				Type: providers.ErrorTypeValidation, Code: code,
				Message: envelope.Error.Message, StatusCode: status,
			}
		}
		return httpStatusError(status, code, fmt.Sprintf("%s (http %d): %s", code, status, envelope.Error.Message))
	}
	msg := strings.TrimSpace(string(raw))
	if len(msg) > 300 {
		msg = msg[:300]
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	return httpStatusError(status, "api_error", fmt.Sprintf("api_error (http %d): %s", status, msg))
}

// httpStatusError 构造带状态码的 LiteLLMError；text 是对外展示的完整文案。
func httpStatusError(status int, code, text string) error {
	return &statusError{LiteLLMError: providers.LiteLLMError{
		Type: providers.ErrorTypeProvider, Code: code, Message: text, StatusCode: status,
	}}
}

// statusError 只改变 Error() 的呈现：直接输出 text，不加 "[litellm:...]" 前缀。
// errors.As 到 *providers.LiteLLMError 仍然成立（经 Unwrap 暴露内嵌值）。
type statusError struct{ providers.LiteLLMError }

func (e *statusError) Error() string { return e.Message }

func (e *statusError) Unwrap() error { return &e.LiteLLMError }

// Probe 用一次最小请求验证凭证与项目是否可用（`gocloudagent config test`）。
// 打 GET .../v1/models：它与对话共用 llm:invoke 权限，因此「probe 通过」意味着
// 这个 Key 真的有权调用模型，而不只是能读项目。
func (c *HTTPClient) Probe(ctx context.Context) error {
	resp, err := c.do(ctx, http.MethodGet, "/models", nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiError(resp.StatusCode, raw)
	}
	return nil
}
