// 文件说明：client_wire.go — OpenAI 兼容 Chat Completions 的 wire 形状
//
// 请求体（messages / tools 的嵌套 function 形状）与非流式响应的解码类型，
// 以及 agentcore.ChatRequest ↔ wire 的转换。流式帧见 client_stream.go。

package cli

import (
	"encoding/json"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

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
