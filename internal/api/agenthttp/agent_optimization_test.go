// 文件说明：agent_optimization_test.go — internal/api 的单元测试（TestAgentThreadPaginationRenameAndRuns 等）
//
// 用法
//
//	go test ./internal/api -run 'TestAgentThreadPaginationRenameAndRuns'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/agent_optimization_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestAgentThreadPaginationRenameAndRuns(t *testing.T)

package agenthttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func TestAgentThreadPaginationRenameAndRuns(t *testing.T) {
	store := openAgentStore(t)
	e := fullAgentEcho(store, nil)
	h := &CloudAgentHandler{store: store, svc: testAgentRun(store, nil), writable: true}
	e.PATCH("/agent-threads/:threadID", h.PatchThread)
	e.GET("/agent-threads/:threadID/runs", h.ListThreadRuns)
	ids := make([]string, 0)
	for _, title := range []string{"one", "two", "three"} {
		rec := agentReq(e, http.MethodPost, "/agent-threads", map[string]string{"title": title})
		if rec.Code != http.StatusCreated {
			t.Fatal(rec.Body.String())
		}
		var th threadDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &th); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, th.ID)
	}
	rec := agentReq(e, http.MethodGet, "/agent-threads?limit=2", nil)
	var page struct {
		Threads []threadDTO `json:"threads"`
		Next    string      `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Threads) != 2 || page.Next == "" {
		t.Fatalf("page=%s", rec.Body.String())
	}
	rec = agentReq(e, http.MethodGet, "/agent-threads?limit=2&cursor="+page.Next, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Threads) != 1 {
		t.Fatalf("page=%s", rec.Body.String())
	}
	if rec = agentReq(e, http.MethodGet, "/agent-threads?cursor=invalid", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("cursor=%d", rec.Code)
	}
	if rec = agentReq(e, http.MethodPatch, "/agent-threads/"+ids[0], map[string]string{"title": ""}); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty=%d", rec.Code)
	}
	if rec = agentReq(e, http.MethodPatch, "/agent-threads/"+ids[0], map[string]string{"title": strings.Repeat("x", 81)}); rec.Code != http.StatusBadRequest {
		t.Fatalf("long=%d", rec.Code)
	}
	if rec = agentReq(e, http.MethodPatch, "/agent-threads/"+ids[0], map[string]string{"title": "renamed"}); rec.Code != http.StatusOK {
		t.Fatalf("rename=%s", rec.Body.String())
	}
	if rec = agentReq(e, http.MethodPatch, "/agent-threads/not-found", map[string]string{"title": "x"}); rec.Code != http.StatusNotFound {
		t.Fatalf("not found=%d", rec.Code)
	}
	ro := false
	h.writable = ro
	if rec = agentReq(e, http.MethodPatch, "/agent-threads/"+ids[0], map[string]string{"title": "x"}); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readonly=%d", rec.Code)
	}
	if _, err := store.CreateAgentRun(context.Background(), systemdb.AgentRun{ID: "r1", ThreadID: ids[0], ProjectID: "testprj1", AgentID: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAgentRunMetrics(context.Background(), "testprj1", "r1", systemdb.AgentRun{DurationMS: 314, PromptTokens: 5, CompletionTokens: 6, ToolCalls: 1}); err != nil {
		t.Fatal(err)
	}
	if rec = agentReq(e, http.MethodGet, "/agent-threads/"+ids[0]+"/runs", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"duration_ms":314`) {
		t.Fatalf("runs=%s", rec.Body.String())
	}
	if rec = agentReq(e, http.MethodGet, "/agent-threads/invalid/runs", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("other thread=%d", rec.Code)
	}
}
