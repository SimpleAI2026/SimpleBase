package cloudagent

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
)

// gatewayChatModel adapts the project LLM gateway to eino's ToolCallingChatModel.
type gatewayChatModel struct {
	client    ChatClient
	projectID string
	modelName string
	protocol  string
	bound     []*schema.ToolInfo
	usage     func(ChatUsage)
}

var unsupportedNativeTools sync.Map // provider|model -> bool

func newGatewayChatModel(client ChatClient, projectID, modelName string) *gatewayChatModel {
	return &gatewayChatModel{client: client, projectID: projectID, modelName: modelName, protocol: "text"}
}

func (m *gatewayChatModel) useNative() bool {
	if m.protocol == "text" {
		return false
	}
	if m.protocol == "native" {
		return true
	}
	_, disabled := unsupportedNativeTools.Load(m.projectID + "|" + m.modelName)
	return !disabled
}

func (m *gatewayChatModel) fallback(err error) bool {
	var upstream *providers.LiteLLMError
	if m.protocol != "auto" || !errors.As(err, &upstream) || upstream.StatusCode != 400 && upstream.StatusCode != 422 ||
		(!strings.Contains(strings.ToLower(err.Error()), "tool") && !strings.Contains(strings.ToLower(err.Error()), "function")) {
		return false
	}
	unsupportedNativeTools.Store(m.projectID+"|"+m.modelName, true)
	return true
}

func (m *gatewayChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
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
	if m.usage != nil {
		m.usage(resp.Usage)
	}
	if len(resp.ToolCalls) > 0 {
		return assistantToolMessage(resp.ToolCalls), nil
	}
	return parseAssistant(resp.Content), nil
}

func (m *gatewayChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
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
		resp, cerr := m.client.Chat(ctx, m.projectID, req)
		if cerr != nil {
			return nil, cerr
		}
		if m.usage != nil {
			m.usage(resp.Usage)
		}
		if len(resp.ToolCalls) > 0 {
			return singleMessageStream(assistantToolMessage(resp.ToolCalls)), nil
		}
		return singleMessageStream(parseAssistant(resp.Content)), nil
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
			if delta.Usage != nil && m.usage != nil {
				m.usage(*delta.Usage)
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
			msg := parseAssistant(pending.String())
			if toolMode && len(msg.ToolCalls) == 0 || !toolMode && msg.Content != "" {
				sw.Send(schema.AssistantMessage(pending.String(), nil), nil)
			} else if len(msg.ToolCalls) > 0 {
				sw.Send(msg, nil)
			}
		}
	}()
	return sr, nil
}

func (m *gatewayChatModel) BindTools(tools []*schema.ToolInfo) error {
	m.bound = tools
	return nil
}

func (m *gatewayChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	cp := *m
	cp.bound = tools
	return &cp, nil
}

func (m *gatewayChatModel) buildRequest(input []*schema.Message, tools []*schema.ToolInfo, options *model.Options, forceText ...bool) (ChatRequest, error) {
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

func parseAssistant(content string) *schema.Message {
	trimmed := strings.TrimSpace(content)
	if name, args, ok := extractToolCall(trimmed); ok {
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

func looksLikeToolCall(s string) bool {
	return strings.Contains(s, "TOOL_CALL")
}

func extractToolCall(s string) (name, args string, ok bool) {
	idx := strings.Index(s, "TOOL_CALL")
	if idx < 0 {
		return "", "", false
	}
	rest := strings.TrimSpace(s[idx+len("TOOL_CALL"):])
	var payload struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(rest), &payload); err != nil {
		// Try first JSON object in the remainder.
		start := strings.Index(rest, "{")
		end := strings.LastIndex(rest, "}")
		if start < 0 || end <= start {
			return "", "", false
		}
		if err := json.Unmarshal([]byte(rest[start:end+1]), &payload); err != nil {
			return "", "", false
		}
	}
	if payload.Name == "" {
		return "", "", false
	}
	arg := "{}"
	if len(payload.Arguments) > 0 {
		arg = string(payload.Arguments)
	}
	return payload.Name, arg, true
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

var _ model.ToolCallingChatModel = (*gatewayChatModel)(nil)
var _ model.ChatModel = (*gatewayChatModel)(nil)
