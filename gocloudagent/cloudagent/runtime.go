// 文件说明：runtime.go — gocloudagent/cloudagent 中 Runtime 等符号的实现
//
// 用法
//
//	type Runtime struct{…}  // Runtime executes agent turns through eino ChatModelAgent.
//	同包调用：runOutcome → runtime_events_test.go；finishRun() → runtime_events_test.go
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/runtime.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//	[关键引用] Runtime ← runthread.go, runtime_history_test.go；runOutcome ← runtime_events_test.go；finishRun ← runtime_events_test.go
//
// 主要符号
//
//	type Runtime struct{…}
//	(r *Runtime) StartRun(ctx context.Context, req RunRequest, emit func(Event)) (RunResult, error)
//	(r *Runtime) prepare(ctx context.Context, req RunRequest, emit func(Event)) (RunPreparation, error)
//	toolNameSet(ctx context.Context, tools []tool.BaseTool) map[string]bool
//	(r *Runtime) maxIterations() int
//	(r *Runtime) newChatModel(scope, modelName string, tr Tracer) (*agentcore.Model, *ChatUsage)
//	newChatAgent(ctx context.Context, req RunRequest, instruction string, chatModel *agentcore.Model, tools []tool...
//	(r *Runtime) startRunTimeout(ctx context.Context, req RunRequest, rc RunContext) (context.Context, func())
//	... 另有 3 个顶层符号

package cloudagent

import (
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

	// 超时与取消登记覆盖整个 run（含 Provider.Prepare 与 MCP 装配）：准备期卡在外部依赖上时，
	// RunTimeout 与 CancelRun 同样生效。
	//
	// 清理顺序（defer 逆序）固定为：stop（取消并清除登记）→ mcpSet.Close → prep.Cleanup →
	// releaseThread。stop 要最先运行，所以它必须在其余清理项之后才注册；而 Prepare 之前就要
	// 有可取消的 ctx。因此用 stopOnce 先创建 ctx，并在所有清理 defer 注册完之后再 defer 它。
	rc := RunContext{RunID: req.RunID, ThreadID: req.ThreadID, Value: req.Context, Emit: emit}
	runCtx, stopOnce := r.startRunTimeout(ctx, req, rc)
	stopRegistered := false
	defer func() {
		if !stopRegistered { // 提前返回（Prepare 失败等）时，清理 defer 尚未注册完整
			stopOnce()
		}
	}()

	prep, err := r.prepare(runCtx, req, emit)
	if prep.Cleanup != nil {
		defer prep.Cleanup()
	}
	if err != nil {
		return RunResult{}, err
	}

	mcpTools, mcpSet, notes, err := buildMCPTools(runCtx, mcpBuildParams{
		Req: req, RC: rc, Connector: r.MCP, Policy: r.MCPPolicy,
		Confirm: r.Confirm, ConfirmTimeout: r.ConfirmTimeout, CallTimeout: r.MCPCallTimeout,
		Trace: r.trace(), Reserved: toolNameSet(runCtx, prep.Tools),
	})
	if mcpSet != nil {
		defer mcpSet.Close()
	}
	if err != nil {
		return RunResult{}, err
	}
	// 清理 defer 已全部注册：此后 stop 作为最后注册的 defer 最先执行。
	stopRegistered = true
	defer stopOnce()
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
		zap.String("user", HashSummary(req.UserText)),
		zap.String("instruction", HashSummary(prep.Instruction)),
	)
	tr.Payload("gocloudagent: instruction", "instruction", prep.Instruction)
	tr.Payload("gocloudagent: user text", "text", req.UserText)

	chatModel, usage := r.newChatModel(prep.ModelScope, prep.Model, tr)
	agent, err := newChatAgent(runCtx, req, prep.Instruction, chatModel, prep.Tools, r.maxIterations())
	if err != nil {
		return RunResult{}, err
	}

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

func (r *Runtime) newChatModel(scope, modelName string, tr Tracer) (*agentcore.Model, *ChatUsage) {
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

func (r *Runtime) consumeEvents(runCtx context.Context, req RunRequest, agent adk.Agent, emit func(Event), tr Tracer, onToolCall func(string)) (string, []map[string]any, map[string]map[string]any, error) {
	runner := adk.NewRunner(runCtx, adk.RunnerConfig{Agent: agent, EnableStreaming: req.Stream})
	msgs := historyToSchema(req.History)
	msgs = append(msgs, schema.UserMessage(agentcore.Truncate(req.UserText, maxUserChars)))
	iter := runner.Run(runCtx, msgs)

	var assistant strings.Builder
	events := newToolEvents(req.RunID, emit, tr, onToolCall)
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
			events.result(mv)
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
		events.calls(msg.ToolCalls)
	}
	return assistant.String(), events.cards, events.byID, runErr
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

func finishRun(req RunRequest, tr Tracer, started time.Time, out runOutcome) (RunResult, error) {
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
	var err error
	switch {
	case errors.Is(out.runErr, adk.ErrExceedMaxIterations):
		result.Content += fmt.Sprintf("\n已达到工具调用上限（%d 次），以上为当前结论", out.iterations)
		result.Reason = "max_iterations"
	case errors.Is(out.runErr, agentcore.ErrMaxTokensExceeded):
		result.Content += "\n\n⚠️ 输出被 max_tokens 截断，内容可能不完整。可尝试拆分任务或提高 token 限额。"
		result.Reason = "length"
	case errors.Is(out.runErr, context.Canceled) || errors.Is(out.runCtx.Err(), context.Canceled):
		result.Reason, err = "canceled", context.Canceled
	case errors.Is(out.runErr, context.DeadlineExceeded) || errors.Is(out.runCtx.Err(), context.DeadlineExceeded):
		result.Reason, err = "timeout", context.DeadlineExceeded
	case out.runErr != nil:
		result.Reason, err = "error", out.runErr
	}
	tr.finish(req, result)
	return result, err
}
