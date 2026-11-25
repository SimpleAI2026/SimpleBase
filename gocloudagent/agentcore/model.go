// 文件说明：model.go — gocloudagent/agentcore 中 Model、ResetUnsupportedNativeTools 等符号的实现
//
// 用法
//
//	NewModel(client ChatClient, scope, modelName string) *Model
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
//	[同包] messages.go, model_stream.go, redact.go, stream.go, types.go
//
// 主要符号
//
//	type Model struct {…}
//	ResetUnsupportedNativeTools()
//	NewModel(client ChatClient, scope, modelName string) *Model
//	(m *Model) Generate(ctx context.Context, input []*schema.Message, opts ...
//	(m *Model) Stream(ctx context.Context, input []*schema.Message, opts ...
//	(m *Model) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error)
//	... 另有若干私有符号（请求构造见本文件，流式见 model_stream.go）

package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/voocel/litellm/providers"
	"go.uber.org/zap"
)

// Model adapts a ChatClient to eino's ToolCallingChatModel.
type Model struct {
	client    ChatClient
	scope     string
	modelName string
	// Protocol 为 text / native / auto；空值语义未定义：实际走 native 分支
	// （受同 scope|model 的降级缓存影响）且不参与 auto 回退。
	// 请经 NewModel 构造（默认 text）或显式赋值，不要依赖零值。
	Protocol string
	bound    []*schema.ToolInfo
	// Usage 每次拿到用量时回调；可为 nil。
	Usage func(ChatUsage)
	// Warn 是降级路径的日志出口；nil 即 no-op。
	Warn func(msg string, fields ...zap.Field)
}

var unsupportedNativeTools sync.Map // provider|model -> bool

// ResetUnsupportedNativeTools 清空 auto 协议降级缓存，仅测试使用：
// 缓存以 scope|model 为键全局持久，同一测试二进制内多次运行
// （-count>1 或跨用例共用项目）会因缓存命中而跳过降级路径，造成剧本错位抖动。
// 并发契约：逐键删除，与并发 useNative/fallback 安全交错；重置进行中的
// Load 可能读到旧值，属测试可接受语义。禁止改为整体重赋值（sync.Map 赋值
// 不是原子的，与并发读写构成数据竞争）。
func ResetUnsupportedNativeTools() {
	unsupportedNativeTools.Range(func(key, _ any) bool {
		unsupportedNativeTools.Delete(key)
		return true
	})
}

func (m *Model) warn(msg string, fields ...zap.Field) {
	if m.Warn != nil {
		m.Warn(msg, fields...)
	}
}

// NewModel 创建网关适配器，默认文本工具协议。
func NewModel(client ChatClient, scope, modelName string) *Model {
	return &Model{client: client, scope: scope, modelName: modelName, Protocol: "text"}
}

func (m *Model) useNative() bool {
	if m.Protocol == "text" {
		return false
	}
	if m.Protocol == "native" {
		return true
	}
	_, disabled := unsupportedNativeTools.Load(m.scope + "|" + m.modelName)
	return !disabled
}

func (m *Model) fallback(err error) bool {
	if m.Protocol != "auto" || !ToolsUnsupported(err) {
		return false
	}
	unsupportedNativeTools.Store(m.scope+"|"+m.modelName, true)
	m.warn("gocloudagent: fall back to text tool protocol",
		zap.String("scope", m.scope),
		zap.String("model", m.modelName),
		zap.String("err", err.Error()),
	)
	return true
}

// ToolsUnsupported 判定一次 Chat 错误是否为「模型不支持 tools / function calling」：
// 400/422 且文案含 tool 或 function。Model 的 native→text 回退与 plan.Planner 的
// 文本协议回退共用这一份判定，不各自复制。
func ToolsUnsupported(err error) bool {
	var upstream *providers.LiteLLMError
	if !errors.As(err, &upstream) || upstream.StatusCode != 400 && upstream.StatusCode != 422 {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "tool") || strings.Contains(lower, "function")
}

func (m *Model) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	options := model.GetCommonOptions(&model.Options{}, opts...)
	tools := mergeTools(m.bound, options.Tools)
	req := m.buildRequest(input, tools, options, m.useNative())
	resp, err := m.client.Chat(ctx, m.scope, req)
	if err != nil && len(req.Tools) > 0 && m.fallback(err) {
		resp, err = m.client.Chat(ctx, m.scope, m.buildRequest(input, tools, options, false))
	}
	if err != nil {
		return nil, err
	}
	if m.Usage != nil {
		m.Usage(resp.Usage)
	}
	msg, truncated := responseMessage(resp)
	if truncated {
		return msg, ErrMaxTokensExceeded
	}
	return msg, nil
}

// responseMessage 把一次非流式响应转成 assistant 消息；truncated 表示输出被 max_tokens
// 截断（finish_reason='length'）：内容本身有效（不同于调用失败），上层据此追加提示。
// Generate 与「流式退化为 Chat」共用，保证两条路径对截断的判定一致。
func responseMessage(resp ChatResponse) (msg *schema.Message, truncated bool) {
	if len(resp.ToolCalls) > 0 {
		msg = assistantToolMessage(resp.ToolCalls)
	} else {
		msg = ParseAssistant(resp.Content)
	}
	return msg, resp.FinishReason == "length"
}

// WithTools 返回绑定了 tools 的新实例（不改接收者，可安全跨 goroutine 共享基实例）。
// eino agent 通过 model.WithTools 选项下发工具，见 adk.ChatModelAgentConfig.Model。
func (m *Model) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	cp := *m
	cp.bound = tools
	return &cp, nil
}

// buildRequest 构造一次模型请求。native 为 true 时走原生 function calling，
// 否则把工具协议写进 system 提示词（文本协议）。
func (m *Model) buildRequest(input []*schema.Message, tools []*schema.ToolInfo, options *model.Options, native bool) ChatRequest {
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
			msgs = append(msgs, schemaToChat(in))
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
			req.Tools = append(req.Tools, ToolSpec{Name: t.Name, Description: t.Desc, Parameters: toolParameters(t)})
		}
	}
	if options.MaxTokens != nil {
		req.MaxTokens = options.MaxTokens
	}
	if options.Temperature != nil {
		t := float64(*options.Temperature)
		req.Temperature = &t
	}
	return req
}

// toolParameters 把工具的参数 schema 编码为 JSON；无 schema 或编码失败时返回 nil
// （参数被省略，与历史行为一致，不向上报错）。
func toolParameters(t *schema.ToolInfo) json.RawMessage {
	if t.ParamsOneOf == nil {
		return nil
	}
	js, err := t.ParamsOneOf.ToJSONSchema()
	if err != nil || js == nil {
		return nil
	}
	b, err := json.Marshal(js)
	if err != nil {
		return nil
	}
	return b
}

func schemaToChat(m *schema.Message) ChatMessage {
	switch m.Role {
	case schema.Tool:
		content := m.Content
		if m.ToolName != "" {
			content = "TOOL_RESULT name=" + m.ToolName + " id=" + m.ToolCallID + "\n" + content
		}
		return ChatMessage{Role: "user", Content: content}
	case schema.Assistant:
		if len(m.ToolCalls) > 0 {
			b, _ := json.Marshal(m.ToolCalls)
			return ChatMessage{Role: "assistant", Content: "TOOL_CALL " + string(b)}
		}
		return ChatMessage{Role: "assistant", Content: m.Content}
	case schema.System:
		return ChatMessage{Role: "system", Content: m.Content}
	default:
		return ChatMessage{Role: "user", Content: m.Content}
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
		list = append(list, spec{Name: t.Name, Desc: t.Desc, Params: toolParameters(t)})
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

// singleMessageStream 产出只有一帧的流；finalErr 非 nil 时在这一帧之后作为流的终止错误送出。
func singleMessageStream(msg *schema.Message, finalErr error) *schema.StreamReader[*schema.Message] {
	sr, sw := schema.Pipe[*schema.Message](2)
	sw.Send(msg, nil)
	if finalErr != nil {
		sw.Send(nil, finalErr)
	}
	sw.Close()
	return sr
}

var _ model.ToolCallingChatModel = (*Model)(nil)
