// 文件说明：compaction.go — gocloudagent/cli 中 Compact、ForceCompact 等符号的实现
//
// 用法
//
//	(r *SessionRunner) Compact(ctx context.Context) (bool, error)  // Compact 按触发阈值判断是否需要压缩，需要则写入一条 compaction 条目
//	(r *SessionRunner) ForceCompact(ctx context.Context) (bool, error)  // ForceCompact 忽略触发阈值直接压缩（`/compact`）
//	同包调用：compact() → command.go, repl.go；ForceCompact() → repl.go；Compact() → runner.go
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/compaction.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/agentcore
//	[同包] client.go, command.go, config.go, prompt.go, repeat_detector.go 等 9 个
//	[关键引用] compact() ← command.go, repl.go
//
// 主要符号
//
//	(r *SessionRunner) Compact(ctx context.Context) (bool, error)
//	(r *SessionRunner) ForceCompact(ctx context.Context) (bool, error)
//	... 另有 16 个顶层符号

package cli

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/cloudwego/eino/schema"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// compaction.go 实现上下文压缩的「生产侧」（planv6.0 §13.5 的 P7）。
//
// compaction 条目此前只有消费方：Messages() 在它处截断上下文、LatestCompaction()
// 取摘要、historyMessages() 把摘要拼回 system、sessions show 渲染它——但没有一行
// 生产代码写入过（只有 session_test.go 手工造过一条）。这里补齐写入侧：
//
//	估算上下文规模 → 按完整回合找切点 → 调模型写摘要 → 追加 compaction 条目
//
// 原文一律留在 jsonl 里，只是不再进上下文（§13.5「原文不删」）。
//
// 与计划里「用 eino summarization 中间件」的差别：那条路是为云端写的——§13.5 的
// 配置表写明「云端没有会话树，落库需要迁移」。CLI 侧会话树已经存在，直接追加一条
// compaction 条目即可；也因此不需要打开 EmitInternalEvents（§13.5 硬约束 4 明确不启用）。

const (
	// defaultCompactTriggerTokens 是触发压缩的估算 token 阈值。
	// §12 Q9：llmgateway 不暴露模型上下文窗口（全仓无 ContextWindow 字段），
	// 只能用绝对值。取保守值——早压缩只多一次摘要调用，低估则会把超限的
	// 上下文直接发给模型。
	defaultCompactTriggerTokens = 32000
	// defaultCompactTriggerMessages 是阈值之外的条数兜底（§12 Q9 要求）。
	defaultCompactTriggerMessages = 120
	// defaultCompactKeepTokens 是压缩后保留的近期 token 预算。
	// §13.5：不照抄 Pi 的 20000——它的编码工具定义比这里的本机工具长。
	defaultCompactKeepTokens = 4096
	// maxCompactSummaryChars 是摘要的字符上限。
	maxCompactSummaryChars = 8 << 10
	// minCompactHeadMessages 是值得压缩的最少消息数。
	// 少于两条时「摘要」不会比原文更短，压了反而丢信息。
	minCompactHeadMessages = 2
)

// compactSummaryPrompt 是生成摘要的指令。摘要只服务于后续上下文，不面向用户。
const compactSummaryPrompt = `Summarize the conversation so far for your own future use. Keep:
- the user's goals, constraints and preferences
- decisions made and their reasons
- files, commands and paths touched
- errors encountered and how they were resolved
- open questions and unfinished work

Drop pleasantries, repeated tool output and superseded attempts. Reply with the summary only, in the language the conversation uses.`

// Compact 按触发阈值判断是否需要压缩，需要则写入一条 compaction 条目。
// 返回是否真的写入了。
func (r *SessionRunner) Compact(ctx context.Context) (bool, error) {
	return r.compact(ctx, false)
}

// ForceCompact 忽略触发阈值直接压缩（`/compact`）。
func (r *SessionRunner) ForceCompact(ctx context.Context) (bool, error) {
	return r.compact(ctx, true)
}

func (r *SessionRunner) compact(ctx context.Context, force bool) (bool, error) {
	if r == nil || r.Session == nil || r.Client == nil {
		return false, nil
	}
	msgs := r.Session.Messages()
	if !force && !r.shouldCompact(msgs) {
		return false, nil
	}
	cut := cutByTurn(msgs, r.keepTokens())
	if cut < minCompactHeadMessages {
		// 没有值得挪出上下文的完整回合（或整段就是一轮），不压。
		return false, nil
	}
	summary, err := r.summarize(ctx, msgs[:cut])
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(summary) == "" {
		return false, errors.New("compaction: model returned an empty summary")
	}
	if err := r.Session.Append(Entry{
		Type: EntryCompaction,
		// Parent 留空交给 Append 取「上一条」：压缩条目是全链末端，父指针指向
		// 最后一条消息，投影回溯时才能看到切点之后的消息（见 session.go 的 Messages）。
		Summary: summary,
		// FirstKept 记「最后一条被压掉的」——投影只保留它之后的消息。
		// 与 Pi 的 firstKeptEntryId（记第一条保留的）语义相反，这里对齐本仓库
		// 既有口径（session_test.go 的 TestSessionMessagesHonorCompactionCut）。
		FirstKept: msgs[cut-1].ID,
	}); err != nil {
		return false, err
	}
	return true, nil
}

// shouldCompact 判定上下文是否已超过阈值（token 估算或条数兜底）。
func (r *SessionRunner) shouldCompact(msgs []Entry) bool {
	if len(msgs) >= defaultCompactTriggerMessages {
		return true
	}
	total := 0
	for _, m := range msgs {
		total += entryTokens(m)
	}
	return total >= r.triggerTokens()
}

func (r *SessionRunner) triggerTokens() int {
	if r.CompactTriggerTokens > 0 {
		return r.CompactTriggerTokens
	}
	return defaultCompactTriggerTokens
}

func (r *SessionRunner) keepTokens() int {
	if r.CompactKeepTokens > 0 {
		return r.CompactKeepTokens
	}
	return defaultCompactKeepTokens
}

// summarize 让模型把 head 压成一段摘要。失败由调用方决定是否降级。
//
// §13.5 硬约束：摘要正文不进日志、不进审计——这里只把它交给调用方落盘，
// 不打任何日志；落盘前统一走 RedactSecrets。
func (r *SessionRunner) summarize(ctx context.Context, head []Entry) (string, error) {
	req := make([]*schema.Message, 0, len(head)+1)
	for _, m := range head {
		req = append(req, entryToSchema(m))
	}
	req = append(req, agentcore.UserSchemaMessage(compactSummaryPrompt))

	model := agentcore.NewModel(r.Client, "", strings.TrimSpace(r.Model))
	model.Warn = r.Warn
	out, err := model.Generate(ctx, req)
	if err != nil {
		return "", err
	}
	if out == nil || len(out.ToolCalls) > 0 {
		// 模型拿摘要指令去调工具，等于没写摘要。
		return "", nil
	}
	return agentcore.Truncate(agentcore.RedactSecrets(out.Content), maxCompactSummaryChars), nil
}

// cutByTurn 返回切点 cut：msgs[:cut] 压成摘要，msgs[cut:] 原样保留，
// 返回 0 表示没有可压缩的完整回合（不该压）。
//
// 按「回合」为单位从尾部累加（回合 = 一条 user 消息起到下一条 user 消息前），
// 直到加上下一个回合会超出 keepTokens 为止，且**至少保留最后一个回合**。
//
// 为什么不能按单条消息累加再对齐：keepTokens 往往小于一个回合的成本，
// 累加会停在回合中间，再向前对齐就一路顶到末尾，结果是永远压不动。
// 以回合为单位从尾累加不会出现这种情况，也天然保证：
//   - 保留段以 user 消息开头，不会出现孤儿工具结果；
//   - 切点两侧都是完整回合（§13.5「切点必须落在完整 turn」）。
func cutByTurn(msgs []Entry, keepTokens int) int {
	if len(msgs) == 0 || keepTokens <= 0 {
		return 0
	}
	starts := make([]int, 0, len(msgs))
	for i, m := range msgs {
		if m.Role == "user" {
			starts = append(starts, i)
		}
	}
	if len(starts) == 0 {
		return 0
	}
	if starts[0] != 0 {
		// 历史不是从 user 开头（异常但可能）：把首条也算作一个回合边界。
		starts = append([]int{0}, starts...)
	}
	if len(starts) < 2 {
		// 只有一轮，没有可挪出上下文的完整回合。
		return 0
	}
	budget := keepTokens
	keepFrom := starts[len(starts)-1]
	for k := len(starts) - 1; k >= 0; k-- {
		start := starts[k]
		end := len(msgs)
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		cost := 0
		for _, m := range msgs[start:end] {
			cost += entryTokens(m)
		}
		// 最后一个回合无条件保留：再贵也得留点上下文，否则整段历史会被
		// 压成一句摘要（§13.5 记录的「失忆」缺陷）。
		if k < len(starts)-1 && cost > budget {
			break
		}
		budget -= cost
		keepFrom = start
	}
	if keepFrom <= 0 {
		return 0
	}
	return keepFrom
}

// entryToSchema 把一条落盘 entry 还原成 eino 消息。
// assistant 的 tool_calls 必须一并还原：只带 tool 结果而不带对应的调用，
// 在原生协议下是孤儿结果。
func entryToSchema(m Entry) *schema.Message {
	switch m.Role {
	case "assistant":
		if len(m.ToolCalls) > 0 {
			calls := make([]schema.ToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				calls = append(calls, schema.ToolCall{
					ID: tc.ID, Type: "function",
					Function: schema.FunctionCall{Name: tc.Name, Arguments: tc.Arguments},
				})
			}
			return schema.AssistantMessage(m.Content, calls)
		}
		return agentcore.AssistantSchemaMessage(m.Content)
	case "tool":
		return agentcore.ToolResultSchemaMessage(m.Name, m.ToolCallID, m.Content)
	default:
		return agentcore.UserSchemaMessage(m.Content)
	}
}

func entryTokens(e Entry) int {
	n := estimateTokens(e.Content)
	for _, tc := range e.ToolCalls {
		n += estimateTokens(tc.Name) + estimateTokens(tc.Arguments)
	}
	return n
}

// estimateTokens 粗估一段文本的 token 数。
//
// 没有 tokenizer（llmgateway 不暴露，§12 Q9），按「CJK 1 字符 ≈ 1 token、
// 其余 4 字符 ≈ 1 token」估算。宁可高估：高估只是早一点压缩。
func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	cjk, other := 0, 0
	for _, r := range s {
		if isCJK(r) {
			cjk++
			continue
		}
		other++
	}
	return cjk + (other+3)/4
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)
}
