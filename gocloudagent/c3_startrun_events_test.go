package gocloudagent

// c3_startrun_events_test.go：planv6.0 C3 表征测试（零生产修改）。
// 在现有 runtime/run_limits/optimization 测试基础上补齐 StartRun 事件
// 归集与结果生成的可观察行为：
//   1. usage 缺失（上游不回用量）→ 结果用量为零、Reason 仍 stop；
//   2. 流式多次增量 token → content 全量拼接、事件顺序 token 先于完成；
//   3. tool 失败（is_error）→ tool_result 事件带 IsError、run 本身不报错；
//   4. 结果生成：ToolCallsJSON 为合法 JSON 数组、call_id/name 配对保留。
// 本文件只补测试；生产代码不动（C 类默认仅分析 + 表征）。

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// c3ErrDB：ListDatabases 恒定失败，制造 tool 错误路径。
type c3ErrDB struct{}

func (*c3ErrDB) ListDatabases(context.Context, Principal, string) ([]DatabaseInfo, error) {
	return nil, errors.New("db down")
}
func (*c3ErrDB) ListCollections(context.Context, Principal, string, string) ([]string, error) {
	return nil, errors.New("db down")
}
func (*c3ErrDB) ReadOnlyQuery(_ context.Context, _ Principal, _, _, _ string, _ int) (SQLResult, error) {
	return SQLResult{}, errors.New("db down")
}

// c3NoUsageLLM：流式输出两个增量，不回 usage。
type c3NoUsageLLM struct{}

func (*c3NoUsageLLM) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, errors.New("unexpected chat")
}
func (*c3NoUsageLLM) Stream(context.Context, string, ChatRequest) (TokenStream, error) {
	return &c3MultiStream{chunks: []string{"你好", "，世界"}}, nil
}

type c3MultiStream struct {
	chunks []string
	idx    int
}

func (s *c3MultiStream) Next() (string, bool, error) {
	if s.idx >= len(s.chunks) {
		return "", true, io.EOF
	}
	s.idx++
	return s.chunks[s.idx-1], false, nil
}
func (*c3MultiStream) Close() error { return nil }

// 1+2. usage 缺失与多增量拼接。
func TestC3_StreamMultiDeltaWithoutUsage(t *testing.T) {
	rt := &Runtime{LLM: &c3NoUsageLLM{}}
	var events []Event
	result, err := rt.StartRun(context.Background(), RunRequest{
		ProjectID: "p", ThreadID: "t", RunID: "r-m",
		UserText: "hi", Stream: true,
		Agent: Agent{ID: "a", Name: "General", Module: ModuleGeneral},
	}, func(ev Event) { events = append(events, ev) })
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "你好，世界" {
		t.Fatalf("content=%q want 你好，世界", result.Content)
	}
	if result.PromptTokens != 0 || result.CompletionTokens != 0 || result.ReasoningTokens != 0 {
		t.Fatalf("usage must stay zero when upstream omits it: %+v", result)
	}
	if result.Reason != "stop" {
		t.Fatalf("reason=%q want stop", result.Reason)
	}
	// 事件轨迹：全部为 token，完成即结束（无 error 事件）。
	for _, ev := range events {
		if ev.Type != "token" {
			t.Fatalf("unexpected event type %q", ev.Type)
		}
	}
	if len(events) < 2 {
		t.Fatalf("expected >=2 token events, got %d", len(events))
	}
	var joined strings.Builder
	for _, ev := range events {
		joined.WriteString(ev.Content)
	}
	if joined.String() != result.Content {
		t.Fatalf("events %q != content %q", joined.String(), result.Content)
	}
}

// 3+4. tool 失败仍产出 is_error 结果卡片；ToolCallsJSON 合法且配对。
func TestC3_ToolFailureResultCard(t *testing.T) {
	// scriptedLLM 第二轮返回结论；list_databases 会命中 fakeDB（成功）。
	// 用错误注入的 DB 制造 tool 失败：fakeDB errors 见 tools_test.go。
	rt := &Runtime{
		LLM: &scriptedLLM{},
		DB:  &c3ErrDB{},
	}
	var events []Event
	result, err := rt.StartRun(context.Background(), RunRequest{
		ProjectID: "p", ThreadID: "t", RunID: "r-f",
		UserText: "list", Stream: true,
		Agent: Agent{ID: "a", Name: "Database", Module: ModuleDatabase, ToolIDs: []string{ToolListDatabases}},
	}, func(ev Event) { events = append(events, ev) })
	if err != nil {
		t.Fatalf("tool failure must not fail the run itself: %v", err)
	}
	var callEv, resultEv *Event
	for i := range events {
		switch events[i].Type {
		case "tool_call":
			if callEv == nil {
				callEv = &events[i]
			}
		case "tool_result":
			if resultEv == nil {
				resultEv = &events[i]
			}
		}
	}
	if callEv == nil || resultEv == nil {
		t.Fatalf("expected tool_call + tool_result events: %+v", events)
	}
	if callEv.Name != "list_databases" || resultEv.Name != "list_databases" {
		t.Fatalf("name pairing broken: call=%s result=%s", callEv.Name, resultEv.Name)
	}
	// fake 工具错误必须体现在 is_error（错误语义由 tool 自身决定）。
	if !resultEv.IsError {
		t.Fatalf("failing tool result must set IsError: %+v", *resultEv)
	}
	// ToolCallsJSON 合法数组。
	var cards []map[string]any
	if err := json.Unmarshal([]byte(result.ToolCallsJSON), &cards); err != nil {
		t.Fatalf("ToolCallsJSON invalid: %v (%s)", err, result.ToolCallsJSON)
	}
	if len(cards) == 0 {
		t.Fatal("expected tool cards")
	}
	found := false
	for _, c := range cards {
		if c["is_error"] == true {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected is_error card in %s", result.ToolCallsJSON)
	}
	if result.ToolCalls != 1 {
		t.Fatalf("tool calls=%d want 1", result.ToolCalls)
	}
}
