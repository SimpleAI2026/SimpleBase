package agentcore

import (
	"context"
	"io"
	"strings"
	"testing"
)

// chunkStream 按给定分片依次吐出文本，模拟网关的 SSE 内容帧。
type chunkStream struct {
	chunks []string
	i      int
}

func (s *chunkStream) Next() (string, bool, error) {
	if s.i >= len(s.chunks) {
		return "", true, io.EOF
	}
	c := s.chunks[s.i]
	s.i++
	return c, false, nil
}
func (s *chunkStream) Close() error { return nil }

type chunkClient struct{ chunks []string }

func (c *chunkClient) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, nil
}

func (c *chunkClient) Stream(context.Context, string, ChatRequest) (TokenStream, error) {
	return &chunkStream{chunks: c.chunks}, nil
}

// streamOutcome 把一次流式调用的结果压成「工具调用 + 正文」，便于断言。
func streamOutcome(t *testing.T, chunks []string) (calls []string, text string) {
	t.Helper()
	m := NewModel(&chunkClient{chunks: chunks}, "", "m")
	sr, err := m.Stream(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for {
		msg, err := sr.Recv()
		if err != nil {
			break
		}
		for _, tc := range msg.ToolCalls {
			calls = append(calls, tc.Function.Name+" "+tc.Function.Arguments)
		}
		b.WriteString(msg.Content)
	}
	return calls, b.String()
}

// 纯 TOOL_CALL 开头：整段被识别为一次工具调用，不把调用文本当正文吐给用户。
func TestStreamPureToolCallIsParsed(t *testing.T) {
	calls, text := streamOutcome(t, []string{`TOOL_CALL {"name":"shell","arguments":{"command":"ls"}}`})
	if len(calls) != 1 || calls[0] != `shell {"command":"ls"}` {
		t.Fatalf("calls = %v", calls)
	}
	if text != "" {
		t.Fatalf("tool call text must not leak as prose: %q", text)
	}
}

// 回归：尾部带空代码块和第二个拼错的调用（用户的真实输出）。
// 之前整段 Unmarshal 失败 → 被当成普通文本 → agent 当轮结束、工具一次都没跑。
func TestStreamToolCallWithTrailingNoiseStillRuns(t *testing.T) {
	calls, _ := streamOutcome(t, []string{
		"TOOL_CALL {\"name\":\"glob\",\"pattern\":\"*\"}\n```json\n```\n\nTO_CALL {\"name\":\"glob\",\"pattern\":\"**/*\"}",
	})
	if len(calls) != 1 || calls[0] != `glob {"pattern":"*"}` {
		t.Fatalf("calls = %v, want exactly the first complete call", calls)
	}
}

// 正文先行、调用在后：当前的流式判定把它当普通文本处理（不识别为调用）。
// 这个用例钉住既有行为——它是「前缀话痨」形态，和本次修复无关，
// 要不要改成识别需要单独决策（会影响云端的流式渲染），这里只防止悄悄变化。
func TestStreamProseBeforeToolCallStaysText(t *testing.T) {
	calls, text := streamOutcome(t, []string{`我先看一下当前工作区。`, `TOOL_CALL {"name":"shell","arguments":{"command":"ls"}}`})
	if len(calls) != 0 {
		t.Fatalf("calls = %v: prose-first output is treated as text today", calls)
	}
	if !strings.Contains(text, "我先看一下当前工作区") {
		t.Fatalf("text = %q", text)
	}
}
