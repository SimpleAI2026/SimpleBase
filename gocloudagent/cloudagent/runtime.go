// 文件说明：runtime.go — Runtime 装配与 StartRun 主流程
//
// 调用架构
//
//	[上游] gocloudagent/cloudagent（Runtime 的宿主）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/runtime.go
//	         │
//	         ▼
//	[下游] cloudagent/{skilltool,tools,mcp,trace,stream}.go、agentcore、eino/adk
//	[同包] callgate.go, prompt.go
//
// 主要符号
//
//	type Runtime struct {…}
//	(r *Runtime) StartRun(ctx, req, emit) (RunResult, error)
//
// 主流程按步骤拆成 resolveModelName / buildInstruction / assembleTools /
// newChatModel / newChatAgent / startRunTimeout / consumeEvents / finishRun，
// StartRun 只负责串起来。

package cloudagent

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	agentskills "github.com/linkxzhou/SimpleBase/gocloudagent/skills"
)

// Runtime executes Cloud Agents via eino ChatModelAgent.
type Runtime struct {
	LLM      ChatClient
	DB       DatabaseAccess
	Obj      ObjectAccess
	Logs     LogAccess
	Settings SettingsAccess
	// Sandbox 为 nil 表示云沙盒未启用（cloud-agent-sandbox-plan §4）。
	Sandbox Sandbox
	// MaxIterations 和 RunTimeout 为零时使用默认值。
	MaxIterations int
	RunTimeout    time.Duration
	ToolProtocol  string
	// SkillsCLI 为真时用 simplebase CLI 工具替换只读数据工具。
	SkillsCLI      bool
	CLI            CLIRunner
	CLIPath        string
	APIBaseURL     string
	Issuer         DelegationIssuer
	Confirm        ConfirmationWaiter
	ConfirmTimeout time.Duration
	DelegationTTL  time.Duration
	// Logger 是运行日志出口（planv6.0 v2 §5）；nil 时全部为 no-op。
	Logger Logger
	// Policy 是命令安全分类的唯一入口（宿主注入 sbcli + sqlguard 规则）。
	// 为 nil 时 CLI 工具一律拒绝执行（fail-closed），不做「确认后放行」兜底。
	Policy CommandPolicy
	// MCP 是外部 MCP Server 的连接器（planv6.2 §4.1）。为 nil 时 MCP 工具不装配；
	// Agent 已选 mcp: id 时明确报错，不静默忽略。
	MCP MCPConnector
	// MCPPolicy 给出每个 MCP 工具的只读/破坏性判定。为 nil 时 MCP 工具一律拒绝执行。
	MCPPolicy MCPPolicy
	// TraceLog 开启内容层日志（提示词/正文，脱敏+截断）。默认关闭。
	TraceLog bool

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	threads map[string]string
	tokens  map[string]string
	gate    *callGate
}

// StartRun executes the agent. emit is optional (streaming).
//
// 主流程只有九步：认领线程 → 解析模型 → 组装提示词 → 装配工具 → 建 agent →
// 起超时 → 消费事件 → 收尾。每一步的实现都在下面的私有方法里，改动时按步骤
// 定位，不要再往这里堆细节。
func (r *Runtime) StartRun(ctx context.Context, req RunRequest, emit func(Event)) (RunResult, error) {
	if r == nil || r.LLM == nil {
		return RunResult{}, &RunError{Code: "llm_not_configured", Message: "模型服务未配置"}
	}
	if err := r.claimThread(req.ThreadID, req.RunID); err != nil {
		return RunResult{}, err
	}
	defer r.releaseThread(req.ThreadID, req.RunID)
	started := time.Now()

	modelName := r.resolveModelName(ctx, req)
	instruction, tr := r.buildInstruction(ctx, req, modelName)

	tools, mcpNotes, cleanup, err := r.assembleTools(ctx, req, tr)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return RunResult{}, err
	}
	if len(mcpNotes) > 0 {
		// 连接失败的 Server 如实告知模型，由它向用户说明（§4.3）。
		instruction += "\n\nMCP status:\n- " + strings.Join(mcpNotes, "\n- ")
	}

	chatModel, usage := r.newChatModel(req, modelName, tr)
	agent, err := newChatAgent(ctx, req, instruction, chatModel, tools, r.maxIterations())
	if err != nil {
		return RunResult{}, err
	}

	runCtx, stop := r.startRunTimeout(ctx, req, emit)
	defer stop()

	assistant, toolCards, calls, runErr := r.consumeEvents(runCtx, req, agent, emit, tr)
	return finishRun(req, tr, started, runOutcome{
		assistant:  assistant,
		toolCards:  toolCards,
		calls:      calls,
		usage:      *usage,
		runErr:     runErr,
		runCtx:     runCtx,
		iterations: r.maxIterations(),
	})
}

// maxIterations 返回工具调用上限（零值取默认 8）。
func (r *Runtime) maxIterations() int {
	if r.MaxIterations > 0 {
		return r.MaxIterations
	}
	return 8
}

// resolveModelName 取 agent 覆盖的模型，没有就用项目默认模型。
func (r *Runtime) resolveModelName(ctx context.Context, req RunRequest) string {
	modelName := strings.TrimSpace(req.Agent.ModelOverride)
	if modelName == "" && r.Settings != nil {
		if st, err := r.Settings.LLMSettings(ctx, req.ProjectID); err == nil {
			modelName = strings.TrimSpace(st.DefaultModel)
		}
	}
	return modelName
}

// buildInstruction 组装系统提示词，并写下本次运行的元数据层日志。
func (r *Runtime) buildInstruction(ctx context.Context, req RunRequest, modelName string) (string, tracer) {
	snapshot := r.buildSnapshot(ctx, req)
	moduleTpl := moduleTemplate(req.Agent.Module)
	if req.StarterStage != "" {
		moduleTpl += "\n\n" + starterPrompt(req.StarterStage)
	}
	parts := PromptParts{
		ModuleTemplate: moduleTpl,
		AgentPrompt:    req.Agent.SystemPrompt,
		ProjectEnv:     fmt.Sprintf("Project env:\n- project_id: %s\n- agent: %s (%s)\n- module: %s", req.ProjectID, req.Agent.Name, req.Agent.ID, req.Agent.Module),
		Snapshot:       snapshot,
	}
	if r.SkillsCLI {
		parts.SkillsCLI = true
		parts.Writable = req.Agent.Writable
		parts.SkillText = agentskills.Prompt(req.Skills)
		if len(req.Skills) > 0 {
			parts.ProjectEnv += "\n- skills: " + strings.Join(req.Skills, ",")
		}
		if req.StarterStage != "" {
			parts.ProjectEnv += "\n- starter_stage: " + req.StarterStage
			if len(req.ReadOnlySkills) > 0 {
				parts.ProjectEnv += "\n- readonly_skills: " + strings.Join(req.ReadOnlySkills, ",")
			}
		}
		if !slices.Contains(req.Skills, ModuleDatabase) && req.Agent.Module != ModuleDatabase {
			parts.Snapshot = ""
		}
	}
	instruction := AssembleInstruction(parts)
	tr := r.trace()
	tr.Meta("gocloudagent: run start",
		zap.String("run_id", req.RunID),
		zap.String("project", req.ProjectID),
		zap.String("thread", req.ThreadID),
		zap.String("agent", req.Agent.ID),
		zap.String("module", req.Agent.Module),
		zap.String("model", modelName),
		zap.Bool("stream", req.Stream),
		zap.Bool("headless", req.Headless),
		zap.Int("history", len(req.History)),
		zap.String("user", hashSummary(req.UserText)),
		zap.String("instruction", hashSummary(instruction)),
		zap.String("skills", strings.Join(req.Skills, ",")),
	)
	// 内容层（默认关闭）：提示词与用户输入，脱敏 + 截断。
	tr.Payload("gocloudagent: instruction", "instruction", instruction)
	tr.Payload("gocloudagent: user text", "text", req.UserText)
	return instruction, tr
}

// assembleTools 装配本次运行的工具，并返回一个按注册逆序回收的 cleanup。
//
// cleanup 一定非 nil：即使中途失败也把已经占用的资源（委托令牌、callGate
// 登记、MCP 连接）放进去，由 StartRun 的 defer 收尾。
func (r *Runtime) assembleTools(ctx context.Context, req RunRequest, tr tracer) ([]tool.BaseTool, []string, func(), error) {
	var cleanups []func()
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	var tools []tool.BaseTool
	var err error
	if r.SkillsCLI {
		if r.Issuer != nil {
			// planv6.0 v2 §3.4 第一层（入口降权）：不可写助手签发的委托令牌
			// 不含 database:write / database:admin，HTTP 路由层的
			// delegable(DatabaseWrite) 会 403 兜底。IssueDelegation 本就是
			// 「调用者权限的子集」，降权不破坏既有约定。
			principal := req.Principal
			if !req.Agent.Writable {
				principal = stripWritePermissions(principal)
			}
			tok, ierr := r.Issuer.Issue(principal, req.ProjectID, req.RunID, r.delegationTTL())
			if ierr != nil {
				return nil, nil, cleanup, ierr
			}
			r.rememberToken(req.RunID, tok)
			cleanups = append(cleanups, func() { r.forgetToken(req.RunID) })
			cleanups = append(cleanups, func() { r.Issuer.RevokeRun(req.RunID) })
		}
		// planv6.0 §3.4：run 结束清理 gate 残留，避免泄漏累积。
		cleanups = append(cleanups, func() { r.ensureGate().dropRun(req.RunID) })
		tools, err = r.buildSkillTools(req)
	} else {
		tools, err = buildTools(req.Agent.ToolIDs, toolDeps{DB: r.DB, Obj: r.Obj, Logs: r.Logs, Sandbox: r.Sandbox})
	}
	if err != nil {
		return nil, nil, cleanup, err
	}
	// MCP 工具：两条分支都要追加，否则 SkillsCLI 模式下 MCP 静默失效（§2）。
	mcpTools, mcpSet, mcpNotes, err := buildMCPTools(ctx, mcpBuildParams{
		Req:            req,
		RC:             RunContext{ProjectID: req.ProjectID, Principal: req.Principal, ThreadID: req.ThreadID, RunID: req.RunID, Skills: req.Skills, ReadOnlySkills: req.ReadOnlySkills, Headless: req.Headless, Writable: req.Agent.Writable},
		Connector:      r.MCP,
		Policy:         r.MCPPolicy,
		Confirm:        r.Confirm,
		ConfirmTimeout: r.confirmTimeout(),
		Trace:          tr,
	})
	if err != nil {
		return nil, nil, cleanup, err
	}
	if mcpSet != nil {
		// per-run 连接：与 Issuer.RevokeRun / gate.dropRun 同风格，失败路径也回收。
		cleanups = append(cleanups, func() { mcpSet.Close() })
	}
	if len(mcpTools) > 0 {
		tools = append(tools, mcpTools...)
	}
	return tools, mcpNotes, cleanup, nil
}

// newChatModel 建模型适配器，并把 usage 累加器挂上去。
func (r *Runtime) newChatModel(req RunRequest, modelName string, tr tracer) (*agentcore.Model, *ChatUsage) {
	chatModel := agentcore.NewModel(r.LLM, req.ProjectID, modelName)
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

// newChatAgent 把提示词、模型与工具装成一个可运行的 agent。
func newChatAgent(ctx context.Context, req RunRequest, instruction string, chatModel *agentcore.Model, tools []tool.BaseTool, iterations int) (adk.Agent, error) {
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        safeAgentName(req.Agent.Name),
		Description: req.Agent.Description,
		Instruction: instruction,
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools},
		},
		MaxIterations: iterations,
	})
	if err != nil {
		return nil, fmt.Errorf("create chat model agent: %w", err)
	}
	return agent, nil
}

// startRunTimeout 起本次运行的超时上下文，并把取消函数登记到 Runtime 上。
//
// 返回的 stop 必须 defer：它先取消上下文再摘掉登记（与原实现的 defer 顺序
// 一致：cancel 先执行、clearCancel 后执行）。
func (r *Runtime) startRunTimeout(ctx context.Context, req RunRequest, emit func(Event)) (context.Context, func()) {
	timeout := r.RunTimeout
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	r.storeCancel(req.RunID, cancel)
	runCtx = withRunContext(runCtx, RunContext{
		ProjectID: req.ProjectID, Principal: req.Principal, ThreadID: req.ThreadID,
		RunID: req.RunID, Skills: req.Skills, ReadOnlySkills: req.ReadOnlySkills, Headless: req.Headless, Emit: emit,
		Writable: req.Agent.Writable,
	})
	return runCtx, func() {
		cancel()
		r.clearCancel(req.RunID)
	}
}

// consumeEvents 消费 agent 的事件流：正文写进 assistant，工具调用与结果整理成
// 卡片，并透传给 emit。
func (r *Runtime) consumeEvents(runCtx context.Context, req RunRequest, agent adk.Agent, emit func(Event), tr tracer) (string, []map[string]any, map[string]map[string]any, error) {
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
				zap.String("run_id", req.RunID),
				zap.String("tool", name),
				zap.String("call_id", id),
				zap.Int64("duration_ms", duration),
				zap.Bool("is_error", isErr),
				zap.Bool("truncated", truncated),
			)
			tr.Payload("gocloudagent: tool result body", "content", content)
			continue
		}
		if mv.Role == schema.Assistant || mv.Role == "" {
			msg, err := consumeAssistant(mv, emit, &assistant)
			if err != nil {
				runErr = err
				break
			}
			if msg != nil && len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					id := tc.ID
					if id == "" {
						id = "call_" + uuid.NewString()
					}
					card := map[string]any{"call_id": id, "name": tc.Function.Name, "arguments": tc.Function.Arguments}
					calls[id] = card
					callStarts[id] = time.Now()
					toolCards = append(toolCards, card)
					if r.SkillsCLI {
						r.ensureGate().release(req.RunID, id)
					}
					if emit != nil {
						emit(Event{Type: "tool_call", CallID: id, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
					}
					tr.Meta("gocloudagent: tool call",
						zap.String("run_id", req.RunID),
						zap.String("tool", tc.Function.Name),
						zap.String("call_id", id),
					)
					tr.Payload("gocloudagent: tool arguments", "arguments", tc.Function.Arguments)
				}
			}
		}
	}
	return assistant.String(), toolCards, calls, runErr
}

// runOutcome 是 consumeEvents 的产出，供 finishRun 组装 RunResult。
type runOutcome struct {
	assistant  string
	toolCards  []map[string]any
	calls      map[string]map[string]any
	usage      ChatUsage
	runErr     error
	runCtx     context.Context
	iterations int
}

// finishRun 把事件循环的产出与错误分类成最终结果。
//
// 取消/超时/迭代上限/max_tokens 都返回 nil error 但带对应 Reason；
// 其余错误原样返回。判断顺序与拆分前一致。
func finishRun(req RunRequest, tr tracer, started time.Time, out runOutcome) (RunResult, error) {
	toolJSON := "[]"
	if len(out.toolCards) > 0 {
		b, _ := json.Marshal(out.toolCards)
		toolJSON = string(b)
	}
	result := RunResult{Content: out.assistant, ToolCallsJSON: toolJSON, DurationMS: time.Since(started).Milliseconds(),
		PromptTokens: out.usage.PromptTokens, CompletionTokens: out.usage.CompletionTokens,
		ReasoningTokens: out.usage.ReasoningTokens, ToolCalls: len(out.calls), Reason: "stop"}
	if errors.Is(out.runErr, adk.ErrExceedMaxIterations) {
		result.Content += fmt.Sprintf("\n已达到工具调用上限（%d 次），以上为当前结论", out.iterations)
		result.Reason = "max_iterations"
		tr.finish(req, result)
		return result, nil
	}
	// planv6.1 O2-5: 检测 max_tokens 截断（finish_reason='length'）。
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
