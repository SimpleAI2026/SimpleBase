// 文件说明：model_length_test.go — gocloudagent/agentcore 的单元测试（TestStreamChatFallbackReportsLengthLikeGenerate、TestStreamChatFallbackNormalFinishEndsWithEOF 等）
//
// 用法
//
//	go test ./gocloudagent/agentcore -run 'TestStreamChatFallbackReportsLengthLikeGenerate|TestStreamChatFallbackNormalFinishEndsWithEOF'
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, gocloudagent/group, gocloudagent/plan 等 6 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/agentcore/model_length_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] emit_token_test.go, messages.go, model.go, model_internal_test.go, model_stream.go 等 13 个
//
// 主要符号
//
//	type streamFailsChat struct{…}
//	(c *streamFailsChat) Chat(context.Context, string, ChatRequest) (ChatResponse, error)
//	(c *streamFailsChat) Stream(context.Context, string, ChatRequest) (TokenStream, error)
//	TestStreamChatFallbackReportsLengthLikeGenerate(t *testing.T)
//	TestStreamChatFallbackNormalFinishEndsWithEOF(t *testing.T)

package agentcore

import (
	"context"
	"errors"
	"testing"
)

// streamFailsChat 的 Stream 返回普通错误（非 LiteLLMError），迫使 Model 退化为一次 Chat。
type streamFailsChat struct{ resp ChatResponse }

func (c *streamFailsChat) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	return c.resp, nil
}

func (c *streamFailsChat) Stream(context.Context, string, ChatRequest) (TokenStream, error) {
	return nil, errors.New("stream not available")
}

// BUG-07：Stream 退化为 Chat 时，finish_reason=length 必须与 Generate 一样报告
// ErrMaxTokensExceeded，而不是当成一次正常的完整回复。
func TestStreamChatFallbackReportsLengthLikeGenerate(t *testing.T) {
	client := &streamFailsChat{resp: ChatResponse{Content: "partial", FinishReason: "length"}}
	m := NewModel(client, "", "m")

	if _, err := m.Generate(context.Background(), nil); !errors.Is(err, ErrMaxTokensExceeded) {
		t.Fatalf("Generate err=%v", err)
	}
	sr, err := m.Stream(context.Background(), nil)
	if err != nil {
		t.Fatalf("Stream open: %v", err)
	}
	defer sr.Close()
	msg, err := sr.Recv()
	if err != nil || msg == nil || msg.Content != "partial" {
		t.Fatalf("first frame must still carry the partial text: msg=%v err=%v", msg, err)
	}
	if _, err := sr.Recv(); !errors.Is(err, ErrMaxTokensExceeded) {
		t.Fatalf("stream must end with ErrMaxTokensExceeded, got %v", err)
	}
}

// 正常结束的回退路径不受影响。
func TestStreamChatFallbackNormalFinishEndsWithEOF(t *testing.T) {
	m := NewModel(&streamFailsChat{resp: ChatResponse{Content: "done", FinishReason: "stop"}}, "", "m")
	sr, err := m.Stream(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Close()
	if msg, err := sr.Recv(); err != nil || msg.Content != "done" {
		t.Fatalf("msg=%v err=%v", msg, err)
	}
	if _, err := sr.Recv(); err == nil || errors.Is(err, ErrMaxTokensExceeded) {
		t.Fatalf("want io.EOF, got %v", err)
	}
}
