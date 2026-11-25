// 文件说明：model_test.go — gocloudagent/cloudagent 的单元测试（TestGatewayChatModelGenerateAndOptions、TestGatewayChatModelStreamPaths 等）
//
// 用法
//
//	go test ./gocloudagent/cloudagent -run 'TestGatewayChatModelGenerateAndOptions|TestGatewayChatModelStreamPaths|TestExtractToolCallBranches'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/model_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//
// 主要符号
//
//	TestGatewayChatModelGenerateAndOptions(t *testing.T)
//	TestGatewayChatModelStreamPaths(t *testing.T)
//	TestExtractToolCallBranches(t *testing.T)

package cloudagent

import (
	"context"
	"errors"
	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestGatewayChatModelGenerateAndOptions(t *testing.T) {
	llm := &scriptedChat{content: "plain answer"}
	m, err := agentcore.NewModel(llm, "p1", "base-model").WithTools([]*schema.ToolInfo{{Name: "bound", Desc: "d"}})
	if err != nil {
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
		m := agentcore.NewModel(llm, "p1", "m")
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
		m := agentcore.NewModel(llm, "p1", "m")
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
		m := agentcore.NewModel(llm, "p1", "m")
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
		m := agentcore.NewModel(llm, "p1", "m")
		if _, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("q")}); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("stream next error", func(t *testing.T) {
		llm := &scriptedChat{streamNextErr: errors.New("chunk fail"), streamChunks: []string{"x"}}
		m := agentcore.NewModel(llm, "p1", "m")
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
		in       string
		wantOK   bool
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
		name, args, ok := agentcore.ExtractToolCall(tc.in)
		if ok != tc.wantOK || name != tc.wantName {
			t.Fatalf("in=%q ok=%v name=%q args=%s want ok=%v name=%q", tc.in, ok, name, args, tc.wantOK, tc.wantName)
		}
		if tc.wantOK && tc.in == `TOOL_CALL {"name":"only"}` && args != "{}" {
			t.Fatalf("default args=%s", args)
		}
	}
	if _, _, ok := agentcore.ExtractToolCall("plain"); ok {
		t.Fatal("plain text should not parse as tool call")
	}
	msg := agentcore.ParseAssistant(`TOOL_CALL {"name":"list_objects","arguments":{}}`)
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("parse tool: %+v", msg)
	}
	if agentcore.ParseAssistant("just text").Content != "just text" {
		t.Fatal("parse text")
	}
}
