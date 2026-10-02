package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
)

// fakeAPISandbox 实现 cloudagent.Sandbox 的测试假件。
type fakeAPISandbox struct {
	available bool
	released  struct {
		projectID, threadID string
	}
}

func (f *fakeAPISandbox) Available() bool { return f.available }

func (f *fakeAPISandbox) Exec(ctx context.Context, projectID, threadID, cmd string, args []string) (cloudagent.SandboxOutput, error) {
	return cloudagent.SandboxOutput{}, nil
}

func (f *fakeAPISandbox) Shell(ctx context.Context, projectID, threadID, command string) (cloudagent.SandboxOutput, error) {
	return cloudagent.SandboxOutput{}, nil
}

func (f *fakeAPISandbox) ReadFile(ctx context.Context, projectID, threadID, path string) (string, error) {
	return "", nil
}

func (f *fakeAPISandbox) WriteFile(ctx context.Context, projectID, threadID, path, content string) error {
	return nil
}

func (f *fakeAPISandbox) ReleaseThread(ctx context.Context, projectID, threadID string) error {
	f.released.projectID, f.released.threadID = projectID, threadID
	return nil
}

func TestModulesSandboxAvailableFlag(t *testing.T) {
	store := openAgentStore(t)
	e := agentEcho(store)
	e.GET("/agents/modules/with", func(c echo.Context) error {
		h := &cloudAgentHandler{store: store, runtime: &cloudagent.Runtime{Sandbox: &fakeAPISandbox{available: true}}}
		return h.ListModules(c)
	})
	rec := agentReq(e, http.MethodGet, "/agents/modules/with", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"id":"sandbox"`) {
		t.Fatalf("sandbox module missing: %s", body)
	}
	if !strings.Contains(body, `"sandbox_available":true`) {
		t.Fatalf("sandbox_available must be true when enabled: %s", body)
	}
	// 未启用时字段缺省（omitempty）。
	rec = agentReq(e, http.MethodGet, "/agents/modules", nil)
	if strings.Contains(rec.Body.String(), `"sandbox_available":true`) {
		t.Fatalf("sandbox_available must be absent when disabled: %s", rec.Body.String())
	}
}

func TestCreateSandboxAgentRejectedWhenDisabled(t *testing.T) {
	store := openAgentStore(t)
	e := agentEcho(store)
	rec := agentReq(e, http.MethodPost, "/agents", map[string]any{
		"name": "Sandbox Agent", "module": "sandbox",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("module=sandbox must 400 when sandbox disabled, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = agentReq(e, http.MethodPost, "/agents", map[string]any{
		"name": "X", "module": "general", "tool_ids": []string{"sandbox_exec"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sandbox tool must 400 when sandbox disabled, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateSandboxAgentAllowedWhenEnabled(t *testing.T) {
	store := openAgentStore(t)
	sb := &fakeAPISandbox{available: true}
	h := &cloudAgentHandler{store: store, runtime: &cloudagent.Runtime{Sandbox: sb}}
	e := agentEcho(store)
	e.POST("/agents/enabled", h.CreateAgent)
	rec := agentReq(e, http.MethodPost, "/agents/enabled", map[string]any{
		"name": "Sandbox Agent", "module": "sandbox",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("module=sandbox must be created when enabled, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"sandbox_exec"`) {
		t.Fatalf("default sandbox tools expected: %s", rec.Body.String())
	}
}

func TestDeleteThreadReleasesSandbox(t *testing.T) {
	store := openAgentStore(t)
	sb := &fakeAPISandbox{available: true}
	h := &cloudAgentHandler{store: store, runtime: &cloudagent.Runtime{Sandbox: sb}}
	e := agentEcho(store)
	e.DELETE("/agent-threads/:threadID", h.DeleteThread)
	e.POST("/agent-threads", h.CreateThread)

	rec := agentReq(e, http.MethodPost, "/agent-threads", map[string]any{"title": "t"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create thread status=%d body=%s", rec.Code, rec.Body.String())
	}
	var th struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &th); err != nil {
		t.Fatal(err)
	}
	rec = agentReq(e, http.MethodDelete, "/agent-threads/"+th.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	if sb.released.threadID != th.ID {
		t.Fatalf("sandbox not released for thread %s (got %q)", th.ID, sb.released.threadID)
	}
}
