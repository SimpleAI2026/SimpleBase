package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func openAgentStore(t *testing.T) *systemdb.Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return systemdb.NewStoreForTest(db)
}

func agentEcho(store *systemdb.Store) *echo.Echo {
	e := echo.New()
	h := &cloudAgentHandler{store: store, runtime: &gocloudagent.Runtime{SkillsCLI: true}}
	e.GET("/agents/modules", h.ListModules)
	e.GET("/agents", h.ListAgents)
	e.POST("/agents", h.CreateAgent)
	e.GET("/agents/:agentID", h.GetAgent)
	e.PATCH("/agents/:agentID", h.PatchAgent)
	e.DELETE("/agents/:agentID", h.DeleteAgent)
	e.POST("/agent-threads", h.CreateThread)
	e.GET("/agent-threads/:threadID/messages", h.ListMessages)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return next
	})
	return e
}

func agentReq(e *echo.Echo, method, path string, body any) *httptest.ResponseRecorder {
	return agentReqAs(e, method, path, body, auth.Principal{APIKeyID: "k1", TenantID: catalog.ReservedTenantID})
}

// agentReqAs 按指定 Principal 发请求（SEC-04 测试用）。
func agentReqAs(e *echo.Echo, method, path string, body any, p auth.Principal) *httptest.ResponseRecorder {
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	ctx := WithProject(r.Context(), ProjectContext{ID: "testprj1", TenantID: catalog.ReservedTenantID})
	ctx = WithPrincipal(ctx, p)
	r = r.WithContext(ctx)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)
	return rec
}

// writePrincipal 返回持 DatabaseWrite 的测试主体（SEC-04 后取消/确认需要）。
func writePrincipal() auth.Principal {
	return auth.Principal{
		APIKeyID: "k-rw", TenantID: catalog.ReservedTenantID,
		Permissions: map[auth.Permission]struct{}{auth.DatabaseWrite: {}},
	}
}

func TestAgentCRUDAndSeed(t *testing.T) {
	store := openAgentStore(t)
	e := agentEcho(store)

	rec := agentReq(e, http.MethodGet, "/agents/modules", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("modules status=%d body=%s", rec.Code, rec.Body.String())
	}
	var directory struct {
		Modules []gocloudagent.ModuleInfo `json:"modules"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &directory); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"kv", "gofunction", "cron"} {
		found := false
		for _, module := range directory.Modules {
			if module.ID == id {
				found = true
				if len(module.DefaultTools) != 0 {
					t.Fatalf("module %s advertises missing built-in tools: %v", id, module.DefaultTools)
				}
			}
		}
		if !found {
			t.Fatalf("module %s missing from directory", id)
		}
	}
	rec = agentReq(e, http.MethodGet, "/agents", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Agents []agentDTO `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Agents) != 7 {
		t.Fatalf("seeded agents=%d", len(listed.Agents))
	}
	if listed.Agents[0].BuiltinKey != "general" {
		t.Fatalf("general agent should sort first, got %+v", listed.Agents[0])
	}

	rec = agentReq(e, http.MethodPost, "/agents", map[string]any{
		"name": "Custom", "module": "general", "system_prompt": "Be brief", "tool_ids": []string{},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created agentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rec = agentReq(e, http.MethodPatch, "/agents/"+created.ID, map[string]any{"name": "Custom2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = agentReq(e, http.MethodDelete, "/agents/"+created.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", rec.Code)
	}

	rec = agentReq(e, http.MethodPost, "/agent-threads", map[string]any{"title": "chat"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("thread status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatisticalModulesUnavailableWithoutCLI(t *testing.T) {
	store := openAgentStore(t)
	e := agentEcho(store)
	h := &cloudAgentHandler{store: store}
	e.GET("/without-cli/modules", h.ListModules)
	e.GET("/without-cli/agents", h.ListAgents)
	e.POST("/without-cli/agents", h.CreateAgent)
	modules := agentReq(e, http.MethodGet, "/without-cli/modules", nil)
	for _, id := range []string{`"id":"kv"`, `"id":"gofunction"`, `"id":"cron"`} {
		if bytes.Contains(modules.Body.Bytes(), []byte(id)) {
			t.Fatalf("module %s advertised without CLI: %s", id, modules.Body.String())
		}
	}
	rec := agentReq(e, http.MethodGet, "/without-cli/agents", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list agents without CLI: %d %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Agents []agentDTO `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Agents) != 4 {
		t.Fatalf("non-CLI agent count=%d", len(listed.Agents))
	}
	rec = agentReq(e, http.MethodPost, "/without-cli/agents", map[string]any{"name": "KV", "module": "kv"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("CLI-only module created without CLI: %d %s", rec.Code, rec.Body.String())
	}
	withCLI := agentReq(e, http.MethodGet, "/agents", nil)
	if withCLI.Code != http.StatusOK {
		t.Fatalf("enable CLI agents: %d %s", withCLI.Code, withCLI.Body.String())
	}
	rec = agentReq(e, http.MethodGet, "/without-cli/agents", nil)
	var after struct {
		Agents []agentDTO `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if len(after.Agents) != 7 {
		t.Fatalf("existing CLI-only agents when CLI disabled=%d", len(after.Agents))
	}
}

func TestExistingAgentsRejectRunsWhenRequiredCapabilityDisabled(t *testing.T) {
	ctx := context.Background()
	store := openAgentStore(t)
	if err := store.SeedDefaultCloudAgentsWithCapabilities(ctx, "testprj1", true, true); err != nil {
		t.Fatal(err)
	}
	agents, err := store.ListCloudAgents(ctx, "testprj1")
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range []string{"kv", "gofunction", "cron", "sandbox"} {
		var agent systemdb.CloudAgent
		for _, candidate := range agents {
			if candidate.Module == module {
				agent = candidate
				break
			}
		}
		if agent.ID == "" {
			t.Fatalf("builtin %s not found", module)
		}
		t.Run(module, func(t *testing.T) {
			e := agentEcho(store)
			h := &cloudAgentHandler{store: store, runtime: &gocloudagent.Runtime{}}
			e.POST("/capability/:threadID/runs", h.CreateRun)
			created := agentReq(e, http.MethodPost, "/agent-threads", map[string]any{"agent_id": agent.ID})
			if created.Code != http.StatusCreated {
				t.Fatalf("thread status=%d body=%s", created.Code, created.Body.String())
			}
			var thread struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(created.Body.Bytes(), &thread); err != nil {
				t.Fatal(err)
			}
			for _, body := range []map[string]any{
				{"content": "list resources", "mentions": []map[string]string{{"agent_id": agent.ID}}},
				{"content": "list resources", "mentions": []map[string]string{{"agent_id": agent.ID}}, "retry_of_run_id": "invalid"},
			} {
				rec := agentReq(e, http.MethodPost, "/capability/"+thread.ID+"/runs", body)
				var got APIErrorBody
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				code := "cli_skills_unavailable"
				if module == "sandbox" {
					code = "sandbox_unavailable"
				}
				if rec.Code != http.StatusServiceUnavailable || got.Error.Code != code {
					t.Fatalf("disabled %s must fail before creating run: %d %s", module, rec.Code, rec.Body.String())
				}
			}
		})
	}
}

func TestCreateRunGeneralThreadDatabaseSkillContract(t *testing.T) {
	store := openAgentStore(t)
	if err := store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	agents, err := store.ListCloudAgents(context.Background(), "testprj1")
	if err != nil {
		t.Fatal(err)
	}
	var general, database systemdb.CloudAgent
	for _, agent := range agents {
		switch agent.BuiltinKey {
		case "general":
			general = agent
		case "database":
			database = agent
		}
	}
	if general.ID == "" || database.ID == "" {
		t.Fatalf("missing seeded agents: general=%q database=%q", general.ID, database.ID)
	}

	e := agentEcho(store)
	e.POST("/agent-threads/:threadID/runs", (&cloudAgentHandler{store: store}).CreateRun)
	threadRec := agentReq(e, http.MethodPost, "/agent-threads", map[string]any{
		"title": "general", "agent_id": general.ID,
	})
	if threadRec.Code != http.StatusCreated {
		t.Fatalf("create thread status=%d body=%s", threadRec.Code, threadRec.Body.String())
	}
	var thread struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(threadRec.Body.Bytes(), &thread); err != nil {
		t.Fatal(err)
	}
	path := "/agent-threads/" + thread.ID + "/runs"
	body := map[string]any{
		"content": "@数据库 查询统计表", "mentions": []map[string]string{{"agent_id": general.ID}},
		"stream": false,
	}
	// 无 LLM 时应停在模型未配置，而不是错误地把技能当成另一个助手。
	rec := agentReq(e, http.MethodPost, path, body)
	if rec.Code != http.StatusServiceUnavailable || !bytes.Contains(rec.Body.Bytes(), []byte("llm_not_configured")) {
		t.Fatalf("general thread with @数据库 text: status=%d body=%s", rec.Code, rec.Body.String())
	}
	body["mentions"] = []map[string]string{{"agent_id": database.ID}}
	rec = agentReq(e, http.MethodPost, path, body)
	var switched APIErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &switched); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusConflict || switched.Error.Code != "agent_switch_required" {
		t.Fatalf("cross-agent mention should request a switch: status=%d body=%s", rec.Code, rec.Body.String())
	}
	body["mentions"] = []map[string]string{{"agent_id": general.ID}, {"agent_id": database.ID}}
	rec = agentReq(e, http.MethodPost, path, body)
	var multiple APIErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &multiple); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest || multiple.Error.Code != "multiple_agents_not_supported" {
		t.Fatalf("multiple agents must not silently select the first: status=%d body=%s", rec.Code, rec.Body.String())
	}
	body["mentions"] = nil
	rec = agentReq(e, http.MethodPost, path, body)
	if rec.Code != http.StatusServiceUnavailable || !bytes.Contains(rec.Body.Bytes(), []byte("llm_not_configured")) {
		t.Fatalf("no mention must use thread owner: status=%d body=%s", rec.Code, rec.Body.String())
	}
	databaseThread := agentReq(e, http.MethodPost, "/agent-threads", map[string]any{
		"title": "database", "agent_id": database.ID,
	})
	if databaseThread.Code != http.StatusCreated {
		t.Fatalf("create database thread status=%d body=%s", databaseThread.Code, databaseThread.Body.String())
	}
	var specialized struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(databaseThread.Body.Bytes(), &specialized); err != nil {
		t.Fatal(err)
	}
	rec = agentReq(e, http.MethodPost, "/agent-threads/"+specialized.ID+"/runs", body)
	if rec.Code != http.StatusServiceUnavailable || !bytes.Contains(rec.Body.Bytes(), []byte("llm_not_configured")) {
		t.Fatalf("no mention in specialist thread must use its owner: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGoFunctionDTOAndScheduleGet(t *testing.T) {
	store := openAgentStore(t)
	e := echo.New()
	h := &agentScheduleHandler{store: store}
	e.GET("/s/:scheduleID", h.GetSchedule)
	e.GET("/s/:scheduleID/runs", h.ListScheduleRuns)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithProject(c.Request().Context(), ProjectContext{ID: "testprj1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	// re-register after middleware? echo Use is global; routes already registered.
	// Serve with injected context via wrapper
	wrap := echo.New()
	wrap.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithProject(c.Request().Context(), ProjectContext{ID: "testprj1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	wrap.GET("/s/:scheduleID", h.GetSchedule)
	wrap.GET("/s/:scheduleID/runs", h.ListScheduleRuns)
	rec := doRequest(wrap, http.MethodGet, "/s/missing", nil)
	if rec.Code == http.StatusOK {
		t.Fatal("missing schedule")
	}
	rec = doRequest(wrap, http.MethodGet, "/s/missing/runs", nil)
	if rec.Code == http.StatusOK {
		t.Fatal("missing schedule runs")
	}
}
