package api

// agent_e2e_usage_retry_test.go：用量、重试与接口行为（planv4.1 §5.2 E11–E19）。

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

// E11：usage 帧累计多轮工具调用 token（两轮之和）。
func TestAgentE2EUsageAggregatesAcrossToolTurns(t *testing.T) {
	toolCall := fakellm.Script{
		Steps: []fakellm.Step{
			{Content: `TOOL_CALL {"name":"list_databases","arguments":{}}`, FinishReason: "tool_calls"},
			{PromptTokens: 5, CompletionTokens: 3},
		},
	}
	final := fakellm.Script{
		Seq: 2,
		Steps: []fakellm.Step{
			{Content: "答复"},
			{FinishReason: "stop"},
			{PromptTokens: 9, CompletionTokens: 4, ReasoningTokens: 1},
		},
	}
	hs := newAgentHarness(t, []fakellm.Script{toolCall, final}, toolCall)
	hs.runtime.ToolProtocol = "text"
	threadID := hs.createThread(t)
	events := hs.createRun(t, threadID, "列库", nil)

	usage := eventsOf(events, "usage")
	if len(usage) != 1 {
		t.Fatalf("usage frames=%d", len(usage))
	}
	if usage[0].PromptTokens != 14 || usage[0].CompletionTokens != 7 {
		t.Fatalf("usage=%+v want prompt=14 completion=7", usage[0])
	}
	// run 落库的用量一致。
	rec := hs.req("GET", "/agent-threads/"+threadID+"/runs", nil)
	if !strings.Contains(rec.Body.String(), `"prompt_tokens":14`) {
		t.Fatalf("run usage not persisted: %s", rec.Body.String())
	}
}

// E12：并发两个 run 拒绝第二个（同一 thread 单飞行）。
// fakellm 用中途阻塞模拟长运行：第二个请求在第一个 run 存活期内被拒。
func TestAgentE2EConcurrentRunRejected(t *testing.T) {
	hs := newAgentHarness(t, nil, fakellm.Script{
		Steps: []fakellm.Step{
			{Content: "生成中", Delay: 8 * time.Second},
			{FinishReason: "stop"},
		},
	})
	threadID := hs.createThread(t)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_ = hs.req("POST", "/agent-threads/"+threadID+"/runs", map[string]any{"content": "第一个", "stream": true})
	}()
	// 等待第一个 run 注册进 runtime（略过启动窗口）。
	time.Sleep(300 * time.Millisecond)

	rec2 := hs.req("POST", "/agent-threads/"+threadID+"/runs", map[string]any{"content": "第二个"})
	if rec2.Code == http.StatusOK {
		t.Fatalf("second concurrent run should be rejected, got %d %s", rec2.Code, rec2.Body.String())
	}
	<-firstDone
}

// E13：重试失败的 run——复用原 user 消息，不重复落库（BUG-03）。
func TestAgentE2ERetryReusesUserMessage(t *testing.T) {
	fail := fakellm.Script{
		HTTPStatus: 500,
		Body:       `{"error":{"message":"boom"}}`,
	}
	ok := fakellm.Script{
		Seq: 2,
		Steps: []fakellm.Step{
			{Content: "重试成功"},
			{FinishReason: "stop"},
			{PromptTokens: 5, CompletionTokens: 3},
		},
	}
	hs := newAgentHarness(t, []fakellm.Script{fail, ok}, fail)
	threadID := hs.createThread(t)
	events := hs.createRun(t, threadID, "问题一", nil)
	if len(eventsOf(events, "error")) != 1 {
		t.Fatalf("first run should fail: %+v", events)
	}
	runID := runIDOf(events)

	// 消息数：1 user + 1 错误占位。
	msgRec := hs.req("GET", "/agent-threads/"+threadID+"/messages", nil)
	var before struct {
		Messages []messageDTO `json:"messages"`
	}
	if err := json.Unmarshal(msgRec.Body.Bytes(), &before); err != nil {
		t.Fatal(err)
	}

	// 重试。
	retry := hs.createRun(t, threadID, "", map[string]any{"retry_of_run_id": runID})
	if len(eventsOf(retry, "error")) != 0 {
		t.Fatalf("retry failed: %+v", retry)
	}
	// 重试后 user 消息数不增（复用原消息）。
	msgRec2 := hs.req("GET", "/agent-threads/"+threadID+"/messages", nil)
	var after struct {
		Messages []messageDTO `json:"messages"`
	}
	if err := json.Unmarshal(msgRec2.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	userBefore, userAfter := 0, 0
	for _, m := range before.Messages {
		if m.Role == "user" {
			userBefore++
		}
	}
	for _, m := range after.Messages {
		if m.Role == "user" {
			userAfter++
		}
	}
	if userBefore != 1 || userAfter != 1 {
		t.Fatalf("retry duplicated user message: before=%d after=%d", userBefore, userAfter)
	}
}

// E14：重试参数校验——非 failed 状态的 run 不可重试。
func TestAgentE2ERetryRejectsNonFailedRun(t *testing.T) {
	ok := fakellm.Script{
		Steps: []fakellm.Step{
			{Content: "正常完成"},
			{FinishReason: "stop"},
			{PromptTokens: 5, CompletionTokens: 3},
		},
	}
	hs := newAgentHarness(t, nil, ok)
	threadID := hs.createThread(t)
	events := hs.createRun(t, threadID, "正常问题", nil)
	runID := runIDOf(events)
	// completed run 不可重试。
	rec := hs.req("POST", "/agent-threads/"+threadID+"/runs", map[string]any{"content": "", "retry_of_run_id": runID})
	if rec.Code == http.StatusBadRequest {
		// 符合预期
	} else if rec.Code == http.StatusOK {
		t.Fatal("completed run must not be retryable")
	}
}

// E15：/agents/models 返回模型列表（BUG-07），不含 base_url/api_key。
func TestAgentE2EModelsEndpoint(t *testing.T) {
	hs := newAgentHarness(t, nil, fakellm.Script{
		Steps: []fakellm.Step{{Content: "x"}, {FinishReason: "stop"}},
	})
	rec := hs.req("GET", "/agents/models", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("models=%d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		DefaultModel string `json:"default_model"`
		Models       []struct {
			Provider string `json:"provider"`
			Name     string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Models) == 0 || out.DefaultModel == "" {
		t.Fatalf("models=%+v default=%s", out.Models, out.DefaultModel)
	}
	if strings.Contains(rec.Body.String(), "api_key") || strings.Contains(rec.Body.String(), "base_url") {
		t.Fatalf("models response leaks secrets: %s", rec.Body.String())
	}
}

// E16：内置播种——列表含通用助手且排首位（BUG-11）。
func TestAgentE2ESeededGeneralAgentFirst(t *testing.T) {
	hs := newAgentHarness(t, nil, fakellm.Script{Steps: []fakellm.Step{{Content: "x"}, {FinishReason: "stop"}}})
	rec := hs.req("GET", "/agents", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("agents=%d", rec.Code)
	}
	var out struct {
		Agents []agentDTO `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Agents) < 4 {
		t.Fatalf("seeded agents=%d", len(out.Agents))
	}
	if out.Agents[0].BuiltinKey != "general" || out.Agents[0].Module != "general" {
		t.Fatalf("first agent=%+v", out.Agents[0])
	}
	if len(out.Agents[0].ToolIDs) != 7 {
		t.Fatalf("general tools=%v", out.Agents[0].ToolIDs)
	}
}
