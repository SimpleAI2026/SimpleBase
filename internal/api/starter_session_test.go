package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// 搭建会话（home-starter-prompts-plan §2.4 / §7）：创建前置校验与阶段白名单。

func starterAdminPrincipal() auth.Principal {
	return auth.Principal{
		APIKeyID: "k-admin", TenantID: catalog.ReservedTenantID,
		Permissions: map[auth.Permission]struct{}{auth.DatabaseAdmin: {}},
	}
}

func starterEnv(t *testing.T, skillsCLI, writable bool) (*echo.Echo, *systemdb.Store, systemdb.CloudAgent, systemdb.CloudAgent) {
	t.Helper()
	store := openAgentStore(t)
	if err := store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	var general, database systemdb.CloudAgent
	agents, err := store.ListCloudAgents(context.Background(), "testprj1")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range agents {
		switch a.BuiltinKey {
		case "general":
			general = a
		case "database":
			database = a
		}
	}
	if general.ID == "" || database.ID == "" {
		t.Fatalf("seed agents: general=%q database=%q", general.ID, database.ID)
	}
	w := writable
	h := &cloudAgentHandler{store: store, runtime: &cloudagent.Runtime{SkillsCLI: skillsCLI}, writable: &w}
	e := echo.New()
	e.POST("/agent-threads", h.CreateThread)
	e.POST("/agent-threads/:threadID/runs", h.CreateRun)
	return e, store, general, database
}

func createStarterThread(t *testing.T, e *echo.Echo, agentID, template string) string {
	t.Helper()
	body := map[string]any{"title": "s", "agent_id": agentID}
	if template != "" {
		body["starter_template"] = template
	}
	rec := agentReqAs(e, http.MethodPost, "/agent-threads", body, starterAdminPrincipal())
	if rec.Code != http.StatusCreated {
		t.Fatalf("thread: %d %s", rec.Code, rec.Body.String())
	}
	var th struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &th); err != nil {
		t.Fatal(err)
	}
	return th.ID
}

func errorCode(rec interface{ Bytes() []byte }) string {
	var body APIErrorBody
	_ = json.Unmarshal(rec.Bytes(), &body)
	return body.Error.Code
}

func TestStarterThreadCreateAllowed(t *testing.T) {
	e, store, general, _ := starterEnv(t, true, true)
	rec := agentReqAs(e, http.MethodPost, "/agent-threads", map[string]any{"title": "d", "agent_id": general.ID, "starter_template": "dashboard"}, starterAdminPrincipal())
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		ID              string `json:"id"`
		StarterTemplate string `json:"starter_template"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.StarterTemplate != "dashboard" {
		t.Fatalf("dto=%s err=%v", rec.Body.String(), err)
	}
	th, err := store.GetAgentThread(context.Background(), "testprj1", out.ID)
	if err != nil || th.StarterTemplate != "dashboard" {
		t.Fatalf("persisted=%+v err=%v", th, err)
	}
}

func TestStarterThreadCreateGuards(t *testing.T) {
	delegated := starterAdminPrincipal()
	delegated.Delegated = true
	cases := []struct {
		name      string
		skillsCLI bool
		writable  bool
		useDB     bool
		noAgent   bool
		template  string
		principal auth.Principal
		wantCode  int
	}{
		{"non-general agent", true, true, true, false, "dashboard", starterAdminPrincipal(), http.StatusBadRequest},
		{"no agent", true, true, false, true, "dashboard", starterAdminPrincipal(), http.StatusBadRequest},
		{"template not whitelisted", true, true, false, false, "nope", starterAdminPrincipal(), http.StatusBadRequest},
		{"write template deferred to B3", true, true, false, false, "todo", starterAdminPrincipal(), http.StatusBadRequest},
		{"skills_cli disabled", false, true, false, false, "dashboard", starterAdminPrincipal(), http.StatusBadRequest},
		{"readonly instance", true, false, false, false, "dashboard", starterAdminPrincipal(), http.StatusServiceUnavailable},
		{"missing database:admin", true, true, false, false, "dashboard", writePrincipal(), http.StatusBadRequest},
		{"delegated principal", true, true, false, false, "dashboard", delegated, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, store, general, database := starterEnv(t, tc.skillsCLI, tc.writable)
			agentID := general.ID
			if tc.useDB {
				agentID = database.ID
			}
			if tc.noAgent {
				agentID = ""
			}
			rec := agentReqAs(e, http.MethodPost, "/agent-threads", map[string]any{"title": "d", "agent_id": agentID, "starter_template": tc.template}, tc.principal)
			if rec.Code != tc.wantCode {
				t.Fatalf("status=%d want %d body=%s", rec.Code, tc.wantCode, rec.Body.String())
			}
			if tc.wantCode == http.StatusBadRequest && errorCode(rec.Body) != "starter_not_allowed" {
				t.Fatalf("code=%q body=%s", errorCode(rec.Body), rec.Body.String())
			}
			threads, err := store.ListAgentThreads(context.Background(), "testprj1", "", 10)
			if err != nil || len(threads) != 0 {
				t.Fatalf("rejected request must not create a thread: %+v err=%v", threads, err)
			}
		})
	}
}

// 系统项目拒绝：搭建会话会写资源，不能落在系统项目里。
func TestStarterRejectsSystemProject(t *testing.T) {
	_, _, general, _ := starterEnv(t, true, true)
	h := &cloudAgentHandler{runtime: &cloudagent.Runtime{SkillsCLI: true}}
	err := h.checkStarterAllowed(catalog.ReservedSystemProjectID, "dashboard", general, starterAdminPrincipal(), true)
	if err != ErrStarterNotAllowed {
		t.Fatalf("system project must be rejected: %v", err)
	}
}

func TestStarterErrorMappings(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{{ErrStarterNotAllowed, "starter_not_allowed"}, {ErrStarterStageInvalid, "starter_stage_invalid"}} {
		ae := Error(tc.err, "rid")
		if ae == nil || ae.HTTPStatus != http.StatusBadRequest || ae.Body.Error.Code != tc.code {
			t.Fatalf("%v → %+v", tc.err, ae)
		}
	}
}

// 非搭建会话传 starter_stage 必须被拒绝；普通通用会话的技能仍由 module 决定（nil）。
func TestStarterStageRejectedOnPlainThread(t *testing.T) {
	e, _, general, _ := starterEnv(t, true, true)
	id := createStarterThread(t, e, general.ID, "")
	rec := agentReqAs(e, http.MethodPost, "/agent-threads/"+id+"/runs", map[string]any{"content": "x", "starter_stage": "data"}, starterAdminPrincipal())
	if rec.Code != http.StatusBadRequest || errorCode(rec.Body) != "starter_stage_invalid" {
		t.Fatalf("plain thread with stage: %d %s", rec.Code, rec.Body.String())
	}
	if got := cloudagent.SkillsForModule(cloudagent.ModuleGeneral); got != nil {
		t.Fatalf("general module must still load no skills: %v", got)
	}
}

func TestStarterStageWhitelist(t *testing.T) {
	e, _, general, _ := starterEnv(t, true, true)
	id := createStarterThread(t, e, general.ID, "dashboard")
	for _, stage := range []string{"nope", "DATA", "kv", "../data"} {
		rec := agentReqAs(e, http.MethodPost, "/agent-threads/"+id+"/runs", map[string]any{"content": "x", "starter_stage": stage}, starterAdminPrincipal())
		if rec.Code != http.StatusBadRequest || errorCode(rec.Body) != "starter_stage_invalid" {
			t.Fatalf("stage %q: %d %s", stage, rec.Code, rec.Body.String())
		}
	}
	// 合法阶段通过校验后停在「模型未配置」，说明没有被阶段白名单拦截。
	for _, stage := range cloudagent.StarterStages() {
		rec := agentReqAs(e, http.MethodPost, "/agent-threads/"+id+"/runs", map[string]any{"content": "x", "starter_stage": stage}, starterAdminPrincipal())
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("valid stage %q: %d %s", stage, rec.Code, rec.Body.String())
		}
	}
}
