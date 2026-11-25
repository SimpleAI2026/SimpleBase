// 文件说明：access_llm.go — 云 Agent 的模型客户端适配（llmgateway.Service → cloudagent.ChatClient）
//
// 用法
//
//	NewCloudAgentLLM(svc llmgateway.Service) cloudagent.ChatClient  // 构造；svc 为 nil 返回 nil（LLM 未启用）
//	(c *llmChatClient) Chat(ctx, projectID, req)  // 非流式补全
//	(c *llmChatClient) Stream(ctx, projectID, req)  // 流式补全（TokenStream 与 DeltaStream 两种读法）
//
// 调用架构
//
//	[上游] internal/app（装配进 cloudagent.Runtime.LLM）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/access_llm.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/agentcore, gocloudagent/cloudagent, internal/llmgateway, litellm/providers
//	[同包] ports.go
//
// 主要符号
//
//	type llmChatClient struct {…}
//	type llmTokenStream struct {…}
//	NewCloudAgentLLM(svc llmgateway.Service) cloudagent.ChatClient
//	toGatewayRequest(req cloudagent.ChatRequest) llmgateway.Request
//	... 另有 7 个顶层符号
//
// 归属（internal-ddd-hexagonal-implementation-plan §4）：Agent 协作上下文作为 LLM 网关的客户，
// 这里是客户侧防腐层，直接消费网关的 Service（不再经 HTTP 层的 LLMService 端口中转）。
// tool_choice 不设置：云 Agent 的循环从不强制选择工具（internal/AGENTS.md）。

package agentbridge

import (
	"context"

	"github.com/voocel/litellm/providers"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
)

// llmChatClient 把 cloudagent.ChatRequest 翻译成网关请求。
type llmChatClient struct{ svc llmgateway.Service }

// NewCloudAgentLLM 构造模型客户端；svc 为 nil 时返回 nil（Runtime 据此判定 llm_not_configured）。
func NewCloudAgentLLM(svc llmgateway.Service) cloudagent.ChatClient {
	if isNilPointer(svc) {
		return nil
	}
	return &llmChatClient{svc: svc}
}

// toGatewayRequest 把 Agent 请求映射为网关形状；工具与工具往返消息原样透传。
func toGatewayRequest(req cloudagent.ChatRequest) llmgateway.Request {
	msgs := make([]providers.Message, len(req.Messages))
	for i, m := range req.Messages {
		pm := providers.Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			pm.ToolCalls = append(pm.ToolCalls, providers.ToolCall{
				ID: tc.ID, Type: "function",
				Function: providers.FunctionCall{Name: tc.Name, Arguments: tc.Arguments},
			})
		}
		msgs[i] = pm
	}
	out := llmgateway.Request{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	for _, t := range req.Tools {
		var params any = map[string]any{"type": "object", "properties": map[string]any{}}
		if len(t.Parameters) > 0 {
			params = t.Parameters
		}
		out.Tools = append(out.Tools, providers.Tool{
			Type:     "function",
			Function: providers.FunctionDef{Name: t.Name, Description: t.Description, Parameters: params},
		})
	}
	return out
}

func (c *llmChatClient) Chat(ctx context.Context, projectID string, req cloudagent.ChatRequest) (cloudagent.ChatResponse, error) {
	resp, err := c.svc.Chat(ctx, projectID, toGatewayRequest(req))
	if err != nil {
		return cloudagent.ChatResponse{}, err
	}
	out := cloudagent.ChatResponse{Content: resp.Content, Model: resp.Model, Provider: resp.Provider, FinishReason: resp.FinishReason,
		Usage: cloudagent.ChatUsage{PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens, ReasoningTokens: resp.Usage.ReasoningTokens}}
	for _, tc := range resp.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, cloudagent.ChatToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	return out, nil
}

func (c *llmChatClient) Stream(ctx context.Context, projectID string, req cloudagent.ChatRequest) (cloudagent.TokenStream, error) {
	reader, err := c.svc.Stream(ctx, projectID, toGatewayRequest(req))
	if err != nil {
		return nil, err
	}
	return &llmTokenStream{r: reader}, nil
}

// llmTokenStream 适配网关的流式读取器：同时提供 DeltaStream（NextDelta）与 TokenStream（Next）两种读法。
type llmTokenStream struct{ r llmgateway.StreamReader }

func (s *llmTokenStream) NextDelta() (cloudagent.StreamDelta, error) {
	chunk, err := s.r.Next()
	if err != nil {
		return cloudagent.StreamDelta{}, err
	}
	if chunk == nil {
		return cloudagent.StreamDelta{}, nil
	}
	out := cloudagent.StreamDelta{Content: chunk.Content, Finish: chunk.Done || chunk.Type == "end"}
	if chunk.Type == "tool_call_delta" {
		delta := &cloudagent.ToolCallDelta{}
		if d := chunk.ToolCallDelta; d != nil {
			delta.Index, delta.ID, delta.Name, delta.ArgsDelta = d.Index, d.ID, d.FunctionName, d.ArgumentsDelta
		}
		out.ToolCall = delta
	}
	if chunk.Usage != nil {
		out.Usage = &cloudagent.ChatUsage{PromptTokens: chunk.Usage.PromptTokens, CompletionTokens: chunk.Usage.CompletionTokens, ReasoningTokens: chunk.Usage.ReasoningTokens}
	}
	return out, nil
}

// Next 返回增量文本；流结束、`end` 帧或带 finish_reason 的帧都算完成。
func (s *llmTokenStream) Next() (string, bool, error) {
	chunk, err := s.r.Next()
	if err != nil {
		return "", true, err
	}
	if chunk == nil {
		return "", false, nil
	}
	return chunk.Content, chunk.Done || chunk.Type == "end" || chunk.FinishReason != "", nil
}

func (s *llmTokenStream) Close() error { return s.r.Close() }
