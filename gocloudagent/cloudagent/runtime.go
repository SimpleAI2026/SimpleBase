package cloudagent

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// Runtime executes agent turns through eino ChatModelAgent.
type Runtime struct {
	LLM           ChatClient
	Provider      RunProvider
	Tools         []tool.BaseTool
	Instruction   string
	MaxIterations int
	RunTimeout    time.Duration
	ToolProtocol  string

	MCP            MCPConnector
	MCPPolicy      MCPPolicy
	MCPCallTimeout time.Duration
	Confirm        ConfirmationWaiter
	ConfirmTimeout time.Duration

	Logger   Logger
	TraceLog bool

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	threads map[string]string
}

// StartRun executes one agent turn. emit is optional.
func (r *Runtime) StartRun(ctx context.Context, req RunRequest, emit func(Event)) (RunResult, error) {
	if r == nil || r.LLM == nil {
		return RunResult{}, &RunError{Code: "llm_not_configured", Message: "模型服务未配置"}
	}
	if err := r.claimThread(req.ThreadID, req.RunID); err != nil {
		return RunResult{}, err
	}
	defer r.releaseThread(req.ThreadID, req.RunID)
	started := time.Now()

	prep, err := r.prepare(ctx, req, emit)
	if prep.Cleanup != nil {
		defer prep.Cleanup()
	}
	if err != nil {
		return RunResult{}, err
	}

	rc := RunContext{RunID: req.RunID, ThreadID: req.ThreadID, Value: req.Context, Emit: emit}
	mcpTools, mcpSet, notes, err := buildMCPTools(ctx, mcpBuildParams{
		Req: req, RC: rc, Connector: r.MCP, Policy: r.MCPPolicy,
		Confirm: r.Confirm, ConfirmTimeout: r.confirmTimeout(), CallTimeout: r.MCPCallTimeout,
		Trace: r.trace(), Reserved: toolNameSet(ctx, prep.Tools),
	})
	if mcpSet != nil {
		defer mcpSet.Close()
	}
	if err != nil {
		return RunResult{}, err
	}
	prep.Tools = wrapToolErrors(append(prep.Tools, mcpTools...))
	prep.Notes = append(prep.Notes, notes...)
	if len(prep.Notes) > 0 {
		prep.Instruction += "\n\nTool status:\n- " + strings.Join(prep.Notes, "\n- ")
	}

	tr := r.trace()
	tr.Meta("gocloudagent: run start",
		zap.String("run_id", req.RunID),
		zap.String("thread", req.ThreadID),
		zap.String("agent", req.Agent.ID),
		zap.String("module", req.Agent.Module),
		zap.String("model", prep.Model),
		zap.Bool("stream", req.Stream),
		zap.Bool("headless", req.Headless),
		zap.Int("history", len(req.History)),
		zap.String("user", hashSummary(req.UserText)),
		zap.String("instruction", hashSummary(prep.Instruction)),
	)
	tr.Payload("gocloudagent: instruction", "instruction", prep.Instruction)
	tr.Payload("gocloudagent: user text", "text", req.UserText)

	chatModel, usage := r.newChatModel(prep.ModelScope, prep.Model, tr)
	agent, err := newChatAgent(ctx, req, prep.Instruction, chatModel, prep.Tools, r.maxIterations())
	if err != nil {
		return RunResult{}, err
	}

	runCtx, stop := r.startRunTimeout(ctx, req, rc)
	defer stop()
	assistant, toolCards, calls, runErr := r.consumeEvents(runCtx, req, agent, emit, tr, prep.OnToolCall)
	return finishRun(req, tr, started, runOutcome{
		assistant: assistant, toolCards: toolCards, calls: calls, usage: *usage,
		runErr: runErr, runCtx: runCtx, iterations: r.maxIterations(),
	})
}

func (r *Runtime) prepare(ctx context.Context, req RunRequest, emit func(Event)) (RunPreparation, error) {
	prep := RunPreparation{
		Model: req.Agent.ModelOverride, ModelScope: req.Agent.ID,
		Instruction: req.Agent.SystemPrompt,
	}
	var err error
	if r.Provider != nil {
		prep, err = r.Provider.Prepare(ctx, req, emit)
	}
	prep.Instruction = AssembleInstruction(r.Instruction, req.Instruction, prep.Instruction)
	if prep.Model == "" {
		prep.Model = req.Agent.ModelOverride
	}
	if prep.ModelScope == "" {
		prep.ModelScope = req.Agent.ID
	}
	if len(r.Tools) > 0 {
		prep.Tools = append(append([]tool.BaseTool(nil), r.Tools...), prep.Tools...)
	}
	return prep, err
}

func toolNameSet(ctx context.Context, tools []tool.BaseTool) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		info, err := t.Info(ctx)
		if err == nil && info != nil {
			out[info.Name] = true
		}
	}
	return out
}

func (r *Runtime) maxIterations() int {
	if r.MaxIterations > 0 {
		return r.MaxIterations
	}
	return 8
}

func (r *Runtime) newChatModel(scope, modelName string, tr tracer) (*agentcore.Model, *ChatUsage) {
	chatModel := agentcore.NewModel(r.LLM, scope, modelName)
	chatModel.Warn = tr.Warn
	if r.ToolProtocol != "" {
		chatModel.Protocol = r.ToolProtocol
	}
	usage := &ChatUsage{}
	chatModel.Usage = func(u ChatUsage) {
		usage.PromptTokens += u.PromptTokens
		usage.CompletionTokens += u.CompletionTokens
		usage.ReasoningTokens += u.ReasoningTokens
	}
	return chatModel, usage
}

func newChatAgent(ctx context.Context, req RunRequest, instruction string, chatModel *agentcore.Model, tools []tool.BaseTool, iterations int) (adk.Agent, error) {
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name: safeAgentName(req.Agent.Name), Description: req.Agent.Description,
		Instruction: instruction, Model: chatModel,
		ToolsConfig:   adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools}},
		MaxIterations: iterations,
	})
	if err != nil {
		return nil, fmt.Errorf("create chat model agent: %w", err)
	}
	return agent, nil
}

func (r *Runtime) startRunTimeout(ctx context.Context, req RunRequest, rc RunContext) (context.Context, func()) {
	timeout := r.RunTimeout
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	r.storeCancel(req.RunID, cancel)
	runCtx = withRunContext(runCtx, rc)
	return runCtx, func() {
		cancel()
		r.clearCancel(req.RunID)
	}
}

func (r *Runtime) consumeEvents(runCtx context.Context, req RunRequest, agent adk.Agent, emit func(Event), tr tracer, onToolCall func(string)) (string, []map[string]any, map[string]map[string]any, error) {
	runner := adk.NewRunner(runCtx, adk.RunnerConfig{Agent: agent, EnableStreaming: req.Stream})
	msgs := historyToSchema(req.History)
	msgs = append(msgs, schema.UserMessage(truncate(req.UserText, maxUserChars)))
	iter := runner.Run(runCtx, msgs)

	var assistant strings.Builder
	var toolCards []map[string]any
	calls := map[string]map[string]any{}
	callStarts := map[string]time.Time{}
	var runErr error
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
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		mv := event.Output.MessageOutput
		if mv.Role == schema.Tool {
			content, name := agentcore.ConsumeVariant(mv)
			name = cmp.Or(mv.ToolName, name)
			id := ""
			if mv.Message != nil {
				id = mv.Message.ToolCallID
			}
			card := calls[id]
			if card == nil {
				for i := len(toolCards) - 1; i >= 0; i-- {
					if toolCards[i]["name"] == name && toolCards[i]["content"] == nil {
						card = toolCards[i]
						id, _ = card["call_id"].(string)
						break
					}
				}
			}
			if card == nil {
				card = map[string]any{"call_id": id, "name": name}
				toolCards = append(toolCards, card)
			}
			const maxToolCardChars = 4000
			truncated := len([]rune(content)) > maxToolCardChars
			content = truncate(content, maxToolCardChars)
			duration := time.Since(callStarts[id]).Milliseconds()
			if callStarts[id].IsZero() {
				duration = 0
			}
			card["content"] = content
			card["duration_ms"] = duration
			card["truncated"] = truncated
			isErr := toolResultIsError(content)
			card["is_error"] = isErr
			if emit != nil {
				emit(Event{Type: "tool_result", CallID: id, Name: name, Content: content, DurationMS: duration, Truncated: truncated, IsError: isErr})
			}
			tr.Meta("gocloudagent: tool result",
				zap.String("run_id", req.RunID), zap.String("tool", name), zap.String("call_id", id),
				zap.Int64("duration_ms", duration), zap.Bool("is_error", isErr), zap.Bool("truncated", truncated),
			)
			tr.Payload("gocloudagent: tool result body", "content", content)
			continue
		}
		if mv.Role != schema.Assistant && mv.Role != "" {
			continue
		}
		msg, err := consumeAssistant(mv, emit, &assistant)
		if err != nil {
			runErr = err
			break
		}
		if msg == nil {
			continue
		}
		for _, tc := range msg.ToolCalls {
			id := tc.ID
			if id == "" {
				id = "call_" + uuid.NewString()
			}
			card := map[string]any{"call_id": id, "name": tc.Function.Name, "arguments": tc.Function.Arguments}
			calls[id] = card
			callStarts[id] = time.Now()
			toolCards = append(toolCards, card)
			if onToolCall != nil {
				onToolCall(id)
			}
			if emit != nil {
				emit(Event{Type: "tool_call", CallID: id, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
			}
			tr.Meta("gocloudagent: tool call", zap.String("run_id", req.RunID), zap.String("tool", tc.Function.Name), zap.String("call_id", id))
			tr.Payload("gocloudagent: tool arguments", "arguments", tc.Function.Arguments)
		}
	}
	return assistant.String(), toolCards, calls, runErr
}

type runOutcome struct {
	assistant  string
	toolCards  []map[string]any
	calls      map[string]map[string]any
	usage      ChatUsage
	runErr     error
	runCtx     context.Context
	iterations int
}

func finishRun(req RunRequest, tr tracer, started time.Time, out runOutcome) (RunResult, error) {
	toolJSON := "[]"
	if len(out.toolCards) > 0 {
		body, _ := json.Marshal(out.toolCards)
		toolJSON = string(body)
	}
	result := RunResult{
		Content: out.assistant, ToolCallsJSON: toolJSON, DurationMS: time.Since(started).Milliseconds(),
		PromptTokens: out.usage.PromptTokens, CompletionTokens: out.usage.CompletionTokens,
		ReasoningTokens: out.usage.ReasoningTokens, ToolCalls: len(out.calls), Reason: "stop",
	}
	if errors.Is(out.runErr, adk.ErrExceedMaxIterations) {
		result.Content += fmt.Sprintf("\n已达到工具调用上限（%d 次），以上为当前结论", out.iterations)
		result.Reason = "max_iterations"
		tr.finish(req, result)
		return result, nil
	}
	if errors.Is(out.runErr, agentcore.ErrMaxTokensExceeded) {
		result.Content += "\n\n⚠️ 输出被 max_tokens 截断，内容可能不完整。可尝试拆分任务或提高 token 限额。"
		result.Reason = "length"
		tr.finish(req, result)
		return result, nil
	}
	if errors.Is(out.runErr, context.Canceled) || errors.Is(out.runCtx.Err(), context.Canceled) {
		result.Reason = "canceled"
		tr.finish(req, result)
		return result, context.Canceled
	}
	if errors.Is(out.runErr, context.DeadlineExceeded) || errors.Is(out.runCtx.Err(), context.DeadlineExceeded) {
		result.Reason = "timeout"
		tr.finish(req, result)
		return result, context.DeadlineExceeded
	}
	if out.runErr != nil {
		result.Reason = "error"
		tr.finish(req, result)
		return result, out.runErr
	}
	tr.finish(req, result)
	return result, nil
}
