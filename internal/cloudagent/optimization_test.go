package cloudagent

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type blockingStream struct {
	start chan struct{}
	first bool
}

func (s *blockingStream) Next() (string, bool, error) {
	if !s.first {
		s.first = true
		return "你好", false, nil
	}
	<-s.start
	return "世界", true, nil
}
func (*blockingStream) Close() error { return nil }

type blockingChat struct{ stream *blockingStream }

func (*blockingChat) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, nil
}
func (c *blockingChat) Stream(context.Context, string, ChatRequest) (TokenStream, error) {
	return c.stream, nil
}

func TestAgentStreamForwardsBeforeUpstreamCompletes(t *testing.T) {
	upstream := &blockingStream{start: make(chan struct{})}
	defer close(upstream.start)
	m := newGatewayChatModel(&blockingChat{stream: upstream}, "p", "m")
	sr, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Close()
	got := make(chan string, 1)
	go func() {
		msg, _ := sr.Recv()
		if msg != nil {
			got <- msg.Content
		}
	}()
	select {
	case text := <-got:
		if text != "你好" {
			t.Fatalf("first chunk=%q", text)
		}
	case <-time.After(time.Second):
		t.Fatal("stream buffered whole response")
	}
}

func TestAgentStreamToolPrefixAndLiteral(t *testing.T) {
	for _, tc := range []struct {
		chunks   []string
		wantTool bool
		wantText string
	}{
		{[]string{"TOOL", `_CALL {"name":"list_databases","arguments":{}}`}, true, ""},
		{[]string{"示例：", "TOOL_CALL 是关键字"}, false, "示例：TOOL_CALL 是关键字"},
		{[]string{"\n  TOOL", `_CALL {"name":"list_databases","arguments":{}}`}, true, ""},
	} {
		m := newGatewayChatModel(&scriptedChat{streamChunks: tc.chunks}, "p", "m")
		sr, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("q")})
		if err != nil {
			t.Fatal(err)
		}
		var text string
		var tools int
		for {
			msg, err := sr.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			text += msg.Content
			tools += len(msg.ToolCalls)
		}
		if text != tc.wantText || (tools > 0) != tc.wantTool {
			t.Fatalf("chunks=%v text=%q tools=%d", tc.chunks, text, tools)
		}
	}
}

func TestAgentHistoryIncludesToolResults(t *testing.T) {
	msg := systemdb.AgentMessage{Role: "assistant", Content: "Done", ToolCallsJSON: `[{"name":"list_databases","content":"orders, users; sk-abcdefghijklmnopqrstuvwxyz"}]`}
	hist := historyToSchema([]systemdb.AgentMessage{msg})
	if len(hist) != 2 || !strings.Contains(hist[0].Content, "orders, users") || strings.Contains(hist[0].Content, "sk-abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("history=%+v", hist)
	}
}

func TestAgentRunToolEventsHaveMatchingCallID(t *testing.T) {
	rt := &Runtime{LLM: &scriptedLLM{}, DB: &fakeDB{dbs: []DatabaseInfo{{ID: "d1", Name: "default", Status: "ready"}}}}
	var calls, results []Event
	res, err := rt.StartRun(context.Background(), RunRequest{
		ProjectID: "p", Principal: auth.Principal{}, ThreadID: "th", RunID: "r1",
		Agent:    systemdb.CloudAgent{ID: "a", Name: "Database", Module: ModuleDatabase, ToolIDs: []string{ToolListDatabases}},
		UserText: "list databases", Stream: true,
	}, func(e Event) {
		if e.Type == "tool_call" {
			calls = append(calls, e)
		}
		if e.Type == "tool_result" {
			results = append(results, e)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || len(results) != 1 || calls[0].CallID == "" || results[0].CallID != calls[0].CallID || res.ToolCalls != 1 {
		t.Fatalf("calls=%+v results=%+v res=%+v", calls, results, res)
	}
}

func TestAgentThreadBusy(t *testing.T) {
	rt := &Runtime{}
	if err := rt.ClaimThread("th", "r1"); err != nil {
		t.Fatal(err)
	}
	if err := rt.ClaimThread("th", "r2"); !errors.Is(err, ErrThreadBusy) {
		t.Fatalf("busy=%v", err)
	}
	rt.ReleaseThread("th", "r1")
	if err := rt.ClaimThread("th", "r2"); err != nil {
		t.Fatal(err)
	}
}
