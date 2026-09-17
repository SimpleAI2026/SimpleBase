package cloudagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// Event is a streaming run event for SSE/NDJSON.
type Event struct {
	Type      string `json:"type"`
	Content   string `json:"content,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Message   string `json:"message,omitempty"`
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
	Content       string
	ToolCallsJSON string
}

// Runtime executes Cloud Agents via eino ChatModelAgent.
type Runtime struct {
	LLM      ChatClient
	DB       DatabaseAccess
	Obj      ObjectAccess
	Logs     LogAccess
	Settings SettingsAccess

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

// StartRun executes the agent. emit is optional (streaming).
func (r *Runtime) StartRun(ctx context.Context, req RunRequest, emit func(Event)) (RunResult, error) {
	if r == nil || r.LLM == nil {
		return RunResult{}, fmt.Errorf("cloud agent runtime is not configured")
	}
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

	tools, err := buildTools(req.Agent.ToolIDs, toolDeps{DB: r.DB, Obj: r.Obj, Logs: r.Logs})
	if err != nil {
		return RunResult{}, err
	}

	chatModel := newGatewayChatModel(r.LLM, req.ProjectID, modelName)
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        safeAgentName(req.Agent.Name),
		Description: req.Agent.Description,
		Instruction: instruction,
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools},
		},
		MaxIterations: 8,
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("create chat model agent: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	r.storeCancel(req.RunID, cancel)
	defer r.clearCancel(req.RunID)
	defer cancel()

	runCtx = withRunContext(runCtx, RunContext{ProjectID: req.ProjectID, Principal: req.Principal})

	runner := adk.NewRunner(runCtx, adk.RunnerConfig{Agent: agent, EnableStreaming: req.Stream})
	msgs := historyToSchema(req.History)
	msgs = append(msgs, schema.UserMessage(truncate(req.UserText, maxUserChars)))
	iter := runner.Run(runCtx, msgs)

	var assistant strings.Builder
	var toolCards []map[string]string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			if errors.Is(event.Err, context.Canceled) {
				return RunResult{Content: assistant.String()}, context.Canceled
			}
			return RunResult{Content: assistant.String()}, event.Err
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		mv := event.Output.MessageOutput
		if mv.Role == schema.Tool {
			content, name := consumeVariant(mv)
			toolCards = append(toolCards, map[string]string{"name": firstNonEmpty(mv.ToolName, name), "content": content})
			if emit != nil {
				emit(Event{Type: "tool_result", Name: firstNonEmpty(mv.ToolName, name), Content: truncate(content, 2000)})
			}
			continue
		}
		if mv.Role == schema.Assistant || mv.Role == "" {
			msg, _ := consumeAssistant(mv, emit, &assistant)
			if msg != nil && len(msg.ToolCalls) > 0 && emit != nil {
				for _, tc := range msg.ToolCalls {
					emit(Event{Type: "tool_call", Name: tc.Function.Name, Arguments: tc.Function.Arguments})
				}
			}
		}
	}

	toolJSON := "[]"
	if len(toolCards) > 0 {
		b, _ := json.Marshal(toolCards)
		toolJSON = string(b)
	}
	return RunResult{Content: assistant.String(), ToolCallsJSON: toolJSON}, nil
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
	out := make([]*schema.Message, 0, len(hist))
	n := 0
	for _, m := range hist {
		if n > maxHistoryChars {
			break
		}
		content := truncate(m.Content, 2000)
		n += len(content)
		switch m.Role {
		case "assistant":
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
			if chunk.Content != "" && len(chunk.ToolCalls) == 0 && !looksLikeToolCall(chunk.Content) {
				acc.WriteString(chunk.Content)
				if emit != nil {
					emit(Event{Type: "token", Content: chunk.Content})
				}
			}
			if len(chunk.ToolCalls) > 0 && emit != nil {
				for _, tc := range chunk.ToolCalls {
					emit(Event{Type: "tool_call", Name: tc.Function.Name, Arguments: tc.Function.Arguments})
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
