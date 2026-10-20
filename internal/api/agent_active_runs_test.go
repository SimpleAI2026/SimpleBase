package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

// 库里 running 的 run 只有在运行时仍持有会话锁时才算「正在回复」；
// 进程重启残留的僵尸行与已结束的 run 都不返回。
func TestListActiveRunsFiltersByRuntime(t *testing.T) {
	hs := newAgentHarness(t, nil, fakellm.Script{})
	ctx := context.Background()
	threadID := hs.createThread(t)
	now := time.Now().UTC()
	live, err := hs.store.CreateAgentRun(ctx, systemdb.AgentRun{ThreadID: threadID, ProjectID: "testprj1", AgentID: "ag-live", Status: systemdb.AgentRunRunning, StartedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hs.store.CreateAgentRun(ctx, systemdb.AgentRun{ThreadID: threadID, ProjectID: "testprj1", AgentID: "ag-zombie", Status: systemdb.AgentRunRunning, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := hs.store.CreateAgentRun(ctx, systemdb.AgentRun{ThreadID: threadID, ProjectID: "testprj1", AgentID: "ag-done", Status: systemdb.AgentRunCompleted, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := hs.runtime.ClaimThread(threadID, live.ID); err != nil {
		t.Fatal(err)
	}

	rec := hs.req(http.MethodGet, "/agent-runs/active", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("active: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Runs []activeRunDTO `json:"runs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Runs) != 1 || out.Runs[0].RunID != live.ID || out.Runs[0].AgentID != "ag-live" || out.Runs[0].ThreadID != threadID {
		t.Fatalf("unexpected active runs: %s", rec.Body.String())
	}

	hs.runtime.ReleaseThread(threadID, live.ID)
	rec = hs.req(http.MethodGet, "/agent-runs/active", nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Runs) != 0 {
		t.Fatalf("released run must disappear: %s", rec.Body.String())
	}
}
