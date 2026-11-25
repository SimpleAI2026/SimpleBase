// 文件说明：sse_test_helpers_test.go — internal/api 的测试辅助代码
//
// 用法
//
//	同包调用：sseEvent → agent_e2e_harness_test.go, agent_plan_tools_e2e_test.go；parseSSE() → agent_e2e_errors_test.go, agent_e2e_harness_test.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sse_test_helpers_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//	[关键引用] sseEvent ← agent_e2e_harness_test.go, agent_plan_tools_e2e_test.go；parseSSE ← agent_e2e_errors_test.go, agent_e2e_harness_test.go
//
// 主要符号
//
//	type sseEvent struct{…}
//	parseSSE(body string) ([]sseEvent, error)

package api

// sse_test_helpers_test.go：SSE 解析辅助（planv4.1 §5.1）。
// 读取 text/event-stream 响应体为带时间戳的事件列表，供帧序与延迟断言。

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	// planv6.7 v2：协作轮次事件（task / human / turn）。
	AgentID   string `json:"agent_id"`
	Status    string `json:"status"`
	Index     int    `json:"index"`
	Total     int    `json:"total"`
	HumanID   string `json:"human_id"`
	HumanName string `json:"human_name"`
	// planv6.8 B14：固定助手单聊的 v3 `plan` 帧（decision 为完整 plan.Decision）。
	Decision json.RawMessage `json:"decision"`
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
