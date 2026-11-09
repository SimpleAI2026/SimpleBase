// 文件说明：stream.go — gocloudagent/agentcore 中 ConsumeEvent、ConsumeVariant 等符号的实现
//
// 用法
//
//	ConsumeVariant(mv *adk.MessageVariant) (content, name string)  // ConsumeVariant 读工具结果变体：把 Content 拼起来，ToolName 取最后一个非空值
//	IsToolDelta(tc schema.ToolCall) bool  // IsToolDelta 判定一个 ToolCall 是否是「参数增量」而非完整调用
//
// 调用架构
//
//	[上游] gocloudagent, gocloudagent/cli
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/agentcore/stream.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] messages.go, model.go, redact.go, types.go
//
// 主要符号
//
//	type ConsumeEvent struct {…}
//	ConsumeVariant(mv *adk.MessageVariant) (content, name string)
//	IsToolDelta(tc schema.ToolCall) bool
//	MergeStreamToolCall(dst []schema.ToolCall, tc schema.ToolCall) []schema.ToolCall
//	ConsumeAssistant(mv *adk.MessageVariant, emit func(ConsumeEvent), acc ...

package agentcore

import (
	"errors"
	"io"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// ConsumeEvent 是从 ADK 消息变体里读出的传输无关事件。
//
// 云端把它映射成 SSE 的 Event（补 run_id 等字段），CLI 映射成自己的展示事件；
// 名字与云端保持一致（token / tool_call_delta / tool_call），脚本可复用解析（§13.8）。
type ConsumeEvent struct {
	// Type 取值：token / tool_call_delta / tool_call。
	Type string
	// Content 是 token 事件的文本。
	Content string
	// CallID / Name / Arguments 是工具事件的字段。
	CallID    string
	Name      string
	Arguments string
}

// ConsumeVariant 读工具结果变体：把 Content 拼起来，ToolName 取最后一个非空值。
//
// MessageStream 读到任何非 EOF 错误都会停；已读到的正文仍然返回，调用方若要把该错误
// 当失败需自行另存（当前实现把非 EOF 与 EOF 同样 break）。
func ConsumeVariant(mv *adk.MessageVariant) (content, name string) {
	if mv == nil {
		return "", ""
	}
	if mv.IsStreaming && mv.MessageStream != nil {
		for {
			chunk, err := mv.MessageStream.Recv()
			if errors.Is(err, io.EOF) || err != nil {
				break
			}
			if chunk != nil {
				content += chunk.Content
				if chunk.ToolName != "" {
					name = chunk.ToolName
				}
			}
		}
		return content, name
	}
	if mv.Message != nil {
		return mv.Message.Content, mv.Message.ToolName
	}
	return "", mv.ToolName
}

// IsToolDelta 判定一个 ToolCall 是否是「参数增量」而非完整调用。
//
// 判据是 Extra["delta"]（适配器在增量帧上设置）。历史上还用 ID 前缀 "delta:"，
// 那会让 call id 在增量阶段不稳定（§13.2 去掉该 hack）。
func IsToolDelta(tc schema.ToolCall) bool {
	if tc.Extra == nil {
		return false
	}
	v, ok := tc.Extra["delta"].(bool)
	return ok && v
}

// MergeStreamToolCall 按 Index 把增量片段拼进 dst。
//
// 与 ConcatMessages 的差异：按 index 找已有元素，name 只在空时填入，arguments 直接累加。
// 它不检查「同一 index 上 name 冲突」——适配器保证 name 只出现一次。
func MergeStreamToolCall(dst []schema.ToolCall, tc schema.ToolCall) []schema.ToolCall {
	idx := 0
	if tc.Index != nil {
		idx = *tc.Index
	}
	id := strings.TrimPrefix(tc.ID, "delta:")
	for i := range dst {
		di := 0
		if dst[i].Index != nil {
			di = *dst[i].Index
		}
		if di != idx {
			continue
		}
		if dst[i].ID == "" {
			dst[i].ID = id
		}
		if dst[i].Function.Name == "" {
			dst[i].Function.Name = tc.Function.Name
		}
		dst[i].Function.Arguments += tc.Function.Arguments
		return dst
	}
	cp := tc
	cp.ID = id
	cp.Extra = nil
	return append(dst, cp)
}

// ConsumeAssistant 读助手变体：发 token / tool_call_delta 事件，并返回完整消息。
//
// 返回的消息在流式下是「已拼好的工具调用」或「最后一帧」；非流式下是 mv.Message。
// acc 累积正文（调用方持有，用于最终 Content）。
func ConsumeAssistant(mv *adk.MessageVariant, emit func(ConsumeEvent), acc *strings.Builder) (*schema.Message, error) {
	if mv == nil {
		return nil, nil
	}
	if mv.IsStreaming && mv.MessageStream != nil {
		var last *schema.Message
		var merged []schema.ToolCall
		for {
			chunk, err := mv.MessageStream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				if len(merged) > 0 {
					return schema.AssistantMessage("", merged), err
				}
				return last, err
			}
			if chunk == nil {
				continue
			}
			if len(chunk.ToolCalls) > 0 && IsToolDelta(chunk.ToolCalls[0]) {
				tc := chunk.ToolCalls[0]
				if emit != nil {
					emit(ConsumeEvent{
						Type: "tool_call_delta", CallID: strings.TrimPrefix(tc.ID, "delta:"),
						Name: tc.Function.Name, Arguments: tc.Function.Arguments,
					})
				}
				merged = MergeStreamToolCall(merged, tc)
				continue
			}
			last = chunk
			if chunk.Content != "" && len(chunk.ToolCalls) == 0 {
				if acc != nil {
					acc.WriteString(chunk.Content)
				}
				if emit != nil {
					emit(ConsumeEvent{Type: "token", Content: chunk.Content})
				}
			}
		}
		if len(merged) > 0 {
			return schema.AssistantMessage("", merged), nil
		}
		return last, nil
	}
	if mv.Message != nil {
		if mv.Message.Content != "" && len(mv.Message.ToolCalls) == 0 {
			if acc != nil {
				acc.WriteString(mv.Message.Content)
			}
			if emit != nil {
				emit(ConsumeEvent{Type: "token", Content: mv.Message.Content})
			}
		}
		return mv.Message, nil
	}
	return nil, nil
}
