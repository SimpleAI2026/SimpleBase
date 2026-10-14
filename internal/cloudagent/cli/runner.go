package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/internal/cloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent/tools"
)

// CLIEvent 是 CLI 的一次流式事件。Type 与云端 SSE 事件名保持一致（§13.8），
// 因此脚本可以复用同一套解析。
type CLIEvent struct {
	Type string `json:"type"`
	// 以下字段按 Type 取用。
	Content   string `json:"content,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Message   string `json:"message,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// SessionRunner 跑一轮对话：装配 agent、消费事件、写 jsonl。
type SessionRunner struct {
	Session *Session
	Client  agentcore.ChatClient
	Tools   []tool.BaseTool
	// Model 是模型名；空表示用服务端默认。
	Model string
	// MaxIterations 是工具调用轮数上限；0 用默认。
	MaxIterations int
	// SystemPrompt 是本轮的系统提示词（CLI 的通用提示，不含项目快照）。
	SystemPrompt string
	// CompactTriggerTokens / CompactKeepTokens 覆盖压缩阈值；0 用默认值（见 compaction.go）。
	CompactTriggerTokens int
	CompactKeepTokens    int
	// Warn 记录降级路径。
	Warn func(msg string, fields ...zap.Field)
	// Redact 在落盘前脱敏；nil 时用 agentcore.RedactSecrets。
	Redact func(string) string
}

// RunResult 是一轮的结果。
type RunResult struct {
	Content          string
	PromptTokens     int
	CompletionTokens int
	ToolCalls        int
	DurationMS       int64
	Reason           string
}

// maxUserChars 是单条用户消息的字符上限。
const maxUserChars = 32 << 10

func (r *SessionRunner) redact(s string) string {
	if r.Redact != nil {
		return r.Redact(s)
	}
	return agentcore.RedactSecrets(s)
}

// Run 执行一轮。emit 可为 nil（headless）。
//
// 落盘的所有文本都经过 RedactSecrets（§5.2 决策）——CLI 是本地工具，
// 用户需要 resume 看到完整历史，所以是「脱敏但不截断」（云端是「不落正文又截断」）。
func (r *SessionRunner) Run(ctx context.Context, userText string, emit func(CLIEvent)) (RunResult, error) {
	if r == nil || r.Client == nil {
		return RunResult{}, errors.New("model client is not configured")
	}
	if r.Session == nil {
		return RunResult{}, errors.New("session is not initialized")
	}
	started := time.Now()
	model := agentcore.NewModel(r.Client, "", strings.TrimSpace(r.Model))
	model.Warn = r.Warn
	var usage agentcore.ChatUsage
	model.Usage = func(u agentcore.ChatUsage) {
		usage.PromptTokens += u.PromptTokens
		usage.CompletionTokens += u.CompletionTokens
		usage.ReasoningTokens += u.ReasoningTokens
	}
	iterations := r.MaxIterations
	if iterations <= 0 {
		iterations = 12
	}
	instruction := r.SystemPrompt
	if strings.TrimSpace(instruction) == "" {
		instruction = defaultInstruction
	}
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "cloudagent-cli",
		Description: "Local coding assistant",
		Instruction: instruction,
		Model:       model,
		ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: r.Tools, UnknownToolsHandler: unknownToolHandler,
			ExecuteSequentially: true,
		}},
		MaxIterations: iterations,
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("create agent: %w", err)
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agent, EnableStreaming: true})
	// 压缩在本轮历史投影之前：摘要一旦写入，historyMessages 就会只带上切点之后的消息。
	// §13.5 硬约束 3：摘要失败必须保留原路径——不丢历史、不改 Reason、不新增可见错误码，
	// 所以这里只记 warn 并照常继续。
	if _, cerr := r.Compact(ctx); cerr != nil {
		r.warn("cloudagent: context compaction skipped", zap.String("err", cerr.Error()))
	}
	msgs := r.historyMessages()
	if _, err := r.Session.AppendMessage("user", r.redact(userText)); err != nil {
		return RunResult{}, err
	}
	msgs = append(msgs, agentcore.UserSchemaMessage(agentcore.Truncate(userText, maxUserChars)))

	var assistant strings.Builder
	var runErr error
	toolCalls := 0
	iter := runner.Run(ctx, msgs)
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			runErr = event.Err
			break
		}
		if event.Output == nil {
			continue
		}
		if event.Output.MessageOutput == nil {
			continue
		}
		mv := event.Output.MessageOutput
		if mv.Role == schema.Tool {
			content, name := agentcore.ConsumeVariant(mv)
			name = firstNonEmpty(mv.ToolName, name)
			callID := ""
			if mv.Message != nil {
				callID = mv.Message.ToolCallID
			}
			content = agentcore.Truncate(content, tools.ShellOutputLimit)
			isErr := resultIsError(content)
			if _, err := r.Session.AppendToolResult(callID, name, r.redact(content), isErr); err != nil {
				runErr = err
				break
			}
			if emit != nil {
				emit(CLIEvent{Type: "tool_result", CallID: callID, Name: name, Content: r.redact(content), IsError: isErr})
			}
			continue
		}
		if mv.Role == schema.Assistant || mv.Role == "" {
			msg, err := agentcore.ConsumeAssistant(mv, func(ce agentcore.ConsumeEvent) {
				if emit != nil {
					emit(CLIEvent{Type: ce.Type, Content: ce.Content, CallID: ce.CallID, Name: ce.Name, Arguments: ce.Arguments})
				}
			}, &assistant)
			if err != nil {
				runErr = err
				break
			}
			if msg != nil && len(msg.ToolCalls) > 0 {
				// 立即落盘这一轮助手（含工具调用），必须在工具结果之前，
				// 否则 jsonl 里会变成 user → tool → assistant 的错误顺序。
				calls := make([]EntryToolCa, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					toolCalls++
					calls = append(calls, EntryToolCa{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
					if emit != nil {
						emit(CLIEvent{Type: "tool_call", CallID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
					}
				}
				if _, err := r.Session.AppendToolCall(r.redact(assistant.String()), calls); err != nil {
					runErr = err
					break
				}
				assistant.Reset()
			}
		}
	}
	// 落盘本轮的最终助手回复（工具轮已在检测到调用时落盘）。
	// 无正文时不写空行——否则 resume 会看到一串空的 assistant 消息。
	if final := strings.TrimSpace(assistant.String()); final != "" {
		if _, err := r.Session.AppendMessage("assistant", r.redact(assistant.String())); err != nil && runErr == nil {
			runErr = err
		}
	}

	result := RunResult{
		Content: r.redact(assistant.String()), PromptTokens: usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens, ToolCalls: toolCalls,
		DurationMS: time.Since(started).Milliseconds(), Reason: "stop",
	}
	if errors.Is(runErr, adk.ErrExceedMaxIterations) {
		result.Content += fmt.Sprintf("\n已达到工具调用上限（%d 轮）", iterations)
		result.Reason = "max_iterations"
		return result, nil
	}
	if errors.Is(runErr, context.Canceled) {
		result.Reason = "canceled"
		return result, context.Canceled
	}
	if runErr != nil {
		result.Reason = "error"
		return result, runErr
	}
	return result, nil
}

// historyMessages 把会话历史投影成 eino 消息（含最近的压缩摘要）。
func (r *SessionRunner) historyMessages() []*schema.Message {
	msgs := r.Session.Messages()
	out := make([]*schema.Message, 0, len(msgs)+2)
	if compaction, ok := r.Session.LatestCompaction(); ok {
		out = append(out, agentcore.SystemSchemaMessage("[compacted]\n"+compaction.Summary))
	}
	for _, m := range msgs {
		switch m.Role {
		case "assistant":
			out = append(out, agentcore.AssistantSchemaMessage(m.Content))
		case "tool":
			out = append(out, agentcore.ToolResultSchemaMessage(m.Name, m.ToolCallID, m.Content))
		default:
			out = append(out, agentcore.UserSchemaMessage(m.Content))
		}
	}
	return out
}

// warn 是降级路径的日志出口；未注入 Warn 时退化为写 stderr。
func (r *SessionRunner) warn(msg string, fields ...zap.Field) {
	if r != nil && r.Warn != nil {
		r.Warn(msg, fields...)
	}
}

// unknownToolHandler 把未注册工具名转成错误结果，而不是让整轮失败（§13.1）。
func unknownToolHandler(_ context.Context, name, _ string) (string, error) {
	return tools.ErrEnvelope("unknown_tool", "未知工具: "+name), nil
}

func resultIsError(content string) bool {
	var env struct {
		OK *bool `json:"ok"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(content)), &env) != nil {
		return false
	}
	return env.OK != nil && !*env.OK
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// defaultInstruction 是 CLI 的通用系统提示。
const defaultInstruction = `You are a local coding assistant running on the user's machine.

Guidelines:
- Inspect before you change: prefer read_file, glob and grep to understand the code.
- Use shell for build, test and git inspection; avoid destructive commands.
- Keep answers short and concrete. Cite file:line when you reference code.
- Never print secrets, tokens or credentials, even if you find them.
- If a tool returns an error envelope, read the code field and adapt instead of repeating the same call.`
