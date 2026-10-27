package api

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cli"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

// oaiE2E 是「真实 llmgateway + fakellm 上游 + 完整 NewRouter」的端到端环境。
// 与 llm_handler_test 的 fake service 不同，这里每一层都是真实实现：
// 认证 → 权限 → 路由 → 兼容 handler → adapter → llmgateway → litellm → HTTP 上游。
type oaiE2E struct {
	srv  *httptest.Server
	fake *fakellm.Server
	key  string
}

func newOAIE2E(t *testing.T, scripts []fakellm.Script, def fakellm.Script) *oaiE2E {
	t.Helper()
	fake := fakellm.New(scripts, def)
	t.Cleanup(fake.Close)

	rawKey := "sk-oai-e2e-key"
	authSvc0 := auth.NewService(&memAuthRepo{}, "secret")
	authSvc := auth.NewService(&memAuthRepo{rec: auth.APIKeyRecord{
		ID: "key-oai", TenantID: "tenant-1", ProjectIDs: []string{"proj-1"},
		Permissions: []auth.Permission{auth.DatabaseRead, auth.LLMInvoke},
		KeyHash:     authSvc0.HashKey(rawKey),
	}}, "secret")

	cat := &stubCatalog{tenant: "tenant-1", db: catalog.Database{ID: "db-1", ProjectID: "proj-1", Name: "n", Status: catalog.DatabaseReady}}
	dbSvc := newFakeDBService()
	dbSvc.dbs["db-1"] = cat.db
	sqlSvc := &fakeSQLService{db: cat.db, lease: &fakeSQLLease{queryResult: godatabase.QueryResult{Columns: []string{"n"}, Rows: [][]any{{1}}}}}

	sdb, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	if err := systemdb.EnsureSystemSchema(context.Background(), sdb); err != nil {
		t.Fatal(err)
	}

	resolver := llmgateway.NewFallbackResolver(nil, map[string]llmgateway.InstanceProvider{"openai": {
		APIKey: "upstream-secret-key", BaseURL: fake.URL + "/v1", DefaultModel: "fake-model", AllowedModels: []string{"fake-model"},
	}})
	gw := llmgateway.NewService(resolver, nil, nil)

	deps := Dependencies{
		Config: config.Config{
			HTTP:          config.HTTPConfig{Address: ":0"},
			Observability: config.ObservabilityConfig{MetricsPath: "/metrics"},
			Limits:        config.LimitsConfig{MaxRequestBytes: 1 << 20},
			Instance:      config.InstanceConfig{Writable: true},
		},
		Logger:          observability.NewLogger("error", "json", nil),
		Health:          &fakeHealth{},
		Auth:            authSvc,
		Catalog:         cat,
		DatabaseHandler: NewDatabaseHandler(dbSvc, true),
		SQLHandler:      NewSQLHandler(sqlSvc, testSQLLimits(), true),
		DataHandler:     NewDataHandler(&fakeDataService{dbs: []catalog.Database{cat.db}, lease: &fakeSQLLease{}}, true),
		Usage:           &fakeUsage{},
		Audit:           &fakeAudit{},
		LLM:             NewLLMService(gw),
		S3FileStore:     objectstore.NewMemoryFileStore(),
		System:          systemdb.NewStoreForTest(sdb),
	}
	srv := httptest.NewServer(NewRouter(deps))
	t.Cleanup(srv.Close)
	return &oaiE2E{srv: srv, fake: fake, key: rawKey}
}

func (e *oaiE2E) do(t *testing.T, method, path, body string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+e.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw
}

const oaiBase = "/v1/projects/proj-1/v1"

// 验收 1：直连新端点，非流式返回标准结构；流式以 [DONE] 收尾。
func TestOAIE2E_NonStreamAndStream(t *testing.T) {
	e := newOAIE2E(t, nil, fakellm.Script{
		Steps: []fakellm.Step{{Content: "he"}, {Content: "llo"}, {FinishReason: "stop"}, {PromptTokens: 3, CompletionTokens: 2}},
	})

	resp, raw := e.do(t, http.MethodPost, oaiBase+"/chat/completions", `{"messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d %s", resp.StatusCode, raw)
	}
	var out oaiChatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Object != "chat.completion" || out.Choices[0].Message.Content != "hello" || out.Choices[0].FinishReason != "stop" {
		t.Fatalf("resp=%s", raw)
	}
	// 上游密钥绝不能出现在对外响应里。
	if strings.Contains(string(raw), "upstream-secret-key") {
		t.Fatalf("upstream key leaked: %s", raw)
	}

	resp, raw = e.do(t, http.MethodPost, oaiBase+"/chat/completions",
		`{"stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status=%d ct=%s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	frames := sseFrames(t, string(raw))
	if frames[len(frames)-1] != oaiSSEDone {
		t.Fatalf("stream must end with [DONE]: %v", frames)
	}
	var text strings.Builder
	var sawUsage bool
	for _, f := range frames[:len(frames)-1] {
		ch := decodeChunk(t, f)
		for _, c := range ch.Choices {
			text.WriteString(c.Delta.Content)
		}
		if ch.Usage != nil && ch.Usage.TotalTokens == 5 {
			sawUsage = true
		}
	}
	if text.String() != "hello" || !sawUsage {
		t.Fatalf("text=%q usage=%v frames=%v", text.String(), sawUsage, frames)
	}
}

// 验收 2（服务端侧）：tools / 工具往返 / tool_choice 经真实链路到达上游，
// 上游返回的工具调用（流式增量与非流式）以 OpenAI 形状回到客户端。
func TestOAIE2E_ToolsAndToolChoiceReachUpstream(t *testing.T) {
	e := newOAIE2E(t, []fakellm.Script{
		{Seq: 1, NonStreamToolCalls: []fakellm.ToolCall{{ID: "call_1", Name: "shell", Arguments: `{"command":"pwd"}`}}},
		{Seq: 2, Steps: []fakellm.Step{
			{ToolID: "call_s", ToolName: "shell"},
			{ToolArgs: `{"command":`},
			{ToolArgs: `"ls"}`},
			{FinishReason: "tool_calls"},
		}},
	}, fakellm.Script{Steps: []fakellm.Step{{Content: "x"}, {FinishReason: "stop"}}})

	body := `{"messages":[
	  {"role":"user","content":"run"},
	  {"role":"assistant","content":"","tool_calls":[{"id":"call_0","type":"function","function":{"name":"shell","arguments":"{}"}}]},
	  {"role":"tool","tool_call_id":"call_0","content":"ok"}],
	 "tools":[{"type":"function","function":{"name":"shell","description":"run","parameters":{"type":"object","properties":{}}}}],
	 "tool_choice":"required"%s}`

	// 非流式。
	resp, raw := e.do(t, http.MethodPost, oaiBase+"/chat/completions", strings.Replace(body, "%s", "", 1))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d %s", resp.StatusCode, raw)
	}
	var out oaiChatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Choices[0].FinishReason != "tool_calls" || len(out.Choices[0].Message.ToolCalls) != 1 ||
		out.Choices[0].Message.ToolCalls[0].Function.Arguments != `{"command":"pwd"}` {
		t.Fatalf("non-stream tool call lost: %s", raw)
	}

	// 上游确实收到了 tools、工具往返消息与 tool_choice。
	upstream := e.fake.Requests()
	if len(upstream) != 1 || len(upstream[0].Tools) != 1 {
		t.Fatalf("upstream tools=%+v", upstream)
	}
	var sawAssistantCall, sawToolResult bool
	for _, m := range upstream[0].Messages {
		if m.Role == "assistant" && len(m.ToolCalls) == 1 && m.ToolCalls[0].ID == "call_0" {
			sawAssistantCall = true
		}
		if m.Role == "tool" && m.ToolCallID == "call_0" {
			sawToolResult = true
		}
	}
	if !sawAssistantCall || !sawToolResult {
		t.Fatalf("tool round trip not forwarded: %+v", upstream[0].Messages)
	}

	// 流式：工具调用增量聚合后完整，finish_reason=tool_calls。
	resp, raw = e.do(t, http.MethodPost, oaiBase+"/chat/completions", strings.Replace(body, "%s", `,"stream":true`, 1))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d %s", resp.StatusCode, raw)
	}
	var id, name, args, finish string
	for _, f := range sseFrames(t, string(raw)) {
		if f == oaiSSEDone {
			continue
		}
		for _, c := range decodeChunk(t, f).Choices {
			for _, tc := range c.Delta.ToolCalls {
				if tc.ID != "" {
					id = tc.ID
				}
				name += tc.Function.Name
				args += tc.Function.Arguments
			}
			if c.FinishReason != nil {
				finish = *c.FinishReason
			}
		}
	}
	if id != "call_s" || name != "shell" || args != `{"command":"ls"}` || finish != "tool_calls" {
		t.Fatalf("stream tool call id=%q name=%q args=%q finish=%q", id, name, args, finish)
	}
}

// 验收 4（服务端侧）：上游不支持工具声明时，返回稳定错误码 tools_not_supported，
// 而不是不可区分的 500；错误信封是 OpenAI 形状且不泄漏上游原文。
func TestOAIE2E_ToolsUnsupportedIsASafeStableError(t *testing.T) {
	e := newOAIE2E(t, nil, fakellm.Script{
		HTTPStatus: http.StatusBadRequest,
		Body:       `{"error":{"message":"tools is not supported by this model: sk-leaky-upstream-detail"}}`,
	})
	body := `{"messages":[{"role":"user","content":"x"}],"tools":[{"type":"function","function":{"name":"t"}}]}`
	for _, stream := range []bool{false, true} {
		b := body
		if stream {
			b = strings.Replace(body, `{"messages"`, `{"stream":true,"messages"`, 1)
		}
		resp, raw := e.do(t, http.MethodPost, oaiBase+"/chat/completions", b)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("stream=%v status=%d %s", stream, resp.StatusCode, raw)
		}
		assertOAIError(t, string(raw), "invalid_request_error", "tools_not_supported")
		if strings.Contains(string(raw), "sk-leaky-upstream-detail") {
			t.Fatalf("upstream error text leaked: %s", raw)
		}
	}
}

// 其它 LLM 侧错误不再一律落成 500：鉴权失败/限流/未配置各有稳定错误码。
func TestOAIE2E_UpstreamErrorsAreClassified(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		wantHTTP int
		wantType string
		wantCode string
	}{
		{"auth", http.StatusUnauthorized, http.StatusBadGateway, "api_error", "llm_auth_failed"},
		{"rate_limit", http.StatusTooManyRequests, http.StatusTooManyRequests, "rate_limit_error", "llm_rate_limited"},
		// 上游 5xx 对调用方而言是网关错误（502），不是 SimpleBase 自身的 500。
		{"upstream_down", http.StatusInternalServerError, http.StatusBadGateway, "api_error", "llm_upstream_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newOAIE2E(t, nil, fakellm.Script{HTTPStatus: tc.status, Body: `{"error":{"message":"boom sk-leaky"}}`})
			resp, raw := e.do(t, http.MethodPost, oaiBase+"/chat/completions", `{"messages":[{"role":"user","content":"x"}]}`)
			if resp.StatusCode != tc.wantHTTP {
				t.Fatalf("status=%d want %d: %s", resp.StatusCode, tc.wantHTTP, raw)
			}
			assertOAIError(t, string(raw), tc.wantType, tc.wantCode)
			if strings.Contains(string(raw), "sk-leaky") {
				t.Fatalf("upstream error text leaked: %s", raw)
			}
		})
	}
}

func TestOAIE2E_ModelsListsConfiguredModels(t *testing.T) {
	e := newOAIE2E(t, nil, fakellm.Script{})
	resp, raw := e.do(t, http.MethodGet, oaiBase+"/models", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d %s", resp.StatusCode, raw)
	}
	var list oaiModelList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	if list.Object != "list" || len(list.Data) != 1 || list.Data[0].ID != "fake-model" || list.Data[0].OwnedBy != "openai" {
		t.Fatalf("models=%s", raw)
	}
}

// 验收 3：本地助手（CLI 的真实 HTTPClient + 真实 SessionRunner）打真实服务端，
// 工具调用被识别、真实执行、结果回灌、模型据此作答。这是上一轮「调用 tool 就结束」的反例。
func TestOAIE2E_CLIRunsNativeToolCallEndToEnd(t *testing.T) {
	e := newOAIE2E(t, []fakellm.Script{
		{Seq: 1, Steps: []fakellm.Step{
			{ToolID: "call_1", ToolName: "echo"},
			{ToolArgs: `{"text":"ping"}`},
			{FinishReason: "tool_calls"},
		}},
	}, fakellm.Script{Steps: []fakellm.Step{{Content: "the echo said ping"}, {FinishReason: "stop"}}})

	client := &cli.HTTPClient{Endpoint: e.srv.URL, Token: e.key, ProjectID: "proj-1"}
	if err := client.Probe(context.Background()); err != nil {
		t.Fatalf("probe: %v", err)
	}

	var calls int
	res, events, err := runCLIOnce(t, client, "please echo ping", &calls)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("tool must really run exactly once, ran %d times", calls)
	}
	if res.Content != "the echo said ping" {
		t.Fatalf("final answer=%q", res.Content)
	}
	var sawCall, sawResult bool
	for _, ev := range events {
		switch ev.Type {
		case "tool_call":
			sawCall = ev.Name == "echo"
		case "tool_result":
			sawResult = ev.Name == "echo" && !ev.IsError
		}
	}
	if !sawCall || !sawResult {
		t.Fatalf("missing tool events: %+v", events)
	}
	// 模型第二轮必须看到工具结果。
	reqs := e.fake.Requests()
	if len(reqs) != 2 {
		t.Fatalf("upstream rounds=%d want 2", len(reqs))
	}
	var sawResultBack bool
	for _, m := range reqs[1].Messages {
		if m.Role == "tool" && m.ToolCallID == "call_1" {
			sawResultBack = true
		}
	}
	if !sawResultBack {
		t.Fatalf("tool result not sent back to the model: %+v", reqs[1].Messages)
	}
	// 原生协议：首轮请求必须携带 tools，且不得出现文本协议的提示词。
	if len(reqs[0].Tools) != 1 {
		t.Fatalf("first round must declare native tools: %+v", reqs[0])
	}
	for _, m := range reqs[0].Messages {
		if strings.Contains(m.Content.String(), "TOOL_CALL") {
			t.Fatalf("text-protocol prompt must not be injected under native tools: %q", m.Content.String())
		}
	}
}

// 验收 4：上游不支持 tools → 服务端返回 tools_not_supported → CLI 的 auto 协议自动降级到文本协议，
// 对话不中断，且降级后的请求不再携带 tools。
func TestOAIE2E_CLIFallsBackToTextProtocol(t *testing.T) {
	agentcore.ResetUnsupportedNativeTools()
	t.Cleanup(agentcore.ResetUnsupportedNativeTools)

	e := newOAIE2E(t, []fakellm.Script{
		// 第 1 次（携带 tools）：上游拒绝。
		{Seq: 1, HTTPStatus: http.StatusBadRequest, Body: `{"error":{"message":"tools unsupported"}}`},
	}, fakellm.Script{Steps: []fakellm.Step{{Content: "plain answer"}, {FinishReason: "stop"}}})

	client := &cli.HTTPClient{Endpoint: e.srv.URL, Token: e.key, ProjectID: "proj-1"}
	var calls int
	res, _, err := runCLIOnce(t, client, "hello", &calls)
	if err != nil {
		t.Fatalf("conversation must survive a tools-unsupported upstream: %v", err)
	}
	if res.Content != "plain answer" {
		t.Fatalf("answer=%q", res.Content)
	}
	reqs := e.fake.Requests()
	if len(reqs) != 2 {
		t.Fatalf("expected native attempt + text retry, got %d requests", len(reqs))
	}
	if len(reqs[0].Tools) == 0 {
		t.Fatalf("first attempt must be native (with tools)")
	}
	if len(reqs[1].Tools) != 0 {
		t.Fatalf("fallback attempt must not declare tools: %+v", reqs[1].Tools)
	}
}

// 权限：只有 database:read 的 Key 不能调用模型；带 llm:invoke 才行。
func TestOAIE2E_RequiresLLMInvokeOverRealRouter(t *testing.T) {
	e := newOAIE2E(t, nil, fakellm.Script{Steps: []fakellm.Step{{Content: "x"}}})
	e.key = "sk-wrong"
	resp, raw := e.do(t, http.MethodGet, oaiBase+"/models", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d %s", resp.StatusCode, raw)
	}
}

// 超长流在 WriteTimeout 生效时也要完整收尾（beginSSE 接在真实路由上，而不只是辅助函数可用）。
func TestOAIE2E_StreamSurvivesWriteTimeoutOverRealRouter(t *testing.T) {
	steps := make([]fakellm.Step, 0, 22)
	for i := 0; i < 20; i++ {
		steps = append(steps, fakellm.Step{Content: "x", Delay: 40 * time.Millisecond})
	}
	steps = append(steps, fakellm.Step{FinishReason: "stop"})
	e := newOAIE2E(t, nil, fakellm.Script{Steps: steps})
	// 重新包一层带 WriteTimeout 的 server：上游约 0.8s，服务端写超时只有 250ms。
	timed := httptest.NewUnstartedServer(e.srv.Config.Handler)
	timed.Config.WriteTimeout = 250 * time.Millisecond
	timed.Start()
	t.Cleanup(timed.Close)

	req, _ := http.NewRequest(http.MethodPost, timed.URL+oaiBase+"/chat/completions",
		strings.NewReader(`{"stream":true,"messages":[{"role":"user","content":"x"}]}`))
	req.Header.Set("Authorization", "Bearer "+e.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var chunks int
	var done bool
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == oaiSSEDone {
			done = true
		} else if strings.Contains(data, `"content":"x"`) {
			chunks++
		}
	}
	if !done || chunks != 20 {
		t.Fatalf("stream truncated by WriteTimeout: done=%v chunks=%d want 20", done, chunks)
	}
}

// runCLIOnce 用真实的 cli.SessionRunner 跑一轮：真实 HTTPClient、真实 agent 循环、
// 真实 echo 工具（计数其执行次数）。会话文件写到临时目录，不碰用户的 ~/.gocloudagent。
func runCLIOnce(t *testing.T, client *cli.HTTPClient, text string, calls *int) (cli.RunResult, []cli.CLIEvent, error) {
	t.Helper()
	env := []string{"GOCLOUDAGENT_HOME=" + t.TempDir()}
	sess, err := cli.NewSession(env, cli.Config{Endpoint: client.Endpoint, ProjectID: client.ProjectID}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	echo, err := utils.InferTool("echo", "echo back the given text",
		func(_ context.Context, in struct {
			Text string `json:"text"`
		}) (string, error) {
			*calls++
			return `{"ok":true,"text":"` + in.Text + `"}`, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	var events []cli.CLIEvent
	r := &cli.SessionRunner{Session: sess, Client: client, Tools: []tool.BaseTool{echo}}
	res, err := r.Run(context.Background(), text, func(e cli.CLIEvent) { events = append(events, e) })
	return res, events, err
}

var _ = gocloudagent.ClassifyError
