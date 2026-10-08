// Package fakellm 提供本地 OpenAI 兼容假模型服务（planv4.1 §5.1）。
// 默认 go test 不访问外网、不需要真实 key：全链路测试把它当作上游。
// 不含任何真实密钥；调用方使用占位 key（如 test-key）并断言脱敏路径。
package fakellm

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// Step 是剧本中的一个流帧。
type Step struct {
	Content          string        `json:"content,omitempty"`           // 正文增量
	ReasoningContent string        `json:"reasoning_content,omitempty"` // 思考增量（DeepSeek 风格）
	ToolID           string        `json:"tool_id,omitempty"`
	ToolName         string        `json:"tool_name,omitempty"`
	ToolArgs         string        `json:"tool_args,omitempty"`
	ToolIndex        int           `json:"tool_index,omitempty"`
	FinishReason     string        `json:"finish_reason,omitempty"`
	Delay            time.Duration `json:"delay,omitempty"`
	// Usage 尾帧：非零时输出 usage chunk 并结束。
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	ReasoningTokens  int `json:"reasoning_tokens,omitempty"`
}

// Script 是一次请求的响应剧本。
type Script struct {
	// MatchKeyword 非空时按最后一条 user 消息包含该关键字选择本剧本。
	MatchKeyword string `json:"match_keyword,omitempty"`
	// Seq 大于 0 时按请求序号（1 起）选择本剧本；与 MatchKeyword 互斥使用时序号优先。
	Seq int `json:"seq,omitempty"`
	// Steps 流式帧序列（stream=true）。
	Steps []Step `json:"steps,omitempty"`
	// NonStreamContent 非流式响应正文（stream=false 时使用；为空则拼接 Steps 的正文）。
	NonStreamContent string `json:"non_stream_content,omitempty"`
	// NonStreamToolCalls 非流式原生 tool_calls。
	NonStreamToolCalls []ToolCall `json:"non_stream_tool_calls,omitempty"`
	// HTTPStatus 非零时直接返回该状态码（错误注入），Body 为响应体。
	HTTPStatus int    `json:"http_status,omitempty"`
	Body       string `json:"body,omitempty"`
	// NoToolID 为 true 时原生 tool_calls 不带 id（BUG-08 复现）。
	NoToolID bool `json:"no_tool_id,omitempty"`
	// AbortMidStream 为 true 时输出一半正文后直接断开（无 finish、无 [DONE]）。
	AbortMidStream bool `json:"abort_mid_stream,omitempty"`
	// NeverFinish 为 true 时输出一帧后永久阻塞（测超时）。
	NeverFinish bool `json:"never_finish,omitempty"`
}

// ToolCall 是非流式响应中的原生工具调用。
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Request 是收到的请求体（供断言）。
type Request struct {
	Model    string           `json:"model"`
	Stream   bool             `json:"stream"`
	Tools    []map[string]any `json:"tools"`
	Messages []Message        `json:"messages"`
}

// Message 是请求消息。Content 兼容 OpenAI 的字符串与 content-parts 数组两种形式
// （litellm 的 openai provider 发送 [{"type":"text","text":...}]）。
type Message struct {
	Role       string          `json:"role"`
	Content    contentValue    `json:"content"`
	ToolCalls  []ReqCall       `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	raw        json.RawMessage `json:"-"`
}

// contentValue 兼容 string 与 [{"type":"text","text":...}]。
type contentValue struct {
	text string
	set  bool
}

func (cv *contentValue) UnmarshalJSON(b []byte) error {
	cv.set = true
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		cv.text = s
		return nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(b, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "" || p.Type == "text" || p.Type == "input_text" {
				b.WriteString(p.Text)
			}
		}
		cv.text = b.String()
		return nil
	}
	return fmt.Errorf("fakellm: unsupported content %q", string(b))
}

// String 返回拼接后的纯文本。
func (cv contentValue) String() string { return cv.text }

// ReqCall 是请求中的 tool_call。
type ReqCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Server 是一个 OpenAI 兼容假上游。
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	requests []Request
	seq      int
}

// New 构造假上游（随机端口）。
func New(scripts []Script, defaultScript Script) *Server {
	return NewAt("", scripts, defaultScript)
}

// NewAt 在指定地址（如 "127.0.0.1:8787"）构造假上游；空地址随机分配。
func NewAt(addr string, scripts []Script, defaultScript Script) *Server {
	s := &Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req Request
		_ = json.Unmarshal(body, &req)
		s.mu.Lock()
		s.requests = append(s.requests, req)
		s.seq++
		seq := s.seq
		s.mu.Unlock()
		script := pickScript(scripts, req, seq, defaultScript)
		s.respond(w, req, script)
	})
	s.Server = httptest.NewServer(mux)
	if addr != "" && addr != "127.0.0.1:0" && addr != ":0" {
		// httptest.NewServer 不支持自定义地址；此处改为监听给定地址。
		s.Server.Close()
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			panic("fakellm: listen " + err.Error())
		}
		s.Server = &httptest.Server{Listener: ln, Config: &http.Server{Handler: mux}}
		s.Server.Start()
	}
	return s
}

// Requests 返回已收到的请求副本。
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.requests))
	copy(out, s.requests)
	return out
}

// RequestCount 返回已收到的请求数。
func (s *Server) RequestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func pickScript(scripts []Script, req Request, seq int, def Script) Script {
	for _, sc := range scripts {
		if sc.Seq > 0 && sc.Seq == seq {
			return sc
		}
	}
	// 取最后一条 user 消息做关键字匹配。
	lastUser := ""
	for _, m := range req.Messages {
		if m.Role == "user" {
			lastUser = m.Content.String()
		}
	}
	for _, sc := range scripts {
		if sc.MatchKeyword != "" && strings.Contains(lastUser, sc.MatchKeyword) {
			return sc
		}
	}
	return def
}

func (s *Server) respond(w http.ResponseWriter, req Request, script Script) {
	if script.HTTPStatus != 0 {
		w.WriteHeader(script.HTTPStatus)
		_, _ = w.Write([]byte(script.Body))
		return
	}
	if script.NeverFinish {
		// 输出一帧后阻塞，直到客户端超时断开。
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		writeSSE(w, req.Model, Step{Content: "partial"}, 0)
		if flusher != nil {
			flusher.Flush()
		}
		select {}
	}
	if !req.Stream {
		s.respondNonStream(w, req, script)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	for i, st := range script.Steps {
		if st.Delay > 0 {
			time.Sleep(st.Delay)
		}
		if script.AbortMidStream && i > len(script.Steps)/2 {
			// 中途断流：利用 panic httptest 不支持，改为 hijack 后关闭。
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					_ = conn.Close()
					return
				}
			}
		}
		writeSSE(w, req.Model, st, i)
		if flusher != nil {
			flusher.Flush()
		}
	}
	if !script.AbortMidStream {
		_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func (s *Server) respondNonStream(w http.ResponseWriter, req Request, script Script) {
	content := script.NonStreamContent
	if content == "" {
		var b strings.Builder
		for _, st := range script.Steps {
			b.WriteString(st.Content)
		}
		content = b.String()
	}
	resp := map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion", "model": req.Model,
		"choices": []any{map[string]any{
			"index": 0,
			"message": map[string]any{
				"role":    "assistant",
				"content": content,
			},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{
			"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15,
		},
	}
	if len(script.NonStreamToolCalls) > 0 {
		calls := make([]any, 0, len(script.NonStreamToolCalls))
		for i, tc := range script.NonStreamToolCalls {
			id := tc.ID
			if script.NoToolID {
				id = ""
			}
			fn := map[string]any{"name": tc.Name, "arguments": tc.Arguments}
			if id == "" {
				calls = append(calls, map[string]any{"type": "function", "function": fn})
			} else {
				calls = append(calls, map[string]any{"id": id, "type": "function", "function": fn})
			}
			_ = i
		}
		resp["choices"] = []any{map[string]any{
			"index": 0,
			"message": map[string]any{
				"role": "assistant", "content": content, "tool_calls": calls,
			},
			"finish_reason": "tool_calls",
		}}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// writeSSE 输出一帧 OpenAI 风格流块。
func writeSSE(w io.Writer, model string, st Step, index int) {
	delta := map[string]any{}
	if st.Content != "" {
		delta["content"] = st.Content
	}
	if st.ReasoningContent != "" {
		delta["reasoning_content"] = st.ReasoningContent
	}
	if st.ToolName != "" || st.ToolID != "" || st.ToolArgs != "" {
		tc := map[string]any{"index": st.ToolIndex}
		if st.ToolID != "" {
			tc["id"] = st.ToolID
			tc["type"] = "function"
		}
		fn := map[string]any{}
		if st.ToolName != "" {
			fn["name"] = st.ToolName
		}
		if st.ToolArgs != "" {
			fn["arguments"] = st.ToolArgs
		}
		if len(fn) > 0 {
			tc["function"] = fn
		}
		delta["tool_calls"] = []any{tc}
	}
	choice := map[string]any{"index": index, "delta": delta}
	if st.FinishReason != "" {
		choice["finish_reason"] = st.FinishReason
	}
	if st.PromptTokens > 0 || st.CompletionTokens > 0 {
		// usage 尾帧：空 choices。
		payload := map[string]any{
			"id": "chatcmpl-fake", "object": "chat.completion.chunk", "model": model,
			"choices": []any{},
			"usage": map[string]any{
				"prompt_tokens": st.PromptTokens, "completion_tokens": st.CompletionTokens,
				"total_tokens": st.PromptTokens + st.CompletionTokens,
				"completion_tokens_details": map[string]any{"reasoning_tokens": st.ReasoningTokens},
			},
		}
		b, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
		return
	}
	payload := map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion.chunk", "model": model,
		"choices": []any{choice},
	}
	b, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
}
