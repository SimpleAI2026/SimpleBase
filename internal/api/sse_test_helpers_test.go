package api

// sse_test_helpers_test.go：SSE 解析辅助（planv4.1 §5.1）。
// 读取 text/event-stream 响应体为带时间戳的事件列表，供帧序与延迟断言。

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// sseEvent 是一帧解析后的 SSE 事件。
type sseEvent struct {
	Type             string `json:"type"`
	Content          string `json:"content"`
	Name             string `json:"name"`
	CallID           string `json:"call_id"`
	RunID            string `json:"run_id"`
	Code             string `json:"code"`
	Message          string `json:"message"`
	Reason           string `json:"reason"`
	ElapsedMS        int64  `json:"elapsed_ms"`
	DurationMS       int64  `json:"duration_ms"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	ReasoningTokens  int    `json:"reasoning_tokens"`
	ToolCalls        int    `json:"tool_calls"`
	IsError          bool   `json:"is_error"`
	Truncated        bool   `json:"truncated"`
}

// parseSSE 把 SSE 文本解析为事件列表；忽略注释（: ping）。
func parseSSE(body string) ([]sseEvent, error) {
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	out := make([]sseEvent, 0, 32)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if strings.TrimSpace(data) == "" {
			continue
		}
		var ev sseEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return nil, fmt.Errorf("parse sse frame %q: %w", data, err)
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("no sse events parsed")
	}
	return out, nil
}

// timedSSE 按到达时间解析流式响应（测首帧延迟）。read 持续读取直到 EOF 或时长超限。
func timedSSE(lines <-chan string) ([]timedEvent, error) {
	out := make([]timedEvent, 0, 32)
	start := time.Now()
	for line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var ev sseEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return nil, err
		}
		out = append(out, timedEvent{Event: ev, At: time.Since(start)})
	}
	return out, nil
}

type timedEvent struct {
	Event sseEvent
	At    time.Duration
}
