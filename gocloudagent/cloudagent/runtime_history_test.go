package cloudagent

import (
	"context"
	"errors"
	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

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
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: Agent{Module: ModuleDatabase}}), "unavailable") {
		t.Fatal("db unavailable")
	}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: Agent{Module: ModuleS3}}), "unavailable") {
		t.Fatal("obj unavailable")
	}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: Agent{Module: ModuleLogs}}), "unavailable") {
		t.Fatal("logs unavailable")
	}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: Agent{Module: ModuleGeneral}}), "no module snapshot") {
		t.Fatal("general snapshot")
	}

	rt.DB = &errDB{err: errors.New("db-fail")}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: Agent{Module: ModuleDatabase}}), "db-fail") {
		t.Fatal("db error")
	}
	many := make([]DatabaseInfo, 21)
	for i := range many {
		many[i] = DatabaseInfo{ID: "id", Name: "n", Status: "ready"}
	}
	rt.DB = &errDB{dbs: many}
	snap := rt.buildSnapshot(context.Background(), RunRequest{Agent: Agent{Module: ModuleDatabase}})
	if !strings.Contains(snap, "…") {
		t.Fatalf("expected truncation: %s", snap)
	}

	rt.Obj = &errObj{err: errors.New("obj-fail")}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: Agent{Module: ModuleS3}}), "obj-fail") {
		t.Fatal("obj error")
	}
	rt.Obj = &errObj{list: []ObjectInfo{{Key: "a/b", Size: 3}}}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: Agent{Module: ModuleS3}}), "a/b") {
		t.Fatal("obj list")
	}

	rt.Logs = &errLogs{err: errors.New("log-fail")}
	if strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: Agent{Module: ModuleLogs}}), "error=") {
		t.Fatal("log stats error is silent")
	}
	rt.Logs = &errLogs{stats: []LogLevelCount{{Level: "info", Count: 2}, {Level: "error", Count: 1}}}
	if !strings.Contains(rt.buildSnapshot(context.Background(), RunRequest{Agent: Agent{Module: ModuleLogs}}), "info=2") {
		t.Fatal("log stats")
	}

	hist := historyToSchema([]Message{
		{Role: "assistant", Content: "a"},
		{Role: "tool", Content: "t"},
		{Role: "system", Content: "s"},
		{Role: "user", Content: "u"},
	})
	if len(hist) != 4 || hist[0].Role != schema.Assistant || hist[2].Role != schema.System {
		t.Fatalf("hist=%+v", hist)
	}
	big := strings.Repeat("x", 3000)
	var lots []Message
	for i := 0; i < 8; i++ {
		lots = append(lots, Message{Role: "user", Content: big})
	}
	if len(historyToSchema(lots)) >= 8 {
		t.Fatal("history should stop after maxHistoryChars")
	}
	if safeAgentName("  ") != "agent" || safeAgentName(" Bot ") != "Bot" {
		t.Fatal("safeAgentName")
	}
}

func TestConsumeVariantAndAssistant(t *testing.T) {
	if c, n := agentcore.ConsumeVariant(nil); c != "" || n != "" {
		t.Fatal("nil variant")
	}
	c, n := agentcore.ConsumeVariant(&adk.MessageVariant{Message: &schema.Message{Content: "body", ToolName: "tn"}})
	if c != "body" || n != "tn" {
		t.Fatalf("non-stream %s %s", c, n)
	}
	if c, n = agentcore.ConsumeVariant(&adk.MessageVariant{ToolName: "only"}); c != "" || n != "only" {
		t.Fatalf("empty message %s %s", c, n)
	}

	sr, sw := schema.Pipe[*schema.Message](4)
	sw.Send(&schema.Message{Content: "p1", ToolName: "tool-a"}, nil)
	sw.Send(nil, nil)
	sw.Send(&schema.Message{Content: "p2"}, nil)
	sw.Close()
	c, n = agentcore.ConsumeVariant(&adk.MessageVariant{IsStreaming: true, MessageStream: sr})
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
		Settings: &fakeSettings{st: LLMSettings{DefaultModel: "from-settings"}},
	}
	var evs []Event
	res, err := rt.StartRun(context.Background(), RunRequest{
		ProjectID: "p1",
		Principal: Principal{},
		Agent: Agent{
			ID: "a1", Name: "  ", Module: ModuleDatabase, ToolIDs: []string{ToolListDatabases},
		},
		RunID:    "run-settings",
		UserText: "What databases exist?",
		History: []Message{
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
		Agent:     Agent{ID: "a2", Name: "Gen", Module: ModuleGeneral},
		RunID:     "run-gen",
		UserText:  "hi",
	}, nil); err != nil {
		t.Fatal(err)
	}

	rt.Obj = &errObj{list: []ObjectInfo{{Key: "k", Size: 1}}}
	if _, err := rt.StartRun(context.Background(), RunRequest{
		ProjectID: "p1",
		Agent:     Agent{ID: "a3", Name: "S3", Module: ModuleS3, ToolIDs: []string{ToolListObjects}},
		RunID:     "run-s3",
		UserText:  "list",
	}, nil); err != nil {
		t.Fatal(err)
	}
	rt.Logs = &errLogs{stats: []LogLevelCount{{Level: "info", Count: 1}}}
	if _, err := rt.StartRun(context.Background(), RunRequest{
		ProjectID: "p1",
		Agent:     Agent{ID: "a4", Name: "Logs", Module: ModuleLogs, ToolIDs: []string{ToolLogLevelStats}},
		RunID:     "run-logs",
		UserText:  "stats",
	}, nil); err != nil {
		t.Fatal(err)
	}
}
