// 文件说明：agent_run_guard_test.go — /runs 的会话类型守卫（planv6.8 §13.15 CB-BE-13）
//
// 群会话的消息必须经 Plan 栅栏（POST /agent-threads/:id/turns）；一轮一助手的 auto 调度、
// 路由分类器与 route 帧已整体下线。/runs 只服务固定助手会话，旧绑定语义保持不变。

package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// 群会话上发 /runs → 400 turns_required，且不落 run、不占会话。
func TestRunsOnGroupThreadRequireTurns(t *testing.T) {
	h, threadID, _ := newCollabEnv(t)
	rec := h.req(http.MethodPost, "/agent-threads/"+threadID+"/runs", map[string]any{
		"content": "hello", "stream": true, "mentions": []map[string]any{},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("runs on group thread: %d %s", rec.Code, rec.Body.String())
	}
	if !jsonBodyContains(rec.Body.String(), "turns_required") {
		t.Fatalf("error code: %s", rec.Body.String())
	}
	runs, err := h.store.ListAgentRuns(t.Context(), "testprj1", threadID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("rejected request must not create a run: %+v", runs)
	}
	msgs, err := h.store.ListAgentMessages(t.Context(), "testprj1", threadID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("rejected request must not persist the user message: %+v", msgs)
	}
}

// 旧绑定会话（agent_id 绑定 + 无工作台元数据）仍走 /runs：@ 另一个助手 → 409 agent_switch_required。
func TestLegacyThreadStillEnforcesSwitchRequired(t *testing.T) {
	h, _, byModule := newCollabEnv(t)
	rec := h.req(http.MethodPost, "/agent-threads", map[string]any{"agent_id": byModule["database"].ID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("legacy thread: %d %s", rec.Code, rec.Body.String())
	}
	var th struct {
		ID           string `json:"id"`
		DispatchMode string `json:"dispatch_mode"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &th)
	if th.DispatchMode != "" {
		t.Fatalf("legacy thread must stay a fixed-agent thread: %+v", th)
	}
	rec = h.req(http.MethodPost, "/agent-threads/"+th.ID+"/runs", map[string]any{
		"content": "x", "stream": true,
		"mentions": []map[string]any{{"agent_id": byModule["s3"].ID}},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("switch required: %d %s", rec.Code, rec.Body.String())
	}
	if !jsonBodyContains(rec.Body.String(), "agent_switch_required") {
		t.Fatalf("error code: %s", rec.Body.String())
	}
}
