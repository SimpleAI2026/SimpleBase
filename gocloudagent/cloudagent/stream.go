// 文件说明：stream.go — ADK 流事件消费

package cloudagent

import (
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// 事件消费与增量合并委托 agentcore（与本地 CLI 共用）：见 agentcore.ConsumeVariant /
// ConsumeAssistant / IsToolDelta / MergeStreamToolCall。gocloudagent 只负责把 ConsumeEvent
// 映射成带 run_id 的 SSE Event。
func consumeAssistant(mv *adk.MessageVariant, emit func(Event), acc *strings.Builder) (*schema.Message, error) {
	return agentcore.ConsumeAssistant(mv, func(ce agentcore.ConsumeEvent) {
		if emit == nil {
			return
		}
		emit(Event{Type: ce.Type, Content: ce.Content, CallID: ce.CallID, Name: ce.Name, Arguments: ce.Arguments})
	}, acc)
}

func safeAgentName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "agent"
	}
	return name
}
