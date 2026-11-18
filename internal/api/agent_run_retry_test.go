package api

// c1_run_validation_test.go：planv6.0 C1 表征测试（零生产修改）。
// 锁定 CreateRun 重试校验与历史筛选的可观察行为，作为未来提取
// 纯验证/历史筛选片段的安全网：
//   1. 重试非 thread 最新 run → 400 invalid_retry（BUG-03 门禁）；
//   2. 重试不存在 run → 400 invalid_retry；
//   3. 重试其他 thread 的 run → 400 invalid_retry；
//   4. retry 后旧 run 的 user 消息不重复出现在重试 run 的历史中
//      （历史筛选语义，经消息列表可观察）；
//   5. claim 失败（并发同 thread）→ 第二请求被拒，不落 run 行。
// 本文件只补测试；生产代码不动（C 类默认仅分析 + 表征）。

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

// c1FailScript 让首轮 run 失败（可重试状态 failed）。
func c1FailScript() fakellm.Script {
	return fakellm.Script{HTTPStatus: 500, Body: `{"error":{"message":"boom"}}`}
}

// TestC1_RetryNonLatestRunRejected：thread 中有更新的 run 时，
// 重试旧 run 必须 400 invalid_retry。
func TestC1_RetryNonLatestRunRejected(t *testing.T) {
	fail := c1FailScript()
	ok := fakellm.Script{
		// MaxRetries=3 时 500 会再试 3 次，共 4 次请求后本轮才失败。
		// 下一次请求（seq 5）才是后面那个成功的 run。
		Seq: 5,
		Steps: []fakellm.Step{
			{Content: "成功"}, {FinishReason: "stop"},
			{PromptTokens: 5, CompletionTokens: 3},
		},
	}
	hs := newAgentHarness(t, []fakellm.Script{fail, ok}, fail)
	threadID := hs.createThread(t)

	// 第一个 run 失败（可重试），第二个 run 成功（成为最新）。
	first := hs.createRun(t, threadID, "问题一", nil)
	if len(eventsOf(first, "error")) != 1 {
		t.Fatalf("first run should fail: %+v", first)
	}
	oldRunID := runIDOf(first)
	second := hs.createRun(t, threadID, "问题二", nil)
	if len(eventsOf(second, "error")) != 0 {
		t.Fatalf("second run should succeed: %+v", second)
	}

	// 重试旧 run（非最新）：400 invalid_retry。
	rec := hs.req("POST", "/agent-threads/"+threadID+"/runs",
		map[string]any{"content": "", "retry_of_run_id": oldRunID})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("retry of non-latest run: got %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_retry") {
		t.Fatalf("expected invalid_retry, got %s", rec.Body.String())
	}
}

// TestC1_RetryUnknownRunRejected：重试不存在的 run → 400 invalid_retry。
func TestC1_RetryUnknownRunRejected(t *testing.T) {
	hs := newAgentHarness(t, nil, c1FailScript())
	threadID := hs.createThread(t)
	rec := hs.req("POST", "/agent-threads/"+threadID+"/runs",
		map[string]any{"content": "", "retry_of_run_id": "no-such-run"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("retry unknown run: got %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_retry") {
		t.Fatalf("expected invalid_retry, got %s", rec.Body.String())
	}
}

// TestC1_RetryRunFromOtherThreadRejected：重试另一 thread 的 run → 400 invalid_retry。
func TestC1_RetryRunFromOtherThreadRejected(t *testing.T) {
	fail := c1FailScript()
	ok := fakellm.Script{
		Seq: 5,
		Steps: []fakellm.Step{
			{Content: "成功"}, {FinishReason: "stop"},
			{PromptTokens: 5, CompletionTokens: 3},
		},
	}
	hs := newAgentHarness(t, []fakellm.Script{fail, ok}, fail)
	threadA := hs.createThread(t)
	threadB := hs.createThread(t)

	failed := hs.createRun(t, threadA, "在 A 中失败", nil)
	if len(eventsOf(failed, "error")) != 1 {
		t.Fatalf("run in thread A should fail: %+v", failed)
	}
	runID := runIDOf(failed)

	// 在 thread B 重试 thread A 的 run：400 invalid_retry。
	rec := hs.req("POST", "/agent-threads/"+threadB+"/runs",
		map[string]any{"content": "", "retry_of_run_id": runID})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cross-thread retry: got %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_retry") {
		t.Fatalf("expected invalid_retry, got %s", rec.Body.String())
	}
}

// TestC1_RetryHistoryExcludesOldRunMessages：重试成功后，消息列表中
// 旧 run 的 user 消息仍只有一条（复用），旧 run 的失败占位消息保留，
// 重试 run 产生新的 assistant 回复。
func TestC1_RetryHistoryExcludesOldRunMessages(t *testing.T) {
	fail := c1FailScript()
	ok := fakellm.Script{
		Seq: 5,
		Steps: []fakellm.Step{
			{Content: "重试答复"}, {FinishReason: "stop"},
			{PromptTokens: 5, CompletionTokens: 3},
		},
	}
	hs := newAgentHarness(t, []fakellm.Script{fail, ok}, fail)
	threadID := hs.createThread(t)

	first := hs.createRun(t, threadID, "原始问题", nil)
	if len(eventsOf(first, "error")) != 1 {
		t.Fatalf("first run should fail: %+v", first)
	}
	runID := runIDOf(first)

	retry := hs.createRun(t, threadID, "", map[string]any{"retry_of_run_id": runID})
	if len(eventsOf(retry, "error")) != 0 {
		t.Fatalf("retry should succeed: %+v", retry)
	}

	msgRec := hs.req("GET", "/agent-threads/"+threadID+"/messages", nil)
	var out struct {
		Messages []messageDTO `json:"messages"`
	}
	if err := json.Unmarshal(msgRec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	userCount, assistantCount := 0, 0
	for _, m := range out.Messages {
		switch m.Role {
		case "user":
			userCount++
			if m.Content != "原始问题" {
				t.Fatalf("retry must reuse original user content, got %q", m.Content)
			}
		case "assistant":
			assistantCount++
		}
	}
	if userCount != 1 {
		t.Fatalf("user messages=%d want 1 (reused)", userCount)
	}
	if assistantCount != 1 {
		t.Fatalf("assistant messages=%d want 1 (retry answer only)", assistantCount)
	}
}

// TestC1_ClaimFailureLeavesNoRunRow：并发同 thread 第二请求被拒后，
// 不落下多余的 run 行（claim 门禁在写库之前）。
func TestC1_ClaimFailureLeavesNoRunRow(t *testing.T) {
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
	time.Sleep(300 * time.Millisecond) // 等待第一个 run 注册进 runtime

	rec := hs.req("POST", "/agent-threads/"+threadID+"/runs", map[string]any{"content": "第二个", "stream": true})
	if rec.Code == http.StatusOK {
		t.Fatalf("concurrent run should be rejected, got %d", rec.Code)
	}
	runsRec := hs.req("GET", "/agent-threads/"+threadID+"/runs", nil)
	var runs struct {
		Runs []runDTO `json:"runs"`
	}
	if err := json.Unmarshal(runsRec.Body.Bytes(), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs.Runs) != 1 {
		t.Fatalf("rejected run must not persist a run row: got %d rows", len(runs.Runs))
	}
	<-firstDone
}
