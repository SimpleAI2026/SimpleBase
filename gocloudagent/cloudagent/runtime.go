// 文件说明：runtime.go — Runtime 装配与 StartRun 主循环

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
func (r *Runtime) StartRun(ctx context.Context, req RunRequest, emit func(Event)) (RunResult, error) {
	if r == nil || r.LLM == nil {
		return RunResult{}, &RunError{Code: "llm_not_configured", Message: "模型服务未配置"}
	}
	if err := r.claimThread(req.ThreadID, req.RunID); err != nil {
		return RunResult{}, err
	}
	defer r.releaseThread(req.ThreadID, req.RunID)
	started := time.Now()
	modelName := strings.TrimSpace(req.Agent.ModelOverride)
	if modelName == "" && r.Settings != nil {
		if st, err := r.Settings.LLMSettings(ctx, req.ProjectID); err == nil {
			modelName = strings.TrimSpace(st.DefaultModel)
		}
	}

	snapshot := r.buildSnapshot(ctx, req)
	parts := PromptParts{
		ModuleTemplate: moduleTemplate(req.Agent.Module),
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
				return RunResult{}, ierr
			}
			r.rememberToken(req.RunID, tok)
			defer r.forgetToken(req.RunID)
			defer r.Issuer.RevokeRun(req.RunID)
		}
		// planv6.0 §3.4：run 结束清理 gate 残留，避免泄漏累积。
		defer r.ensureGate().dropRun(req.RunID)
		tools, err = r.buildSkillTools(req)
	} else {
		tools, err = buildTools(req.Agent.ToolIDs, toolDeps{DB: r.DB, Obj: r.Obj, Logs: r.Logs, Sandbox: r.Sandbox})
	}
	if err != nil {
		return RunResult{}, err
	}
	// MCP 工具：两条分支都要追加，否则 SkillsCLI 模式下 MCP 静默失效（§2）。
	mcpTools, mcpSet, mcpNotes, err := buildMCPTools(ctx, mcpBuildParams{
		Req:            req,
		RC:             RunContext{ProjectID: req.ProjectID, Principal: req.Principal, ThreadID: req.ThreadID, RunID: req.RunID, Skills: req.Skills, Headless: req.Headless, Writable: req.Agent.Writable},
		Connector:      r.MCP,
		Policy:         r.MCPPolicy,
		Confirm:        r.Confirm,
		ConfirmTimeout: r.confirmTimeout(),
		Trace:          tr,
	})
	if err != nil {
		return RunResult{}, err
	}
	if mcpSet != nil {
		// per-run 连接：与 Issuer.RevokeRun / gate.dropRun 同风格，失败路径也回收。
		defer mcpSet.Close()
	}
	if len(mcpTools) > 0 {
		tools = append(tools, mcpTools...)
	}
	if len(mcpNotes) > 0 {
		// 连接失败的 Server 如实告知模型，由它向用户说明（§4.3）。
		instruction += "\n\nMCP status:\n- " + strings.Join(mcpNotes, "\n- ")
	}

	chatModel := agentcore.NewModel(r.LLM, req.ProjectID, modelName)
	chatModel.Warn = tr.Warn
	if r.ToolProtocol != "" {
		chatModel.Protocol = r.ToolProtocol
	}
	var usage ChatUsage
	chatModel.Usage = func(u ChatUsage) {
		usage.PromptTokens += u.PromptTokens
		usage.CompletionTokens += u.CompletionTokens
		usage.ReasoningTokens += u.ReasoningTokens
	}
	iterations := r.MaxIterations
	if iterations <= 0 {
		iterations = 8
	}
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
		return RunResult{}, fmt.Errorf("create chat model agent: %w", err)
	}

	timeout := r.RunTimeout
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	r.storeCancel(req.RunID, cancel)
	defer r.clearCancel(req.RunID)
	defer cancel()

	runCtx = withRunContext(runCtx, RunContext{
		ProjectID: req.ProjectID, Principal: req.Principal, ThreadID: req.ThreadID,
		RunID: req.RunID, Skills: req.Skills, Headless: req.Headless, Emit: emit,
		Writable: req.Agent.Writable,
	})

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
	toolJSON := "[]"
	if len(toolCards) > 0 {
		b, _ := json.Marshal(toolCards)
		toolJSON = string(b)
	}
	result := RunResult{Content: assistant.String(), ToolCallsJSON: toolJSON, DurationMS: time.Since(started).Milliseconds(),
		PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
		ReasoningTokens: usage.ReasoningTokens, ToolCalls: len(calls), Reason: "stop"}
	if errors.Is(runErr, adk.ErrExceedMaxIterations) {
		result.Content += fmt.Sprintf("\n已达到工具调用上限（%d 次），以上为当前结论", iterations)
		result.Reason = "max_iterations"
		tr.finish(req, result)
		return result, nil
	}
	// planv6.1 O2-5: 检测 max_tokens 截断（finish_reason='length'）。
	if errors.Is(runErr, agentcore.ErrMaxTokensExceeded) {
		result.Content += "\n\n⚠️ 输出被 max_tokens 截断，内容可能不完整。可尝试拆分任务或提高 token 限额。"
		result.Reason = "length"
		tr.finish(req, result)
		return result, nil
	}
	if errors.Is(runErr, context.Canceled) || errors.Is(runCtx.Err(), context.Canceled) {
		result.Reason = "canceled"
		tr.finish(req, result)
		return result, context.Canceled
	}
	if errors.Is(runErr, context.DeadlineExceeded) || errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		result.Reason = "timeout"
		tr.finish(req, result)
		return result, context.DeadlineExceeded
	}
	if runErr != nil {
		result.Reason = "error"
		tr.finish(req, result)
		return result, runErr
	}
	tr.finish(req, result)
	return result, nil
}
