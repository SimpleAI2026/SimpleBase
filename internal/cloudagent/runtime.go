package cloudagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// Event is a streaming run event for SSE/NDJSON.
type Event struct {
	Type             string `json:"type"`
	Content          string `json:"content,omitempty"`
	Name             string `json:"name,omitempty"`
	Arguments        string `json:"arguments,omitempty"`
	CallID           string `json:"call_id,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	Message          string `json:"message,omitempty"`
	Code             string `json:"code,omitempty"`
	Reason           string `json:"reason,omitempty"`
	ElapsedMS        int64  `json:"elapsed_ms,omitempty"`
	DurationMS       int64  `json:"duration_ms,omitempty"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	ReasoningTokens  int    `json:"reasoning_tokens,omitempty"`
	ToolCalls        int    `json:"tool_calls,omitempty"`
}

// RunRequest starts a single-agent turn.
type RunRequest struct {
	ProjectID string
	Principal auth.Principal
	Agent     systemdb.CloudAgent
	ThreadID  string
	RunID     string
	UserText  string
	History   []systemdb.AgentMessage
	Stream    bool
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

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	threads map[string]string
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
			modelName = st.DefaultModel
		}
	}

	snapshot := r.buildSnapshot(ctx, req)
	instruction := AssembleInstruction(PromptParts{
		ModuleTemplate: ModuleTemplate(req.Agent.Module),
		AgentPrompt:    req.Agent.SystemPrompt,
		ProjectEnv:     fmt.Sprintf("Project env:\n- project_id: %s\n- agent: %s (%s)\n- module: %s", req.ProjectID, req.Agent.Name, req.Agent.ID, req.Agent.Module),
		Snapshot:       snapshot,
	})

	tools, err := buildTools(req.Agent.ToolIDs, toolDeps{DB: r.DB, Obj: r.Obj, Logs: r.Logs, Sandbox: r.Sandbox})
	if err != nil {
		return RunResult{}, err
	}

	chatModel := newGatewayChatModel(r.LLM, req.ProjectID, modelName)
	if r.ToolProtocol != "" {
		chatModel.protocol = r.ToolProtocol
	}
	var usage ChatUsage
	chatModel.usage = func(u ChatUsage) {
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

	runCtx = withRunContext(runCtx, RunContext{ProjectID: req.ProjectID, Principal: req.Principal, ThreadID: req.ThreadID})

	runner := adk.NewRunner(runCtx, adk.RunnerConfig{Agent: agent, EnableStreaming: req.Stream})
	msgs := historyToSchema(req.History)
	msgs = append(msgs, schema.UserMessage(truncate(req.UserText, maxUserChars)))
	iter := runner.Run(runCtx, msgs)

	var assistant strings.Builder
	var toolCards []map[string]any
	calls := map[string]map[string]any{}
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
			name = firstNonEmpty(mv.ToolName, name)
			id := ""
			if mv.Message != nil {
				id = mv.Message.ToolCallID
			}
			card := calls[id]
			if card == nil {
				card = map[string]any{"call_id": id, "name": name}
				toolCards = append(toolCards, card)
			}
			card["content"] = content
			if emit != nil {
				emit(Event{Type: "tool_result", CallID: id, Name: name, Content: truncate(content, 2000)})
			}
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
					toolCards = append(toolCards, card)
					if emit != nil {
						emit(Event{Type: "tool_call", CallID: id, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
					}
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
		return result, nil
	}
	if errors.Is(runErr, context.Canceled) || errors.Is(runCtx.Err(), context.Canceled) {
		result.Reason = "canceled"
		return result, context.Canceled
	}
	if errors.Is(runErr, context.DeadlineExceeded) || errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return result, context.DeadlineExceeded
	}
	if runErr != nil {
		return result, runErr
	}
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

func historyToSchema(hist []systemdb.AgentMessage) []*schema.Message {
	// 从最近的会话向前选取，正文优先，工具摘要在剩余预算中插入。
	selected := make([]systemdb.AgentMessage, 0, len(hist))
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

func consumeVariant(mv *adk.MessageVariant) (content, name string) {
	if mv == nil {
		return "", ""
	}
	if mv.IsStreaming && mv.MessageStream != nil {
		for {
			chunk, err := mv.MessageStream.Recv()
			if errors.Is(err, io.EOF) || err != nil {
				break
			}
			if chunk != nil {
				content += chunk.Content
				if chunk.ToolName != "" {
					name = chunk.ToolName
				}
			}
		}
		return content, name
	}
	if mv.Message != nil {
		return mv.Message.Content, mv.Message.ToolName
	}
	return "", mv.ToolName
}

func consumeAssistant(mv *adk.MessageVariant, emit func(Event), acc *strings.Builder) (*schema.Message, error) {
	if mv == nil {
		return nil, nil
	}
	if mv.IsStreaming && mv.MessageStream != nil {
		var last *schema.Message
		for {
			chunk, err := mv.MessageStream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return last, err
			}
			if chunk == nil {
				continue
			}
			last = chunk
			if chunk.Content != "" && len(chunk.ToolCalls) == 0 {
				acc.WriteString(chunk.Content)
				if emit != nil {
					emit(Event{Type: "token", Content: chunk.Content})
				}
			}
		}
		return last, nil
	}
	if mv.Message != nil {
		if mv.Message.Content != "" && len(mv.Message.ToolCalls) == 0 {
			acc.WriteString(mv.Message.Content)
			if emit != nil {
				emit(Event{Type: "token", Content: mv.Message.Content})
			}
		}
		return mv.Message, nil
	}
	return nil, nil
}

func safeAgentName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "agent"
	}
	return name
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
