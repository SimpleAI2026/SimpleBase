package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func TestAgentThreadPaginationRenameAndRuns(t *testing.T) {
	store := openAgentStore(t)
	e := fullAgentEcho(store, nil)
	h := &cloudAgentHandler{store: store}
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
	h.writable = &ro
	if rec = agentReq(e, http.MethodPatch, "/agent-threads/"+ids[0], map[string]string{"title": "x"}); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readonly=%d", rec.Code)
	}
	if _, err := store.CreateAgentRun(context.Background(), systemdb.AgentRun{ID: "r1", ThreadID: ids[0], ProjectID: catalog.DevProjectID, AgentID: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAgentRunMetrics(context.Background(), catalog.DevProjectID, "r1", systemdb.AgentRun{DurationMS: 314, PromptTokens: 5, CompletionTokens: 6, ToolCalls: 1}); err != nil {
		t.Fatal(err)
	}
	if rec = agentReq(e, http.MethodGet, "/agent-threads/"+ids[0]+"/runs", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"duration_ms":314`) {
		t.Fatalf("runs=%s", rec.Body.String())
	}
	if rec = agentReq(e, http.MethodGet, "/agent-threads/invalid/runs", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("other thread=%d", rec.Code)
	}
}

func TestAgentRunMetricsAuditOmitsConversation(t *testing.T) {
	store := openAgentStore(t)
	thread, err := store.CreateAgentThread(context.Background(), systemdb.AgentThread{ProjectID: catalog.DevProjectID, Title: "title"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreateAgentRun(context.Background(), systemdb.AgentRun{ID: "audit-test-run", ThreadID: thread.ID, ProjectID: catalog.DevProjectID, AgentID: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	audit := &fakeAudit{}
	h := &cloudAgentHandler{store: store, audit: audit}
	res := cloudagent.RunResult{Content: "private conversation", ToolCallsJSON: "[]", DurationMS: 500, PromptTokens: 12, CompletionTokens: 3, ToolCalls: 1}
	if err := h.finishAgentRun(context.Background(), catalog.DevProjectID, run, systemdb.CloudAgent{ID: "agent"}, res, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(audit.last.Detail, res.Content) || audit.last.Kind != "agent.run" || !strings.Contains(audit.last.Detail, "prompt_tokens=12") {
		t.Fatalf("audit=%+v", audit.last)
	}
	row, err := store.GetAgentRun(context.Background(), catalog.DevProjectID, run.ID)
	if err != nil || row.DurationMS != 500 || row.CompletionTokens != 3 || row.ToolCalls != 1 {
		t.Fatalf("run=%+v err=%v", row, err)
	}
}
