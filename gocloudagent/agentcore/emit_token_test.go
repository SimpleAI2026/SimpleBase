// 文件说明：emit_token_test.go — gocloudagent/agentcore 的单元测试（TestEmitTokenAccumulatesThenEmits、TestEmitTokenSkipsEmptyAndToolCallMessages 等）
//
// 用法
//
//	go test ./gocloudagent/agentcore -run 'TestEmitTokenAccumulatesThenEmits|TestEmitTokenSkipsEmptyAndToolCallMessages|TestEmitTokenNilAccAndNilEmit'
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, gocloudagent/group, gocloudagent/plan 等 6 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/agentcore/emit_token_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] messages.go, model.go, model_internal_test.go, model_length_test.go, model_stream.go 等 13 个
//
// 主要符号
//
//	TestEmitTokenAccumulatesThenEmits(t *testing.T)
//	TestEmitTokenSkipsEmptyAndToolCallMessages(t *testing.T)
//	TestEmitTokenNilAccAndNilEmit(t *testing.T)

package agentcore

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestEmitTokenAccumulatesThenEmits(t *testing.T) {
	var acc strings.Builder
	var seen []string
	emit := func(e ConsumeEvent) {
		// emit 看到的 acc 必须已包含本次内容（先累积、后发事件）。
		seen = append(seen, e.Type+":"+e.Content+"|"+acc.String())
	}
	emitToken(schema.AssistantMessage("hi", nil), emit, &acc)
	if len(seen) != 1 || seen[0] != "token:hi|hi" {
		t.Fatalf("seen=%v", seen)
	}
}

func TestEmitTokenSkipsEmptyAndToolCallMessages(t *testing.T) {
	var acc strings.Builder
	calls := 0
	emit := func(ConsumeEvent) { calls++ }
	emitToken(schema.AssistantMessage("", nil), emit, &acc)
	emitToken(schema.AssistantMessage("text", []schema.ToolCall{{ID: "c"}}), emit, &acc)
	if calls != 0 || acc.Len() != 0 {
		t.Fatalf("calls=%d acc=%q", calls, acc.String())
	}
}

func TestEmitTokenNilAccAndNilEmit(t *testing.T) {
	emitToken(schema.AssistantMessage("x", nil), nil, nil) // 不得 panic
	var acc strings.Builder
	emitToken(schema.AssistantMessage("x", nil), nil, &acc)
	if acc.String() != "x" {
		t.Fatalf("acc=%q", acc.String())
	}
	n := 0
	emitToken(schema.AssistantMessage("x", nil), func(ConsumeEvent) { n++ }, nil)
	if n != 1 {
		t.Fatalf("emit calls=%d", n)
	}
}
