// 文件说明：oai_dto.go — SimpleBase 对外 LLM 出口的 OpenAI Chat Completions 兼容协议
//
// 用法
//
//	type oaiChatRequest（包内使用）
//	type oaiStreamOptions（包内使用）
//	同包调用：raw() → adapter.go, agent_handler.go 等；write() → agent_handler.go, api_key_policy.go 等；start() → cronjob_handler.go, e2b_process_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/oai_dto.go
//	         │ 依赖
//	         ▼
//	[下游] internal/agentbridge, internal/llmgateway
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] raw() ← adapter.go, agent_handler.go, collection_filter.go
//
// 主要符号
//
//	const oaiObjectChatCompletion
//	const oaiObjectChatCompletionChunk
//	const oaiObjectModel
//	const oaiObjectList
//	const oaiSSEDone
//	type oaiChatRequest struct {…}
//	... 另有 44 个顶层符号

package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/voocel/litellm/providers"
)

// OpenAI 对象的 object 字段取值。
const (
	oaiObjectChatCompletion      = "chat.completion"
	oaiObjectChatCompletionChunk = "chat.completion.chunk"
	oaiObjectModel               = "model"
	oaiObjectList                = "list"
)

// oaiSSEDone 是 OpenAI 流式协议的结束哨兵。
const oaiSSEDone = "[DONE]"

// ---------------------------------------------------------------------------
// 请求
// ---------------------------------------------------------------------------

// oaiChatRequest 是 POST /v1/projects/:projectID/v1/chat/completions 的请求体。
// 只覆盖 SimpleBase 实际支持的子集，不伪造未实现的 OpenAI 能力。
type oaiChatRequest struct {
	Model       string          `json:"model"`
	Messages    []oaiMessage    `json:"messages"`
	Tools       []oaiTool       `json:"tools,omitempty"`
	ToolChoice  json.RawMessage `json:"tool_choice,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	// StreamOptions 仅支持 include_usage；其余子字段忽略。
	StreamOptions *oaiStreamOptions `json:"stream_options,omitempty"`
}

type oaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// oaiMessage 是 OpenAI 消息；工具往返靠 ToolCalls 与 ToolCallID。
type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ToolCalls 仅 assistant 消息有值：模型发起过的工具调用。
	ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
	// ToolCallID 仅 tool 消息有值：对应哪一次调用。
	ToolCallID string `json:"tool_call_id,omitempty"`
	// Name 仅 tool 消息可能有值：工具名，便于上游识别。
	Name string `json:"name,omitempty"`
}

// oaiTool 是工具声明。Type 只接受 "function"。
type oaiTool struct {
	Type     string      `json:"type"`
	Function oaiFunction `json:"function"`
}

type oaiFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// oaiToolCall 既用于请求侧的历史回传，也用于响应侧返回。
//
// Function.Arguments 是 JSON 字符串而不是对象——这是 OpenAI 的既有约定，
// 上游与本服务都按字符串透传，不做反序列化（否则会丢掉非法 JSON 的原始信息，
// 也无法原样回灌给供应商）。
type oaiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function oaiCallFunction `json:"function"`
}

type oaiCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// oaiToLLMRequest 把兼容请求映射到既有 LLMRequest。
// 这是补齐工具调用能力的唯一入口：此前 llmMessageDTO 只有 role/content，
// 工具声明与往返消息在类型层面就不存在，导致调用方只能退回文本协议。
//
// 命名带 oai 前缀：gocloudagent_access.go 已有 toLLMRequest（gocloudagent.ChatRequest →
// LLMRequest），两者入参不同，不能共用名字。
func oaiToLLMRequest(req oaiChatRequest) LLMRequest {
	out := LLMRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		ToolChoice:  req.ToolChoice,
	}
	for _, m := range req.Messages {
		msg := LLMMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, LLMToolCall{
				ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
			})
		}
		out.Messages = append(out.Messages, msg)
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, LLMTool{
			Name: t.Function.Name, Description: t.Function.Description, Parameters: t.Function.Parameters,
		})
	}
	return out
}

// validate 检查请求是否可以进入网关。返回 nil 表示通过。
// message 会进入兼容错误信封，因此不得包含密钥或正文。
func (r oaiChatRequest) validate() (status int, code, message string) {
	if len(r.Messages) == 0 {
		return http.StatusBadRequest, "invalid_request", "messages must not be empty"
	}
	for i, t := range r.Tools {
		if t.Type != "function" {
			// 不伪造 code_interpreter / file_search 等未实现类型，明确拒绝。
			return http.StatusBadRequest, "invalid_request",
				fmt.Sprintf("tools[%d].type must be \"function\", got %q", i, t.Type)
		}
		if strings.TrimSpace(t.Function.Name) == "" {
			return http.StatusBadRequest, "invalid_request",
				fmt.Sprintf("tools[%d].function.name must not be empty", i)
		}
	}
	// tool_choice 非法值在此以 400 拒绝；adapter 的 decodeToolChoice 对非法 JSON 只做
	// 「不指定」兜底，不再报错，所以必须在入口把关，否则调用方的强制调用意图会被静默吞掉。
	if msg := validateToolChoice(r.ToolChoice, len(r.Tools)); msg != "" {
		return http.StatusBadRequest, "invalid_request", msg
	}
	return 0, "", ""
}

// validateToolChoice 校验 tool_choice：
//   - 字符串只允许 auto / none / required；
//   - 对象必须是 {"type":"function","function":{"name":"..."}}；
//   - 指定了非 none 的策略却没有声明 tools，没有意义，直接拒绝。
//
// 返回空字符串表示合法。
func validateToolChoice(raw json.RawMessage, toolCount int) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "none":
			return ""
		case "auto", "required":
			if toolCount == 0 {
				return "tool_choice requires tools to be provided"
			}
			return ""
		default:
			return fmt.Sprintf("tool_choice must be \"auto\", \"none\", \"required\" or a function object, got %q", s)
		}
	}
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "tool_choice must be a string or an object"
	}
	if obj.Type != "function" || strings.TrimSpace(obj.Function.Name) == "" {
		return "tool_choice object must be {\"type\":\"function\",\"function\":{\"name\":\"...\"}}"
	}
	if toolCount == 0 {
		return "tool_choice requires tools to be provided"
	}
	return ""
}

// ---------------------------------------------------------------------------
// 响应（非流式）
// ---------------------------------------------------------------------------

type oaiChatResponse struct {
	ID      string      `json:"id"`
	Object  string      `json:"object"`
	Created int64       `json:"created"`
	Model   string      `json:"model"`
	Choices []oaiChoice `json:"choices"`
	Usage   oaiUsage    `json:"usage"`
}

type oaiChoice struct {
	Index        int        `json:"index"`
	Message      oaiMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// CompletionTokensDetails 是 OpenAI 官方的推理 token 位置。
	// SimpleBase 的 reasoning_tokens 从这里输出，兼容严格客户端。
	CompletionTokensDetails *oaiCompletionDetails `json:"completion_tokens_details,omitempty"`
}

type oaiCompletionDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// newOAIResponse 构造非流式响应。
func newOAIResponse(resp LLMResponse) oaiChatResponse {
	msg := oaiMessage{Role: "assistant", Content: resp.Content}
	for _, tc := range resp.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, oaiToolCall{
			ID: tc.ID, Type: "function",
			Function: oaiCallFunction{Name: tc.Name, Arguments: tc.Arguments},
		})
	}
	usage := oaiUsage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}
	if usage.TotalTokens == 0 {
		// 上游未回填总数时按分段求和，避免客户端看到全 0。
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if resp.Usage.ReasoningTokens > 0 {
		usage.CompletionTokensDetails = &oaiCompletionDetails{ReasoningTokens: resp.Usage.ReasoningTokens}
	}
	return oaiChatResponse{
		ID:      newOAIID("chatcmpl"),
		Object:  oaiObjectChatCompletion,
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []oaiChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: normalizeFinishReason(resp.FinishReason, len(msg.ToolCalls)),
		}},
		Usage: usage,
	}
}

// normalizeFinishReason 归一结束原因；空值按有无工具调用推断。
func normalizeFinishReason(reason string, toolCalls int) string {
	switch strings.TrimSpace(reason) {
	case "":
		if toolCalls > 0 {
			return "tool_calls"
		}
		return "stop"
	case "tool_use": // Anthropic 语义，映射到 OpenAI 取值
		return "tool_calls"
	default:
		return reason
	}
}

// oaiTokenUsage 把网关用量转成兼容形状，供流式尾帧使用。
func oaiTokenUsage(u LLMTokenUsage) oaiUsage {
	out := oaiUsage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.PromptTokens + out.CompletionTokens
	}
	if u.ReasoningTokens > 0 {
		out.CompletionTokensDetails = &oaiCompletionDetails{ReasoningTokens: u.ReasoningTokens}
	}
	return out
}

// newOAIID 生成 chatcmpl-<24 位十六进制>。随机失败时退化为时间戳，不 panic。
func newOAIID(prefix string) string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buf)
}

// ---------------------------------------------------------------------------
// 模型列表
// ---------------------------------------------------------------------------

type oaiModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type oaiModelList struct {
	Object string     `json:"object"`
	Data   []oaiModel `json:"data"`
}

// newOAIModelList 把「供应商 → 模型名」展平成严格 OpenAI list。
//
// 按供应商名排序，保证同一项目多次请求返回顺序稳定（前端与测试都依赖可复现顺序）。
// user 决定用纯 OpenAI 形状，因此不附 provider 私有字段。
func newOAIModelList(models map[string][]string) oaiModelList {
	providers := make([]string, 0, len(models))
	for p := range models {
		providers = append(providers, p)
	}
	sort.Strings(providers)
	created := time.Now().Unix()
	out := oaiModelList{Object: oaiObjectList, Data: []oaiModel{}}
	for _, p := range providers {
		names := append([]string(nil), models[p]...)
		sort.Strings(names)
		for _, name := range names {
			out.Data = append(out.Data, oaiModel{
				ID: name, Object: oaiObjectModel, Created: created, OwnedBy: p,
			})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// 错误信封
// ---------------------------------------------------------------------------

// oaiErrorBody 是 OpenAI 错误形状。
type oaiErrorBody struct {
	Error oaiErrorDetail `json:"error"`
}

type oaiErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   any    `json:"param"`
	Code    string `json:"code"`
}

// writeOAIError 输出标准 OpenAI 错误信封。
// 兼容路由组内的错误必须走这里；其余 handler 继续用 WriteError。
func writeOAIError(c echo.Context, status int, code, message string) error {
	return c.JSON(status, oaiErrorBody{Error: oaiErrorDetail{
		Message: message,
		Type:    oaiErrorType(status),
		Param:   nil,
		Code:    code,
	}})
}

// writeOAIErrorFrom 把任意 err 经既有映射降级为兼容错误信封。
//
// 复用 api.Error 的分类结果（错误码与脱敏文案），只替换信封形状。
// 无法分类的错误按 500 internal_error 处理，细节不外泄。
func writeOAIErrorFrom(c echo.Context, err error) error {
	rid := RequestIDFromContext(c.Request().Context())
	if ae := Error(err, rid); ae != nil {
		return writeOAIError(c, ae.HTTPStatus, ae.Body.Error.Code, ae.Body.Error.Message)
	}
	if isLLMSideError(err) {
		return writeOAILLMError(c, err, rid)
	}
	logUnhandledError(c, err, rid)
	return writeOAIError(c, http.StatusInternalServerError, "internal_error", "internal error")
}

// oaiToolsUnsupportedCode 是「上游模型不支持工具调用」的稳定错误码。
// 客户端（如 CLI 的 HTTPClient）据此识别并降级到文本工具协议，不能改名。
const oaiToolsUnsupportedCode = "tools_not_supported"

// isLLMSideError 判断 err 是否来自 LLM 网关/上游（而非配额、鉴权等 api 自身错误）。
// 限定范围是为了避免把无关错误误报成「模型服务不可用」。
func isLLMSideError(err error) bool {
	var le *providers.LiteLLMError
	return errors.As(err, &le) ||
		errors.Is(err, llmgateway.ErrModelNotAllowed) ||
		errors.Is(err, llmgateway.ErrNoProviders) ||
		errors.Is(err, llmgateway.ErrProviderNotFound) ||
		errors.Is(err, llmgateway.ErrModelNotConfigured)
}

// isToolsUnsupported 与 agentcore.Model.fallback 的判定口径一致：
// 上游返回 400/422 且错误文本提到 tool/function，视为该模型不支持工具声明。
func isToolsUnsupported(err error) bool {
	var le *providers.LiteLLMError
	if !errors.As(err, &le) || (le.StatusCode != http.StatusBadRequest && le.StatusCode != http.StatusUnprocessableEntity) {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "tool") || strings.Contains(msg, "function")
}

// writeOAILLMError 把 LLM 侧错误映射成可安全展示的 OpenAI 错误信封。
//
// 此前这类错误一律落成 500 internal_error：客户端既看不到原因，也无法判断是否该降级。
// 这里复用云助手已有的分类（gocloudagent.ClassifyError）与状态码映射（Error 对 RunError 的处理），
// 只使用固定文案，不透传上游原文，避免泄漏供应商地址或凭据片段。
func writeOAILLMError(c echo.Context, err error, rid string) error {
	if isToolsUnsupported(err) {
		return writeOAIError(c, http.StatusBadRequest, oaiToolsUnsupportedCode,
			"the upstream model does not support tool calling")
	}
	re := agentbridge.ClassifyError(err)
	if ae := Error(re, rid); ae != nil {
		return writeOAIError(c, ae.HTTPStatus, ae.Body.Error.Code, ae.Body.Error.Message)
	}
	return writeOAIError(c, http.StatusBadGateway, re.Code, re.Message)
}

// oaiErrorType 把 HTTP 状态映射为 OpenAI 的 error.type。
func oaiErrorType(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusNotFound, http.StatusBadRequest, http.StatusUnprocessableEntity,
		http.StatusRequestEntityTooLarge, http.StatusGone:
		return "invalid_request_error"
	default:
		return "api_error"
	}
}

// ---------------------------------------------------------------------------
// 流式（SSE）
// ---------------------------------------------------------------------------

// oaiChunk 是 chat.completion.chunk 帧。
type oaiChunk struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []oaiChunkChoice `json:"choices"`
	Usage   *oaiUsage        `json:"usage,omitempty"`
}

type oaiChunkChoice struct {
	Index int           `json:"index"`
	Delta oaiChunkDelta `json:"delta"`
	// FinishReason 是指针：未结束时输出 null，这是 OpenAI 的形状。
	FinishReason *string `json:"finish_reason"`
}

// oaiChunkDelta 是增量。所有字段都 omitempty，结束帧因此序列化为 delta:{}，
// 这正是 OpenAI 结束帧的形状。
type oaiChunkDelta struct {
	Role      string         `json:"role,omitempty"`
	Content   string         `json:"content,omitempty"`
	ToolCalls []oaiDeltaTool `json:"tool_calls,omitempty"`
}

// oaiDeltaTool 是流式工具调用的一个分片。
// Index 是同一消息内的调用序号，客户端据此把多个分片拼成一次调用。
type oaiDeltaTool struct {
	Index    int          `json:"index"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function oaiDeltaFunc `json:"function"`
}

type oaiDeltaFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// oaiStreamWriter 逐帧写 OpenAI SSE，并在需要时输出 usage 尾帧与 [DONE]。
//
// 逐帧 Flush，不聚合缓冲，保持低延迟；流中不插入任何非标准帧——
// 保活只能靠 SSE 注释行（本次未使用）。
type oaiStreamWriter struct {
	c       echo.Context
	flusher http.Flusher
	id      string
	model   string
	created int64
	// includeUsage 由请求的 stream_options.include_usage 决定是否输出 usage 尾帧。
	// 它只影响「是否回给客户端」，不影响用量记录——后者由 llmgateway 的
	// usageRecordingReader 独立完成。
	includeUsage bool

	started  bool
	finished bool
	doneSent bool
	// sawToolCall 记录本流是否出现过工具调用，用于推断空 finish_reason。
	sawToolCall bool
	usage       *oaiUsage
}

func newOAIStreamWriter(c echo.Context, model string, includeUsage bool) *oaiStreamWriter {
	flusher, _ := c.Response().Writer.(http.Flusher)
	return &oaiStreamWriter{
		c:            c,
		flusher:      flusher,
		id:           newOAIID("chatcmpl"),
		model:        model,
		created:      time.Now().Unix(),
		includeUsage: includeUsage,
	}
}

// start 发送首帧：delta.role="assistant"。
// 部分严格客户端依赖它建立消息对象，缺失会导致解析失败。
func (w *oaiStreamWriter) start() error {
	if w.started {
		return nil
	}
	w.started = true
	return w.write(&oaiChunk{
		ID: w.id, Object: oaiObjectChatCompletionChunk, Created: w.created, Model: w.model,
		Choices: []oaiChunkChoice{{Index: 0, Delta: oaiChunkDelta{Role: "assistant"}}},
	})
}

// content 发送一个正文增量。
func (w *oaiStreamWriter) content(text string) error {
	if text == "" {
		return nil
	}
	if err := w.start(); err != nil {
		return err
	}
	return w.write(&oaiChunk{
		ID: w.id, Object: oaiObjectChatCompletionChunk, Created: w.created, Model: w.model,
		Choices: []oaiChunkChoice{{Index: 0, Delta: oaiChunkDelta{Content: text}}},
	})
}

// toolCall 发送一个工具调用增量分片。
// id/name 只在该调用的首个分片出现，后续分片只带 arguments——
// 因此调用方必须在首个分片给出 id 与 name，不能每帧都当作独立调用。
func (w *oaiStreamWriter) toolCall(index int, id, name, argsDelta string) error {
	if err := w.start(); err != nil {
		return err
	}
	w.sawToolCall = true
	delta := oaiChunkDelta{ToolCalls: []oaiDeltaTool{{
		Index: index, ID: id, Function: oaiDeltaFunc{Name: name, Arguments: argsDelta},
	}}}
	if id != "" || name != "" {
		delta.ToolCalls[0].Type = "function"
	}
	return w.write(&oaiChunk{
		ID: w.id, Object: oaiObjectChatCompletionChunk, Created: w.created, Model: w.model,
		Choices: []oaiChunkChoice{{Index: 0, Delta: delta}},
	})
}

// setUsage 暂存用量，finish 时按 include_usage 决定是否输出尾帧。
func (w *oaiStreamWriter) setUsage(u LLMTokenUsage) {
	converted := oaiTokenUsage(u)
	w.usage = &converted
}

// finish 发送结束帧（finish_reason），随后按需发 usage 帧与 [DONE]。
func (w *oaiStreamWriter) finish(reason string) error {
	if w.finished {
		return nil
	}
	w.finished = true
	if err := w.start(); err != nil {
		return err
	}
	// 空 reason 按本流是否出现过工具调用推断：带调用的流应是 tool_calls，
	// 否则客户端会把「模型要求执行工具」误判成「已回答完毕」。
	toolCalls := 0
	if w.sawToolCall {
		toolCalls = 1
	}
	normalized := normalizeFinishReason(reason, toolCalls)
	chunk := &oaiChunk{
		ID: w.id, Object: oaiObjectChatCompletionChunk, Created: w.created, Model: w.model,
		Choices: []oaiChunkChoice{{Index: 0, Delta: oaiChunkDelta{}, FinishReason: &normalized}},
	}
	if err := w.write(chunk); err != nil {
		return err
	}
	// usage 尾帧：choices 为空数组，这是 OpenAI 的既有形状。
	if w.includeUsage && w.usage != nil {
		if err := w.write(&oaiChunk{
			ID: w.id, Object: oaiObjectChatCompletionChunk, Created: w.created, Model: w.model,
			Choices: []oaiChunkChoice{}, Usage: w.usage,
		}); err != nil {
			return err
		}
	}
	return w.done()
}

// done 写出结束哨兵（幂等）。
func (w *oaiStreamWriter) done() error {
	if w.doneSent {
		return nil
	}
	w.doneSent = true
	return w.raw(oaiSSEDone)
}

func (w *oaiStreamWriter) write(chunk *oaiChunk) error {
	data, err := json.Marshal(chunk)
	if err != nil {
		return err
	}
	return w.raw(string(data))
}

// raw 写一帧 `data: <payload>\n\n` 并立即 Flush。
// 编码失败（几乎不可能：payload 是已构造好的结构或字面量）不写入半帧。
func (w *oaiStreamWriter) raw(payload string) error {
	if _, err := w.c.Response().Write([]byte("data: " + payload + "\n\n")); err != nil {
		return err
	}
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return nil
}
