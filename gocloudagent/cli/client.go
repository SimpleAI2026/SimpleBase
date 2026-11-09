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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/voocel/litellm/providers"
)

// HTTPClient 通过 SimpleBase 的 OpenAI 兼容出口调用模型（决策 D-D1）。
// CLI 不自管供应商密钥，模型出口复用服务端已有的 llmgateway 配置。
//
// 端点是 POST {endpoint}/v1/projects/{project}/v1/chat/completions，
// 请求与响应都是标准 OpenAI Chat Completions 形状（含 tools / tool_calls）。
// 路径里的两段 v1：前一段是 SimpleBase 的 API 版本，后一段是 OpenAI 资源前缀。
type HTTPClient struct {
	Endpoint  string
	Token     string
	ProjectID string
	HTTP      *http.Client
}

var _ agentcore.ChatClient = (*HTTPClient)(nil)

// clientTimeout 是 CLI 等待一次模型 HTTP 响应的整体上限（含首字节与正文）。
//
// 不另设更短的 ResponseHeaderTimeout。SimpleBase 的 SSE 要等上游响应头才写自己的
// 响应头，所以客户端的「等响应头」量的是上游首字节，不是本地建连。推理模型的首字节
// 经常超过 30s；30s 会把还在思考的一轮掐掉，agent 只读完文件就退出（HumanEval
// 158→150、MBPP 352→317 的主因）。建连黑洞由 llmgateway 的 ConnectTimeout 处理，
// 静默上游由 provider RequestTimeout 和这里的整体上限处理。
const clientTimeout = 5 * time.Minute

func (c *HTTPClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = clientTimeout
	return &http.Client{Timeout: clientTimeout, Transport: transport}
}

// url 拼出项目级 OpenAI 兼容路径；path 以 "/" 开头，如 "/chat/completions"。
func (c *HTTPClient) url(path string) string {
	return strings.TrimRight(c.Endpoint, "/") + "/v1/projects/" + c.ProjectID + "/v1" + path
}

// ---------------------------------------------------------------------------
// 请求（OpenAI 标准形状）
// ---------------------------------------------------------------------------

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// wireToolCall 是 OpenAI 的嵌套形状：{id, type, function:{name, arguments}}。
// 旧版是平铺的 {id, name, arguments}——那是自定义协议，已随旧端点一并废弃。
type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireCallFunction `json:"function"`
}

type wireCallFunction struct {
	Name string `json:"name"`
	// Arguments 是 JSON 字符串而不是对象，这是 OpenAI 的既有约定。
	Arguments string `json:"arguments"`
}

type wireRequest struct {
	Model         string            `json:"model,omitempty"`
	Messages      []wireMessage     `json:"messages"`
	MaxTokens     *int              `json:"max_tokens,omitempty"`
	Temperature   *float64          `json:"temperature,omitempty"`
	Tools         []wireTool        `json:"tools,omitempty"`
	Stream        bool              `json:"stream,omitempty"`
	StreamOptions *wireStreamOption `json:"stream_options,omitempty"`
}

// wireStreamOption 要求服务端在流末尾带上 usage，CLI 据此统计 token。
type wireStreamOption struct {
	IncludeUsage bool `json:"include_usage"`
}

// wireTool 是 OpenAI 的工具声明：{type:"function", function:{name, description, parameters}}。
// 旧版是平铺的 {name, description, parameters}。
type wireTool struct {
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

func toWire(req agentcore.ChatRequest) wireRequest {
	out := wireRequest{Model: req.Model, MaxTokens: req.MaxTokens, Temperature: req.Temperature}
	for _, m := range req.Messages {
		wm := wireMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID: tc.ID, Type: "function",
				Function: wireCallFunction{Name: tc.Name, Arguments: tc.Arguments},
			})
		}
		out.Messages = append(out.Messages, wm)
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, wireTool{
			Type:     "function",
			Function: wireFunction{Name: t.Name, Description: t.Description, Parameters: t.Parameters},
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// 响应（非流式）
// ---------------------------------------------------------------------------

type wireChatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   string         `json:"content"`
			ToolCalls []wireToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage wireUsage `json:"usage"`
}

type wireUsage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	TotalTokens             int `json:"total_tokens"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (u wireUsage) toChatUsage() agentcore.ChatUsage {
	out := agentcore.ChatUsage{PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens}
	if u.CompletionTokensDetails != nil {
		out.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	return out
}

// Chat 走 POST .../v1/chat/completions（非流式）。
// 重试由后端 llmgateway 的 Resilience 统一处理；本客户端只保留整体超时。
func (c *HTTPClient) Chat(ctx context.Context, _ string, req agentcore.ChatRequest) (agentcore.ChatResponse, error) {
	body, err := json.Marshal(toWire(req))
	if err != nil {
		return agentcore.ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return agentcore.ChatResponse{}, err
	}
	c.setHeaders(httpReq)
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return agentcore.ChatResponse{}, fmt.Errorf("call %s: %w", c.Endpoint, err)
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

// ---------------------------------------------------------------------------
// 流式（SSE）
// ---------------------------------------------------------------------------

// wireChunk 是 chat.completion.chunk 帧。
type wireChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage `json:"usage"`
}

// httpTokenStream 把 OpenAI SSE 读取器包成 agentcore.TokenStream，并实现 DeltaStream。
//
// 必须实现 NextDelta：agentcore.Model 只有在拿到 DeltaStream 时才会读取工具调用增量
// 与 usage，退化成 Next() 就只剩正文——工具调用会被整个丢掉。
type httpTokenStream struct {
	body   io.ReadCloser
	reader *bufio.Reader
	closed bool
	// pending 缓存一帧里拆出的多个增量（一帧可能同时带正文与多个工具分片）。
	pending []agentcore.StreamDelta
	done    bool
}

// NextDelta 返回下一个增量；流结束返回 io.EOF。
func (s *httpTokenStream) NextDelta() (agentcore.StreamDelta, error) {
	for {
		if len(s.pending) > 0 {
			d := s.pending[0]
			s.pending = s.pending[1:]
			return d, nil
		}
		if s.done {
			return agentcore.StreamDelta{Finish: true}, io.EOF
		}
		line, err := s.reader.ReadString('\n')
		if err != nil {
			s.done = true
			if errors.Is(err, io.EOF) {
				return agentcore.StreamDelta{Finish: true}, io.EOF
			}
			return agentcore.StreamDelta{}, err
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue // 空行、注释行（": ping"）、event:/id: 行都忽略
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			s.done = true
			return agentcore.StreamDelta{Finish: true}, io.EOF
		}
		var chunk wireChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue // 非 JSON 帧忽略，与前端解析器的口径一致
		}
		s.pending = append(s.pending, deltasFromChunk(chunk)...)
	}
}

// deltasFromChunk 把一帧拆成若干增量。
func deltasFromChunk(chunk wireChunk) []agentcore.StreamDelta {
	var out []agentcore.StreamDelta
	for _, ch := range chunk.Choices {
		if ch.Delta.Content != "" {
			out = append(out, agentcore.StreamDelta{Content: ch.Delta.Content})
		}
		for _, tc := range ch.Delta.ToolCalls {
			out = append(out, agentcore.StreamDelta{ToolCall: &agentcore.ToolCallDelta{
				Index: tc.Index, ID: tc.ID, Name: tc.Function.Name, ArgsDelta: tc.Function.Arguments,
			}})
		}
	}
	if chunk.Usage != nil {
		u := chunk.Usage.toChatUsage()
		out = append(out, agentcore.StreamDelta{Usage: &u})
	}
	return out
}

// Next 是 TokenStream 的兼容形态：只返回正文增量。
func (s *httpTokenStream) Next() (string, bool, error) {
	for {
		d, err := s.NextDelta()
		if err != nil {
			return "", true, err
		}
		if d.Content != "" {
			return d.Content, false, nil
		}
	}
}

func (s *httpTokenStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
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
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", c.Endpoint, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		return nil, apiError(resp.StatusCode, raw)
	}
	return &httpTokenStream{body: resp.Body, reader: bufio.NewReader(resp.Body)}, nil
}

func (c *HTTPClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
}

// apiError 把服务端错误信封转成可读错误（携带 code 便于脚本判断）。
//
// 新端点返回 OpenAI 错误形状 {"error":{"message","type","param","code"}}。
// 服务端其余路由仍是 {"error":{"code","message","request_id"}}，两者 message 与 code
// 字段同名，因此同一份解析即可兼容。
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
		// 服务端用稳定错误码声明「上游模型不支持工具调用」。agentcore 的 auto 协议只认
		// *providers.LiteLLMError（400/422 且文案含 tool），所以必须还原成该类型，
		// 否则降级到文本工具协议的路径对 CLI 永远不会触发，对话直接失败。
		if code == "tools_not_supported" {
			return &providers.LiteLLMError{
				Type: providers.ErrorTypeValidation, Code: code,
				Message: envelope.Error.Message, StatusCode: status,
			}
		}
		return fmt.Errorf("%s (http %d): %s", code, status, envelope.Error.Message)
	}
	msg := strings.TrimSpace(string(raw))
	if len(msg) > 300 {
		msg = msg[:300]
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	return fmt.Errorf("api_error (http %d): %s", status, msg)
}

// Probe 用一次最小请求验证凭证与项目是否可用（`gocloudagent config test`）。
// 打 GET .../v1/models：它与对话共用 llm:invoke 权限，因此「probe 通过」意味着
// 这个 Key 真的有权调用模型，而不只是能读项目。
func (c *HTTPClient) Probe(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/models"), nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("call %s: %w", c.Endpoint, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiError(resp.StatusCode, raw)
	}
	return nil
}
