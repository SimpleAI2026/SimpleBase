// 文件说明：stream_pump_test.go — gocloudagent/agentcore 的单元测试（TestStreamPumpNativeToolCallDeltasConcatenate、TestStreamPumpInterleavedIndexesStaySeparate 等）
//
// 用法
//
//	go test ./gocloudagent/agentcore -run 'TestStreamPumpNativeToolCallDeltasConcatenate|TestStreamPumpInterleavedIndexesStaySeparate|TestStreamPumpEmptyToolDeltaProducesNoFrame|TestStreamPumpReportsUsagePerDelta'
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, gocloudagent/group, gocloudagent/plan 等 6 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/agentcore/stream_pump_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] emit_token_test.go, messages.go, model.go, model_internal_test.go, model_length_test.go 等 13 个
//
// 主要符号
//
//	type deltaStream struct{…}
//	(s *deltaStream) Next() (string, bool, error)
//	(s *deltaStream) NextDelta() (StreamDelta, error)
//	(s *deltaStream) Close() error
//	type deltaClient struct{…}
//	(c *deltaClient) Chat(context.Context, string, ChatRequest) (ChatResponse, error)
//	(c *deltaClient) Stream(context.Context, string, ChatRequest) (TokenStream, error)
//	drain(sr *schema.StreamReader[*schema.Message]) (msgs []*schema.Message, err error)
//	... 另有 20 个顶层符号

package agentcore

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// deltaStream 按给定增量依次吐出，并记录 Close 的调用情况。
type deltaStream struct {
	deltas []StreamDelta
	endErr error // 增量耗尽后返回的错误，默认 io.EOF
	i      int

	mu       sync.Mutex
	closed   int
	closeLog *[]string
}

func (s *deltaStream) Next() (string, bool, error) {
	d, err := s.NextDelta()
	return d.Content, d.Finish, err
}

func (s *deltaStream) NextDelta() (StreamDelta, error) {
	if s.i >= len(s.deltas) {
		if s.endErr != nil {
			return StreamDelta{}, s.endErr
		}
		return StreamDelta{}, io.EOF
	}
	d := s.deltas[s.i]
	s.i++
	return d, nil
}

func (s *deltaStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed++
	if s.closeLog != nil {
		*s.closeLog = append(*s.closeLog, "upstream")
	}
	return nil
}

type deltaClient struct{ stream *deltaStream }

func (c *deltaClient) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, errors.New("unexpected chat")
}

func (c *deltaClient) Stream(context.Context, string, ChatRequest) (TokenStream, error) {
	return c.stream, nil
}

// drain 读完整条流，返回所有帧与终止错误（io.EOF 视为 nil）。
func drain(sr *schema.StreamReader[*schema.Message]) (msgs []*schema.Message, err error) {
	for {
		m, e := sr.Recv()
		if e != nil {
			if errors.Is(e, io.EOF) {
				return msgs, nil
			}
			return msgs, e
		}
		msgs = append(msgs, m)
	}
}

func runDeltaStream(t *testing.T, stream *deltaStream, usage func(ChatUsage)) ([]*schema.Message, error) {
	t.Helper()
	m := NewModel(&deltaClient{stream: stream}, "", "m")
	m.Protocol = "native"
	m.Usage = usage
	sr, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Close()
	return drain(sr)
}

// 原生分片：同 Index 的片段按顺序拼接，帧带 delta 标记与 Index，ConcatMessages 能还原成一次完整调用。
func TestStreamPumpNativeToolCallDeltasConcatenate(t *testing.T) {
	msgs, err := runDeltaStream(t, &deltaStream{deltas: []StreamDelta{
		{ToolCall: &ToolCallDelta{Index: 0, ID: "c1", Name: "shell"}},
		{ToolCall: &ToolCallDelta{Index: 0, ArgsDelta: `{"command"`}},
		{ToolCall: &ToolCallDelta{Index: 0, ArgsDelta: `:"ls"}`}},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Fatalf("frames=%d, want one forwarded frame per delta", len(msgs))
	}
	for _, m := range msgs {
		if len(m.ToolCalls) != 1 || m.ToolCalls[0].Index == nil || m.ToolCalls[0].Extra["delta"] != true {
			t.Fatalf("frame must carry Index and delta marker: %+v", m)
		}
	}
	full, err := schema.ConcatMessages(msgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.ToolCalls) != 1 || full.ToolCalls[0].Function.Name != "shell" || full.ToolCalls[0].Function.Arguments != `{"command":"ls"}` {
		t.Fatalf("assembled=%+v", full.ToolCalls)
	}
}

// 交错的多个 Index 各自归并，互不串扰。
func TestStreamPumpInterleavedIndexesStaySeparate(t *testing.T) {
	msgs, err := runDeltaStream(t, &deltaStream{deltas: []StreamDelta{
		{ToolCall: &ToolCallDelta{Index: 0, ID: "a", Name: "one", ArgsDelta: `{"x"`}},
		{ToolCall: &ToolCallDelta{Index: 1, ID: "b", Name: "two", ArgsDelta: `{"y"`}},
		{ToolCall: &ToolCallDelta{Index: 0, ArgsDelta: `:1}`}},
		{ToolCall: &ToolCallDelta{Index: 1, ArgsDelta: `:2}`}},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	full, err := schema.ConcatMessages(msgs)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, tc := range full.ToolCalls {
		got[tc.Function.Name] = tc.Function.Arguments
	}
	if got["one"] != `{"x":1}` || got["two"] != `{"y":2}` || len(full.ToolCalls) != 2 {
		t.Fatalf("assembled=%+v", full.ToolCalls)
	}
}

// 没有任何内容的空分片（无 ID/名/参数）不产生帧。
func TestStreamPumpEmptyToolDeltaProducesNoFrame(t *testing.T) {
	msgs, err := runDeltaStream(t, &deltaStream{deltas: []StreamDelta{{ToolCall: &ToolCallDelta{Index: 0}}}}, nil)
	if err != nil || len(msgs) != 0 {
		t.Fatalf("msgs=%d err=%v", len(msgs), err)
	}
}

// 用量：每个带 Usage 的增量回调一次，与 Finish 同帧的 Usage 也不丢。
func TestStreamPumpReportsUsagePerDelta(t *testing.T) {
	var got []ChatUsage
	_, err := runDeltaStream(t, &deltaStream{deltas: []StreamDelta{
		{Content: "a", Usage: &ChatUsage{PromptTokens: 1}},
		{Content: "b", Usage: &ChatUsage{CompletionTokens: 2}, Finish: true},
		{Content: "after-finish-must-not-be-read"},
	}}, func(u ChatUsage) { got = append(got, u) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].PromptTokens != 1 || got[1].CompletionTokens != 2 {
		t.Fatalf("usage=%+v", got)
	}
}

// Finish 之后不再读取上游。
func TestStreamPumpStopsReadingAtFinish(t *testing.T) {
	stream := &deltaStream{deltas: []StreamDelta{{Content: "x", Finish: true}, {Content: "never"}}}
	msgs, err := runDeltaStream(t, stream, nil)
	if err != nil {
		t.Fatal(err)
	}
	if text := joinContent(msgs); text != "x" {
		t.Fatalf("text=%q", text)
	}
	if stream.i != 1 {
		t.Fatalf("read %d deltas, want 1", stream.i)
	}
}

// 读错误：已转发的帧保留，错误作为终止错误送出，且不补发半截的文本协议内容。
func TestStreamPumpReadErrorKeepsForwardedFramesAndSurfacesError(t *testing.T) {
	boom := errors.New("connection reset")
	stream := &deltaStream{deltas: []StreamDelta{{Content: "你好，这是正文"}}, endErr: boom}
	m := NewModel(&deltaClient{stream: stream}, "", "m") // 文本协议
	sr, err := m.Stream(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Close()
	msgs, rerr := drain(sr)
	if !errors.Is(rerr, boom) {
		t.Fatalf("err=%v, want the upstream read error", rerr)
	}
	if joinContent(msgs) != "你好，这是正文" {
		t.Fatalf("forwarded prose must be kept: %q", joinContent(msgs))
	}
}

// 读错误发生在「半个 TOOL_CALL 前缀」时：不得把残缺内容补发成工具调用。
func TestStreamPumpReadErrorWhileBufferingDoesNotFlushPartialToolCall(t *testing.T) {
	boom := errors.New("eof early")
	stream := &deltaStream{deltas: []StreamDelta{{Content: `TOOL_CALL {"name":"sh`}}, endErr: boom}
	m := NewModel(&deltaClient{stream: stream}, "", "m")
	sr, err := m.Stream(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Close()
	msgs, rerr := drain(sr)
	if !errors.Is(rerr, boom) {
		t.Fatalf("err=%v", rerr)
	}
	for _, m := range msgs {
		if len(m.ToolCalls) != 0 {
			t.Fatalf("partial call must not be emitted: %+v", m.ToolCalls)
		}
	}
}

// 半个 marker 到流结束：没有后续内容时按原文本作为正文送出（不吞字）。
func TestStreamPumpHalfMarkerAtEOFIsFlushedAsText(t *testing.T) {
	msgs, err := runDeltaStream2(t, []string{"TOOL_"})
	if err != nil {
		t.Fatal(err)
	}
	if joinContent(msgs) != "TOOL_" {
		t.Fatalf("text=%q", joinContent(msgs))
	}
}

// 以空白开头再接 marker：仍识别为工具调用；分片可以切在任意位置。
func TestStreamPumpMarkerSplitAtEveryPosition(t *testing.T) {
	full := "  \n" + `TOOL_CALL {"name":"shell","arguments":{"command":"ls"}}`
	for cut := 1; cut < len(full); cut++ {
		msgs, err := runDeltaStream2(t, []string{full[:cut], full[cut:]})
		if err != nil {
			t.Fatal(err)
		}
		var calls []string
		for _, m := range msgs {
			for _, tc := range m.ToolCalls {
				calls = append(calls, tc.Function.Name+" "+tc.Function.Arguments)
			}
		}
		if len(calls) != 1 || calls[0] != `shell {"command":"ls"}` || strings.TrimSpace(joinContent(msgs)) != "" {
			t.Fatalf("cut=%d calls=%v text=%q", cut, calls, joinContent(msgs))
		}
	}
}

// 普通文本在判定后逐帧直通：第二帧到达前，第一帧已经可读。
func TestStreamPumpForwardsProseWithoutWaitingForUpstreamEnd(t *testing.T) {
	gate := make(chan struct{})
	stream := &gatedStream{first: "hello ", gate: gate, second: "world"}
	m := NewModel(&deltaClient2{stream: stream}, "", "m")
	sr, err := m.Stream(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Close()
	first, err := sr.Recv()
	if err != nil || first.Content != "hello " {
		t.Fatalf("first frame must arrive before upstream completes: %v %v", first, err)
	}
	close(gate)
	second, err := sr.Recv()
	if err != nil || second.Content != "world" {
		t.Fatalf("second=%v err=%v", second, err)
	}
}

// 上游 Close 恰好调用一次，且先于写端关闭（读端能看到 EOF 时上游已经关了）。
func TestStreamPumpClosesUpstreamExactlyOnceBeforeWriterEnds(t *testing.T) {
	var log []string
	stream := &deltaStream{deltas: []StreamDelta{{Content: "a"}}, closeLog: &log}
	m := NewModel(&deltaClient{stream: stream}, "", "m")
	sr, err := m.Stream(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := drain(sr); err != nil {
		t.Fatal(err)
	}
	sr.Close()
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.closed != 1 {
		t.Fatalf("upstream closed %d times, want exactly 1", stream.closed)
	}
}

// ---- helpers ----

func joinContent(msgs []*schema.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(m.Content)
	}
	return b.String()
}

func runDeltaStream2(t *testing.T, chunks []string) ([]*schema.Message, error) {
	t.Helper()
	m := NewModel(&chunkClient{chunks: chunks}, "", "m")
	sr, err := m.Stream(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Close()
	return drain(sr)
}

type gatedStream struct {
	first, second string
	gate          chan struct{}
	n             int
}

func (s *gatedStream) Next() (string, bool, error) {
	switch s.n {
	case 0:
		s.n++
		return s.first, false, nil
	case 1:
		<-s.gate
		s.n++
		return s.second, false, nil
	}
	return "", true, io.EOF
}
func (s *gatedStream) Close() error { return nil }

type deltaClient2 struct{ stream TokenStream }

func (c *deltaClient2) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, errors.New("unexpected chat")
}
func (c *deltaClient2) Stream(context.Context, string, ChatRequest) (TokenStream, error) {
	return c.stream, nil
}
