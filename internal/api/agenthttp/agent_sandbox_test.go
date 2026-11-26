// 文件说明：agent_sandbox_test.go — internal/api 的单元测试（TestModulesSandboxAvailableFlag、TestCreateSandboxAgentRejectedWhenDisabled 等）
//
// 用法
//
//	go test ./internal/api -run 'TestModulesSandboxAvailableFlag|TestCreateSandboxAgentRejectedWhenDisabled|TestCreateSandboxAgentAllowedWhenEnabled|TestDeleteThreadReleasesSandbox'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/agent_sandbox_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	type fakeAPISandbox struct{…}
//	(f *fakeAPISandbox) Available() bool
//	(f *fakeAPISandbox) Exec(ctx context.Context, projectID, threadID, cmd string, args []string) (agentbridge.San...
//	(f *fakeAPISandbox) Shell(ctx context.Context, projectID, threadID, command string) (agentbridge.SandboxOutput...
//	(f *fakeAPISandbox) ReadFile(ctx context.Context, projectID, threadID, path string) (string, error)
//	(f *fakeAPISandbox) WriteFile(ctx context.Context, projectID, threadID, path, content string) error
//	(f *fakeAPISandbox) ReleaseThread(ctx context.Context, projectID, threadID string) error
//	sandboxRuntime(sandbox agentbridge.Sandbox) *cloudagent.Runtime
//	... 另有 4 个顶层符号

package agenthttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/agentrun"
)

// fakeAPISandbox implements agentbridge.Sandbox.
type fakeAPISandbox struct {
	available bool
	released  struct {
		projectID, threadID string
	}
}

func (f *fakeAPISandbox) Available() bool { return f.available }

func (f *fakeAPISandbox) Exec(ctx context.Context, projectID, threadID, cmd string, args []string) (agentbridge.SandboxOutput, error) {
	return agentbridge.SandboxOutput{}, nil
}

func (f *fakeAPISandbox) Shell(ctx context.Context, projectID, threadID, command string) (agentbridge.SandboxOutput, error) {
	return agentbridge.SandboxOutput{}, nil
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

func sandboxRuntime(sandbox agentbridge.Sandbox) *cloudagent.Runtime {
	return &cloudagent.Runtime{Provider: &agentbridge.RuntimeProvider{Sandbox: sandbox}}
}

func TestModulesSandboxAvailableFlag(t *testing.T) {
	store := openAgentStore(t)
	e := agentEcho(store)
	e.GET("/agents/modules/with", func(c echo.Context) error {
		h := &CloudAgentHandler{store: store, svc: agentrun.NewServiceWithStore(agentrun.Config{Sandbox: true}, store, sandboxRuntime(&fakeAPISandbox{available: true}), nil), writable: true}
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
	h := &CloudAgentHandler{store: store, svc: agentrun.NewServiceWithStore(agentrun.Config{Sandbox: sb.Available()}, store, sandboxRuntime(sb), nil), writable: true}
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
	h := &CloudAgentHandler{store: store, svc: agentrun.NewServiceWithStore(agentrun.Config{Sandbox: sb.Available()}, store, sandboxRuntime(sb), nil), writable: true}
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
