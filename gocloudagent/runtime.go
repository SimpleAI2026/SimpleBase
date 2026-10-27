// 文件说明：runtime.go — gocloudagent 中 Event、RunRequest 等符号的实现
//
// 用法
//
//	(r *Runtime) StartRun(ctx context.Context, req RunRequest, emit func(Event)) ...  // StartRun executes the agent. emit is optional (streaming).
//	(r *Runtime) ClaimThread(threadID, runID string) error  // ClaimThread 在持久化用户消息之前锁定会话，避免并发交错
//	同包调用：Runtime → access.go, cli_tool.go 等；Event → cli_entry.go, cli_tool.go 等；RunRequest → cli_tool.go, trace.go
//
// 调用架构
//
//	[上游] gocloudagent/cmd/gocloudagent, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/runtime.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/agentcore
//	[同包] access.go, cli_entry.go, cli_runner.go, cli_tool.go, errors.go 等 11 个
//	[关键引用] Runtime ← access.go, cli_tool.go, trace.go
//
// 主要符号
//
//	type Event struct {…}
//	type RunRequest struct {…}
//	type RunResult struct {…}
//	type Runtime struct {…}
//	(r *Runtime) StartRun(ctx context.Context, req RunRequest, emit func(Event)) ...
//	(r *Runtime) ClaimThread(threadID, runID string) error
//	... 另有 13 个顶层符号

package gocloudagent

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
)

// Event is a streaming run event for SSE/NDJSON.
type Event struct {
	Type             string   `json:"type"`
	Content          string   `json:"content,omitempty"`
	Name             string   `json:"name,omitempty"`
	Arguments        string   `json:"arguments,omitempty"`
	CallID           string   `json:"call_id,omitempty"`
	RunID            string   `json:"run_id,omitempty"`
	Message          string   `json:"message,omitempty"`
	Code             string   `json:"code,omitempty"`
	Reason           string   `json:"reason,omitempty"`
	ElapsedMS        int64    `json:"elapsed_ms,omitempty"`
	DurationMS       int64    `json:"duration_ms,omitempty"`
	PromptTokens     int      `json:"prompt_tokens,omitempty"`
	CompletionTokens int      `json:"completion_tokens,omitempty"`
	ReasoningTokens  int      `json:"reasoning_tokens,omitempty"`
	ToolCalls        int      `json:"tool_calls,omitempty"`
	IsError          bool     `json:"is_error,omitempty"`
	Truncated        bool     `json:"truncated,omitempty"`
	Argv             []string `json:"argv,omitempty"`
	Approve          *bool    `json:"approve,omitempty"`
}

// RunRequest starts a single-agent turn.
type RunRequest struct {
	ProjectID string
	Principal Principal
	Agent     Agent
	ThreadID  string
	RunID     string
	UserText  string
	History   []Message
	Stream    bool
	// Skills 是本轮已校验的 skill id。Headless 为定时任务，不能确认破坏性命令。
	Skills   []string
	Headless bool
}

// RunResult is the completed assistant turn.
type RunResult struct {
	Content          string
	ToolCallsJSON    string
	DurationMS       int64
	PromptTokens     int
	CompletionTokens int
	ReasoningTokens  int
	ToolCalls        int
	Reason           string
}

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
		ModuleTemplate: ModuleTemplate(req.Agent.Module),
		AgentPrompt:    req.Agent.SystemPrompt,
		ProjectEnv:     fmt.Sprintf("Project env:\n- project_id: %s\n- agent: %s (%s)\n- module: %s", req.ProjectID, req.Agent.Name, req.Agent.ID, req.Agent.Module),
		Snapshot:       snapshot,
	}
	if r.SkillsCLI {
		parts.SkillsCLI = true
		parts.Writable = req.Agent.Writable
		parts.SkillText = SkillPrompt(req.Skills)
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
			content, name := consumeVariant(mv)
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
						r.ensureGate().release(id)
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

// ClaimThread 在持久化用户消息之前锁定会话，避免并发交错。
func (r *Runtime) ClaimThread(threadID, runID string) error {
	if r == nil {
		return &RunError{Code: "llm_not_configured", Message: "模型服务未配置"}
	}
	return r.claimThread(threadID, runID)
}

func (r *Runtime) claimThread(threadID, runID string) error {
	if threadID == "" || runID == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.threads == nil {
		r.threads = map[string]string{}
	}
	if active := r.threads[threadID]; active != "" && active != runID {
		r.trace().Warn("gocloudagent: thread busy",
			zap.String("thread", threadID),
			zap.String("run_id", runID),
			zap.String("active_run", active),
		)
		return ErrThreadBusy
	}
	r.threads[threadID] = runID
	return nil
}

// ReleaseThread 释放失败/取消/完成的会话运行锁。
func (r *Runtime) ReleaseThread(threadID, runID string) {
	if r != nil {
		r.releaseThread(threadID, runID)
	}
}

func (r *Runtime) releaseThread(threadID, runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.threads[threadID] == runID {
		delete(r.threads, threadID)
	}
}

// IsRunActive 报告 run 是否仍在本进程内执行（持有会话锁）。
// 会话锁覆盖「落库前 Claim → StartRun → 收尾 Release」全过程，交互与定时任务两条路径都会登记。
func (r *Runtime) IsRunActive(runID string) bool {
	if r == nil || runID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range r.threads {
		if id == runID {
			return true
		}
	}
	_, ok := r.cancels[runID]
	return ok
}

// CancelRun cancels an in-flight run.
func (r *Runtime) CancelRun(runID string) bool {
	if r == nil || runID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cancels[runID]
	if !ok {
		return false
	}
	c()
	return true
}

func (r *Runtime) storeCancel(id string, cancel context.CancelFunc) {
	if r == nil || id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancels == nil {
		r.cancels = map[string]context.CancelFunc{}
	}
	r.cancels[id] = cancel
}

func (r *Runtime) clearCancel(id string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, id)
}

func (r *Runtime) buildSnapshot(ctx context.Context, req RunRequest) string {
	var b strings.Builder
	b.WriteString("Readonly snapshot (truncated, no secrets):\n")
	switch req.Agent.Module {
	case ModuleDatabase:
		if r.DB == nil {
			b.WriteString("- databases: unavailable\n")
			break
		}
		list, err := r.DB.ListDatabases(ctx, req.Principal, req.ProjectID)
		if err != nil {
			b.WriteString("- databases: " + err.Error() + "\n")
			break
		}
		b.WriteString(fmt.Sprintf("- databases (%d):\n", len(list)))
		for i, d := range list {
			if i >= 20 {
				b.WriteString("  …\n")
				break
			}
			b.WriteString(fmt.Sprintf("  - %s %s status=%s\n", d.Name, d.ID, d.Status))
		}
	case ModuleS3:
		if r.Obj == nil {
			b.WriteString("- objects: unavailable\n")
			break
		}
		list, err := r.Obj.ListObjects(ctx, req.ProjectID, "", 20)
		if err != nil {
			b.WriteString("- objects: " + err.Error() + "\n")
			break
		}
		b.WriteString(fmt.Sprintf("- objects (%d shown):\n", len(list)))
		for _, o := range list {
			b.WriteString(fmt.Sprintf("  - %s size=%d\n", o.Key, o.Size))
		}
	case ModuleLogs:
		if r.Logs == nil {
			b.WriteString("- logs: unavailable\n")
			break
		}
		if stats, err := r.Logs.LevelStats(ctx, req.ProjectID); err == nil {
			b.WriteString("- log levels: ")
			for i, s := range stats {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(fmt.Sprintf("%s=%d", s.Level, s.Count))
			}
			b.WriteString("\n")
		}
	default:
		b.WriteString("- no module snapshot\n")
	}
	return RedactSecrets(b.String())
}

func historyToSchema(hist []Message) []*schema.Message {
	// 从最近的会话向前选取，正文优先，工具摘要在剩余预算中插入。
	selected := make([]Message, 0, len(hist))
	budget := maxHistoryChars
	for i := len(hist) - 1; i >= 0; i-- {
		content := truncate(hist[i].Content, 2000)
		if len(content) > budget {
			break
		}
		budget -= len(content)
		selected = append(selected, hist[i])
	}
	out := make([]*schema.Message, 0, len(selected)*2)
	for i := len(selected) - 1; i >= 0; i-- {
		m := selected[i]
		content := truncate(m.Content, 2000)
		switch m.Role {
		case "assistant":
			if summary := toolHistorySummary(m.ToolCallsJSON); summary != "" && len(summary) <= budget {
				out = append(out, schema.UserMessage(summary))
				budget -= len(summary)
			}
			out = append(out, schema.AssistantMessage(content, nil))
		case "tool":
			out = append(out, schema.UserMessage("TOOL_RESULT\n"+content))
		case "system":
			out = append(out, schema.SystemMessage(content))
		default:
			out = append(out, schema.UserMessage(content))
		}
	}
	return out
}

func toolHistorySummary(raw string) string {
	if raw == "" || raw == "[]" {
		return ""
	}
	var cards []struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		Content   string `json:"content"`
	}
	if json.Unmarshal([]byte(raw), &cards) != nil || len(cards) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("TOOL_HISTORY\n")
	for i, card := range cards {
		if i >= 5 {
			break
		}
		b.WriteString("- " + truncate(card.Name+" "+card.Arguments+" → "+RedactSecrets(card.Content), 600) + "\n")
	}
	return b.String()
}

// 事件消费与增量合并委托 agentcore（与本地 CLI 共用）：见 agentcore.ConsumeVariant /
// ConsumeAssistant / IsToolDelta / MergeStreamToolCall。gocloudagent 只负责把 ConsumeEvent
// 映射成带 run_id 的 SSE Event。
func consumeVariant(mv *adk.MessageVariant) (content, name string) {
	return agentcore.ConsumeVariant(mv)
}

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
