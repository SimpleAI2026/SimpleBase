// 文件说明：runtime_events.go — 一次 run 内的工具调用/结果卡片收集
//
// toolEvents 持有 cards（按出现顺序）与 byID（call_id 索引）。二者指向同一批 map，
// 结果到达时就地补全卡片，因此序列化 cards 即得到完整的工具调用记录。

package cloudagent

import (
	"cmp"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// maxToolCardChars 是工具结果卡片正文的展示上限；失败标志在截断前判定。
const maxToolCardChars = 4000

type toolEvents struct {
	runID      string
	emit       func(Event)
	tr         Tracer
	onToolCall func(string)

	cards  []map[string]any
	byID   map[string]map[string]any
	starts map[string]time.Time
}

func newToolEvents(runID string, emit func(Event), tr Tracer, onToolCall func(string)) *toolEvents {
	return &toolEvents{
		runID: runID, emit: emit, tr: tr, onToolCall: onToolCall,
		byID: map[string]map[string]any{}, starts: map[string]time.Time{},
	}
}

// result 处理一条工具结果：优先按 call_id 关联卡片，缺失时回退到最近一个同名未完结卡片。
func (e *toolEvents) result(mv *adk.MessageVariant) {
	content, name := agentcore.ConsumeVariant(mv)
	name = cmp.Or(mv.ToolName, name)
	id := ""
	if mv.Message != nil {
		id = mv.Message.ToolCallID
	}
	card := e.byID[id]
	if card == nil {
		for i := len(e.cards) - 1; i >= 0; i-- {
			if e.cards[i]["name"] == name && e.cards[i]["content"] == nil {
				card = e.cards[i]
				id, _ = card["call_id"].(string)
				break
			}
		}
	}
	if card == nil {
		card = map[string]any{"call_id": id, "name": name}
		e.cards = append(e.cards, card)
	}
	// 先对完整结果判定失败，再截断展示：超长失败信封截断后不再是合法 JSON。
	isErr := agentcore.ToolResultIsError(content)
	truncated := len([]rune(content)) > maxToolCardChars
	content = agentcore.Truncate(content, maxToolCardChars)
	duration := time.Since(e.starts[id]).Milliseconds()
	if e.starts[id].IsZero() {
		duration = 0
	}
	card["content"] = content
	card["duration_ms"] = duration
	card["truncated"] = truncated
	card["is_error"] = isErr
	if e.emit != nil {
		e.emit(Event{Type: "tool_result", CallID: id, Name: name, Content: content, DurationMS: duration, Truncated: truncated, IsError: isErr})
	}
	e.tr.Meta("gocloudagent: tool result",
		zap.String("run_id", e.runID), zap.String("tool", name), zap.String("call_id", id),
		zap.Int64("duration_ms", duration), zap.Bool("is_error", isErr), zap.Bool("truncated", truncated),
	)
	e.tr.Payload("gocloudagent: tool result body", "content", content)
}

// calls 登记助手本轮发起的工具调用：先登记卡片与计时，再发 tool_call 事件，最后释放宿主闸门。
func (e *toolEvents) calls(toolCalls []schema.ToolCall) {
	for _, tc := range toolCalls {
		id := tc.ID
		if id == "" {
			id = "call_" + uuid.NewString()
		}
		card := map[string]any{"call_id": id, "name": tc.Function.Name, "arguments": tc.Function.Arguments}
		e.byID[id] = card
		e.starts[id] = time.Now()
		e.cards = append(e.cards, card)
		if e.emit != nil {
			e.emit(Event{Type: "tool_call", CallID: id, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
		}
		// 事件发出之后才放行：闸门一旦释放，工具即可开始执行并发出 confirmation_required /
		// tool_start，订阅方必须先看到对应的 tool_call。
		if e.onToolCall != nil {
			e.onToolCall(id)
		}
		e.tr.Meta("gocloudagent: tool call", zap.String("run_id", e.runID), zap.String("tool", tc.Function.Name), zap.String("call_id", id))
		e.tr.Payload("gocloudagent: tool arguments", "arguments", tc.Function.Arguments)
	}
}
