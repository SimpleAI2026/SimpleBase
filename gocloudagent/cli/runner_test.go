package cli

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// scriptedClient 按顺序返回预设回复，并记录收到的请求。
type scriptedClient struct {
	mu       sync.Mutex
	replies  []agentcore.ChatResponse
	requests []agentcore.ChatRequest
	// delay 让极快的假客户端也能产生非零的毫秒耗时。
	delay time.Duration
}

func (c *scriptedClient) Chat(_ context.Context, _ string, req agentcore.ChatRequest) (agentcore.ChatResponse, error) {
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
	if len(c.replies) == 0 {
		return agentcore.ChatResponse{Content: "done"}, nil
	}
	r := c.replies[0]
	c.replies = c.replies[1:]
	return r, nil
}

// Stream 必须把工具调用也变成增量帧：ADK 在 EnableStreaming 下只走 Stream，
// 丢掉 ToolCalls 会让工具永远不被执行（这里正是 TestRunnerToolCallSequence 的断言点）。
func (c *scriptedClient) Stream(ctx context.Context, projectID string, req agentcore.ChatRequest) (agentcore.TokenStream, error) {
	resp, err := c.Chat(ctx, projectID, req)
	if err != nil {
		return nil, err
	}
	return &scriptedDeltaStream{resp: resp, phase: 0}, nil
}

// scriptedDeltaStream 实现 DeltaStream：先发工具调用增量，再发正文，最后发 usage。
type scriptedDeltaStream struct {
	resp  agentcore.ChatResponse
	phase int // 0=content/toolcall, 1=usage, 2=finish
}

func (s *scriptedDeltaStream) Next() (string, bool, error) { return "", true, io.EOF }

func (s *scriptedDeltaStream) NextDelta() (agentcore.StreamDelta, error) {
	switch s.phase {
	case 0:
		s.phase++
		if len(s.resp.ToolCalls) > 0 {
			tc := s.resp.ToolCalls[0]
			return agentcore.StreamDelta{ToolCall: &agentcore.ToolCallDelta{
				Index: 0, ID: tc.ID, Name: tc.Name, ArgsDelta: tc.Arguments,
			}}, nil
		}
		return agentcore.StreamDelta{Content: s.resp.Content}, nil
	case 1:
		s.phase++
		return agentcore.StreamDelta{Usage: &s.resp.Usage}, nil
	default:
		return agentcore.StreamDelta{Finish: true}, io.EOF
	}
}

func (s *scriptedDeltaStream) Close() error { return nil }

func (c *scriptedClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.requests)
}

// echoTool 是一个可控的工具，用于验证事件序列与落盘。
func echoTool(t *testing.T, calls *int) tool.BaseTool {
	t.Helper()
	tl, err := utils.InferTool("echo", "echo back the given text",
		func(_ context.Context, in struct {
			Text string `json:"text"`
		}) (string, error) {
			*calls++
			return `{"ok":true,"text":"` + in.Text + `"}`, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	return tl
}

// 普通文本回复：token 事件、无工具、落盘 user+assistant 两行。
func TestRunnerPlainReply(t *testing.T) {
	s := newTestSession(t)
	client := &scriptedClient{replies: []agentcore.ChatResponse{{Content: "hello there"}}}
	var events []CLIEvent
	r := &SessionRunner{Session: s, Client: client}
	res, err := r.Run(context.Background(), "hi", func(e CLIEvent) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "hello there" || res.Reason != "stop" {
		t.Fatalf("result = %+v", res)
	}
	if len(events) == 0 || events[0].Type != "token" {
		t.Fatalf("events = %+v", events)
	}
	msgs := s.Messages()
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("messages = %+v", msgs)
	}
	if msgs[0].Content != "hi" || msgs[1].Content != "hello there" {
		t.Fatalf("content = %+v", msgs)
	}
}

// 工具调用：tool_call + tool_result 事件出现，工具落盘，且结果被回灌给模型。
func TestRunnerToolCallSequence(t *testing.T) {
	s := newTestSession(t)
	calls := 0
	client := &scriptedClient{replies: []agentcore.ChatResponse{
		{ToolCalls: []agentcore.ChatToolCall{{ID: "c1", Name: "echo", Arguments: `{"text":"ping"}`}}},
		{Content: "final answer"},
	}}
	var events []CLIEvent
	r := &SessionRunner{Session: s, Client: client, Tools: []tool.BaseTool{echoTool(t, &calls)}}
	res, err := r.Run(context.Background(), "please echo", func(e CLIEvent) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("tool calls = %d, want 1", calls)
	}
	if res.ToolCalls != 1 {
		t.Fatalf("result.ToolCalls = %d", res.ToolCalls)
	}
	var sawCall, sawResult bool
	for _, e := range events {
		switch e.Type {
		case "tool_call":
			sawCall = true
			if e.CallID != "c1" || e.Name != "echo" {
				t.Fatalf("tool_call event = %+v", e)
			}
		case "tool_result":
			sawResult = true
			if e.CallID != "c1" || e.IsError {
				t.Fatalf("tool_result event = %+v", e)
			}
		}
	}
	if !sawCall || !sawResult {
		t.Fatalf("missing tool events: %+v", events)
	}
	// 落盘：user / assistant(带 tool_calls) / tool / assistant(最终)
	roles := []string{}
	for _, m := range s.Messages() {
		roles = append(roles, m.Role)
	}
	if strings.Join(roles, ",") != "user,assistant,tool,assistant" {
		t.Fatalf("roles = %v", roles)
	}
	for _, m := range s.Messages() {
		if m.Role == "assistant" && len(m.ToolCalls) == 1 && m.ToolCalls[0].Name != "echo" {
			t.Fatalf("tool call not persisted: %+v", m)
		}
	}
	if res.Content != "final answer" {
		t.Fatalf("content = %q", res.Content)
	}
}

// 未知工具名不能让整轮失败（§13.1）——错误结果回给模型，模型继续作答。
func TestRunnerUnknownToolDoesNotFailRun(t *testing.T) {
	s := newTestSession(t)
	client := &scriptedClient{replies: []agentcore.ChatResponse{
		{ToolCalls: []agentcore.ChatToolCall{{ID: "c1", Name: "no_such_tool", Arguments: "{}"}}},
		{Content: "recovered"},
	}}
	// 必须有至少一个真实工具，ADK 才会建 ToolsNode 去处理（真实 CLI 始终有 shell 等）。
	calls := 0
	r := &SessionRunner{Session: s, Client: client, Tools: []tool.BaseTool{echoTool(t, &calls)}}
	res, err := r.Run(context.Background(), "go", nil)
	if err != nil {
		t.Fatalf("unknown tool must not fail the run: %v", err)
	}
	if res.Content != "recovered" {
		t.Fatalf("content = %q", res.Content)
	}
	// eino 的 unknown-tool 任务不设 tool name（compose.newUnknownToolTask），
	// 因此它的结果不会作为 tool 消息进入事件流，也就不会落成 tool 行。
	// 这里的契约是「整轮不失败，且模型确实收到了 unknown_tool 错误」——
	// 通过第二次请求的 tool 结果消息来验证。
	var delivered bool
	for _, req := range client.requests {
		for _, m := range req.Messages {
			if strings.Contains(m.Content, "unknown_tool") {
				delivered = true
			}
		}
	}
	if !delivered {
		t.Fatal("the unknown_tool error must be delivered back to the model")
	}
	// 回归保护：真实工具的结果仍然必须落盘为 tool 行。
	if res.ToolCalls != 1 {
		t.Fatalf("res.ToolCalls = %d, want 1", res.ToolCalls)
	}
}

// 落盘内容与返回内容都经过脱敏。
func TestRunnerRedactsSecrets(t *testing.T) {
	s := newTestSession(t)
	secret := "sk-abcdefghijklmnopqrstuvwxyz"
	client := &scriptedClient{replies: []agentcore.ChatResponse{{Content: "the key is " + secret}}}
	r := &SessionRunner{Session: s, Client: client}
	res, err := r.Run(context.Background(), "what is the key "+secret, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Content, secret) {
		t.Fatalf("returned content leaked the secret: %q", res.Content)
	}
	for _, m := range s.Messages() {
		if strings.Contains(m.Content, secret) {
			t.Fatalf("persisted message leaked the secret: %+v", m)
		}
	}
}

// 未配置客户端时明确报错，不 panic。
func TestRunnerRequiresClientAndSession(t *testing.T) {
	if _, err := (&SessionRunner{}).Run(context.Background(), "x", nil); err == nil {
		t.Fatal("missing client must error")
	}
	s := newTestSession(t)
	if _, err := (&SessionRunner{Client: &scriptedClient{}}).Run(context.Background(), "x", nil); err == nil {
		t.Fatal("missing session must error")
	}
	_ = s
}

// 客户端报错时返回错误，且已写下的 user 行保留（会话可继续）。
func TestRunnerPropagatesClientError(t *testing.T) {
	s := newTestSession(t)
	client := &scriptedClient{}
	client.replies = nil
	rc := &erroringClient{}
	r := &SessionRunner{Session: s, Client: rc}
	if _, err := r.Run(context.Background(), "x", nil); err == nil {
		t.Fatal("want error from client")
	}
	if len(s.Messages()) == 0 {
		t.Fatal("user message should still be persisted")
	}
}

type erroringClient struct{}

func (erroringClient) Chat(context.Context, string, agentcore.ChatRequest) (agentcore.ChatResponse, error) {
	return agentcore.ChatResponse{}, errors.New("upstream down")
}

func (erroringClient) Stream(context.Context, string, agentcore.ChatRequest) (agentcore.TokenStream, error) {
	return nil, errors.New("upstream down")
}

// RunResult 必须包含 usage 与 model（planv6.1 O1-1）——让成本核算与护栏能接通。
func TestRunResultIncludesUsageAndModel(t *testing.T) {
	s := newTestSession(t)
	client := &scriptedClient{replies: []agentcore.ChatResponse{{
		Content: "answer",
		Usage:   agentcore.ChatUsage{PromptTokens: 100, CompletionTokens: 50},
		Model:   "test-model",
	}}}
	client.delay = 5 * time.Millisecond
	r := &SessionRunner{Session: s, Client: client, Model: "test-model"}
	res, err := r.Run(context.Background(), "question", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.PromptTokens != 100 || res.CompletionTokens != 50 {
		t.Fatalf("usage = %d/%d, want 100/50", res.PromptTokens, res.CompletionTokens)
	}
	if res.Model != "test-model" {
		t.Fatalf("model = %q, want %q", res.Model, "test-model")
	}
	if res.DurationMS <= 0 {
		t.Fatal("duration must be > 0")
	}
}

var _ = schema.AssistantMessage
