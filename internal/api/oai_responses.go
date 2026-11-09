// 文件说明：oai_responses.go — OpenAI Responses API 兼容出口：
//
// 用法
//
//	(h *LLMHandler) Responses(c echo.Context) error  // Responses 处理 POST .../v1/responses
//	type oaiRespRequest（包内使用）
//	type oaiRespTextConfig（包内使用）
//	同包调用：object() → adapter.go, api_key_policy.go 等；text() → agent_handler.go, collection_filter.go 等；start() → cronjob_handler.go, e2b_process_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/oai_responses.go
//	         │ 依赖
//	         ▼
//	[下游] internal/agentbridge
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] object() ← adapter.go, api_key_policy.go, collection_pick.go
//
// 主要符号
//
//	(h *LLMHandler) Responses(c echo.Context) error
//	... 另有 50 个顶层符号

package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
)

const (
	respObject           = "response"
	respStatusCompleted  = "completed"
	respStatusIncomplete = "incomplete"
	respStatusInProgress = "in_progress"
	respStatusFailed     = "failed"
)

// ---------------------------------------------------------------------------
// 请求
// ---------------------------------------------------------------------------

// oaiRespRequest 是 POST .../v1/responses 的请求体。
type oaiRespRequest struct {
	Model           string          `json:"model"`
	Input           json.RawMessage `json:"input"`
	Instructions    string          `json:"instructions,omitempty"`
	Tools           []oaiRespTool   `json:"tools,omitempty"`
	ToolChoice      json.RawMessage `json:"tool_choice,omitempty"`
	MaxOutputTokens *int            `json:"max_output_tokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	Stream          bool            `json:"stream,omitempty"`

	// 以下字段只为拒绝不支持的用法而解析。
	PreviousResponseID string             `json:"previous_response_id,omitempty"`
	Background         bool               `json:"background,omitempty"`
	Conversation       json.RawMessage    `json:"conversation,omitempty"`
	Text               *oaiRespTextConfig `json:"text,omitempty"`
}

type oaiRespTextConfig struct {
	Format *struct {
		Type string `json:"type"`
	} `json:"format"`
}

// oaiRespTool 是 Responses 的工具声明。与 chat 不同，它是平铺的：
// {type, name, description, parameters}，没有嵌套的 function 对象。
type oaiRespTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// oaiRespItem 是 input 数组里的一个条目；按 Type 取用不同字段。
type oaiRespItem struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	// function_call
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	// function_call_output
	Output json.RawMessage `json:"output"`
}

// toLLMRequest 校验并转换为既有 LLMRequest。返回的 string 非空表示请求不合法，
// 内容会进入 400 错误信封，因此不得包含密钥或对话正文。
func (r oaiRespRequest) toLLMRequest() (LLMRequest, string) {
	if strings.TrimSpace(r.PreviousResponseID) != "" {
		return LLMRequest{}, "previous_response_id is not supported: this endpoint is stateless, resend the full conversation in input"
	}
	if r.Background {
		return LLMRequest{}, "background mode is not supported"
	}
	if isPresentJSON(r.Conversation) {
		return LLMRequest{}, "conversation is not supported: this endpoint is stateless, resend the full conversation in input"
	}
	if r.Text != nil && r.Text.Format != nil && r.Text.Format.Type != "" && r.Text.Format.Type != "text" {
		return LLMRequest{}, fmt.Sprintf("text.format.type %q is not supported: structured outputs are unavailable", r.Text.Format.Type)
	}

	out := LLMRequest{Model: r.Model, MaxTokens: r.MaxOutputTokens, Temperature: r.Temperature}
	for i, t := range r.Tools {
		if t.Type != "function" {
			// 内置工具（web_search / file_search / code_interpreter / mcp ...）由 OpenAI
			// 自己执行，本服务无法代跑，明确拒绝而不是静默忽略。
			return LLMRequest{}, fmt.Sprintf("tools[%d].type must be \"function\", got %q", i, t.Type)
		}
		if strings.TrimSpace(t.Name) == "" {
			return LLMRequest{}, fmt.Sprintf("tools[%d].name must not be empty", i)
		}
		out.Tools = append(out.Tools, LLMTool{Name: t.Name, Description: t.Description, Parameters: t.Parameters})
	}

	choice, msg := respToolChoice(r.ToolChoice)
	if msg != "" {
		return LLMRequest{}, msg
	}
	if msg := validateToolChoice(choice, len(out.Tools)); msg != "" {
		return LLMRequest{}, msg
	}
	out.ToolChoice = choice

	if r.Instructions != "" {
		out.Messages = append(out.Messages, LLMMessage{Role: "system", Content: r.Instructions})
	}
	msgs, msg := respInputMessages(r.Input)
	if msg != "" {
		return LLMRequest{}, msg
	}
	out.Messages = append(out.Messages, msgs...)
	if len(out.Messages) == 0 {
		return LLMRequest{}, "input must not be empty"
	}
	return out, ""
}

// respToolChoice 把 Responses 的 tool_choice 转成 chat 的形状，供 validateToolChoice 复用：
//
//	"auto" | "none" | "required"                        → 原样
//	{"type":"function","name":"x"}                       → {"type":"function","function":{"name":"x"}}
//	{"type":"function","function":{"name":"x"}}（chat 形状）→ 原样接受
//
// 其它对象（allowed_tools、内置工具选择等）返回错误。
func respToolChoice(raw json.RawMessage) (json.RawMessage, string) {
	if !isPresentJSON(raw) {
		return nil, ""
	}
	trimmed := bytes.TrimSpace(raw)
	if trimmed[0] != '{' {
		return raw, "" // 字符串或其它类型，交给 validateToolChoice 判定
	}
	var obj struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, "tool_choice must be a string or an object"
	}
	if obj.Type == "function" && strings.TrimSpace(obj.Function.Name) != "" {
		return raw, ""
	}
	if obj.Type == "function" && strings.TrimSpace(obj.Name) != "" {
		converted, _ := json.Marshal(map[string]any{
			"type": "function", "function": map[string]any{"name": obj.Name},
		})
		return converted, ""
	}
	return nil, "tool_choice object must be {\"type\":\"function\",\"name\":\"...\"}"
}

// respInputMessages 把 input 转成 chat 消息。
func respInputMessages(raw json.RawMessage) ([]LLMMessage, string) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, "input is required"
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, "input must be a string or an array of items"
		}
		return []LLMMessage{{Role: "user", Content: s}}, ""
	case '[':
		var items []oaiRespItem
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, "input must be a string or an array of items"
		}
		return respItemsToMessages(items)
	default:
		return nil, "input must be a string or an array of items"
	}
}

func respItemsToMessages(items []oaiRespItem) ([]LLMMessage, string) {
	var msgs []LLMMessage
	for i, it := range items {
		typ := it.Type
		if typ == "" && it.Role != "" {
			typ = "message" // 简写形式 {"role":"user","content":"..."}
		}
		switch typ {
		case "message":
			role := it.Role
			if role == "developer" {
				role = "system" // chat 协议里 developer 的对应角色
			}
			switch role {
			case "system", "user", "assistant":
			default:
				return nil, fmt.Sprintf("input[%d].role %q is not supported", i, it.Role)
			}
			text, msg := respContentText(it.Content)
			if msg != "" {
				return nil, fmt.Sprintf("input[%d]: %s", i, msg)
			}
			msgs = append(msgs, LLMMessage{Role: role, Content: text})
		case "function_call":
			if strings.TrimSpace(it.CallID) == "" || strings.TrimSpace(it.Name) == "" {
				return nil, fmt.Sprintf("input[%d]: function_call requires call_id and name", i)
			}
			args := it.Arguments
			if strings.TrimSpace(args) == "" {
				args = "{}" // 部分上游拒绝空 arguments
			}
			call := LLMToolCall{ID: it.CallID, Name: it.Name, Arguments: args}
			// 并行工具调用：紧邻的 function_call 合并进同一条 assistant 消息，
			// 否则 chat 协议里会出现「assistant(tool_calls) 后缺少对应 tool 消息」的非法序列。
			if n := len(msgs); n > 0 && msgs[n-1].Role == "assistant" && msgs[n-1].ToolCallID == "" {
				msgs[n-1].ToolCalls = append(msgs[n-1].ToolCalls, call)
			} else {
				msgs = append(msgs, LLMMessage{Role: "assistant", ToolCalls: []LLMToolCall{call}})
			}
		case "function_call_output":
			if strings.TrimSpace(it.CallID) == "" {
				return nil, fmt.Sprintf("input[%d]: function_call_output requires call_id", i)
			}
			msgs = append(msgs, LLMMessage{Role: "tool", ToolCallID: it.CallID, Content: respOutputText(it.Output)})
		case "reasoning":
			continue
		default:
			return nil, fmt.Sprintf("input[%d].type %q is not supported", i, it.Type)
		}
	}
	return msgs, ""
}

// respContentText 取出消息内容里的纯文本。content 可以是字符串，或内容块数组；
// 只接受文本块，图片、文件等非文本块明确拒绝。
func respContentText(raw json.RawMessage) (string, string) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", ""
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return "", "content must be a string or an array of content parts"
		}
		return s, ""
	case '[':
		var parts []struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Refusal string `json:"refusal"`
		}
		if err := json.Unmarshal(trimmed, &parts); err != nil {
			return "", "content must be a string or an array of content parts"
		}
		var b strings.Builder
		for _, p := range parts {
			switch p.Type {
			case "input_text", "output_text", "text":
				b.WriteString(p.Text)
			case "refusal":
				b.WriteString(p.Refusal)
			default:
				return "", fmt.Sprintf("content part type %q is not supported (text only)", p.Type)
			}
		}
		return b.String(), ""
	default:
		return "", "content must be a string or an array of content parts"
	}
}

// respOutputText 取 function_call_output.output：通常是字符串，也可能是内容块数组，
// 其余形状（对象等）按原始 JSON 文本交给模型，不丢信息。
func respOutputText(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return ""
	}
	switch trimmed[0] {
	case '"':
		var s string
		if json.Unmarshal(trimmed, &s) == nil {
			return s
		}
	case '[':
		if text, msg := respContentText(trimmed); msg == "" {
			return text
		}
	}
	return string(trimmed)
}

func isPresentJSON(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && string(trimmed) != "null"
}

// ---------------------------------------------------------------------------
// 响应对象
// ---------------------------------------------------------------------------

// respMeta 保存一次响应里不随输出变化的部分。Responses 的响应对象会回显请求里的
// instructions / tools / tool_choice / temperature，官方 SDK 的模型把它们当作必填字段。
type respMeta struct {
	ID           string
	Model        string
	CreatedAt    int64
	Instructions string
	Tools        []oaiRespTool
	ToolChoice   json.RawMessage
	Temperature  *float64
}

func newRespMeta(r oaiRespRequest) respMeta {
	return respMeta{
		ID: newRespID("resp"), Model: r.Model, CreatedAt: time.Now().Unix(),
		Instructions: r.Instructions, Tools: r.Tools, ToolChoice: r.ToolChoice, Temperature: r.Temperature,
	}
}

type oaiRespObject struct {
	ID                string             `json:"id"`
	Object            string             `json:"object"`
	CreatedAt         int64              `json:"created_at"`
	Status            string             `json:"status"`
	Error             *oaiRespError      `json:"error"`
	IncompleteDetails *oaiRespIncomplete `json:"incomplete_details"`
	Instructions      *string            `json:"instructions"`
	Metadata          map[string]string  `json:"metadata"`
	Model             string             `json:"model"`
	Output            []any              `json:"output"`
	ParallelToolCalls bool               `json:"parallel_tool_calls"`
	Temperature       *float64           `json:"temperature"`
	ToolChoice        json.RawMessage    `json:"tool_choice"`
	Tools             []oaiRespTool      `json:"tools"`
	TopP              *float64           `json:"top_p"`
	Usage             *oaiRespUsage      `json:"usage"`
}

type oaiRespError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type oaiRespIncomplete struct {
	Reason string `json:"reason"`
}

type oaiRespUsage struct {
	InputTokens        int `json:"input_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokens        int `json:"output_tokens"`
	OutputTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
	TotalTokens int `json:"total_tokens"`
}

// oaiRespMessageItem / oaiRespFunctionItem 是 output 数组里的两种条目。
// 拆成两个类型而不是一个带 omitempty 的结构：function_call 的 arguments 即使为空串
// 也必须出现（SDK 把它当必填字符串），omitempty 会把它丢掉。
type oaiRespMessageItem struct {
	ID      string           `json:"id"`
	Type    string           `json:"type"`
	Status  string           `json:"status"`
	Role    string           `json:"role"`
	Content []oaiRespContent `json:"content"`
}

type oaiRespContent struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Annotations []any  `json:"annotations"`
}

type oaiRespFunctionItem struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func respTextContent(text string) oaiRespContent {
	return oaiRespContent{Type: "output_text", Text: text, Annotations: []any{}}
}

func respMessageItem(id, status string, content []oaiRespContent) oaiRespMessageItem {
	return oaiRespMessageItem{ID: id, Type: "message", Status: status, Role: "assistant", Content: content}
}

// object 组装响应对象。output 不得为 nil（JSON 必须是 [] 而不是 null）。
func (m respMeta) object(status, incompleteReason string, output []any, usage *oaiRespUsage) oaiRespObject {
	if output == nil {
		output = []any{}
	}
	tools := m.Tools
	if tools == nil {
		tools = []oaiRespTool{}
	}
	choice := m.ToolChoice
	if !isPresentJSON(choice) {
		choice = json.RawMessage(`"auto"`)
	}
	obj := oaiRespObject{
		ID: m.ID, Object: respObject, CreatedAt: m.CreatedAt, Status: status,
		Metadata: map[string]string{}, Model: m.Model, Output: output,
		ParallelToolCalls: true, Temperature: m.Temperature, ToolChoice: choice, Tools: tools,
		Usage: usage,
	}
	if m.Instructions != "" {
		instr := m.Instructions
		obj.Instructions = &instr
	}
	if incompleteReason != "" {
		obj.IncompleteDetails = &oaiRespIncomplete{Reason: incompleteReason}
	}
	return obj
}

// respStatusFor 把结束原因映射到响应状态：长度截断与内容过滤对应 incomplete。
func respStatusFor(reason string) (status, incompleteReason string) {
	switch strings.TrimSpace(reason) {
	case "length":
		return respStatusIncomplete, "max_output_tokens"
	case "content_filter":
		return respStatusIncomplete, "content_filter"
	}
	return respStatusCompleted, ""
}

func newRespUsage(u LLMTokenUsage) *oaiRespUsage {
	out := &oaiRespUsage{InputTokens: u.PromptTokens, OutputTokens: u.CompletionTokens, TotalTokens: u.TotalTokens}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.InputTokens + out.OutputTokens
	}
	out.OutputTokensDetails.ReasoningTokens = u.ReasoningTokens
	return out
}

// respOutputFromLLM 把 chat 响应转成 output 条目：正文一条 message，每个工具调用一条 function_call。
// 既无正文也无工具调用时仍输出一条空 message，保证 output 非空（与 OpenAI 行为一致）。
func respOutputFromLLM(resp LLMResponse) []any {
	var out []any
	if resp.Content != "" || len(resp.ToolCalls) == 0 {
		out = append(out, respMessageItem(newRespID("msg"), respStatusCompleted, []oaiRespContent{respTextContent(resp.Content)}))
	}
	for _, tc := range resp.ToolCalls {
		callID := tc.ID
		if callID == "" {
			callID = newRespID("call")
		}
		out = append(out, oaiRespFunctionItem{
			ID: newRespID("fc"), Type: "function_call", Status: respStatusCompleted,
			CallID: callID, Name: tc.Name, Arguments: tc.Arguments,
		})
	}
	return out
}

// newRespID 生成 prefix_<24 位十六进制>（Responses 的 id 用下划线，chat 的 chatcmpl 用连字符）。
func newRespID(prefix string) string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

// ---------------------------------------------------------------------------
// 流式（SSE 事件）
// ---------------------------------------------------------------------------

// respItemState 记录流式过程中一个 output 条目的累积状态。
type respItemState struct {
	outputIndex int
	id          string
	kind        string // "message" | "function_call"
	text        strings.Builder
	callID      string
	// callIDSynth 为 true 表示 callID 是我们在上游没给 id 时合成的，
	// 之后上游若补发真实 id 可以替换；已是上游给的 id 则不再覆盖。
	callIDSynth bool
	name        string
	args        strings.Builder
	closed      bool
}

func (s *respItemState) item(status string) any {
	if s.kind == "message" {
		return respMessageItem(s.id, status, []oaiRespContent{respTextContent(s.text.String())})
	}
	return oaiRespFunctionItem{
		ID: s.id, Type: "function_call", Status: status,
		CallID: s.callID, Name: s.name, Arguments: s.args.String(),
	}
}

// respStreamWriter 把 chat 流式块翻译成 Responses 事件序列。
//
// 事件格式是 `event: <type>\ndata: {...}\n\n`，每个事件带单调递增的 sequence_number。
// Responses 流没有 [DONE] 哨兵，以 response.completed / incomplete / failed 收尾。
// 逐事件 Flush，不聚合缓冲。
type respStreamWriter struct {
	c       echo.Context
	flusher http.Flusher
	meta    respMeta
	seq     int

	items  []*respItemState       // 按 output 顺序
	curMsg *respItemState         // 当前打开的 message 条目
	tools  map[int]*respItemState // 上游工具调用 index → 条目
	usage  *oaiRespUsage
}

func newRespStreamWriter(c echo.Context, meta respMeta) *respStreamWriter {
	flusher, _ := c.Response().Writer.(http.Flusher)
	return &respStreamWriter{c: c, flusher: flusher, meta: meta, tools: map[int]*respItemState{}}
}

func (w *respStreamWriter) event(typ string, payload map[string]any) error {
	payload["type"] = typ
	payload["sequence_number"] = w.seq
	w.seq++
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := w.c.Response().Write([]byte("event: " + typ + "\ndata: " + string(data) + "\n\n")); err != nil {
		return err
	}
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return nil
}

// start 发送 response.created 与 response.in_progress，此时 output 为空、usage 为 null。
func (w *respStreamWriter) start() error {
	skeleton := w.meta.object(respStatusInProgress, "", nil, nil)
	if err := w.event("response.created", map[string]any{"response": skeleton}); err != nil {
		return err
	}
	return w.event("response.in_progress", map[string]any{"response": skeleton})
}

func (w *respStreamWriter) openMessage() error {
	st := &respItemState{kind: "message", id: newRespID("msg"), outputIndex: len(w.items)}
	w.items = append(w.items, st)
	w.curMsg = st
	if err := w.event("response.output_item.added", map[string]any{
		"output_index": st.outputIndex,
		"item":         respMessageItem(st.id, respStatusInProgress, []oaiRespContent{}),
	}); err != nil {
		return err
	}
	return w.event("response.content_part.added", map[string]any{
		"item_id": st.id, "output_index": st.outputIndex, "content_index": 0,
		"part": respTextContent(""),
	})
}

// text 发送一个正文增量；没有打开的 message 条目时先打开一个。
func (w *respStreamWriter) text(delta string) error {
	if delta == "" {
		return nil
	}
	if w.curMsg == nil {
		if err := w.openMessage(); err != nil {
			return err
		}
	}
	w.curMsg.text.WriteString(delta)
	return w.event("response.output_text.delta", map[string]any{
		"item_id": w.curMsg.id, "output_index": w.curMsg.outputIndex, "content_index": 0,
		"delta": delta, "logprobs": []any{},
	})
}

func (w *respStreamWriter) closeMessage() error {
	st := w.curMsg
	if st == nil {
		return nil
	}
	w.curMsg = nil
	st.closed = true
	text := st.text.String()
	if err := w.event("response.output_text.done", map[string]any{
		"item_id": st.id, "output_index": st.outputIndex, "content_index": 0,
		"text": text, "logprobs": []any{},
	}); err != nil {
		return err
	}
	if err := w.event("response.content_part.done", map[string]any{
		"item_id": st.id, "output_index": st.outputIndex, "content_index": 0,
		"part": respTextContent(text),
	}); err != nil {
		return err
	}
	return w.event("response.output_item.done", map[string]any{
		"output_index": st.outputIndex, "item": st.item(respStatusCompleted),
	})
}

// toolCall 处理上游的一个工具调用增量分片（按上游 index 聚合）。
// id/name 只在首个分片出现，后续分片只带 arguments。
// 工具调用开始前先收尾当前 message 条目，保证 output 顺序：文本在前，调用在后。
func (w *respStreamWriter) toolCall(index int, id, name, argsDelta string) error {
	if err := w.closeMessage(); err != nil {
		return err
	}
	st := w.tools[index]
	if st == nil {
		callID, synth := id, false
		if callID == "" {
			callID, synth = newRespID("call"), true
		}
		st = &respItemState{
			kind: "function_call", id: newRespID("fc"), outputIndex: len(w.items),
			callID: callID, callIDSynth: synth, name: name,
		}
		w.items = append(w.items, st)
		w.tools[index] = st
		if err := w.event("response.output_item.added", map[string]any{
			"output_index": st.outputIndex,
			"item": oaiRespFunctionItem{
				ID: st.id, Type: "function_call", Status: respStatusInProgress,
				CallID: st.callID, Name: st.name, Arguments: "",
			},
		}); err != nil {
			return err
		}
	} else {
		// 个别上游把 id/name 放在后续分片里：补齐，但不覆盖上游已给的值。
		// added 事件已发出，客户端看到的 call_id 以 done 事件里的最终值为准。
		if st.name == "" && name != "" {
			st.name = name
		}
		if id != "" && st.callIDSynth {
			st.callID, st.callIDSynth = id, false
		}
	}
	if argsDelta == "" {
		return nil
	}
	st.args.WriteString(argsDelta)
	return w.event("response.function_call_arguments.delta", map[string]any{
		"item_id": st.id, "output_index": st.outputIndex, "delta": argsDelta,
	})
}

func (w *respStreamWriter) closeTool(st *respItemState) error {
	st.closed = true
	if err := w.event("response.function_call_arguments.done", map[string]any{
		"item_id": st.id, "output_index": st.outputIndex, "name": st.name, "arguments": st.args.String(),
	}); err != nil {
		return err
	}
	return w.event("response.output_item.done", map[string]any{
		"output_index": st.outputIndex, "item": st.item(respStatusCompleted),
	})
}

func (w *respStreamWriter) setUsage(u LLMTokenUsage) { w.usage = newRespUsage(u) }

func (w *respStreamWriter) snapshot(status string) []any {
	out := make([]any, 0, len(w.items))
	for _, st := range w.items {
		out = append(out, st.item(status))
	}
	return out
}

// finish 收尾所有条目并发送终态事件：completed，或因长度/过滤截断时的 incomplete。
func (w *respStreamWriter) finish(reason string) error {
	if err := w.closeMessage(); err != nil {
		return err
	}
	if len(w.items) == 0 {
		// 模型什么都没输出：补一条空 message，保证 output 非空。
		if err := w.openMessage(); err != nil {
			return err
		}
		if err := w.closeMessage(); err != nil {
			return err
		}
	}
	for _, st := range w.items {
		if st.kind == "function_call" && !st.closed {
			if err := w.closeTool(st); err != nil {
				return err
			}
		}
	}
	status, incomplete := respStatusFor(reason)
	typ := "response.completed"
	if status == respStatusIncomplete {
		typ = "response.incomplete"
	}
	return w.event(typ, map[string]any{"response": w.meta.object(status, incomplete, w.snapshot(respStatusCompleted), w.usage)})
}

// fail 在流已开始后发送 response.failed。与 chat 流不同，Responses 协议为失败定义了
// 标准事件，SDK 能正确解析，所以这里发事件而不是静默断流。
// code/message 来自 ClassifyError 的固定文案，不透传上游原文，避免泄漏供应商地址或凭据。
func (w *respStreamWriter) fail(err error) {
	re := agentbridge.ClassifyError(err)
	obj := w.meta.object(respStatusFailed, "", w.snapshot(respStatusIncomplete), w.usage)
	obj.Error = &oaiRespError{Code: re.Code, Message: re.Message}
	_ = w.event("response.failed", map[string]any{"response": obj})
}

// ---------------------------------------------------------------------------
// handler
// ---------------------------------------------------------------------------

// Responses 处理 POST .../v1/responses。
func (h *LLMHandler) Responses(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return writeOAIError(c, http.StatusInternalServerError, "internal_error", "project context missing")
	}
	var req oaiRespRequest
	if err := c.Bind(&req); err != nil {
		return writeOAIError(c, http.StatusBadRequest, "invalid_request", "invalid json body")
	}
	lreq, msg := req.toLLMRequest()
	if msg != "" {
		return writeOAIError(c, http.StatusBadRequest, "invalid_request", msg)
	}
	if h.usage != nil {
		if err := h.usage.CheckQuota(c.Request().Context(), pc.ID, "llm"); err != nil {
			return writeOAIErrorFrom(c, err)
		}
	}
	meta := newRespMeta(req)
	if req.Stream {
		return h.respStream(c, pc.ID, meta, lreq)
	}
	return h.respComplete(c, pc.ID, meta, lreq)
}

func (h *LLMHandler) respComplete(c echo.Context, projectID string, meta respMeta, lreq LLMRequest) error {
	resp, err := h.svc.Chat(c.Request().Context(), projectID, lreq)
	if err != nil {
		return writeOAIErrorFrom(c, err)
	}
	h.recordAudit(c, projectID, "llm_responses", "ok")
	h.persistChat(c, projectID, persistRequestOf(lreq), resp)
	if resp.Model != "" {
		meta.Model = resp.Model
	}
	status, incomplete := respStatusFor(resp.FinishReason)
	return c.JSON(http.StatusOK, meta.object(status, incomplete, respOutputFromLLM(resp), newRespUsage(resp.Usage)))
}

func (h *LLMHandler) respStream(c echo.Context, projectID string, meta respMeta, lreq LLMRequest) error {
	reader, err := h.svc.Stream(c.Request().Context(), projectID, lreq)
	if err != nil {
		// 尚未写任何字节，可以返回正常的 HTTP 错误。
		return writeOAIErrorFrom(c, err)
	}
	defer reader.Close()

	beginSSE(c) // SSE 长连接需解除 WriteTimeout，否则会在 write_timeout 到点时被切断。
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	w := newRespStreamWriter(c, meta)
	if err := w.start(); err != nil {
		return nil // 客户端断开
	}
	finishReason := ""
	for {
		chunk, rerr := reader.Next()
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			w.fail(rerr)
			return nil
		}
		if chunk == nil {
			continue
		}
		if chunk.Usage != nil {
			w.setUsage(*chunk.Usage)
		}
		var werr error
		switch chunk.Type {
		case "content":
			werr = w.text(chunk.Content)
		case "tool_call_delta":
			werr = w.toolCall(chunk.ToolCallIndex, chunk.ToolCallID, chunk.ToolCallName, chunk.ToolCallArgs)
		}
		if werr != nil {
			return nil
		}
		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}
		if chunk.Done {
			break
		}
	}
	if err := w.finish(finishReason); err != nil {
		return nil
	}
	h.recordAudit(c, projectID, "llm_responses", "ok")
	return nil
}

// persistRequestOf 取 persistChat 需要的最后一条请求消息。
func persistRequestOf(r LLMRequest) oaiChatRequest {
	var msgs []oaiMessage
	if n := len(r.Messages); n > 0 {
		last := r.Messages[n-1]
		msgs = []oaiMessage{{Role: last.Role, Content: last.Content}}
	}
	return oaiChatRequest{Messages: msgs}
}
