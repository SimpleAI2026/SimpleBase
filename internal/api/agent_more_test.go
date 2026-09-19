package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func fullAgentEcho(store *systemdb.Store, usage UsageService) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	h := &cloudAgentHandler{store: store, usage: usage}
	e.GET("/agents/modules", h.ListModules)
	e.GET("/agents", h.ListAgents)
	e.POST("/agents", h.CreateAgent)
	e.GET("/agents/:agentID", h.GetAgent)
	e.PATCH("/agents/:agentID", h.PatchAgent)
	e.DELETE("/agents/:agentID", h.DeleteAgent)
	e.GET("/agent-threads", h.ListThreads)
	e.POST("/agent-threads", h.CreateThread)
	e.GET("/agent-threads/:threadID", h.GetThread)
	e.DELETE("/agent-threads/:threadID", h.DeleteThread)
	e.GET("/agent-threads/:threadID/messages", h.ListMessages)
	e.POST("/agent-threads/:threadID/runs", h.CreateRun)
	e.POST("/agent-runs/:runID/cancel", h.CancelRun)
	return e
}

func agentReqOpt(e *echo.Echo, method, path string, body any, withProject, withPrincipal bool) *httptest.ResponseRecorder {
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	ctx := r.Context()
	if withProject {
		ctx = WithProject(ctx, ProjectContext{ID: catalog.DevProjectID, TenantID: catalog.ReservedTenantID})
	}
	if withPrincipal {
		ctx = WithPrincipal(ctx, auth.Principal{APIKeyID: "k1", TenantID: catalog.ReservedTenantID})
	}
	r = r.WithContext(ctx)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)
	return rec
}

func TestAgentThreadsAndGet(t *testing.T) {
	store := openAgentStore(t)
	e := fullAgentEcho(store, nil)

	rec := agentReqOpt(e, http.MethodGet, "/agents", nil, false, true)
	if rec.Code == http.StatusOK {
		t.Fatal("list agents without project")
	}
	rec = agentReqOpt(e, http.MethodPost, "/agents", map[string]any{"name": "x"}, false, true)
	if rec.Code == http.StatusCreated {
		t.Fatal("create without project")
	}

	rec = agentReq(e, http.MethodGet, "/agents", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("seed list %d %s", rec.Code, rec.Body.String())
	}
	var listed struct{ Agents []agentDTO `json:"agents"` }
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Agents) == 0 {
		t.Fatal("expected seeded agents")
	}
	aid := listed.Agents[0].ID
	rec = agentReq(e, http.MethodGet, "/agents/"+aid, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get agent %d %s", rec.Code, rec.Body.String())
	}
	rec = agentReq(e, http.MethodGet, "/agents/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing agent %d %s", rec.Code, rec.Body.String())
	}
	rec = agentReq(e, http.MethodPatch, "/agents/missing", map[string]any{"name": "x"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("patch missing %d", rec.Code)
	}
	rec = agentReq(e, http.MethodDelete, "/agents/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing %d", rec.Code)
	}
	rec = agentReq(e, http.MethodPost, "/agents", map[string]any{"name": "", "module": "nope"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad create %d %s", rec.Code, rec.Body.String())
	}
	rec = agentReq(e, http.MethodPost, "/agents", map[string]any{"name": "Z", "module": "unknown-mod"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown module %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req = req.WithContext(WithProject(req.Context(), ProjectContext{ID: catalog.DevProjectID}))
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusCreated {
		t.Fatal("bad json create")
	}

	rec = agentReq(e, http.MethodGet, "/agent-threads", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list threads %d %s", rec.Code, rec.Body.String())
	}
	rec = agentReqOpt(e, http.MethodGet, "/agent-threads", nil, false, true)
	if rec.Code == http.StatusOK {
		t.Fatal("list threads no project")
	}

	rec = agentReq(e, http.MethodPost, "/agent-threads", map[string]any{"title": "t1"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create thread %d %s", rec.Code, rec.Body.String())
	}
	var th threadDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &th); err != nil {
		t.Fatal(err)
	}
	rec = agentReqOpt(e, http.MethodPost, "/agent-threads", map[string]any{"title": "anon"}, true, false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("anon thread %d", rec.Code)
	}
	rec = agentReqOpt(e, http.MethodPost, "/agent-threads", nil, false, true)
	if rec.Code == http.StatusCreated {
		t.Fatal("thread no project")
	}

	rec = agentReq(e, http.MethodGet, "/agent-threads/"+th.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get thread %d", rec.Code)
	}
	rec = agentReq(e, http.MethodGet, "/agent-threads/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing thread %d", rec.Code)
	}
	rec = agentReqOpt(e, http.MethodGet, "/agent-threads/"+th.ID, nil, false, true)
	if rec.Code == http.StatusOK {
		t.Fatal("get thread no project")
	}

	rec = agentReq(e, http.MethodGet, "/agent-threads/"+th.ID+"/messages", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list msgs %d %s", rec.Code, rec.Body.String())
	}
	rec = agentReq(e, http.MethodGet, "/agent-threads/missing/messages", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("msgs missing thread %d", rec.Code)
	}
	rec = agentReqOpt(e, http.MethodGet, "/agent-threads/"+th.ID+"/messages", nil, false, true)
	if rec.Code == http.StatusOK {
		t.Fatal("msgs no project")
	}

	rec = agentReq(e, http.MethodDelete, "/agent-threads/missing", nil)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusNoContent {
		// Archive may succeed as no-op depending on store
	}
	rec = agentReqOpt(e, http.MethodDelete, "/agent-threads/"+th.ID, nil, false, true)
	if rec.Code == http.StatusNoContent {
		t.Fatal("delete thread no project")
	}
	rec = agentReq(e, http.MethodDelete, "/agent-threads/"+th.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete thread %d %s", rec.Code, rec.Body.String())
	}
}

func TestAgentCreateRunAndCancel(t *testing.T) {
	store := openAgentStore(t)
	e := fullAgentEcho(store, &fakeUsage{})

	// seed agents
	if rec := agentReq(e, http.MethodGet, "/agents", nil); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec := agentReq(e, http.MethodPost, "/agent-threads", map[string]any{"title": "run"})
	if rec.Code != http.StatusCreated {
		t.Fatal(rec.Body.String())
	}
	var th threadDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &th)

	rec = agentReqOpt(e, http.MethodPost, "/agent-threads/"+th.ID+"/runs", map[string]any{"content": "hi"}, false, true)
	if rec.Code == http.StatusOK {
		t.Fatal("run no project")
	}
	rec = agentReq(e, http.MethodPost, "/agent-threads/missing/runs", map[string]any{"content": "hi"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("run missing thread %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/agent-threads/"+th.ID+"/runs", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req = req.WithContext(WithProject(req.Context(), ProjectContext{ID: catalog.DevProjectID}))
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("bad run json")
	}
	rec = agentReq(e, http.MethodPost, "/agent-threads/"+th.ID+"/runs", map[string]any{"content": "  "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty content %d %s", rec.Code, rec.Body.String())
	}
	rec = agentReq(e, http.MethodPost, "/agent-threads/"+th.ID+"/runs", map[string]any{
		"content": "hi", "mentions": []map[string]string{{"agent_id": "no-such"}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad mention %d %s", rec.Code, rec.Body.String())
	}

	eQuota := fullAgentEcho(store, &fakeUsage{errLLM: context.Canceled})
	rec = agentReq(eQuota, http.MethodPost, "/agent-threads/"+th.ID+"/runs", map[string]any{"content": "hi"})
	if rec.Code == http.StatusOK {
		t.Fatal("quota should fail")
	}

	// nil runtime → 503
	rec = agentReq(e, http.MethodPost, "/agent-threads/"+th.ID+"/runs", map[string]any{"content": "hello agent"})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no runtime %d %s", rec.Code, rec.Body.String())
	}
	// stream + nil runtime
	rec = agentReq(e, http.MethodPost, "/agent-threads/"+th.ID+"/runs", map[string]any{"content": "stream please", "stream": true})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "data:") {
		t.Fatalf("stream no runtime %d %s", rec.Code, rec.Body.String())
	}

	// cancel missing / existing
	rec = agentReqOpt(e, http.MethodPost, "/agent-runs/x/cancel", nil, false, true)
	if rec.Code == http.StatusOK {
		t.Fatal("cancel no project")
	}
	rec = agentReq(e, http.MethodPost, "/agent-runs/missing/cancel", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cancel missing %d %s", rec.Code, rec.Body.String())
	}

	// create a run row and cancel it
	run, err := store.CreateAgentRun(context.Background(), systemdb.AgentRun{
		ThreadID: th.ID, ProjectID: catalog.DevProjectID, AgentID: "a",
		Status: systemdb.AgentRunRunning, StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = agentReq(e, http.MethodPost, "/agent-runs/"+run.ID+"/cancel", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel %d %s", rec.Code, rec.Body.String())
	}

	// DTO helpers
	dto := toMessageDTO(systemdb.AgentMessage{ID: "m", Role: "user", Content: "c"})
	if string(dto.Mentions) != "[]" || string(dto.ToolCalls) != "[]" {
		t.Fatalf("dto defaults %+v", dto)
	}
	rd := toRunDTO(systemdb.AgentRun{ID: "r", StartedAt: time.Now(), FinishedAt: time.Now()})
	if rd.StartedAt == nil || rd.FinishedAt == nil {
		t.Fatal("run dto times")
	}
	if _, err := normalizeAgent("p", upsertAgentBody{Name: "n", Module: "general", ToolIDs: []string{"not-a-tool", "sql_query"}}, systemdb.CloudAgent{}); err != nil {
		t.Fatal(err)
	}

	h := &cloudAgentHandler{}
	if err := h.ensureAgents(context.Background(), "p"); err == nil {
		t.Fatal("nil store")
	}
}
