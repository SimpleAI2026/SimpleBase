package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

// TestAgentRunUsesProviderSavedViaSettingsAPI 复现：llm.enabled 且 YAML 没有供应商时，
// 设置 API 写入系统库凭证后，云助手 run 不再返回 llm_not_configured，且无需重启进程。
func TestAgentRunUsesProviderSavedViaSettingsAPI(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := systemdb.ApplySystemMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := systemdb.NewStoreForTest(db)
	cat := catalog.NewService(catalog.NewSQLRepository(db), objectstore.KeyBuilder{}, nil, objectstore.DuckLakeStorage{}, nil)

	one := fakellm.New(nil, fakellm.Script{Steps: []fakellm.Step{
		{Content: "from-one"},
		{FinishReason: "stop"},
		{PromptTokens: 4, CompletionTokens: 2},
	}})
	t.Cleanup(one.Close)
	two := fakellm.New(nil, fakellm.Script{Steps: []fakellm.Step{
		{Content: "from-two"},
		{FinishReason: "stop"},
	}})
	t.Cleanup(two.Close)

	// 与 app.assembleDeps 相同：只读系统库项目凭证，不注入 YAML 实例供应商。
	resolver := llmgateway.NewFirstUsableResolver(
		llmgateway.NewProjectCredResolver(llmgateway.NewSystemCredSource(store)),
		llmgateway.NewCatalogResolver(cat, nil),
	)
	gw := llmgateway.NewService(resolver, nil, nil)
	rt := &cloudagent.Runtime{
		LLM:          NewCloudAgentLLM(NewLLMService(gw)),
		Settings:     cloudagent.NewSettingsAccess(store),
		ToolProtocol: "native",
		RunTimeout:   30 * time.Second,
	}
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = errorHandler(Dependencies{})
	h := &cloudAgentHandler{store: store, runtime: rt, llm: NewLLMService(gw)}
	registerAgentRoutes(e, h)
	sess := &llmSessionHandler{store: store}
	credh := &llmProviderCredHandler{store: store}
	e.PUT("/llm/settings", sess.PutSettings)
	e.PUT("/llm/providers/:provider", credh.PutProviderCred)

	const projectID = "proj-cred"
	const apiKey = "sk-test-provider-cred"
	req := func(project, method, path string, body any) *httptest.ResponseRecorder {
		return savedProviderReq(e, project, method, path, body)
	}

	threadID := savedProviderThread(t, req, projectID)
	before := savedProviderRun(t, req, projectID, threadID, "hello")
	if savedProviderCode(before) != "llm_not_configured" {
		t.Fatalf("before save: %+v", before)
	}
	if one.RequestCount() != 0 {
		t.Fatal("unconfigured run must not call the model")
	}

	settings := req(projectID, http.MethodPut, "/llm/settings", map[string]any{
		"default_provider": "custom_openai",
		"default_model":    "fake-model",
	})
	if settings.Code != http.StatusOK {
		t.Fatalf("put settings %d %s", settings.Code, settings.Body.String())
	}
	saved := req(projectID, http.MethodPut, "/llm/providers/custom_openai", map[string]any{
		"credentials": map[string]string{
			"api_key":  apiKey,
			"base_url": one.URL + "/v1",
		},
		"default_model": "fake-model",
	})
	if saved.Code != http.StatusOK {
		t.Fatalf("put provider %d %s", saved.Code, saved.Body.String())
	}
	if strings.Contains(saved.Body.String(), apiKey) {
		t.Fatal("provider response leaked api key")
	}

	after := savedProviderRun(t, req, projectID, threadID, "hello")
	if savedProviderCode(after) == "llm_not_configured" {
		t.Fatalf("configured run still llm_not_configured: %+v", after)
	}
	if code := savedProviderCode(after); code != "" {
		t.Fatalf("configured run error %s: %+v", code, after)
	}
	if !sseContains(after, "from-one") {
		t.Fatalf("events=%+v", after)
	}
	if strings.Contains(joinSSE(after), apiKey) {
		t.Fatal("run events leaked api key")
	}
	reqs := one.Requests()
	if len(reqs) == 0 || reqs[0].Model != "fake-model" {
		t.Fatalf("model requests=%+v", reqs)
	}

	otherThread := savedProviderThread(t, req, "proj-other")
	other := savedProviderRun(t, req, "proj-other", otherThread, "hello")
	if savedProviderCode(other) != "llm_not_configured" {
		t.Fatalf("other project should stay unconfigured: %+v", other)
	}
	if one.RequestCount() != len(reqs) {
		t.Fatal("other project must not use proj-cred credentials")
	}

	updated := req(projectID, http.MethodPut, "/llm/providers/custom_openai", map[string]any{
		"credentials": map[string]string{
			"api_key":  "",
			"base_url": two.URL + "/v1",
		},
		"default_model": "fake-model",
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("update provider %d %s", updated.Code, updated.Body.String())
	}
	again := savedProviderRun(t, req, projectID, threadID, "hello again")
	if savedProviderCode(again) != "" || !sseContains(again, "from-two") {
		t.Fatalf("updated base url was not used without restart: %+v", again)
	}
	if two.RequestCount() < 1 {
		t.Fatal("second fake server was not called")
	}
}

func savedProviderReq(e *echo.Echo, project, method, path string, body any) *httptest.ResponseRecorder {
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, strings.NewReader(string(b)))
		r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	ctx := WithProject(r.Context(), ProjectContext{ID: project, TenantID: catalog.ReservedTenantID})
	ctx = WithPrincipal(ctx, auth.Principal{APIKeyID: "k1", TenantID: catalog.ReservedTenantID})
	r = r.WithContext(ctx)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)
	return rec
}

func savedProviderThread(t *testing.T, req func(project, method, path string, body any) *httptest.ResponseRecorder, project string) string {
	t.Helper()
	rec := req(project, http.MethodPost, "/agent-threads", map[string]any{"title": "t"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create thread: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.ID == "" {
		t.Fatalf("thread id: %v %s", err, rec.Body.String())
	}
	return out.ID
}

func savedProviderRun(t *testing.T, req func(project, method, path string, body any) *httptest.ResponseRecorder, project, threadID, content string) []sseEvent {
	t.Helper()
	rec := req(project, http.MethodPost, "/agent-threads/"+threadID+"/runs", map[string]any{
		"content": content, "stream": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create run: %d %s", rec.Code, rec.Body.String())
	}
	events, err := parseSSE(rec.Body.String())
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func savedProviderCode(events []sseEvent) string {
	for _, ev := range events {
		if ev.Type == "error" {
			return ev.Code
		}
	}
	return ""
}

func sseContains(events []sseEvent, text string) bool {
	for _, ev := range events {
		if strings.Contains(ev.Content, text) {
			return true
		}
	}
	return false
}

func joinSSE(events []sseEvent) string {
	var b strings.Builder
	for _, ev := range events {
		b.WriteString(ev.Content)
		b.WriteString(ev.Message)
		b.WriteString(ev.Code)
	}
	return b.String()
}
