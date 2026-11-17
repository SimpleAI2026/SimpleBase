package cloudagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"

	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
)

// fakeSession 是 mcp.Session 的测试替身。
type fakeSession struct {
	id       string
	tools    []mcp.ToolInfo
	listErr  error
	callErr  error
	result   mcp.CallResult
	calls    []string
	closed   bool
	lastArgs []byte
}

func (f *fakeSession) ServerID() string { return f.id }

func (f *fakeSession) ListTools(context.Context) ([]mcp.ToolInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.tools, nil
}

func (f *fakeSession) CallTool(_ context.Context, name string, args []byte) (mcp.CallResult, error) {
	f.calls = append(f.calls, name)
	f.lastArgs = args
	if f.callErr != nil {
		return mcp.CallResult{}, f.callErr
	}
	return f.result, nil
}

func (f *fakeSession) Close() error {
	f.closed = true
	return nil
}

type fakeConnector struct {
	sessions []mcp.Session
	err      error
	gotIDs   []string
}

func (c *fakeConnector) Connect(_ context.Context, _ Principal, _ string, ids []string) (mcp.SessionSet, error) {
	c.gotIDs = ids
	if c.err != nil {
		return nil, c.err
	}
	return mcp.NewSessionSet(c.sessions), nil
}

type fakePolicy struct {
	decision CommandDecision
	err      error
	gotTool  string
}

func (p *fakePolicy) Classify(_ context.Context, _, _, tool string) (CommandDecision, error) {
	p.gotTool = tool
	return p.decision, p.err
}

func objectSchema() []byte {
	return []byte(`{"type":"object","properties":{"q":{"type":"string"}}}`)
}

func mcpTestReq(ids ...string) RunRequest {
	return RunRequest{
		ProjectID: "p1",
		RunID:     "run-1",
		Agent:     Agent{ID: "ag-1", ToolIDs: ids, Writable: true},
	}
}

func TestMCPToolRefsAndServerIDs(t *testing.T) {
	refs := mcpToolRefs([]string{"mcp:github", "mcp:fs/read", "simplebase", "mcp:github/create_issue", "mcp:"})
	if len(refs) != 3 {
		t.Fatalf("refs=%d want 3: %#v", len(refs), refs)
	}
	ids := mcpServerIDs(refs)
	if len(ids) != 2 || ids[0] != "github" || ids[1] != "fs" {
		t.Fatalf("server ids=%v", ids)
	}
	if !HasMCPTools([]string{"mcp:x"}) || HasMCPTools([]string{"simplebase"}) {
		t.Fatal("HasMCPTools mismatch")
	}
}

func TestBuildMCPToolsRequiresConnector(t *testing.T) {
	_, _, _, err := buildMCPTools(context.Background(), mcpBuildParams{Req: mcpTestReq("mcp:github")})
	if err == nil || !strings.Contains(err.Error(), "mcp is not configured") {
		t.Fatalf("err=%v", err)
	}
}

func TestBuildMCPToolsSkipsFailedServerAndKeepsOthers(t *testing.T) {
	bad := &fakeSession{id: "bad", listErr: errors.New("boom")}
	good := &fakeSession{id: "good", tools: []mcp.ToolInfo{{Name: "search", InputSchema: objectSchema()}}}
	conn := &fakeConnector{sessions: []mcp.Session{bad, good}}
	tools, set, notes, err := buildMCPTools(context.Background(), mcpBuildParams{
		Req:       mcpTestReq("mcp:bad", "mcp:good"),
		Connector: conn,
		Policy:    &fakePolicy{decision: CommandDecision{ReadOnly: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	if len(tools) != 1 {
		t.Fatalf("tools=%d want 1", len(tools))
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "bad unavailable") {
		t.Fatalf("notes=%v", notes)
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "mcp__good__search" {
		t.Fatalf("name=%q", info.Name)
	}
}

func TestBuildMCPToolsFiltersByToolRef(t *testing.T) {
	sess := &fakeSession{id: "fs", tools: []mcp.ToolInfo{
		{Name: "read", InputSchema: objectSchema()},
		{Name: "write", InputSchema: objectSchema()},
	}}
	conn := &fakeConnector{sessions: []mcp.Session{sess}}
	tools, set, _, err := buildMCPTools(context.Background(), mcpBuildParams{
		Req:       mcpTestReq("mcp:fs/read"),
		Connector: conn,
		Policy:    &fakePolicy{decision: CommandDecision{ReadOnly: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	if len(tools) != 1 {
		t.Fatalf("tools=%d want 1", len(tools))
	}
	info, _ := tools[0].Info(context.Background())
	if info.Name != "mcp__fs__read" {
		t.Fatalf("name=%q", info.Name)
	}
}

func TestBuildMCPToolsSkipsInvalidSchemaAndClosesUnusedSet(t *testing.T) {
	sess := &fakeSession{id: "s", tools: []mcp.ToolInfo{
		{Name: "bad", InputSchema: []byte(`{"type":"array"}`)},
	}}
	conn := &fakeConnector{sessions: []mcp.Session{sess}}
	tools, set, notes, err := buildMCPTools(context.Background(), mcpBuildParams{
		Req:       mcpTestReq("mcp:s"),
		Connector: conn,
		Policy:    &fakePolicy{decision: CommandDecision{ReadOnly: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Fatalf("tools=%d want 0", len(tools))
	}
	// 没有可用工具时会话集合已被回收，调用方拿到 nil。
	if set != nil {
		t.Fatalf("set=%v want nil", set)
	}
	if !sess.closed {
		t.Fatal("unused session set should be closed")
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "invalid schema") {
		t.Fatalf("notes=%v", notes)
	}
}

func TestMCPToolReadOnlyCallSucceeds(t *testing.T) {
	sess := &fakeSession{id: "s", tools: []mcp.ToolInfo{{Name: "search", InputSchema: objectSchema()}}, result: mcp.CallResult{Text: "ok"}}
	policy := &fakePolicy{decision: CommandDecision{ReadOnly: true}}
	tools, set, _, err := buildMCPTools(context.Background(), mcpBuildParams{
		Req:       mcpTestReq("mcp:s"),
		Connector: &fakeConnector{sessions: []mcp.Session{sess}},
		Policy:    policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	inv := tools[0].(tool.InvokableTool)
	out, err := inv.InvokableRun(context.Background(), `{"q":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Fatalf("out=%q", out)
	}
	if len(sess.calls) != 1 || sess.calls[0] != "search" {
		t.Fatalf("calls=%v", sess.calls)
	}
	if string(sess.lastArgs) != `{"q":"x"}` {
		t.Fatalf("args=%s", sess.lastArgs)
	}
}

func TestMCPToolPolicyMissingFailsClosed(t *testing.T) {
	sess := &fakeSession{id: "s", tools: []mcp.ToolInfo{{Name: "search", InputSchema: objectSchema()}}, result: mcp.CallResult{Text: "ok"}}
	tools, set, _, err := buildMCPTools(context.Background(), mcpBuildParams{
		Req:       mcpTestReq("mcp:s"),
		Connector: &fakeConnector{sessions: []mcp.Session{sess}},
		// Policy 为 nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	out, err := tools[0].(tool.InvokableTool).InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, mcp.CodePolicyUnavailable) {
		t.Fatalf("out=%q", out)
	}
	if len(sess.calls) != 0 {
		t.Fatal("tool must not be called when policy is missing")
	}
}

func TestMCPToolWriteNotAllowedBeforeConfirm(t *testing.T) {
	sess := &fakeSession{id: "s", tools: []mcp.ToolInfo{{Name: "search", InputSchema: objectSchema()}}, result: mcp.CallResult{Text: "ok"}}
	req := mcpTestReq("mcp:s")
	req.Agent.Writable = false
	waiter := &mcpTestWaiter{}
	tools, set, _, err := buildMCPTools(context.Background(), mcpBuildParams{
		Req:       req,
		Connector: &fakeConnector{sessions: []mcp.Session{sess}},
		Policy:    &fakePolicy{decision: CommandDecision{Destructive: true}},
		Confirm:   waiter,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	out, err := tools[0].(tool.InvokableTool).InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, mcp.CodeWriteNotAllowed) {
		t.Fatalf("out=%q", out)
	}
	// 必须在确认之前拦截：确认器不应被调用。
	if waiter.calls != 0 {
		t.Fatalf("confirm called %d times", waiter.calls)
	}
}

func TestMCPToolHeadlessDestructiveRejected(t *testing.T) {
	sess := &fakeSession{id: "s", tools: []mcp.ToolInfo{{Name: "search", InputSchema: objectSchema()}}, result: mcp.CallResult{Text: "ok"}}
	req := mcpTestReq("mcp:s")
	req.Headless = true
	var events []Event
	tools, set, _, err := buildMCPTools(context.Background(), mcpBuildParams{
		Req:       req,
		RC:        RunContext{ProjectID: "p1", Headless: true, Emit: func(e Event) { events = append(events, e) }},
		Connector: &fakeConnector{sessions: []mcp.Session{sess}},
		Policy:    &fakePolicy{decision: CommandDecision{Destructive: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	out, err := tools[0].(tool.InvokableTool).InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, mcp.CodeConfirmationUnavailable) {
		t.Fatalf("out=%q", out)
	}
	if len(events) != 1 || events[0].Type != "confirmation_resolved" {
		t.Fatalf("events=%v", events)
	}
	if len(sess.calls) != 0 {
		t.Fatal("headless must not call the tool")
	}
}

func TestMCPToolDestructiveConfirmFlow(t *testing.T) {
	sess := &fakeSession{id: "s", tools: []mcp.ToolInfo{{Name: "search", InputSchema: objectSchema()}}, result: mcp.CallResult{Text: "done"}}
	waiter := &mcpTestWaiter{approve: true}
	var events []Event
	tools, set, _, err := buildMCPTools(context.Background(), mcpBuildParams{
		Req:       mcpTestReq("mcp:s"),
		RC:        RunContext{ProjectID: "p1", Emit: func(e Event) { events = append(events, e) }},
		Connector: &fakeConnector{sessions: []mcp.Session{sess}},
		Policy:    &fakePolicy{decision: CommandDecision{Destructive: true}},
		Confirm:   waiter,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	out, err := tools[0].(tool.InvokableTool).InvokableRun(context.Background(), `{"id":"1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "done" {
		t.Fatalf("out=%q", out)
	}
	if waiter.calls != 1 {
		t.Fatalf("confirm calls=%d", waiter.calls)
	}
	var types []string
	for _, e := range events {
		types = append(types, e.Type)
	}
	want := []string{"confirmation_required", "confirmation_resolved", "tool_start"}
	if strings.Join(types, ",") != strings.Join(want, ",") {
		t.Fatalf("events=%v want %v", types, want)
	}
	// 确认参数里带脱敏摘要，不含原始 JSON 全文。
	if len(events[0].Argv) != 3 || events[0].Argv[0] != "s" || events[0].Argv[1] != "search" {
		t.Fatalf("argv=%v", events[0].Argv)
	}
}

func TestMCPToolDeniedReturnsEnvelope(t *testing.T) {
	sess := &fakeSession{id: "s", tools: []mcp.ToolInfo{{Name: "search", InputSchema: objectSchema()}}, result: mcp.CallResult{Text: "done"}}
	waiter := &mcpTestWaiter{approve: false}
	tools, set, _, err := buildMCPTools(context.Background(), mcpBuildParams{
		Req:       mcpTestReq("mcp:s"),
		Connector: &fakeConnector{sessions: []mcp.Session{sess}},
		Policy:    &fakePolicy{decision: CommandDecision{Destructive: true}},
		Confirm:   waiter,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	out, err := tools[0].(tool.InvokableTool).InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, mcp.CodeConfirmationDenied) {
		t.Fatalf("out=%q", out)
	}
	if len(sess.calls) != 0 {
		t.Fatal("denied tool must not run")
	}
}

func TestMCPToolErrorAndTimeoutEnvelopes(t *testing.T) {
	sess := &fakeSession{id: "s", tools: []mcp.ToolInfo{{Name: "search", InputSchema: objectSchema()}}, result: mcp.CallResult{Text: "bad input", IsError: true}}
	tools, set, _, err := buildMCPTools(context.Background(), mcpBuildParams{
		Req:       mcpTestReq("mcp:s"),
		Connector: &fakeConnector{sessions: []mcp.Session{sess}},
		Policy:    &fakePolicy{decision: CommandDecision{ReadOnly: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := tools[0].(tool.InvokableTool).InvokableRun(context.Background(), `{}`)
	if !strings.Contains(out, mcp.CodeToolError) || !strings.Contains(out, "bad input") {
		t.Fatalf("out=%q", out)
	}
	set.Close()

	// 传输错误 → mcp_unavailable。
	failing := &fakeSession{id: "s", tools: []mcp.ToolInfo{{Name: "search", InputSchema: objectSchema()}}, callErr: errors.New("dial failed")}
	tools, set, _, err = buildMCPTools(context.Background(), mcpBuildParams{
		Req:       mcpTestReq("mcp:s"),
		Connector: &fakeConnector{sessions: []mcp.Session{failing}},
		Policy:    &fakePolicy{decision: CommandDecision{ReadOnly: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	out, _ = tools[0].(tool.InvokableTool).InvokableRun(context.Background(), `{}`)
	if !strings.Contains(out, mcp.CodeUnavailable) {
		t.Fatalf("out=%q", out)
	}
}

func TestMCPToolTruncatedResultMarked(t *testing.T) {
	sess := &fakeSession{id: "s", tools: []mcp.ToolInfo{{Name: "search", InputSchema: objectSchema()}}, result: mcp.CallResult{Text: "partial", Truncated: true}}
	tools, set, _, err := buildMCPTools(context.Background(), mcpBuildParams{
		Req:       mcpTestReq("mcp:s"),
		Connector: &fakeConnector{sessions: []mcp.Session{sess}},
		Policy:    &fakePolicy{decision: CommandDecision{ReadOnly: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	out, _ := tools[0].(tool.InvokableTool).InvokableRun(context.Background(), `{}`)
	if !strings.Contains(out, "[truncated]") {
		t.Fatalf("out=%q", out)
	}
}

func TestMCPToolInfoCarriesSchema(t *testing.T) {
	info, err := mcpToolInfo("mcp__s__t", mcp.ToolInfo{
		Name:        "t",
		Description: "desc",
		InputSchema: objectSchema(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "mcp__s__t" || info.Desc != "desc" {
		t.Fatalf("info=%#v", info)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	if js == nil || js.Type != "object" {
		t.Fatalf("schema=%#v", js)
	}
	// 非法 schema 报错，由调用方跳过该工具。
	if _, err := mcpToolInfo("x", mcp.ToolInfo{InputSchema: []byte("not json")}); err == nil {
		t.Fatal("invalid schema should error")
	}
}

func TestMCPConfirmArgvTruncates(t *testing.T) {
	argv := mcpConfirmArgv("s", "t", strings.Repeat("a", 500))
	// truncate 会追加 "\n…[truncated]"，所以按前缀长度断言。
	if len(argv) != 3 || !strings.HasPrefix(argv[2], strings.Repeat("a", 200)) {
		t.Fatalf("argv=%v", argv)
	}
	if got := mcpConfirmArgv("s", "t", "  "); len(got) != 2 {
		t.Fatalf("empty args argv=%v", got)
	}
}

// mcpTestWaiter 记录确认调用次数并返回预设结果。
type mcpTestWaiter struct {
	approve bool
	calls   int
}

func (w *mcpTestWaiter) Wait(context.Context, string, string, string, time.Duration) (bool, error) {
	w.calls++
	return w.approve, nil
}

func (w *mcpTestWaiter) Prepare(string, string, string) {}

// 确保 skillFail 信封是合法 JSON（前端与模型都按 code 分支）。
func TestSkillFailEnvelopeIsJSON(t *testing.T) {
	var v map[string]any
	if err := json.Unmarshal([]byte(skillFail(mcp.CodeTimeout, "x")), &v); err != nil {
		t.Fatal(err)
	}
	if v["ok"] != false {
		t.Fatalf("envelope=%v", v)
	}
}
