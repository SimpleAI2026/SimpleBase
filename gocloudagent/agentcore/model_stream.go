// 文件说明：model_stream.go — Model.Stream：流式请求、回退与帧转换
//
// 请求建立（含 native→text、stream→chat 两级回退）在 Stream；
// 帧状态机（TOOL_CALL 前缀判定、原生工具增量归并）在 streamPump。
// 回退判定依赖原始 *providers.LiteLLMError，这里不得提前归一化错误。

package agentcore

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/voocel/litellm/providers"
	"go.uber.org/zap"
)

func (m *Model) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	options := model.GetCommonOptions(&model.Options{}, opts...)
	tools := mergeTools(m.bound, options.Tools)
	stream, req, err := m.openStream(ctx, input, tools, options)
	if err != nil {
		// 上游鉴权、配额、模型白名单及超时不可绕过重试为 Chat。
		var upstream *providers.LiteLLMError
		if errors.As(err, &upstream) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return m.chatInsteadOfStream(ctx, req, err)
	}

	sr, sw := schema.Pipe[*schema.Message](8)
	pump := &streamPump{m: m, stream: stream, sw: sw}
	go func() {
		defer sw.Close()
		defer stream.Close()
		pump.run()
	}()
	return sr, nil
}

// openStream 建立上游流，处理 native→text 降级：原生工具协议被上游拒绝时，
// 用文本协议重建请求再试一次。返回最终使用的请求，供后续 stream→chat 回退复用。
// 回退判定依赖原始 *providers.LiteLLMError，这里不得提前归一化错误。
func (m *Model) openStream(ctx context.Context, input []*schema.Message, tools []*schema.ToolInfo, options *model.Options) (TokenStream, ChatRequest, error) {
	req := m.buildRequest(input, tools, options, m.useNative())
	stream, err := m.client.Stream(ctx, m.scope, req)
	if err != nil && len(req.Tools) > 0 && m.fallback(err) {
		req = m.buildRequest(input, tools, options, false)
		stream, err = m.client.Stream(ctx, m.scope, req)
	}
	return stream, req, err
}

// chatInsteadOfStream 在流式不可用时退化为一次非流式 Chat，产出单帧流。
// 退化路径没有 thinking 间隔，记 warn 便于定位「首帧一次到齐」。
func (m *Model) chatInsteadOfStream(ctx context.Context, req ChatRequest, streamErr error) (*schema.StreamReader[*schema.Message], error) {
	m.warn("gocloudagent: stream unavailable, falling back to non-stream Chat",
		zap.String("scope", m.scope),
		zap.String("model", m.modelName),
		zap.String("err", streamErr.Error()),
	)
	resp, err := m.client.Chat(ctx, m.scope, req)
	if err != nil {
		return nil, err
	}
	if m.Usage != nil {
		m.Usage(resp.Usage)
	}
	msg, truncated := responseMessage(resp)
	if truncated {
		return singleMessageStream(msg, ErrMaxTokensExceeded), nil
	}
	return singleMessageStream(msg, nil), nil
}

// toolCallMarker 是文本协议的工具调用前缀。
const toolCallMarker = "TOOL_CALL"

// streamPump 把上游增量帧转成 eino 消息流。
//
// 两种工具协议共用一条流：原生 function calling 的分片按 Index 归并并即时转发；
// 文本协议则缓冲到能判定「是普通文本还是 TOOL_CALL 前缀」为止，普通文本一经判定
// 即逐帧转发，保证首 token 不被拖到上游结束。
type streamPump struct {
	m      *Model
	stream TokenStream
	sw     *schema.StreamWriter[*schema.Message]

	pending  strings.Builder
	textMode bool
	toolMode bool
	calls    map[int]*ChatToolCall
}

func (p *streamPump) next() (StreamDelta, error) {
	if ds, ok := p.stream.(DeltaStream); ok {
		return ds.NextDelta()
	}
	content, finish, err := p.stream.Next()
	return StreamDelta{Content: content, Finish: finish}, err
}

func (p *streamPump) run() {
	p.calls = map[int]*ChatToolCall{}
	var streamErr error
	for {
		delta, err := p.next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				streamErr = err
			}
			break
		}
		if delta.Usage != nil && p.m.Usage != nil {
			p.m.Usage(*delta.Usage)
		}
		if delta.ToolCall != nil {
			p.nativeToolCall(delta.ToolCall)
		}
		if delta.Content != "" {
			p.content(delta.Content)
		}
		if delta.Finish {
			break
		}
	}
	if streamErr != nil {
		p.sw.Send(nil, streamErr)
		return
	}
	p.flush()
}

// nativeToolCall 累计一个原生工具分片并立即转发。
func (p *streamPump) nativeToolCall(tc *ToolCallDelta) {
	call := p.calls[tc.Index]
	if call == nil {
		call = &ChatToolCall{}
		p.calls[tc.Index] = call
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
	if tc.Name == "" && tc.ArgsDelta == "" && tc.ID == "" {
		return
	}
	// Index 让 ConcatMessages 把增量收成一次调用。
	// 不带 index 时每个片段都是独立 ToolCall，空 name 的片段会让工具节点报 not found。
	idx := tc.Index
	p.sw.Send(&schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
		Index:    &idx,
		ID:       id,
		Type:     "function",
		Function: schema.FunctionCall{Name: tc.Name, Arguments: tc.ArgsDelta},
		Extra:    map[string]any{"delta": true},
	}}}, nil)
}

// content 处理一段正文增量：已判定为普通文本则直通，否则继续缓冲判定。
func (p *streamPump) content(text string) {
	if p.textMode {
		p.sw.Send(schema.AssistantMessage(text, nil), nil)
		return
	}
	p.pending.WriteString(text)
	trimmed := strings.TrimLeft(p.pending.String(), " \t\r\n")
	switch {
	case !strings.HasPrefix(toolCallMarker, trimmed) && !strings.HasPrefix(trimmed, toolCallMarker):
		p.textMode = true
		p.sw.Send(schema.AssistantMessage(p.pending.String(), nil), nil)
		p.pending.Reset()
	case strings.HasPrefix(trimmed, toolCallMarker):
		p.toolMode = true
	}
}

// flush 在流结束时处理仍缓冲的文本协议内容。
func (p *streamPump) flush() {
	if len(p.calls) != 0 || p.textMode {
		return
	}
	msg := ParseAssistant(p.pending.String())
	if p.toolMode && len(msg.ToolCalls) == 0 || !p.toolMode && msg.Content != "" {
		p.sw.Send(schema.AssistantMessage(p.pending.String(), nil), nil)
	} else if len(msg.ToolCalls) > 0 {
		p.sw.Send(msg, nil)
	}
}
