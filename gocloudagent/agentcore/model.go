// 文件说明：model.go — gocloudagent/agentcore 中 Model、ResetUnsupportedNativeTools 等符号的实现
//
// 用法
//
//	NewModel(client ChatClient, projectID, modelName string) *Model
//	ResetUnsupportedNativeTools()  // ResetUnsupportedNativeTools 清空 auto 协议降级缓存，仅测试使用：
//	(m *Model) Generate(ctx context.Context, input []*schema.Message, opts ...
//
// 调用架构
//
//	[上游] gocloudagent, gocloudagent/cli
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/agentcore/model.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] messages.go, redact.go, stream.go, types.go
//
// 主要符号
//
//	type Model struct {…}
//	ResetUnsupportedNativeTools()
//	NewModel(client ChatClient, projectID, modelName string) *Model
//	(m *Model) Generate(ctx context.Context, input []*schema.Message, opts ...
//	(m *Model) Stream(ctx context.Context, input []*schema.Message, opts ...
//	(m *Model) BindTools(tools []*schema.ToolInfo) error
//	... 另有 17 个顶层符号

package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/voocel/litellm/providers"
	"go.uber.org/zap"
)

// Model adapts the project LLM gateway to eino's ToolCallingChatModel.
type Model struct {
	client    ChatClient
	projectID string
	modelName string
	// Protocol 为 text / native / auto；空值按 text 处理。
	Protocol string
	bound    []*schema.ToolInfo
	// Usage 每次拿到用量时回调；可为 nil。
	Usage func(ChatUsage)
	// Warn 是降级路径的日志出口；nil 即 no-op。
	Warn func(msg string, fields ...zap.Field)
}

var unsupportedNativeTools sync.Map // provider|model -> bool

// ResetUnsupportedNativeTools 清空 auto 协议降级缓存，仅测试使用：
// 缓存以 projectID|model 为键全局持久，同一测试二进制内多次运行
// （-count>1 或跨用例共用项目）会因缓存命中而跳过降级路径，造成剧本错位抖动。
func ResetUnsupportedNativeTools() {
	unsupportedNativeTools = sync.Map{}
}

func (m *Model) warn(msg string, fields ...zap.Field) {
	if m.Warn != nil {
		m.Warn(msg, fields...)
	}
}

// NewModel 创建网关适配器，默认文本工具协议。
func NewModel(client ChatClient, projectID, modelName string) *Model {
	return &Model{client: client, projectID: projectID, modelName: modelName, Protocol: "text"}
}

func (m *Model) useNative() bool {
	if m.Protocol == "text" {
		return false
	}
	if m.Protocol == "native" {
		return true
	}
	_, disabled := unsupportedNativeTools.Load(m.projectID + "|" + m.modelName)
	return !disabled
}

func (m *Model) fallback(err error) bool {
	var upstream *providers.LiteLLMError
	if m.Protocol != "auto" || !errors.As(err, &upstream) || upstream.StatusCode != 400 && upstream.StatusCode != 422 ||
		(!strings.Contains(strings.ToLower(err.Error()), "tool") && !strings.Contains(strings.ToLower(err.Error()), "function")) {
		return false
	}
	unsupportedNativeTools.Store(m.projectID+"|"+m.modelName, true)
	m.warn("gocloudagent: fall back to text tool protocol",
		zap.String("project", m.projectID),
		zap.String("model", m.modelName),
		zap.String("err", err.Error()),
	)
	return true
}

func (m *Model) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	options := model.GetCommonOptions(&model.Options{}, opts...)
	tools := mergeTools(m.bound, options.Tools)
	req, err := m.buildRequest(input, tools, options)
	if err != nil {
		return nil, err
	}
	resp, err := m.client.Chat(ctx, m.projectID, req)
	if err != nil && len(req.Tools) > 0 && m.fallback(err) {
		req, err = m.buildRequest(input, tools, options, false)
		if err == nil {
			resp, err = m.client.Chat(ctx, m.projectID, req)
		}
	}
	if err != nil {
		return nil, err
	}
	if m.Usage != nil {
		m.Usage(resp.Usage)
	}
	// planv6.1 O2-5: 检测 max_tokens 截断。finish_reason='length' 表示输出不完整，
	// 但内容本身有效（不同于调用失败）。返回专用错误，上层追加提示。
	if resp.FinishReason == "length" {
		msg := ParseAssistant(resp.Content)
		if len(resp.ToolCalls) > 0 {
			msg = assistantToolMessage(resp.ToolCalls)
		}
		return msg, ErrMaxTokensExceeded
	}
	if len(resp.ToolCalls) > 0 {
		return assistantToolMessage(resp.ToolCalls), nil
	}
	return ParseAssistant(resp.Content), nil
}

func (m *Model) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	options := model.GetCommonOptions(&model.Options{}, opts...)
	tools := mergeTools(m.bound, options.Tools)
	req, err := m.buildRequest(input, tools, options)
	if err != nil {
		return nil, err
	}
	stream, err := m.client.Stream(ctx, m.projectID, req)
	if err != nil && len(req.Tools) > 0 && m.fallback(err) {
		req, err = m.buildRequest(input, tools, options, false)
		if err == nil {
			stream, err = m.client.Stream(ctx, m.projectID, req)
		}
	}
	if err != nil {
		// 上游鉴权、配额、模型白名单及超时不可绕过重试为 Chat。
		var upstream *providers.LiteLLMError
		if errors.As(err, &upstream) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		// 退化路径会产出单帧流（无 thinking 间隔），记 warn 便于定位「首帧一次到齐」。
		m.warn("gocloudagent: stream unavailable, falling back to non-stream Chat",
			zap.String("project", m.projectID),
			zap.String("model", m.modelName),
			zap.String("err", err.Error()),
		)
		resp, cerr := m.client.Chat(ctx, m.projectID, req)
		if cerr != nil {
			return nil, cerr
		}
		if m.Usage != nil {
			m.Usage(resp.Usage)
		}
		if len(resp.ToolCalls) > 0 {
			return singleMessageStream(assistantToolMessage(resp.ToolCalls)), nil
		}
		return singleMessageStream(ParseAssistant(resp.Content)), nil
	}

	sr, sw := schema.Pipe[*schema.Message](8)
	go func() {
		defer sw.Close()
		defer stream.Close()
		const marker = "TOOL_CALL"
		var pending strings.Builder
		textMode, toolMode := false, false
		calls := map[int]*ChatToolCall{}
		var streamErr error
		next := func() (StreamDelta, error) {
			if ds, ok := stream.(DeltaStream); ok {
				return ds.NextDelta()
			}
			content, finish, err := stream.Next()
			return StreamDelta{Content: content, Finish: finish}, err
		}
		for {
			delta, nerr := next()
			if nerr != nil {
				if !errors.Is(nerr, io.EOF) {
					streamErr = nerr
				}
				break
			}
			if delta.Usage != nil && m.Usage != nil {
				m.Usage(*delta.Usage)
			}
			if tc := delta.ToolCall; tc != nil {
				call := calls[tc.Index]
				if call == nil {
					call = &ChatToolCall{}
					calls[tc.Index] = call
				}
				if tc.ID != "" {
					call.ID = tc.ID
				}
				call.Name += tc.Name
				call.Arguments += tc.ArgsDelta
				id := call.ID
				if id == "" {
					id = "pending"
				}
				if tc.Name != "" || tc.ArgsDelta != "" || tc.ID != "" {
					// Index 让 ConcatMessages 把增量收成一次调用。
					// 不带 index 时每个片段都是独立 ToolCall，空 name 的片段会让工具节点报 not found。
					idx := tc.Index
					sw.Send(&schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
						Index:    &idx,
						ID:       id,
						Type:     "function",
						Function: schema.FunctionCall{Name: tc.Name, Arguments: tc.ArgsDelta},
						Extra:    map[string]any{"delta": true},
					}}}, nil)
				}
			}
			if delta.Content != "" {
				if textMode {
					sw.Send(schema.AssistantMessage(delta.Content, nil), nil)
				} else {
					pending.WriteString(delta.Content)
					trimmed := strings.TrimLeft(pending.String(), " \t\r\n")
					if !strings.HasPrefix(marker, trimmed) && !strings.HasPrefix(trimmed, marker) {
						textMode = true
						sw.Send(schema.AssistantMessage(pending.String(), nil), nil)
						pending.Reset()
					} else if strings.HasPrefix(trimmed, marker) {
						toolMode = true
					}
				}
			}
			if delta.Finish {
				break
			}
		}
		if streamErr != nil {
			sw.Send(nil, streamErr)
			return
		}
		if len(calls) == 0 && !textMode {
			msg := ParseAssistant(pending.String())
			if toolMode && len(msg.ToolCalls) == 0 || !toolMode && msg.Content != "" {
				sw.Send(schema.AssistantMessage(pending.String(), nil), nil)
			} else if len(msg.ToolCalls) > 0 {
				sw.Send(msg, nil)
			}
		}
	}()
	return sr, nil
}

func (m *Model) BindTools(tools []*schema.ToolInfo) error {
	m.bound = tools
	return nil
}

func (m *Model) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	cp := *m
	cp.bound = tools
	return &cp, nil
}

func (m *Model) buildRequest(input []*schema.Message, tools []*schema.ToolInfo, options *model.Options, forceText ...bool) (ChatRequest, error) {
	native := m.useNative() && (len(forceText) == 0 || forceText[0])
	msgs := make([]ChatMessage, 0, len(input)+1)
	if len(tools) > 0 && !native {
		msgs = append(msgs, ChatMessage{Role: "system", Content: toolProtocolPrompt(tools)})
	}
	for _, in := range input {
		if in == nil {
			continue
		}
		if native {
			msg := ChatMessage{Role: string(in.Role), Content: in.Content, ToolCallID: in.ToolCallID}
			for _, tc := range in.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, ChatToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
			}
			msgs = append(msgs, msg)
		} else {
			msgs = append(msgs, schemaToChat(in)...)
		}
	}
	modelName := m.modelName
	if options.Model != nil && *options.Model != "" {
		modelName = *options.Model
	}
	req := ChatRequest{Model: modelName, Messages: msgs}
	if native {
		for _, t := range tools {
			if t == nil {
				continue
			}
			spec := ToolSpec{Name: t.Name, Description: t.Desc}
			if t.ParamsOneOf != nil {
				if js, err := t.ParamsOneOf.ToJSONSchema(); err == nil && js != nil {
					spec.Parameters, _ = json.Marshal(js)
				}
			}
			req.Tools = append(req.Tools, spec)
		}
	}
	if options.MaxTokens != nil {
		req.MaxTokens = options.MaxTokens
	}
	if options.Temperature != nil {
		t := float64(*options.Temperature)
		req.Temperature = &t
	}
	return req, nil
}

func schemaToChat(m *schema.Message) []ChatMessage {
	switch m.Role {
	case schema.Tool:
		content := m.Content
		if m.ToolName != "" {
			content = "TOOL_RESULT name=" + m.ToolName + " id=" + m.ToolCallID + "\n" + content
		}
		return []ChatMessage{{Role: "user", Content: content}}
	case schema.Assistant:
		if len(m.ToolCalls) > 0 {
			b, _ := json.Marshal(m.ToolCalls)
			return []ChatMessage{{Role: "assistant", Content: "TOOL_CALL " + string(b)}}
		}
		return []ChatMessage{{Role: "assistant", Content: m.Content}}
	case schema.System:
		return []ChatMessage{{Role: "system", Content: m.Content}}
	default:
		return []ChatMessage{{Role: "user", Content: m.Content}}
	}
}

func toolProtocolPrompt(tools []*schema.ToolInfo) string {
	type spec struct {
		Name   string          `json:"name"`
		Desc   string          `json:"description"`
		Params json.RawMessage `json:"parameters,omitempty"`
	}
	list := make([]spec, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		item := spec{Name: t.Name, Desc: t.Desc}
		if t.ParamsOneOf != nil {
			if js, err := t.ParamsOneOf.ToJSONSchema(); err == nil && js != nil {
				if b, err := json.Marshal(js); err == nil {
					item.Params = b
				}
			}
		}
		list = append(list, item)
	}
	body, _ := json.Marshal(list)
	return "Available tools (JSON):\n" + string(body) + "\n\n" +
		"To call a tool, reply with a single line and nothing else:\n" +
		`TOOL_CALL {"name":"<tool>","arguments":{...}}` + "\n" +
		"After you receive TOOL_RESULT, continue or give the final answer in plain text. Never include secrets."
}

func assistantToolMessage(calls []ChatToolCall) *schema.Message {
	out := schema.AssistantMessage("", nil)
	for _, tc := range calls {
		id := tc.ID
		if id == "" {
			id = "call_" + uuid.NewString()
		}
		args := tc.Arguments
		if args == "" {
			args = "{}"
		}
		out.ToolCalls = append(out.ToolCalls, schema.ToolCall{ID: id, Type: "function", Function: schema.FunctionCall{Name: tc.Name, Arguments: args}})
	}
	return out
}

func ParseAssistant(content string) *schema.Message {
	trimmed := strings.TrimSpace(content)
	if name, args, ok := ExtractToolCall(trimmed); ok {
		return schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call_" + uuid.NewString(),
			Type: "function",
			Function: schema.FunctionCall{
				Name:      name,
				Arguments: args,
			},
		}})
	}
	return schema.AssistantMessage(content, nil)
}

func ExtractToolCall(s string) (name, args string, ok bool) {
	idx := strings.Index(s, "TOOL_CALL")
	if idx < 0 {
		return "", "", false
	}
	rest := strings.TrimSpace(s[idx+len("TOOL_CALL"):])
	raw, ok := decodeToolCallObject(rest)
	if !ok {
		// 前面夹了噪声时，从第一个 { 起解第一个完整对象。
		start := strings.Index(rest, "{")
		if start < 0 {
			return "", "", false
		}
		raw, ok = decodeToolCallObject(rest[start:])
		if !ok {
			return "", "", false
		}
	}
	var name0 string
	if v, has := raw["name"]; has {
		_ = json.Unmarshal(v, &name0)
	}
	if name0 == "" {
		return "", "", false
	}
	if v, has := raw["arguments"]; has && len(v) > 0 && string(v) != "null" {
		return name0, string(v), true
	}
	return name0, flattenToolCallArgs(raw), true
}

// decodeToolCallObject 解出 s 开头的第一个完整 JSON 对象为「键 → 原始值」，其后的内容忽略。
//
// 用 Decoder 而不是整段 Unmarshal：模型常在一条消息里连写多个 TOOL_CALL，或在
// 调用后追加说明文字、空代码块。整段 Unmarshal 会因为"尾部多余内容"整体失败，
// 于是一个本来完整的调用被当成普通文本，整轮在这里提前结束。
// 只取第一个对象，也符合协议"一次回复只发一行调用"的约定。
func decodeToolCallObject(s string) (map[string]json.RawMessage, bool) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(strings.NewReader(s)).Decode(&raw); err != nil || raw == nil {
		return nil, false
	}
	return raw, true
}

// flattenToolCallArgs 在模型漏写 "arguments" 包装时兜底：把 name 以外的顶层字段
// 当作参数，即 {"name":"shell","command":"ls"} 按 {"command":"ls"} 处理。
//
// 这是对模型格式偏差的容错，不是第二种协议：只在 arguments 缺失时才生效，
// 所以格式正确的调用（含 arguments 为空对象）完全不受影响。没有任何多余字段时
// 返回 "{}"，与「无参工具」的既有行为一致。
func flattenToolCallArgs(raw map[string]json.RawMessage) string {
	extra := make(map[string]json.RawMessage, len(raw))
	for k, v := range raw {
		if k == "name" || k == "arguments" {
			continue
		}
		extra[k] = v
	}
	if len(extra) == 0 {
		return "{}"
	}
	b, err := json.Marshal(extra)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func mergeTools(bound, opt []*schema.ToolInfo) []*schema.ToolInfo {
	if len(opt) > 0 {
		return opt
	}
	return bound
}

func singleMessageStream(msg *schema.Message) *schema.StreamReader[*schema.Message] {
	sr, sw := schema.Pipe[*schema.Message](1)
	sw.Send(msg, nil)
	sw.Close()
	return sr
}

var _ model.ToolCallingChatModel = (*Model)(nil)
var _ model.ChatModel = (*Model)(nil)
