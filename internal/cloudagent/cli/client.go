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

	"github.com/linkxzhou/SimpleBase/internal/cloudagent/agentcore"
)

// HTTPClient 通过 SimpleBase 的 LLM Gateway 调用模型（决策 D-D1）。
// CLI 不自管供应商密钥，模型出口复用服务端已有的 llmgateway 配置。
type HTTPClient struct {
	Endpoint  string
	Token     string
	ProjectID string
	HTTP      *http.Client
}

var _ agentcore.ChatClient = (*HTTPClient)(nil)

func (c *HTTPClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func (c *HTTPClient) url(path string) string {
	return strings.TrimRight(c.Endpoint, "/") + "/v1/projects/" + c.ProjectID + path
}

// wireMessage 是 gateway 的请求消息形状（与 api.llmMessageDTO 对齐）。
type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireRequest struct {
	Model       string        `json:"model,omitempty"`
	Messages    []wireMessage `json:"messages"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	Tools       []wireTool    `json:"tools,omitempty"`
}

type wireTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type wireChatResponse struct {
	Content      string `json:"content"`
	FinishReason string `json:"finish_reason"`
	Model        string `json:"model"`
	Provider     string `json:"provider"`
	Usage        struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		ReasoningTokens  int `json:"reasoning_tokens"`
	} `json:"usage"`
	ToolCalls []wireToolCall `json:"tool_calls,omitempty"`
}

func toWire(req agentcore.ChatRequest) wireRequest {
	out := wireRequest{Model: req.Model, MaxTokens: req.MaxTokens, Temperature: req.Temperature}
	for _, m := range req.Messages {
		wm := wireMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
		}
		out.Messages = append(out.Messages, wm)
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, wireTool{Name: t.Name, Description: t.Description, Parameters: t.Parameters})
	}
	return out
}

// Chat 走 /llm/chat（非流式）。
func (c *HTTPClient) Chat(ctx context.Context, _ string, req agentcore.ChatRequest) (agentcore.ChatResponse, error) {
	body, err := json.Marshal(toWire(req))
	if err != nil {
		return agentcore.ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/llm/chat"), bytes.NewReader(body))
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
	out := agentcore.ChatResponse{
		Content: parsed.Content, Model: parsed.Model, Provider: parsed.Provider,
		FinishReason: parsed.FinishReason,
	}
	out.Usage = agentcore.ChatUsage{
		PromptTokens: parsed.Usage.PromptTokens, CompletionTokens: parsed.Usage.CompletionTokens,
		ReasoningTokens: parsed.Usage.ReasoningTokens,
	}
	for _, tc := range parsed.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, agentcore.ChatToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}
	return out, nil
}

// httpTokenStream 把 SSE 读取器包成 agentcore.TokenStream。
type httpTokenStream struct {
	body   io.ReadCloser
	reader *bufio.Reader
	closed bool
}

func (s *httpTokenStream) Next() (string, bool, error) {
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", true, io.EOF
			}
			return "", false, err
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var chunk struct {
			Type         string `json:"type"`
			Content      string `json:"content"`
			FinishReason string `json:"finish_reason"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Type == "end" {
			return "", true, io.EOF
		}
		return chunk.Content, chunk.FinishReason != "", nil
	}
}

func (s *httpTokenStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}

// Stream 走 /llm/stream（SSE）。
func (c *HTTPClient) Stream(ctx context.Context, _ string, req agentcore.ChatRequest) (agentcore.TokenStream, error) {
	body, err := json.Marshal(toWire(req))
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/llm/stream"), bytes.NewReader(body))
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

// Probe 用一次最小请求验证凭证与项目是否可用（`cloudagent config test`）。
func (c *HTTPClient) Probe(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/llm/providers"), nil)
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
