package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func TestRunEventHubGapAndConfirmTimeout(t *testing.T) {
	hub := newRunEventHub()
	// 容量 2000。丢掉两条之后，客户端只看到 id=1 时中间的 id=2 已经不在环里。
	for i := 0; i < 2002; i++ {
		hub.append("run", gocloudagent.Event{Type: "token", Content: "x"})
	}
	if _, gap, _, _ := hub.replay("run", 1); !gap {
		t.Fatal("expected stream gap")
	}
	confirms := newConfirmHub()
	ok, err := confirms.Wait(context.Background(), "run", "call", "u1", 30*time.Millisecond)
	if ok || err != nil {
		t.Fatalf("timeout approve=%v err=%v", ok, err)
	}
}

type skillHit struct {
	Method string
	Path   string
	Status int
}

type skillData struct {
	mu     sync.Mutex
	hits   []skillHit
	reg    *auth.DelegationRegistry
	secret string
}

func (d *skillData) reset() {
	d.mu.Lock()
	d.hits = nil
	d.mu.Unlock()
}

func (d *skillData) count(method, pathPart string, only2xx bool) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, h := range d.hits {
		if h.Method == method && strings.Contains(h.Path, pathPart) && (!only2xx || (h.Status >= 200 && h.Status < 300)) {
			n++
		}
	}
	return n
}

func (d *skillData) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	claims, err := auth.VerifyJWT(d.secret, token, time.Now())
	if err != nil || d.reg.Revoked(claims.JWTID, time.Now()) {
		writeAPI(w, http.StatusUnauthorized, "invalid_token")
		d.record(r.Method, r.URL.Path, http.StatusUnauthorized)
		return
	}
	principal, err := auth.PrincipalFromDelegation(claims)
	if err != nil || claims.ProjectID == "" || !pathMatchesProject(r.URL.Path, claims.ProjectID) {
		writeAPI(w, http.StatusForbidden, "forbidden")
		d.record(r.Method, r.URL.Path, http.StatusForbidden)
		return
	}
	if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "simplebase-system") {
		writeAPI(w, http.StatusForbidden, "system_database_protected")
		d.record(r.Method, r.URL.Path, http.StatusForbidden)
		return
	}
	if skillWrite(r.Method, r.URL.Path, body) {
		if _, ok := principal.Permissions[auth.DatabaseWrite]; !ok {
			writeAPI(w, http.StatusForbidden, "forbidden")
			d.record(r.Method, r.URL.Path, http.StatusForbidden)
			return
		}
	}
	if strings.Contains(string(body), "slow") {
		d.record(r.Method, r.URL.Path, 0)
		<-r.Context().Done()
		return
	}
	status, payload := skillPayload(r.Method, r.URL.Path)
	d.record(r.Method, r.URL.Path, status)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(payload))
}

func (d *skillData) record(method, path string, status int) {
	d.mu.Lock()
	d.hits = append(d.hits, skillHit{method, path, status})
	d.mu.Unlock()
}

func pathMatchesProject(path, project string) bool {
	if strings.HasPrefix(path, "/go/") {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		return len(parts) >= 2 && parts[1] == project
	}
	return strings.Contains(path, "/projects/"+project+"/") || strings.HasSuffix(path, "/projects/"+project)
}

func skillWrite(method, path string, body []byte) bool {
	if method == http.MethodGet || method == http.MethodHead {
		return false
	}
	if strings.Contains(path, "/query") || strings.HasPrefix(path, "/go/") {
		return false
	}
	if strings.Contains(path, "/kv") && strings.Contains(string(body), `"GET"`) {
		return false
	}
	return true
}

func skillPayload(method, path string) (int, string) {
	switch {
	case method == http.MethodPost && strings.HasSuffix(path, "/databases"):
		return http.StatusCreated, `{"id":"shop","name":"shop","status":"ready"}`
	case strings.Contains(path, "/schema/tables"):
		return http.StatusCreated, `{"name":"orders"}`
	case strings.Contains(path, "/execute"):
		return http.StatusOK, `{"rows_affected":1}`
	case strings.Contains(path, "/query"):
		return http.StatusOK, `{"columns":["amount"],"rows":[[10]],"row_count":1}`
	case strings.Contains(path, "/kv"):
		return http.StatusOK, `{"ok":true}`
	case strings.Contains(path, "/trigger"):
		return http.StatusAccepted, `{"status":"accepted"}`
	case strings.Contains(path, "/cron-jobs"):
		return http.StatusCreated, `{"id":"job1"}`
	case strings.HasPrefix(path, "/go/"):
		return http.StatusOK, `{"result":"hi"}`
	case strings.Contains(path, "/gofunctions"):
		return http.StatusCreated, `{"name":"hello"}`
	case strings.Contains(path, "/s3/objects"):
		return http.StatusCreated, `{"key":"readme","size":5}`
	default:
		return http.StatusOK, `{"ok":true}`
	}
}

func writeAPI(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": code}})
}

type scriptLLM struct {
	mu    sync.Mutex
	steps [][]gocloudagent.StreamDelta
	n     int
}

func (s *scriptLLM) reset(steps [][]gocloudagent.StreamDelta) {
	s.mu.Lock()
	s.steps = steps
	s.n = 0
	s.mu.Unlock()
}

func (s *scriptLLM) Chat(context.Context, string, gocloudagent.ChatRequest) (gocloudagent.ChatResponse, error) {
	return gocloudagent.ChatResponse{Content: "done"}, nil
}

func (s *scriptLLM) Stream(context.Context, string, gocloudagent.ChatRequest) (gocloudagent.TokenStream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var deltas []gocloudagent.StreamDelta
	if s.n < len(s.steps) {
		deltas = s.steps[s.n]
		s.n++
	} else {
		deltas = []gocloudagent.StreamDelta{{Content: "done", Finish: true}}
	}
	return &stepDelta{deltas: deltas}, nil
}

type stepDelta struct {
	deltas []gocloudagent.StreamDelta
	i      int
}

func (s *stepDelta) NextDelta() (gocloudagent.StreamDelta, error) {
	if s.i >= len(s.deltas) {
		return gocloudagent.StreamDelta{}, io.EOF
	}
	d := s.deltas[s.i]
	s.i++
	return d, nil
}

func (s *stepDelta) Next() (string, bool, error) {
	d, err := s.NextDelta()
	if err != nil {
		return "", true, err
	}
	return d.Content, d.Finish, nil
}

func (s *stepDelta) Close() error { return nil }

func toolDeltas(id string, argv []string) []gocloudagent.StreamDelta {
	raw, _ := json.Marshal(map[string]any{"argv": argv})
	s := string(raw)
	cut := len(s) / 2
	return []gocloudagent.StreamDelta{
		{ToolCall: &gocloudagent.ToolCallDelta{Index: 0, ID: id, Name: "simplebase", ArgsDelta: s[:cut]}},
		{ToolCall: &gocloudagent.ToolCallDelta{Index: 0, ArgsDelta: s[cut:]}},
		{Finish: true},
	}
}

// toolStdinDeltas 构造带内联 stdin 的工具调用（planv6.0 D3：--file -）。
func toolStdinDeltas(id string, argv []string, stdin string) []gocloudagent.StreamDelta {
	raw, _ := json.Marshal(map[string]any{"argv": argv, "stdin": stdin})
	s := string(raw)
	cut := len(s) / 2
	return []gocloudagent.StreamDelta{
		{ToolCall: &gocloudagent.ToolCallDelta{Index: 0, ID: id, Name: "simplebase", ArgsDelta: s[:cut]}},
		{ToolCall: &gocloudagent.ToolCallDelta{Index: 0, ArgsDelta: s[cut:]}},
		{Finish: true},
	}
}

func textDeltas(text string) []gocloudagent.StreamDelta {
	return []gocloudagent.StreamDelta{{Content: text, Finish: true}}
}

type sseEv struct {
	ID   int64
	Type string
	Raw  map[string]any
	Data string
}

func readSSE(t *testing.T, r io.Reader, stop func(sseEv) bool) []sseEv {
	t.Helper()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var frame []string
	var out []sseEv
	flush := func() bool {
		ev, ok := parseSSEFrame(frame)
		frame = nil
		if !ok {
			return false
		}
		out = append(out, ev)
		return stop != nil && stop(ev)
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			if flush() {
				return out
			}
			continue
		}
		frame = append(frame, line)
	}
	flush()
	return out
}

func parseSSEFrame(lines []string) (sseEv, bool) {
	var id int64
	var data string
	for _, line := range lines {
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "id:") {
			id, _ = strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "id:")), 10, 64)
		}
		if strings.HasPrefix(line, "data:") {
			data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
	if data == "" {
		return sseEv{}, false
	}
	var raw map[string]any
	if json.Unmarshal([]byte(data), &raw) != nil {
		return sseEv{}, false
	}
	typ, _ := raw["type"].(string)
	return sseEv{ID: id, Type: typ, Raw: raw, Data: data}, true
}

// mustListAgents 返回某项目的全部助手（测试辅助）。
func mustListAgents(t *testing.T, store *systemdb.Store) []systemdb.CloudAgent {
	t.Helper()
	list, err := store.ListCloudAgents(context.Background(), "testprj1")
	if err != nil {
		t.Fatal(err)
	}
	return list
}

// setAgentsWritable 切换测试项目全部助手的写开关。
func setAgentsWritable(t *testing.T, store *systemdb.Store, v bool) {
	t.Helper()
	for _, a := range mustListAgents(t, store) {
		a.Writable = v
		if _, err := store.UpdateCloudAgent(context.Background(), a); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAgentSkillsE2E(t *testing.T) {
	secret := "test-secret"
	reg := auth.NewDelegationRegistry()
	data := &skillData{reg: reg, secret: secret}
	dataSrv := httptest.NewServer(data)
	defer dataSrv.Close()

	db := openMemoryDB(t)
	if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := systemdb.NewStoreForTest(db)
	// planv6.0 v2 D-C1a：v48 起助手默认只读。本用例集验证的是写流程，
	// 因此把内置助手显式标为可写；只读拒绝由 14/15 单独覆盖。
	if err := store.SeedDefaultCloudAgentsWithCapabilities(context.Background(), "testprj1", false, true); err != nil {
		t.Fatal(err)
	}
	for _, a := range mustListAgents(t, store) {
		if !a.Writable {
			a.Writable = true
			if _, err := store.UpdateCloudAgent(context.Background(), a); err != nil {
				t.Fatal(err)
			}
		}
	}
	llm := &scriptLLM{}
	confirms := newConfirmHub()
	events := newRunEventHub()
	rt := &gocloudagent.Runtime{
		LLM: llm, SkillsCLI: true, CLI: agentbridge.InProcessCLI{},
		APIBaseURL: dataSrv.URL, Policy: agentbridge.NewCommandPolicy(), Issuer: agentbridge.NewDelegationIssuer(secret, reg),
		// 确认窗口必须短于 RunTimeout。CI 上同机并行包会把调度拖过几秒，
		// 3s 确认或 20s 运行上限都会在批准发出前到期。超时路径由
		// "confirmation timeout" 子测试把窗口改成 200ms 覆盖。
		Confirm: confirms, ConfirmTimeout: 30 * time.Second, DelegationTTL: time.Minute,
		ToolProtocol: "native", RunTimeout: 2 * time.Minute, MaxIterations: 8,
	}
	h := &cloudAgentHandler{store: store, runtime: rt, events: events, confirms: confirms}
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			role := auth.RoleUser
			if c.Request().Header.Get("X-Test-Role") == "admin" {
				role = auth.RoleAdmin
			}
			ctx := WithProject(c.Request().Context(), ProjectContext{ID: "testprj1", TenantID: catalog.ReservedTenantID})
			ctx = WithPrincipal(ctx, auth.Principal{
				UserID: "u1", Username: "ada", Role: role, TenantID: catalog.ReservedTenantID,
				Permissions: auth.PermissionsForRole(role),
				ProjectIDs:  map[string]struct{}{"testprj1": {}},
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	registerAgentRoutes(e, h)
	agentSrv := httptest.NewServer(e)
	defer agentSrv.Close()

	postJSON := func(t *testing.T, path, role string, body any) *http.Response {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPost, agentSrv.URL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Role", role)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	newThread := func(t *testing.T, agentID string) string {
		t.Helper()
		body := map[string]any{"title": "t"}
		if agentID != "" {
			body["agent_id"] = agentID
		}
		resp := postJSON(t, "/agent-threads", "user", body)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("thread %d %s", resp.StatusCode, b)
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out.ID
	}

	// agentIDForModule 取某个模块的内置助手：技能由助手 module 决定，
	// `@` 只点名助手，不再按消息文本选技能。
	agentIDForModule := func(t *testing.T, module string) string {
		t.Helper()
		for _, a := range mustListAgents(t, store) {
			if a.Module == module && a.BuiltinKey == module {
				return a.ID
			}
		}
		t.Fatalf("seeded agent for module %s not found", module)
		return ""
	}

	approve := func(t *testing.T, runID, callID string, ok bool) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for {
			resp := postJSON(t, "/agent-runs/"+runID+"/confirmations/"+callID, "user", map[string]any{"approve": ok})
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("confirm %s: %d", callID, resp.StatusCode)
			}
			time.Sleep(15 * time.Millisecond)
		}
	}

	run := func(t *testing.T, content string, module string, role string, steps [][]gocloudagent.StreamDelta) (string, []sseEv) {
		t.Helper()
		data.reset()
		llm.reset(steps)
		threadID := newThread(t, agentIDForModule(t, module))
		resp := postJSON(t, "/agent-threads/"+threadID+"/runs", role, map[string]any{"content": content, "stream": true})
		t.Cleanup(func() { resp.Body.Close() })
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("run %d %s", resp.StatusCode, b)
		}
		return "", readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
	}

	// The signature above always reads to end. Destructive samples need a pause.
	// runPaused returns the body so the caller can continue.
	openRun := func(t *testing.T, content string, module string, role string, steps [][]gocloudagent.StreamDelta) (string, *http.Response) {
		t.Helper()
		data.reset()
		llm.reset(steps)
		threadID := newThread(t, agentIDForModule(t, module))
		resp := postJSON(t, "/agent-threads/"+threadID+"/runs", role, map[string]any{"content": content, "stream": true})
		t.Cleanup(func() { resp.Body.Close() })
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("run %d %s", resp.StatusCode, b)
		}
		return threadID, resp
	}
	_ = run

	assertOrder := func(t *testing.T, evs []sseEv) {
		t.Helper()
		var prev int64
		seenDelta, seenCall, seenStart, seenResult := false, false, false, false
		blob := strings.Builder{}
		for _, ev := range evs {
			blob.WriteString(ev.Data)
			if ev.ID > 0 {
				if ev.ID <= prev {
					t.Fatalf("id not increasing: %d then %d", prev, ev.ID)
				}
				prev = ev.ID
			}
			switch ev.Type {
			case "tool_call_delta":
				seenDelta = true
				if seenCall {
					t.Fatal("delta after full tool_call")
				}
			case "tool_call":
				seenCall = true
			case "tool_start":
				if !seenCall {
					t.Fatal("tool_start before tool_call")
				}
				seenStart = true
			case "tool_result":
				if !seenStart && ev.Raw["is_error"] != true {
					// skill_not_loaded still emits tool_start before the rejection?
					// rejection happens before tool_start. Allow result without start only when is_error.
					t.Fatal("tool_result before tool_start")
				}
				seenResult = true
			}
		}
		if strings.Contains(blob.String(), "SIMPLEBASE_TOKEN") || strings.Contains(blob.String(), secret) {
			t.Fatal("token leaked into SSE")
		}
		_ = seenDelta
		_ = seenResult
	}

	t.Run("1 create database", func(t *testing.T) {
		_, resp := openRun(t, "建一个名为 shop 的 SQL 库", "database", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"database", "create", "--name", "shop", "--data-model", "sql"}),
			textDeltas("已创建"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		assertOrder(t, evs)
		if data.count(http.MethodPost, "/databases", true) != 1 {
			t.Fatalf("create hits %+v", data.hits)
		}
		if !hasType(evs, "tool_call_delta") || !hasType(evs, "tool_result") || lastReason(evs) != "stop" {
			t.Fatalf("events %s", typesOf(evs))
		}
	})

	t.Run("2 create table", func(t *testing.T) {
		_, resp := openRun(t, "在 shop 建 orders 表", "database", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"schema", "create-table", "--database", "shop", "--name", "orders", "--columns", `[{"name":"id","type":"BIGINT"},{"name":"amount","type":"DOUBLE"}]`}),
			textDeltas("ok"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if data.count(http.MethodPost, "/schema/tables", true) != 1 || lastReason(evs) != "stop" {
			t.Fatalf("%+v %s %s", data.hits, typesOf(evs), joinResults(evs))
		}
	})

	t.Run("3 insert", func(t *testing.T) {
		_, resp := openRun(t, "插入一行", "database", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"sql", "exec", "--database", "shop", "--statement", "INSERT INTO orders VALUES (1, 10)"}),
			textDeltas("ok"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if data.count(http.MethodPost, "/execute", true) != 1 || lastReason(evs) != "stop" {
			t.Fatalf("%+v", data.hits)
		}
	})

	t.Run("4 query", func(t *testing.T) {
		_, resp := openRun(t, "查出 amount", "database", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"sql", "query", "--database", "shop", "--statement", "SELECT amount FROM orders"}),
			textDeltas("10"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if data.count(http.MethodPost, "/query", true) != 1 || !strings.Contains(joinResults(evs), "amount") {
			t.Fatalf("query %+v %s", data.hits, joinResults(evs))
		}
	})

	t.Run("5 delete confirms", func(t *testing.T) {
		_, resp := openRun(t, "删掉 shop 库", "database", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"database", "delete", "--id", "shop"}),
			textDeltas("deleted"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "confirmation_required" })
		if data.count(http.MethodDelete, "/databases/", false) != 0 {
			t.Fatalf("delete before confirm %+v", data.hits)
		}
		callID, runID := callAndRun(t, evs)
		approve(t, runID, callID, true)
		rest := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if data.count(http.MethodDelete, "/databases/shop", true) != 1 || lastReason(rest) != "stop" {
			t.Fatalf("after %+v %s", data.hits, typesOf(rest))
		}
	})

	t.Run("6 system database", func(t *testing.T) {
		_, resp := openRun(t, "系统库也删掉", "database", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"database", "delete", "--id", "simplebase-system"}),
			textDeltas("no"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "confirmation_required" })
		callID, runID := callAndRun(t, evs)
		approve(t, runID, callID, true)
		rest := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if !resultIsError(rest, "system_database_protected") || lastReason(rest) != "stop" {
			t.Fatalf("system %+v %s", data.hits, joinResults(rest))
		}
	})

	t.Run("7 function", func(t *testing.T) {
		src := "package main\nimport \"fmt\"\ntype Req struct{}\nfunc Hello(req Req) string { fmt.Println(\"hi\"); return \"hi\" }\n"
		_, resp := openRun(t, "写一个 Hello 云函数并调用", "gofunction", "user", [][]gocloudagent.StreamDelta{
			toolStdinDeltas("c1", []string{"function", "create", "--name", "hello", "--file", "-"}, src),
			toolDeltas("c2", []string{"function", "invoke", "--name", "hello", "--export", "Hello"}),
			textDeltas("hi"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if data.count(http.MethodPost, "/gofunctions", true) != 1 || data.count(http.MethodPost, "/go/", true) != 1 || lastReason(evs) != "stop" {
			t.Fatalf("%+v %s %s", data.hits, typesOf(evs), joinResults(evs))
		}
	})

	t.Run("8 cron", func(t *testing.T) {
		_, resp := openRun(t, "每天 0 点跑", "cron", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"cron", "create", "--schedule-kind", "cron", "--cron", "0 0 * * *"}),
			textDeltas("ok"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if data.count(http.MethodPost, "/cron-jobs", true) != 1 || lastReason(evs) != "stop" {
			t.Fatalf("%+v", data.hits)
		}
	})

	t.Run("9 trigger confirms", func(t *testing.T) {
		_, resp := openRun(t, "现在就跑一次", "cron", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"cron", "trigger", "--id", "job1"}),
			textDeltas("ok"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "confirmation_required" })
		if data.count(http.MethodPost, "/trigger", false) != 0 {
			t.Fatal(data.hits)
		}
		callID, runID := callAndRun(t, evs)
		approve(t, runID, callID, true)
		rest := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if data.count(http.MethodPost, "/trigger", true) != 1 || lastReason(rest) != "stop" {
			t.Fatalf("%+v %s", data.hits, typesOf(rest))
		}
	})

	t.Run("10 kv", func(t *testing.T) {
		_, resp := openRun(t, "设一个键再读", "kv", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"kv", "exec", "--command", "SET", "--arg", "a", "--arg", "1"}),
			toolDeltas("c2", []string{"kv", "exec", "--command", "GET", "--arg", "a"}),
			textDeltas("1"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if data.count(http.MethodPost, "/kv", true) != 2 || lastReason(evs) != "stop" {
			t.Fatalf("%+v %s %s", data.hits, typesOf(evs), joinResults(evs))
		}
	})

	t.Run("11 kv delete confirms", func(t *testing.T) {
		_, resp := openRun(t, "删掉键 a", "kv", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"kv", "exec", "--command", "DEL", "--arg", "a"}),
			textDeltas("ok"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "confirmation_required" })
		if data.count(http.MethodPost, "/kv", false) != 0 {
			t.Fatal(data.hits)
		}
		callID, runID := callAndRun(t, evs)
		approve(t, runID, callID, true)
		rest := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if data.count(http.MethodPost, "/kv", true) != 1 || lastReason(rest) != "stop" {
			t.Fatalf("%+v", data.hits)
		}
	})

	t.Run("12 upload", func(t *testing.T) {
		_, resp := openRun(t, "上传 readme", "s3", "user", [][]gocloudagent.StreamDelta{
			toolStdinDeltas("c1", []string{"object", "upload", "--key", "readme", "--file", "-"}, "hello"),
			textDeltas("ok"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if data.count(http.MethodPost, "/s3/objects", true) != 1 || lastReason(evs) != "stop" {
			t.Fatalf("%+v %s %s", data.hits, typesOf(evs), joinResults(evs))
		}
	})

	t.Run("13 skill not loaded", func(t *testing.T) {
		_, resp := openRun(t, "删掉所有库", "general", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"database", "delete", "--id", "shop"}),
			textDeltas("no"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if len(data.hits) != 0 || !resultIsError(evs, "skill_not_loaded") {
			t.Fatalf("hits %+v results %s", data.hits, joinResults(evs))
		}
	})

	t.Run("14 admin forbidden", func(t *testing.T) {
		_, resp := openRun(t, "插入一行", "database", "admin", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"sql", "exec", "--database", "shop", "--statement", "INSERT INTO orders VALUES (1, 10)"}),
			textDeltas("no"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if !resultIsError(evs, "forbidden") || lastReason(evs) != "stop" {
			t.Fatalf("%+v %s", data.hits, joinResults(evs))
		}
	})

	t.Run("14b readonly agent rejects writes", func(t *testing.T) {
		// planv6.0 v2 §3.8：助手级只读开关对写命令的拒绝，且不进入确认流程。
		setAgentsWritable(t, store, false)
		t.Cleanup(func() { setAgentsWritable(t, store, true) })
		_, resp := openRun(t, "删掉 shop 库", "database", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"database", "delete", "--id", "shop"}),
			textDeltas("no"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if !resultIsError(evs, "write_not_allowed") {
			t.Fatalf("readonly write must be rejected: %s", joinResults(evs))
		}
		// 未进入确认：不应有 confirmation_required，也不应有删除请求。
		if hasType(evs, "confirmation_required") {
			t.Fatal("readonly rejection must happen before confirmation")
		}
		if data.count(http.MethodDelete, "/databases/", false) != 0 {
			t.Fatalf("delete must not run: %+v", data.hits)
		}
	})

	t.Run("14c readonly agent still allows reads", func(t *testing.T) {
		setAgentsWritable(t, store, false)
		t.Cleanup(func() { setAgentsWritable(t, store, true) })
		_, resp := openRun(t, "查一下", "database", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"sql", "query", "--database", "shop", "--statement", "SELECT 1"}),
			textDeltas("ok"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if lastReason(evs) != "stop" || resultIsError(evs, "write_not_allowed") {
			t.Fatalf("read must be allowed: %s", joinResults(evs))
		}
		if data.count(http.MethodPost, "/query", true) != 1 {
			t.Fatalf("query hits %+v", data.hits)
		}
	})

	t.Run("15 cancel", func(t *testing.T) {
		_, resp := openRun(t, "停掉", "database", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"sql", "query", "--database", "shop", "--statement", "SELECT slow"}),
			textDeltas("no"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "tool_start" })
		_, runID := callAndRun(t, evs)
		cancel := postJSON(t, "/agent-runs/"+runID+"/cancel", "user", map[string]any{})
		cancel.Body.Close()
		if cancel.StatusCode != http.StatusOK {
			t.Fatalf("cancel %d", cancel.StatusCode)
		}
		rest := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if lastReason(rest) != "canceled" || data.count(http.MethodPost, "/execute", true) != 0 {
			t.Fatalf("reason %s hits %+v", lastReason(rest), data.hits)
		}
	})

	t.Run("client skills are ignored", func(t *testing.T) {
		// `@` 只点名助手：请求体里的 skills 不再是入口，未知 id 也不再报 unknown_skill，
		// 真正生效的技能由助手 module 决定（general 不加载子技能）。
		data.reset()
		llm.reset([][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"database", "list"}),
			textDeltas("no"),
		})
		threadID := newThread(t, agentIDForModule(t, "general"))
		resp := postJSON(t, "/agent-threads/"+threadID+"/runs", "user", map[string]any{"content": "x", "skills": []string{"nope"}, "stream": true})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("%d %s", resp.StatusCode, b)
		}
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if !resultIsError(evs, "skill_not_loaded") || len(data.hits) != 0 {
			t.Fatalf("general must not inherit client skills: hits %+v results %s", data.hits, joinResults(evs))
		}
	})

	t.Run("confirmation timeout", func(t *testing.T) {
		prev := rt.ConfirmTimeout
		rt.ConfirmTimeout = 200 * time.Millisecond
		t.Cleanup(func() { rt.ConfirmTimeout = prev })
		_, resp := openRun(t, "删掉", "database", "user", [][]gocloudagent.StreamDelta{
			toolDeltas("c1", []string{"database", "delete", "--id", "shop"}),
			textDeltas("no"),
		})
		evs := readSSE(t, resp.Body, func(ev sseEv) bool { return ev.Type == "end" })
		if data.count(http.MethodDelete, "/databases/", false) != 0 || !resultIsError(evs, "confirmation_denied") {
			t.Fatalf("%+v %s", data.hits, joinResults(evs))
		}
	})

	t.Run("replay and comments", func(t *testing.T) {
		raw := ": ping\n\nid: 2\ndata: {\"type\":\"token\",\"content\":\"hi\"}\n\n"
		evs := readSSE(t, strings.NewReader(raw), nil)
		if len(evs) != 1 || evs[0].Type != "token" || evs[0].ID != 2 {
			t.Fatalf("%+v", evs)
		}
	})
}

func hasType(evs []sseEv, typ string) bool {
	for _, ev := range evs {
		if ev.Type == typ {
			return true
		}
	}
	return false
}

func typesOf(evs []sseEv) string {
	var b strings.Builder
	for _, ev := range evs {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(ev.Type)
	}
	return b.String()
}

func lastReason(evs []sseEv) string {
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == "end" {
			reason, _ := evs[i].Raw["reason"].(string)
			return reason
		}
	}
	return ""
}

func joinResults(evs []sseEv) string {
	var b strings.Builder
	for _, ev := range evs {
		if ev.Type == "tool_result" || ev.Type == "error" {
			b.WriteString(ev.Data)
		}
	}
	return b.String()
}

func resultIsError(evs []sseEv, code string) bool {
	for _, ev := range evs {
		if ev.Type != "tool_result" {
			continue
		}
		if ev.Raw["is_error"] != true {
			continue
		}
		if strings.Contains(ev.Data, code) {
			return true
		}
	}
	return false
}

func callAndRun(t *testing.T, evs []sseEv) (string, string) {
	t.Helper()
	for i := len(evs) - 1; i >= 0; i-- {
		ev := evs[i]
		if ev.Type == "confirmation_required" || ev.Type == "tool_start" || ev.Type == "run" {
			callID, _ := ev.Raw["call_id"].(string)
			runID, _ := ev.Raw["run_id"].(string)
			if ev.Type == "run" && runID != "" && callID == "" {
				continue
			}
			if runID != "" {
				return callID, runID
			}
		}
	}
	// run id is on every event written by streamRun.
	for _, ev := range evs {
		if runID, _ := ev.Raw["run_id"].(string); runID != "" {
			callID, _ := ev.Raw["call_id"].(string)
			return callID, runID
		}
	}
	t.Fatalf("no run id in %s", typesOf(evs))
	return "", ""
}
