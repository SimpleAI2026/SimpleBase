package cloudagent

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	_ "github.com/uglyer/go-sqlite3"
)

func TestModulesCatalogAndDefaults(t *testing.T) {
	if !KnownModule(" DATABASE ") || !KnownModule("S3") || !KnownModule("Logs") || !KnownModule("GENERAL") {
		t.Fatal("KnownModule should be case/space insensitive for built-ins")
	}
	if KnownModule("") || KnownModule("team") {
		t.Fatal("unknown modules")
	}
	if got := ModuleTemplate("database"); !strings.Contains(got, "Module: database") {
		t.Fatalf("database template: %s", got)
	}
	if got := ModuleTemplate("S3"); !strings.Contains(got, "Module: s3") {
		t.Fatalf("s3 template: %s", got)
	}
	if got := ModuleTemplate(" logs "); !strings.Contains(got, "Module: logs") {
		t.Fatalf("logs template: %s", got)
	}
	if got := ModuleTemplate("other"); !strings.Contains(got, "Module: general") {
		t.Fatalf("default template: %s", got)
	}
	if tools := DefaultToolsForModule(ModuleDatabase); len(tools) != 3 {
		t.Fatalf("db tools=%v", tools)
	}
	if tools := DefaultToolsForModule(ModuleS3); len(tools) != 2 {
		t.Fatalf("s3 tools=%v", tools)
	}
	if tools := DefaultToolsForModule(ModuleLogs); len(tools) != 2 {
		t.Fatalf("logs tools=%v", tools)
	}
	if tools := DefaultToolsForModule(ModuleGeneral); tools != nil {
		t.Fatalf("general tools=%v", tools)
	}
	if tools := DefaultToolsForModule("nope"); tools != nil {
		t.Fatalf("unknown tools=%v", tools)
	}
	if KnownTool("") || KnownTool("write_sql") {
		t.Fatal("unknown tool")
	}
	for _, id := range []string{ToolListDatabases, ToolListCollections, ToolReadonlySQL, ToolListObjects, ToolHeadObject, ToolSearchLogs, ToolLogLevelStats} {
		if !KnownTool(id) {
			t.Fatalf("expected known tool %s", id)
		}
	}
}

func TestPromptRedactAndTruncate(t *testing.T) {
	if RedactSecrets("") != "" {
		t.Fatal("empty redact")
	}
	got := RedactSecrets("Bearer abcdef.ghij+/= password=hunter2")
	if strings.Contains(got, "abcdef") || strings.Contains(got, "hunter2") {
		t.Fatalf("leaked: %s", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("expected redaction: %s", got)
	}
	if truncate("ab", 0) != "ab" {
		t.Fatal("max<=0 should return original")
	}
	if truncate("ab", 10) != "ab" {
		t.Fatal("short string")
	}
	long := strings.Repeat("你", 8)
	out := truncate(long, 3)
	if !strings.HasSuffix(out, "\n…[truncated]") {
		t.Fatalf("truncate suffix: %q", out)
	}
	empty := AssembleInstruction(PromptParts{})
	if !strings.Contains(empty, "SimpleBase Cloud Agent") {
		t.Fatal("platform prompt required")
	}
}

func TestGatewayChatModelGenerateAndOptions(t *testing.T) {
	llm := &scriptedChat{content: "plain answer"}
	m := newGatewayChatModel(llm, "p1", "base-model")
	if err := m.BindTools([]*schema.ToolInfo{{Name: "bound", Desc: "d"}}); err != nil {
		t.Fatal(err)
	}
	cloned, err := m.WithTools([]*schema.ToolInfo{{Name: "opt", Desc: "other"}})
	if err != nil {
		t.Fatal(err)
	}
	maxTok := 32
	temp := float32(0.2)
	modelName := "override-model"
	msg, err := cloned.Generate(context.Background(), []*schema.Message{
		nil,
		schema.SystemMessage("sys"),
		schema.UserMessage("hello"),
		schema.AssistantMessage("prev", nil),
		schema.AssistantMessage("", []schema.ToolCall{{Function: schema.FunctionCall{Name: "list_databases", Arguments: "{}"}}}),
		{Role: schema.Tool, Content: "rows", ToolName: "list_databases", ToolCallID: "c1"},
		{Role: schema.Tool, Content: "bare-tool"},
	}, model.WithModel(modelName), model.WithMaxTokens(maxTok), model.WithTemperature(temp),
		model.WithTools([]*schema.ToolInfo{
			nil,
			{Name: "list_databases", Desc: "list", ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"q": {Type: schema.String, Desc: "query", Required: false},
			})},
		}))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "plain answer" {
		t.Fatalf("content=%q", msg.Content)
	}
	if llm.last.Model != modelName {
		t.Fatalf("model=%q", llm.last.Model)
	}
	if llm.last.MaxTokens == nil || *llm.last.MaxTokens != maxTok {
		t.Fatalf("max tokens=%v", llm.last.MaxTokens)
	}
	if llm.last.Temperature == nil || *llm.last.Temperature != float64(temp) {
		t.Fatalf("temp=%v", llm.last.Temperature)
	}
	joined := ""
	for _, cm := range llm.last.Messages {
		joined += cm.Role + ":" + cm.Content + "\n"
	}
	if !strings.Contains(joined, "Available tools") || !strings.Contains(joined, "TOOL_CALL") || !strings.Contains(joined, "TOOL_RESULT") {
		t.Fatalf("messages:\n%s", joined)
	}

	llm.err = errors.New("chat down")
	if _, err := m.Generate(context.Background(), []*schema.Message{schema.UserMessage("x")}); err == nil {
		t.Fatal("expected chat error")
	}
}

func TestGatewayChatModelStreamPaths(t *testing.T) {
	t.Run("stream text chunks", func(t *testing.T) {
		llm := &scriptedChat{streamChunks: []string{"Hello ", "world from stream"}}
		m := newGatewayChatModel(llm, "p1", "m")
		sr, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("q")})
		if err != nil {
			t.Fatal(err)
		}
		var got strings.Builder
		for {
			chunk, err := sr.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if chunk != nil {
				got.WriteString(chunk.Content)
			}
		}
		if !strings.Contains(got.String(), "Hello") {
			t.Fatalf("got %q", got.String())
		}
	})
	t.Run("stream tool call and empty", func(t *testing.T) {
		llm := &scriptedChat{streamChunks: []string{`TOOL_CALL {"name":"list_databases","arguments":{}}`}}
		m := newGatewayChatModel(llm, "p1", "m")
		sr, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("q")})
		if err != nil {
			t.Fatal(err)
		}
		msg, err := sr.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if len(msg.ToolCalls) == 0 {
			t.Fatalf("want tool calls, got %+v", msg)
		}
		_, _ = sr.Recv()
	})
	t.Run("stream error then fallback chat", func(t *testing.T) {
		llm := &scriptedChat{streamErr: errors.New("no stream"), content: "fallback-ok"}
		m := newGatewayChatModel(llm, "p1", "m")
		sr, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("q")})
		if err != nil {
			t.Fatal(err)
		}
		msg, err := sr.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if msg.Content != "fallback-ok" {
			t.Fatalf("content=%q", msg.Content)
		}
	})
	t.Run("stream and chat both fail", func(t *testing.T) {
		llm := &scriptedChat{streamErr: errors.New("no stream"), err: errors.New("no chat")}
		m := newGatewayChatModel(llm, "p1", "m")
		if _, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("q")}); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("stream next error", func(t *testing.T) {
		llm := &scriptedChat{streamNextErr: errors.New("chunk fail"), streamChunks: []string{"x"}}
		m := newGatewayChatModel(llm, "p1", "m")
		sr, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("q")})
		if err != nil {
			t.Fatal(err)
		}
		for {
			_, err := sr.Recv()
			if err != nil {
				break
			}
		}
	})
}

func TestExtractToolCallBranches(t *testing.T) {
	cases := []struct {
		in      string
		wantOK  bool
		wantName string
	}{
		{"hello", false, ""},
		{`TOOL_CALL {"name":"t","arguments":{"a":1}}`, true, "t"},
		{"prefix TOOL_CALL not-json no braces", false, ""},
		{"TOOL_CALL {not json}", false, ""},
		{`noise TOOL_CALL ignore {"name":"x","arguments":{"k":true}} trail`, true, "x"},
		{`TOOL_CALL {"name":"","arguments":{}}`, false, ""},
		{`TOOL_CALL {"name":"only"}`, true, "only"},
	}
	for _, tc := range cases {
		name, args, ok := extractToolCall(tc.in)
		if ok != tc.wantOK || name != tc.wantName {
			t.Fatalf("in=%q ok=%v name=%q args=%s want ok=%v name=%q", tc.in, ok, name, args, tc.wantOK, tc.wantName)
		}
		if tc.wantOK && tc.in == `TOOL_CALL {"name":"only"}` && args != "{}" {
			t.Fatalf("default args=%s", args)
		}
	}
	if !looksLikeToolCall("x TOOL_CALL y") || looksLikeToolCall("plain") {
		t.Fatal("looksLikeToolCall")
	}
	msg := parseAssistant(`TOOL_CALL {"name":"list_objects","arguments":{}}`)
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("parse tool: %+v", msg)
	}
	if parseAssistant("just text").Content != "just text" {
		t.Fatal("parse text")
	}
	if mergeTools([]*schema.ToolInfo{{Name: "b"}}, nil)[0].Name != "b" {
		t.Fatal("merge bound")
	}
	if mergeTools([]*schema.ToolInfo{{Name: "b"}}, []*schema.ToolInfo{{Name: "o"}})[0].Name != "o" {
		t.Fatal("merge opt")
	}
	sr := singleMessageStream(schema.AssistantMessage("one", nil))
	got, err := sr.Recv()
	if err != nil || got.Content != "one" {
		t.Fatalf("single stream %v %v", got, err)
	}
	_, err = sr.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("eof=%v", err)
	}
}

func TestRuntimeNilAndCancel(t *testing.T) {
	var rt *Runtime
	if _, err := rt.StartRun(context.Background(), RunRequest{}, nil); err == nil {
		t.Fatal("nil runtime")
	}
	rt = &Runtime{}
	if _, err := rt.StartRun(context.Background(), RunRequest{}, nil); err == nil {
		t.Fatal("nil llm")
	}
	if rt.CancelRun("") || rt.CancelRun("missing") {
		t.Fatal("cancel empty/missing")
	}
	rt.storeCancel("", func() {})
	called := false
	rt.storeCancel("r1", func() { called = true })
	if !rt.CancelRun("r1") || !called {
		t.Fatal("cancel in-flight")
	}
	rt.clearCancel("r1")
	rt.clearCancel("gone")
	(*Runtime)(nil).storeCancel("x", func() {})
	(*Runtime)(nil).clearCancel("x")
	if (*Runtime)(nil).CancelRun("x") {
		t.Fatal("nil cancel")
	}
}

func TestRuntimeBuildSnapshotAndHistory(t *testing.T) {
	rt := &Runtime{}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: systemdb.CloudAgent{Module: ModuleDatabase}}), "unavailable") {
		t.Fatal("db unavailable")
	}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: systemdb.CloudAgent{Module: ModuleS3}}), "unavailable") {
		t.Fatal("obj unavailable")
	}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: systemdb.CloudAgent{Module: ModuleLogs}}), "unavailable") {
		t.Fatal("logs unavailable")
	}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: systemdb.CloudAgent{Module: ModuleGeneral}}), "no module snapshot") {
		t.Fatal("general snapshot")
	}

	rt.DB = &errDB{err: errors.New("db-fail")}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: systemdb.CloudAgent{Module: ModuleDatabase}}), "db-fail") {
		t.Fatal("db error")
	}
	many := make([]DatabaseInfo, 21)
	for i := range many {
		many[i] = DatabaseInfo{ID: "id", Name: "n", Status: "ready"}
	}
	rt.DB = &errDB{dbs: many}
	snap := rt.buildSnapshot(context.Background(), RunRequest{Agent: systemdb.CloudAgent{Module: ModuleDatabase}})
	if !strings.Contains(snap, "…") {
		t.Fatalf("expected truncation: %s", snap)
	}

	rt.Obj = &errObj{err: errors.New("obj-fail")}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: systemdb.CloudAgent{Module: ModuleS3}}), "obj-fail") {
		t.Fatal("obj error")
	}
	rt.Obj = &errObj{list: []ObjectInfo{{Key: "a/b", Size: 3}}}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: systemdb.CloudAgent{Module: ModuleS3}}), "a/b") {
		t.Fatal("obj list")
	}

	rt.Logs = &errLogs{err: errors.New("log-fail")}
	if strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: systemdb.CloudAgent{Module: ModuleLogs}}), "error=") {
		t.Fatal("log stats error is silent")
	}
	rt.Logs = &errLogs{stats: []systemdb.LogLevelCount{{Level: "info", Count: 2}, {Level: "error", Count: 1}}}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: systemdb.CloudAgent{Module: ModuleLogs}}), "info=2") {
		t.Fatal("log stats")
	}

	hist := historyToSchema([]systemdb.AgentMessage{
		{Role: "assistant", Content: "a"},
		{Role: "tool", Content: "t"},
		{Role: "system", Content: "s"},
		{Role: "user", Content: "u"},
	})
	if len(hist) != 4 || hist[0].Role != schema.Assistant || hist[2].Role != schema.System {
		t.Fatalf("hist=%+v", hist)
	}
	big := strings.Repeat("x", 3000)
	var lots []systemdb.AgentMessage
	for i := 0; i < 8; i++ {
		lots = append(lots, systemdb.AgentMessage{Role: "user", Content: big})
	}
	if len(historyToSchema(lots)) >= 8 {
		t.Fatal("history should stop after maxHistoryChars")
	}
	if safeAgentName("  ") != "agent" || safeAgentName(" Bot ") != "Bot" {
		t.Fatal("safeAgentName")
	}
	if firstNonEmpty("", "b") != "b" || firstNonEmpty("a", "b") != "a" {
		t.Fatal("firstNonEmpty")
	}
}

func TestConsumeVariantAndAssistant(t *testing.T) {
	if c, n := consumeVariant(nil); c != "" || n != "" {
		t.Fatal("nil variant")
	}
	c, n := consumeVariant(&adk.MessageVariant{Message: &schema.Message{Content: "body", ToolName: "tn"}})
	if c != "body" || n != "tn" {
		t.Fatalf("non-stream %s %s", c, n)
	}
	if c, n = consumeVariant(&adk.MessageVariant{ToolName: "only"}); c != "" || n != "only" {
		t.Fatalf("empty message %s %s", c, n)
	}

	sr, sw := schema.Pipe[*schema.Message](4)
	sw.Send(&schema.Message{Content: "p1", ToolName: "tool-a"}, nil)
	sw.Send(nil, nil)
	sw.Send(&schema.Message{Content: "p2"}, nil)
	sw.Close()
	c, n = consumeVariant(&adk.MessageVariant{IsStreaming: true, MessageStream: sr})
	if c != "p1p2" || n != "tool-a" {
		t.Fatalf("stream variant %q %q", c, n)
	}

	if msg, err := consumeAssistant(nil, nil, nil); msg != nil || err != nil {
		t.Fatal("nil assistant")
	}
	var acc strings.Builder
	var emitted []Event
	emit := func(e Event) { emitted = append(emitted, e) }
	msg, err := consumeAssistant(&adk.MessageVariant{Message: schema.AssistantMessage("tok", nil)}, emit, &acc)
	if err != nil || msg.Content != "tok" || acc.String() != "tok" || len(emitted) != 1 {
		t.Fatalf("non-stream assistant acc=%q ev=%v err=%v", acc.String(), emitted, err)
	}
	msg, err = consumeAssistant(&adk.MessageVariant{Message: schema.AssistantMessage("", []schema.ToolCall{{Function: schema.FunctionCall{Name: "t"}}})}, emit, &acc)
	if err != nil || msg == nil || len(msg.ToolCalls) != 1 {
		t.Fatal("tool-call message not accumulated")
	}
	if msg, err = consumeAssistant(&adk.MessageVariant{}, nil, &acc); msg != nil || err != nil {
		t.Fatal("empty variant")
	}

	sr, sw = schema.Pipe[*schema.Message](8)
	sw.Send(nil, nil)
	sw.Send(&schema.Message{Content: "chunk", Role: schema.Assistant}, nil)
	sw.Send(&schema.Message{Content: "TOOL_CALL {}", Role: schema.Assistant}, nil)
	sw.Send(&schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{Function: schema.FunctionCall{Name: "x", Arguments: "{}"}}}}, nil)
	sw.Send(nil, errors.New("mid-stream"))
	sw.Close()
	acc.Reset()
	emitted = nil
	last, err := consumeAssistant(&adk.MessageVariant{IsStreaming: true, MessageStream: sr}, emit, &acc)
	if err == nil || last == nil {
		t.Fatalf("want mid-stream error last=%v err=%v", last, err)
	}
	if !strings.Contains(acc.String(), "chunk") {
		t.Fatalf("acc=%q", acc.String())
	}

	sr, sw = schema.Pipe[*schema.Message](2)
	sw.Close()
	if _, err := consumeAssistant(&adk.MessageVariant{IsStreaming: true, MessageStream: sr}, nil, &acc); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeStartRunSettingsStreamAndHistory(t *testing.T) {
	rt := &Runtime{
		LLM:      &scriptedLLM{},
		DB:       &fakeDB{dbs: []DatabaseInfo{{ID: "d1", Name: "default", Status: "ready"}}},
		Settings: &fakeSettings{st: systemdb.LLMSettings{DefaultModel: "from-settings"}},
	}
	var evs []Event
	res, err := rt.StartRun(context.Background(), RunRequest{
		ProjectID: "p1",
		Principal: auth.Principal{},
		Agent: systemdb.CloudAgent{
			ID: "a1", Name: "  ", Module: ModuleDatabase, ToolIDs: []string{ToolListDatabases},
		},
		RunID:    "run-settings",
		UserText: "What databases exist?",
		History: []systemdb.AgentMessage{
			{Role: "user", Content: "earlier"},
			{Role: "assistant", Content: "ok"},
			{Role: "tool", Content: "[]"},
			{Role: "system", Content: "note"},
		},
		Stream: true,
	}, func(e Event) { evs = append(evs, e) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "default") {
		t.Fatalf("content=%q", res.Content)
	}
	if len(evs) == 0 {
		t.Fatal("expected stream events")
	}

	rt.Settings = &fakeSettings{err: errors.New("settings down")}
	if _, err := rt.StartRun(context.Background(), RunRequest{
		ProjectID: "p1",
		Agent:     systemdb.CloudAgent{ID: "a2", Name: "Gen", Module: ModuleGeneral},
		RunID:     "run-gen",
		UserText:  "hi",
	}, nil); err != nil {
		t.Fatal(err)
	}

	rt.Obj = &errObj{list: []ObjectInfo{{Key: "k", Size: 1}}}
	if _, err := rt.StartRun(context.Background(), RunRequest{
		ProjectID: "p1",
		Agent:     systemdb.CloudAgent{ID: "a3", Name: "S3", Module: ModuleS3, ToolIDs: []string{ToolListObjects}},
		RunID:     "run-s3",
		UserText:  "list",
	}, nil); err != nil {
		t.Fatal(err)
	}
	rt.Logs = &errLogs{stats: []systemdb.LogLevelCount{{Level: "info", Count: 1}}}
	if _, err := rt.StartRun(context.Background(), RunRequest{
		ProjectID: "p1",
		Agent:     systemdb.CloudAgent{ID: "a4", Name: "Logs", Module: ModuleLogs, ToolIDs: []string{ToolLogLevelStats}},
		RunID:     "run-logs",
		UserText:  "stats",
	}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestStoreAccessWrappers(t *testing.T) {
	if NewLogAccess(nil) != nil || NewSettingsAccess(nil) != nil {
		t.Fatal("nil store")
	}
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
	store.RecordLog(systemdb.LogEvent{ProjectID: "p1", Level: "info", Logger: "t", Message: "hello coverage"})
	if err := store.FlushLogs(ctx); err != nil {
		t.Fatal(err)
	}
	logs := NewLogAccess(store)
	events, err := logs.SearchLogs(ctx, "p1", "info", "coverage", 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("search n=%d err=%v", len(events), err)
	}
	stats, err := logs.LevelStats(ctx, "p1")
	if err != nil || len(stats) == 0 {
		t.Fatalf("stats=%v err=%v", stats, err)
	}
	settings := NewSettingsAccess(store)
	st, err := settings.LLMSettings(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if st.ProjectID != "p1" {
		t.Fatalf("default settings %+v", st)
	}
	if err := store.PutLLMSettings(ctx, systemdb.LLMSettings{ProjectID: "p1", DefaultModel: "m1", Temperature: 0.1, MaxTokens: 16}); err != nil {
		t.Fatal(err)
	}
	st, err = settings.LLMSettings(ctx, "p1")
	if err != nil || st.DefaultModel != "m1" {
		t.Fatalf("settings %+v err=%v", st, err)
	}
}

type scriptedChat struct {
	content      string
	err          error
	streamErr    error
	streamNextErr error
	streamChunks []string
	last         ChatRequest
}

func (s *scriptedChat) Chat(_ context.Context, _ string, req ChatRequest) (ChatResponse, error) {
	s.last = req
	if s.err != nil {
		return ChatResponse{}, s.err
	}
	return ChatResponse{Content: s.content}, nil
}

func (s *scriptedChat) Stream(_ context.Context, _ string, req ChatRequest) (TokenStream, error) {
	s.last = req
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	return &seqStream{chunks: s.streamChunks, err: s.streamNextErr}, nil
}

type seqStream struct {
	chunks []string
	i      int
	err    error
}

func (s *seqStream) Next() (string, bool, error) {
	if s.i >= len(s.chunks) {
		if s.err != nil {
			return "", false, s.err
		}
		return "", true, io.EOF
	}
	c := s.chunks[s.i]
	s.i++
	finish := s.i >= len(s.chunks)
	return c, finish, nil
}
func (s *seqStream) Close() error { return nil }

type fakeSettings struct {
	st  systemdb.LLMSettings
	err error
}

func (f *fakeSettings) LLMSettings(context.Context, string) (systemdb.LLMSettings, error) {
	return f.st, f.err
}

type errDB struct {
	dbs []DatabaseInfo
	err error
}

func (f *errDB) ListDatabases(context.Context, auth.Principal, string) ([]DatabaseInfo, error) {
	return f.dbs, f.err
}
func (f *errDB) ListCollections(context.Context, auth.Principal, string, string) ([]string, error) {
	return nil, f.err
}
func (f *errDB) ReadOnlyQuery(context.Context, auth.Principal, string, string, string, int) (SQLResult, error) {
	return SQLResult{}, f.err
}

type errObj struct {
	list []ObjectInfo
	head ObjectInfo
	err  error
}

func (f *errObj) ListObjects(context.Context, string, string, int) ([]ObjectInfo, error) {
	return f.list, f.err
}
func (f *errObj) HeadObject(context.Context, string, string) (ObjectInfo, error) {
	return f.head, f.err
}

type errLogs struct {
	events []systemdb.LogEvent
	stats  []systemdb.LogLevelCount
	err    error
}

func (f *errLogs) SearchLogs(context.Context, string, string, string, int) ([]systemdb.LogEvent, error) {
	return f.events, f.err
}
func (f *errLogs) LevelStats(context.Context, string) ([]systemdb.LogLevelCount, error) {
	return f.stats, f.err
}
