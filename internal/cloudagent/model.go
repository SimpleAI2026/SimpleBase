package cloudagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

// gatewayChatModel adapts the project LLM gateway to eino's ToolCallingChatModel.
// Native provider tool-calling is not assumed; tools are described in-prompt and
// parsed from a TOOL_CALL JSON line when the model requests a tool.
type gatewayChatModel struct {
	client    ChatClient
	projectID string
	modelName string
	bound     []*schema.ToolInfo
}

func newGatewayChatModel(client ChatClient, projectID, modelName string) *gatewayChatModel {
	return &gatewayChatModel{client: client, projectID: projectID, modelName: modelName}
}

func (m *gatewayChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	options := model.GetCommonOptions(&model.Options{}, opts...)
	tools := mergeTools(m.bound, options.Tools)
	req, err := m.buildRequest(input, tools, options)
	if err != nil {
		return nil, err
	}
	resp, err := m.client.Chat(ctx, m.projectID, req)
	if err != nil {
		return nil, err
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
	if err != nil {
		// Gateway may not support stream; fall back to Chat and emit one chunk.
		resp, cerr := m.client.Chat(ctx, m.projectID, req)
		if cerr != nil {
			return nil, err
		}
		return singleMessageStream(parseAssistant(resp.Content)), nil
	}

	sr, sw := schema.Pipe[*schema.Message](8)
	go func() {
		defer sw.Close()
		defer stream.Close()
		var buf strings.Builder
		for {
			chunk, finish, nerr := stream.Next()
			if nerr != nil {
				if !errors.Is(nerr, io.EOF) {
					sw.Send(nil, nerr)
				}
				break
			}
			if chunk != "" {
				buf.WriteString(chunk)
			}
			if finish {
				break
			}
		}
		msg := parseAssistant(buf.String())
		if len(msg.ToolCalls) > 0 || msg.Content == "" {
			sw.Send(msg, nil)
			return
		}
		runes := []rune(msg.Content)
		for i := 0; i < len(runes); i += 16 {
			end := i + 16
			if end > len(runes) {
				end = len(runes)
			}
			sw.Send(&schema.Message{Role: schema.Assistant, Content: string(runes[i:end])}, nil)
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

func (m *gatewayChatModel) buildRequest(input []*schema.Message, tools []*schema.ToolInfo, options *model.Options) (ChatRequest, error) {
	msgs := make([]ChatMessage, 0, len(input)+1)
	if len(tools) > 0 {
		msgs = append(msgs, ChatMessage{Role: "system", Content: toolProtocolPrompt(tools)})
	}
	for _, in := range input {
		if in == nil {
			continue
		}
		msgs = append(msgs, schemaToChat(in)...)
	}
	modelName := m.modelName
	if options.Model != nil && *options.Model != "" {
		modelName = *options.Model
	}
	req := ChatRequest{Model: modelName, Messages: msgs}
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
