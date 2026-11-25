// 文件说明：runtime_events_test.go — gocloudagent/cloudagent 的单元测试（TestToolEventsClassifyBeforeTruncate、TestToolEventsResultFallbackAndOrphan 等）
//
// 用法
//
//	go test ./gocloudagent/cloudagent -run 'TestToolEventsClassifyBeforeTruncate|TestToolEventsResultFallbackAndOrphan|TestToolEventsEmitToolCallBeforeReleasingGate|TestToolEventsGateStaysClosedWhileEmitIsRunning'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/runtime_events_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//
// 主要符号
//
//	toolResultVariant(id, name, content string) *adk.MessageVariant
//	TestToolEventsClassifyBeforeTruncate(t *testing.T)
//	TestToolEventsResultFallbackAndOrphan(t *testing.T)
//	TestToolEventsEmitToolCallBeforeReleasingGate(t *testing.T)
//	TestToolEventsGateStaysClosedWhileEmitIsRunning(t *testing.T)
//	TestToolEventsNilEmitStillReleasesGate(t *testing.T)
//	TestFinishRunPriority(t *testing.T)
//	TestHistoryToSchemaKeepsInputAndTruncatesOnce(t *testing.T)

package cloudagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

func toolResultVariant(id, name, content string) *adk.MessageVariant {
	return &adk.MessageVariant{
		Role:     schema.Tool,
		ToolName: name,
		Message:  &schema.Message{Role: schema.Tool, ToolCallID: id, ToolName: name, Content: content},
	}
}

// 超长失败信封在截断展示后不再是合法 JSON，失败标志必须在截断前判定。
func TestToolEventsClassifyBeforeTruncate(t *testing.T) {
	long := `{"ok":false,"error":{"code":"x"},"padding":"` + strings.Repeat("a", maxToolCardChars+10) + `"}`
	var got []Event
	e := newToolEvents("r", func(ev Event) { got = append(got, ev) }, Tracer{}, nil)
	e.calls([]schema.ToolCall{{ID: "c1", Function: schema.FunctionCall{Name: "t", Arguments: "{}"}}})
	e.result(toolResultVariant("c1", "t", long))
	if len(e.cards) != 1 || e.cards[0]["is_error"] != true || e.cards[0]["truncated"] != true {
		t.Fatalf("card=%v", e.cards)
	}
	last := got[len(got)-1]
	if last.Type != "tool_result" || !last.IsError || !last.Truncated {
		t.Fatalf("event=%+v", last)
	}
}

// 无 call_id 的结果回退到最近一个同名未完结卡片；孤立结果新建卡片但不计入 byID。
func TestToolEventsResultFallbackAndOrphan(t *testing.T) {
	e := newToolEvents("r", nil, Tracer{}, nil)
	e.calls([]schema.ToolCall{
		{ID: "a", Function: schema.FunctionCall{Name: "t"}},
		{ID: "b", Function: schema.FunctionCall{Name: "t"}},
	})
	e.result(toolResultVariant("", "t", "second"))
	if e.cards[1]["content"] != "second" || e.cards[0]["content"] != nil {
		t.Fatalf("fallback must pick the latest unfinished same-name card: %v", e.cards)
	}
	e.result(toolResultVariant("zzz", "other", "orphan"))
	if len(e.cards) != 3 || len(e.byID) != 2 {
		t.Fatalf("cards=%d byID=%d", len(e.cards), len(e.byID))
	}
}

// BUG-02：先发 tool_call 事件，再释放闸门。闸门放行意味着工具可以开始执行并发出
// confirmation_required / tool_start，因此 tool_call 必须已经先发出。
func TestToolEventsEmitToolCallBeforeReleasingGate(t *testing.T) {
	var order []string
	e := newToolEvents("r", func(ev Event) { order = append(order, "emit:"+ev.Type) }, Tracer{}, func(id string) { order = append(order, "release:"+id) })
	e.calls([]schema.ToolCall{{ID: "c", Function: schema.FunctionCall{Name: "t"}}})
	if strings.Join(order, ",") != "emit:tool_call,release:c" {
		t.Fatalf("order=%v", order)
	}
}

// emit 正在执行（可能阻塞）时，闸门不得已被释放。
func TestToolEventsGateStaysClosedWhileEmitIsRunning(t *testing.T) {
	var gate ToolCallGate
	inEmit := make(chan struct{})
	finishEmit := make(chan struct{})
	e := newToolEvents("r", func(ev Event) {
		close(inEmit)
		<-finishEmit
	}, Tracer{}, func(id string) { gate.Release("r", id) })

	done := make(chan struct{})
	go func() {
		e.calls([]schema.ToolCall{{ID: "c", Function: schema.FunctionCall{Name: "t"}}})
		close(done)
	}()
	<-inEmit

	waited := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		gate.Wait("r", "c", 150*time.Millisecond)
		waited <- time.Since(start)
	}()
	if d := <-waited; d < 120*time.Millisecond {
		t.Fatalf("gate was released while tool_call emit was still running (waited only %v)", d)
	}
	close(finishEmit)
	<-done
}

// nil emit 时仍然释放闸门（无事件订阅者不应让工具一直等到超时）。
func TestToolEventsNilEmitStillReleasesGate(t *testing.T) {
	var released []string
	e := newToolEvents("r", nil, Tracer{}, func(id string) { released = append(released, id) })
	e.calls([]schema.ToolCall{{ID: "c1", Function: schema.FunctionCall{Name: "t"}}, {ID: "c2", Function: schema.FunctionCall{Name: "t"}}})
	if strings.Join(released, ",") != "c1,c2" {
		t.Fatalf("released=%v", released)
	}
}

func TestFinishRunPriority(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithTimeout(context.Background(), time.Nanosecond)
	defer stop()
	<-expired.Done()
	boom := errors.New("boom")
	cases := []struct {
		name    string
		out     runOutcome
		reason  string
		wantErr error
	}{
		{"max_iterations", runOutcome{runErr: adk.ErrExceedMaxIterations, runCtx: context.Background()}, "max_iterations", nil},
		{"length", runOutcome{runErr: agentcore.ErrMaxTokensExceeded, runCtx: context.Background()}, "length", nil},
		{"canceled via ctx", runOutcome{runCtx: canceled}, "canceled", context.Canceled},
		{"timeout via ctx", runOutcome{runCtx: expired}, "timeout", context.DeadlineExceeded},
		{"error", runOutcome{runErr: boom, runCtx: context.Background()}, "error", boom},
		{"stop", runOutcome{runCtx: context.Background()}, "stop", nil},
	}
	for _, tc := range cases {
		res, err := finishRun(RunRequest{}, Tracer{}, time.Now(), tc.out)
		if res.Reason != tc.reason || !errors.Is(err, tc.wantErr) && err != tc.wantErr {
			t.Fatalf("%s: reason=%q err=%v", tc.name, res.Reason, err)
		}
	}
}

// 历史投影不修改入参，且预算按截断后的字节计算。
func TestHistoryToSchemaKeepsInputAndTruncatesOnce(t *testing.T) {
	long := strings.Repeat("字", 3000)
	history := []Message{{Role: "user", Content: long}}
	out := historyToSchema(history)
	if history[0].Content != long {
		t.Fatal("input history must not be modified")
	}
	if len(out) != 1 || strings.Count(out[0].Content, "[truncated]") != 1 {
		t.Fatalf("out=%v", out)
	}
}
