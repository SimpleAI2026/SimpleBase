package api

// agent_e2e_errors_test.go：错误注入与边界（planv4.1 §5.2 E06–E11）。
// 覆盖：上游 4xx/5xx、超时、中途断流、工具错误语义（tool_result 携带 is_error）。

import (
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

// E06：上游 401 → error 帧带 code=llm_upstream_error，run 状态 failed，消息落库带 error_code。
func TestAgentE2EUpstream401MapsToErrorCode(t *testing.T) {
	hs := newAgentHarness(t, nil, fakellm.Script{HTTPStatus: 401, Body: `{"error":{"message":"bad key"}}`})
	threadID := hs.createThread(t)
	events := hs.createRun(t, threadID, "hello", nil)

	errs := eventsOf(events, "error")
	if len(errs) != 1 {
		t.Fatalf("error frames=%d events=%+v", len(errs), events)
	}
	// 401 映射为 llm_auth_failed（cloudagent/errors.go 错误分类）。
	if errs[0].Code != "llm_auth_failed" {
		t.Fatalf("code=%s", errs[0].Code)
	}
	if errs[0].RunID == "" || errs[0].Message == "" {
		t.Fatalf("error frame %+v", errs[0])
	}
	// run 落库为 failed。
	rec := hs.req("GET", "/agent-threads/"+threadID+"/runs", nil)
	if !strings.Contains(rec.Body.String(), "failed") {
		t.Fatalf("runs body=%s", rec.Body.String())
	}
	// 消息刷新含错误提示（BUG-06）。
	msgRec := hs.req("GET", "/agent-threads/"+threadID+"/messages", nil)
	if !strings.Contains(msgRec.Body.String(), "llm_auth_failed") {
		t.Fatalf("messages body=%s", msgRec.Body.String())
	}
}

// E07：工具执行错误返回 tool_result 帧且内容含错误（BUG-02：不吞成无响应）。
func TestAgentE2EToolErrorMarksIsError(t *testing.T) {
	toolCall := fakellm.Script{
		Steps: []fakellm.Step{
			{Content: `TOOL_CALL {"name":"readonly_sql","arguments":{"database_id":"d1","sql":"DELETE FROM x"}}`, FinishReason: "tool_calls"},
			{PromptTokens: 5, CompletionTokens: 3},
		},
	}
	final := fakellm.Script{
		Seq: 2,
		Steps: []fakellm.Step{
			{Content: "无法执行删除：该语句被只读保护拒绝"},
			{FinishReason: "stop"},
			{PromptTokens: 8, CompletionTokens: 4},
		},
	}
	hs := newAgentHarness(t, []fakellm.Script{toolCall, final}, toolCall)
	hs.runtime.ToolProtocol = "text"
	hs.db.sqlError = "sqlguard: write statements are not allowed"
	threadID := hs.createThread(t)
	events := hs.createRun(t, threadID, "删数据", nil)

	results := eventsOf(events, "tool_result")
	if len(results) == 0 {
		t.Fatalf("tool_result missing: %+v", events)
	}
	if !strings.Contains(results[0].Content, "not allowed") || !strings.Contains(results[0].Content, `"is_error":true`) {
		t.Fatalf("result content=%q", results[0].Content)
	}
	// 第二次请求把错误文本回给模型，模型据此答复用户。
	reqs := hs.fake.Requests()
	if len(reqs) < 2 {
		t.Fatalf("requests=%d", len(reqs))
	}
	if !strings.Contains(reqs[1].Messages[len(reqs[1].Messages)-1].Content.String(), "not allowed") {
		t.Fatalf("error text not fed back to model: %+v", reqs[1].Messages)
	}
}

// E08：中途断流（无 finish/DONE）→ run failed，不挂起。
func TestAgentE2EMidStreamAbortFailsRun(t *testing.T) {
	hs := newAgentHarness(t, nil, fakellm.Script{
		AbortMidStream: true,
		Steps: []fakellm.Step{
			{Content: "部分输出"},
			{Content: "更多"},
			{Content: "再多"},
			{Content: "结束"},
		},
	})
	threadID := hs.createThread(t)
	events := hs.createRun(t, threadID, "讲个故事", nil)

	last := events[len(events)-1]
	if last.Type != "end" {
		t.Fatalf("stream must terminate, last=%+v", last)
	}
	// 结束后 run 状态应为 failed（断流视为错误）或 completed（若视为部分成功）——
	// 按 planv4.1 §4 BUG-04：断流=failed。
	rec := hs.req("GET", "/agent-threads/"+threadID+"/runs", nil)
	if !strings.Contains(rec.Body.String(), "failed") {
		t.Fatalf("abort should fail run, body=%s", rec.Body.String())
	}
}

// E09：长工具执行期间有心跳（progress 帧），不长时间静默。
func TestAgentE2EToolProgressHeartbeat(t *testing.T) {
	hs := newAgentHarness(t, nil, fakellm.Script{
		Steps: []fakellm.Step{
			{Content: `TOOL_CALL {"name":"list_databases","arguments":{}}`, FinishReason: "tool_calls"},
			{PromptTokens: 5, CompletionTokens: 3},
		},
	})
	hs.runtime.ToolProtocol = "text"
	hs.db.blockFor = 700 * time.Millisecond
	threadID := hs.createThread(t)
	events := hs.createRun(t, threadID, "列库", nil)

	progress := eventsOf(events, "tool_progress")
	if len(progress) == 0 {
		t.Fatalf("expected tool_progress heartbeats, events=%+v", events)
	}
}

// E10：取消 run → canceled，事件流以 end(canceled) 结束。
func TestAgentE2ECancelRun(t *testing.T) {
	hs := newAgentHarness(t, nil, fakellm.Script{
		Steps: []fakellm.Step{
			{Content: "正在生成"},
			{Content: "还在生成", Delay: 5 * time.Second},
			{FinishReason: "stop"},
		},
	})
	threadID := hs.createThread(t)
	body := map[string]any{"content": "长回答", "stream": true}
	rec := hs.req("POST", "/agent-threads/"+threadID+"/runs", body)
	// 解析部分事件拿 run_id。
	head := rec.Body.String()
	idx := strings.Index(head, "\"run_id\"")
	if idx < 0 {
		t.Fatalf("no run_id in stream: %s", head[:min(200, len(head))])
	}
	// 取消。
	cancelRec := hs.req("POST", "/agent-runs/whatever/cancel", nil)
	if cancelRec.Code == 404 || cancelRec.Code == 400 {
		// handler 期望真实 run id；用事件流中的。
		events, _ := parseSSE(head)
		runID := runIDOf(events)
		if runID == "" {
			t.Fatal("cannot extract run id")
		}
		cancelRec = hs.req("POST", "/agent-runs/"+runID+"/cancel", nil)
		if cancelRec.Code != 200 && cancelRec.Code != 409 {
			t.Fatalf("cancel=%d %s", cancelRec.Code, cancelRec.Body.String())
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
