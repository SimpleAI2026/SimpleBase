package agenthttp

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

type mentionDTO struct {
	AgentID string `json:"agent_id"`
}

type createRunBody struct {
	Content  string       `json:"content"`
	Mentions []mentionDTO `json:"mentions"`
	Stream   bool         `json:"stream"`
	// RetryOfRunID 非空表示重试该 run：复用其 user 消息，不重复落库（BUG-03）。
	RetryOfRunID string `json:"retry_of_run_id"`
}

func (h *CloudAgentHandler) CreateRun(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	principal := httpkit.PrincipalOf(c)
	threadID := c.Param("threadID")
	thread, err := h.svc.LookupThread(c.Request().Context(), pc.ID, threadID)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	var body createRunBody
	if err := c.Bind(&body); err != nil {
		return httpkit.WriteError(c, err)
	}
	// 群会话的消息必须经 Plan 栅栏（planv6.8 §13）：/runs 只服务固定助手会话。
	ws, err := h.store.GetAgentThreadWorkspace(c.Request().Context(), pc.ID, threadID)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if ws.DispatchMode == systemdb.DispatchCollaborative {
		return httpkit.WriteError(c, agentrun.ErrTurnsRequired)
	}
	return h.createDirectRun(c, pc, principal, thread, body)
}

// createDirectRun 处理固定助手会话的 CreateRun：校验与落库在 agentrun.StartDirectRun，
// 流式走事件环 + SSE，非流式同步等待结果。
func (h *CloudAgentHandler) createDirectRun(c echo.Context, pc httpkit.ProjectContext, principal auth.Principal, thread systemdb.AgentThread, body createRunBody) error {
	mentions := make([]agentrun.Mention, 0, len(body.Mentions))
	for _, mention := range body.Mentions {
		mentions = append(mentions, agentrun.Mention{AgentID: mention.AgentID})
	}
	d, err := h.svc.StartDirectRun(c.Request().Context(), agentrun.DirectRunInput{
		ProjectID: pc.ID, Thread: thread, Principal: principal,
		Content: body.Content, Mentions: mentions, RetryOfRunID: body.RetryOfRunID, Stream: body.Stream,
	})
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if d.LLMNotConfigured {
		if !body.Stream {
			return httpkit.WriteError(c, &cloudagent.RunError{Code: "llm_not_configured", Message: "模型服务未配置"})
		}
		// SSE 消费端仍收到规范的 error → end 帧；不持久化用户正文。
		return h.streamRun(c, c.Request().Context(), pc.ID, systemdb.AgentRun{ID: uuid.NewString()}, d.Agent, cloudagent.RunRequest{}, true)
	}
	if body.Stream {
		return h.streamRun(c, c.Request().Context(), pc.ID, d.Run, d.Agent, d.Request, false)
	}
	return h.completeRun(c, c.Request().Context(), d)
}

// completeRun 同步执行 run（请求取消即取消 run，D6-b）并返回 run 与 assistant 消息。
func (h *CloudAgentHandler) completeRun(c echo.Context, ctx context.Context, d agentrun.DirectRun) error {
	out := h.svc.ExecuteRun(ctx, d, nil)
	if out.PersistErr != nil {
		return httpkit.WriteError(c, out.PersistErr)
	}
	if out.RunErr != nil {
		return httpkit.WriteError(c, agentbridge.ClassifyError(out.RunErr))
	}
	if out.ReadErr != nil {
		return httpkit.WriteError(c, out.ReadErr)
	}
	var asst messageDTO
	if out.Assistant != nil {
		asst = toMessageDTO(*out.Assistant)
	}
	return c.JSON(http.StatusOK, map[string]any{"run": toRunDTO(out.Run), "message": asst, "agent_id": d.Agent.ID})
}

func (h *CloudAgentHandler) streamRun(c echo.Context, runCtx context.Context, projectID string, run systemdb.AgentRun, agent systemdb.CloudAgent, req cloudagent.RunRequest, llmMissing bool) error {
	w := h.openStream(c, run)
	h.runStream(w, c, runCtx, projectID, run, agent, req, llmMissing)
	return nil
}

// openStream 建立 run 的 SSE 连接并返回写入器；流建立与运行分开，便于调用方在两者之间插入帧。
// 写 deadline 的解除见 beginSSE（planv6.0 v2 §11）。
//
// runID 放进响应头而不只在首个 SSE 帧里下发：连接在 run 帧到达前断开时，
// 客户端否则没有任何 runID 可用于续传（BUG-B5，planv6.0 v2 §2.2）。
func (h *CloudAgentHandler) openStream(c echo.Context, run systemdb.AgentRun) *httpkit.SSEWriter {
	return httpkit.NewSSEWriter(c, "X-Run-Id", run.ID)
}

// runStream 在已建立的 SSE 连接上执行 run。
func (h *CloudAgentHandler) runStream(w *httpkit.SSEWriter, c echo.Context, runCtx context.Context, projectID string, run systemdb.AgentRun, agent systemdb.CloudAgent, req cloudagent.RunRequest, llmMissing bool) {
	write, writeBytes := newRunStreamOutput(c, h.svc, run.ID, w.Flusher)
	write(cloudagent.Event{Type: "run", RunID: run.ID})
	if llmMissing {
		write(cloudagent.Event{Type: "error", Code: "llm_not_configured", Message: "模型服务未配置"})
		write(cloudagent.Event{Type: "end"})
		return
	}
	stopWorkers, trackEvent := startRunStreamWorkers(write, writeBytes)
	defer stopWorkers() // panic / 提前返回路径兜底
	// 运行与 HTTP 连接解耦：前端切换界面 / 刷新只会断开 SSE，run 继续执行，
	// 事件照常写入事件环供 ReplayEvents 续接；真正取消只走 CancelRun
	// （runtime 内部登记的 cancel）、RunTimeout 或服务停机。
	out := h.svc.ExecuteRun(nil, agentrun.DirectRun{Run: run, Agent: agent, Request: req}, func(ev cloudagent.Event) {
		trackEvent(ev)
		write(ev)
	})
	// 正常路径：StartRun 返回后先停后台 goroutine，再写收尾事件。
	stopWorkers()
	if out.PersistErr != nil {
		write(cloudagent.Event{Type: "error", Code: "llm_upstream_error", Message: "保存运行结果失败"})
		write(cloudagent.Event{Type: "end"})
		return
	}
	if out.RunErr != nil && !errors.Is(out.RunErr, context.Canceled) {
		re := agentbridge.ClassifyError(out.RunErr)
		write(cloudagent.Event{Type: "error", Code: re.Code, Message: re.Message})
	}
	res := out.Result
	write(cloudagent.Event{Type: "usage", DurationMS: res.DurationMS, PromptTokens: res.PromptTokens,
		CompletionTokens: res.CompletionTokens, ReasoningTokens: res.ReasoningTokens, ToolCalls: res.ToolCalls})
	if errors.Is(out.RunErr, context.Canceled) {
		res.Reason = "canceled"
	}
	write(cloudagent.Event{Type: "end", Reason: res.Reason})
}

// newRunStreamOutput 返回共享同一把输出锁的事件帧与原始字节写入函数。
// 事件 ID 分配、事件缓存写入与 SSE 输出必须在锁内完成。
func newRunStreamOutput(c echo.Context, svc *agentrun.Service, runID string, flusher http.Flusher) (func(cloudagent.Event), func([]byte)) {
	var mu sync.Mutex
	var seq int64
	write := func(ev cloudagent.Event) {
		if ev.RunID == "" {
			ev.RunID = runID
		}
		mu.Lock()
		defer mu.Unlock()
		var id int64
		if svc != nil {
			id = svc.AppendEvent(runID, ev)
		} else {
			seq++
			id = seq
		}
		httpkit.WriteSSEFrame(c.Response().Writer, flusher, id, ev)
	}
	writeBytes := func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = c.Response().Write(data)
		if flusher != nil {
			flusher.Flush()
		}
	}
	return write, writeBytes
}

// startRunStreamWorkers 启动 heartbeat 与 thinking 提示，并返回停止函数和事件进度跟踪函数。
func startRunStreamWorkers(write func(cloudagent.Event), writeBytes func([]byte)) (func(), func(cloudagent.Event)) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	var stopOnce sync.Once
	stopHeartbeat := make(chan struct{})
	stopThinking := make(chan struct{})
	stop := func() {
		stopOnce.Do(func() {
			close(stopHeartbeat)
			close(stopThinking)
			wg.Wait()
		})
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-ticker.C:
				writeBytes([]byte(": ping\n\n"))
			}
		}
	}()
	started := time.Now()
	lastOutput := time.Now()
	toolStarted := map[string]time.Time{}
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopThinking:
				return
			case <-ticker.C:
				mu.Lock()
				idle := time.Since(lastOutput)
				var activeID string
				var activeSince time.Time
				for id, since := range toolStarted {
					activeID, activeSince = id, since
					break
				}
				mu.Unlock()
				if activeID != "" {
					write(cloudagent.Event{Type: "tool_progress", CallID: activeID, ElapsedMS: time.Since(activeSince).Milliseconds()})
				} else if idle >= 2*time.Second {
					write(cloudagent.Event{Type: "thinking", ElapsedMS: time.Since(started).Milliseconds()})
				}
			}
		}
	}()
	track := func(ev cloudagent.Event) {
		switch ev.Type {
		case "token", "tool_call", "tool_call_delta", "tool_start", "tool_result", "confirmation_required", "confirmation_resolved":
			mu.Lock()
			lastOutput = time.Now()
			if (ev.Type == "tool_call" || ev.Type == "tool_start") && ev.CallID != "" {
				toolStarted[ev.CallID] = lastOutput
			} else if ev.Type == "tool_result" {
				delete(toolStarted, ev.CallID)
			}
			mu.Unlock()
		}
	}
	return stop, track
}
