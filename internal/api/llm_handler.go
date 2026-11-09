// 文件说明：llm_handler.go — LLM Gateway 的 OpenAI 兼容 HTTP handler
//
// 用法
//
//	(h *LLMHandler) ChatCompletions(c echo.Context) error  // ChatCompletions 处理 POST .../v1/chat/completions
//	(h *LLMHandler) ListModels(c echo.Context) error  // ListModels 处理 GET .../v1/models，返回严格 OpenAI list
//	同包调用：stream() → agent_handler.go, e2b_envd_handler.go 等；recordAudit() → cronjob_handler.go, gofunction_handler.go 等；LLMHandler → oai_responses.go, router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/llm_handler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/systemdb
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] stream() ← agent_handler.go, e2b_envd_handler.go, e2b_process_handler.go
//
// 主要符号
//
//	type LLMHandler struct {…}
//	(h *LLMHandler) ChatCompletions(c echo.Context) error
//	(h *LLMHandler) ListModels(c echo.Context) error
//	... 另有 4 个顶层符号

package api

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// LLMHandler 依赖 LLMService、UsageService、AuditService。
type LLMHandler struct {
	svc   LLMService
	usage UsageService
	audit AuditService
	store *systemdb.Store
}

// ChatCompletions 处理 POST .../v1/chat/completions。
// 非流式返回 chat.completion；stream:true 时返回 SSE 并以 [DONE] 收尾。
func (h *LLMHandler) ChatCompletions(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return writeOAIError(c, http.StatusInternalServerError, "internal_error", "project context missing")
	}
	var req oaiChatRequest
	if err := c.Bind(&req); err != nil {
		return writeOAIError(c, http.StatusBadRequest, "invalid_request", "invalid json body")
	}
	if status, code, msg := req.validate(); status != 0 {
		return writeOAIError(c, status, code, msg)
	}
	if h.usage != nil {
		if err := h.usage.CheckQuota(c.Request().Context(), pc.ID, "llm"); err != nil {
			return writeOAIErrorFrom(c, err)
		}
	}
	lreq := oaiToLLMRequest(req)
	if req.Stream {
		return h.stream(c, pc.ID, req, lreq)
	}
	return h.complete(c, pc.ID, req, lreq)
}

// complete 处理非流式调用。
func (h *LLMHandler) complete(c echo.Context, projectID string, req oaiChatRequest, lreq LLMRequest) error {
	resp, err := h.svc.Chat(c.Request().Context(), projectID, lreq)
	if err != nil {
		return writeOAIErrorFrom(c, err)
	}
	h.recordAudit(c, projectID, "llm_chat_completions", "ok")
	h.persistChat(c, projectID, req, resp)
	return c.JSON(http.StatusOK, newOAIResponse(resp))
}

// stream 处理流式调用，逐帧转成 OpenAI chat.completion.chunk。
//
// 网关块 → OpenAI 帧的映射：
//   - Type=content        → delta.content
//   - Type=tool_call_delta → delta.tool_calls[index]（id/name 仅首个分片出现）
//   - FinishReason 非空   → 记下，流结束时统一发 finish 帧
//   - Usage 非空          → 暂存，按 include_usage 决定是否发尾帧
//   - Done                → 结束
//
// 其余块类型（reasoning 等）不属于 OpenAI 标准帧，直接丢弃，绝不在流中插入非标准帧。
func (h *LLMHandler) stream(c echo.Context, projectID string, req oaiChatRequest, lreq LLMRequest) error {
	// 先解除写超时，再等上游。WriteTimeout 从读完请求头起算；若等模型首字节期间
	// 还挂着默认 30s，连接会在响应头写出之前被切断。此时还没写 body，失败仍可返回 JSON 错误。
	beginSSE(c)
	reader, err := h.svc.Stream(c.Request().Context(), projectID, lreq)
	if err != nil {
		// 此时尚未写任何字节，可以返回正常的 HTTP 错误。
		return writeOAIErrorFrom(c, err)
	}
	defer reader.Close()

	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	includeUsage := req.StreamOptions != nil && req.StreamOptions.IncludeUsage
	w := newOAIStreamWriter(c, req.Model, includeUsage)

	finishReason := ""
	for {
		chunk, rerr := reader.Next()
		if rerr != nil {
			// io.EOF 是正常结束；其他错误发生在已写出响应头之后，只能终止流。
			// 不发明错误帧（会破坏严格客户端的解析），客户端凭缺失 [DONE] 判定异常。
			if !errors.Is(rerr, io.EOF) {
				return nil
			}
			break
		}
		if chunk == nil {
			continue
		}
		if chunk.Usage != nil {
			w.setUsage(*chunk.Usage)
		}
		switch chunk.Type {
		case "content":
			if err := w.content(chunk.Content); err != nil {
				return nil // 客户端断开
			}
		case "tool_call_delta":
			if err := w.toolCall(chunk.ToolCallIndex, chunk.ToolCallID, chunk.ToolCallName, chunk.ToolCallArgs); err != nil {
				return nil
			}
		}
		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}
		if chunk.Done {
			break
		}
	}
	if err := w.finish(finishReason); err != nil {
		return nil
	}
	h.recordAudit(c, projectID, "llm_chat_completions", "ok")
	return nil
}

// ListModels 处理 GET .../v1/models，返回严格 OpenAI list。
func (h *LLMHandler) ListModels(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return writeOAIError(c, http.StatusInternalServerError, "internal_error", "project context missing")
	}
	models, err := h.svc.ProviderModels(c.Request().Context(), pc.ID)
	if err != nil {
		return writeOAIErrorFrom(c, err)
	}
	return c.JSON(http.StatusOK, newOAIModelList(models))
}

func (h *LLMHandler) recordAudit(c echo.Context, projectID, kind, status string) {
	if h.audit == nil {
		return
	}
	principal, ok := PrincipalFromContext(c.Request().Context())
	pID := ""
	if ok {
		pID = string(principal.APIKeyID)
	}
	_ = h.audit.Record(context.Background(), AuditEvent{
		ProjectID:   projectID,
		PrincipalID: pID,
		Kind:        kind,
		RequestID:   c.Response().Header().Get(echo.HeaderXRequestID),
		Status:      status,
	})
}

// persistChat 把一次非流式对话落到系统库（sys_llm_sessions / sys_llm_messages）。
//
// OpenAI 协议没有 session_id，且实测没有任何客户端发送过它，因此每次请求新建一条
// 会话记录；旧实现里按 session_id 复用的分支是死代码，已随旧端点一并删除。
// 只落最后一条请求消息与最终 assistant 正文，不存工具参数。
func (h *LLMHandler) persistChat(c echo.Context, projectID string, req oaiChatRequest, resp LLMResponse) {
	if h.store == nil {
		return
	}
	ctx := context.Background()
	createdBy := ""
	if p, ok := PrincipalFromContext(c.Request().Context()); ok {
		createdBy = p.APIKeyID
	}
	sess, err := h.store.CreateLLMSession(ctx, systemdb.LLMSession{
		ProjectID: projectID, Provider: resp.Provider, Model: resp.Model, CreatedBy: createdBy,
	})
	if err != nil {
		return
	}
	rid := RequestIDFromContext(c.Request().Context())
	if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1]
		_, _ = h.store.AppendLLMMessage(ctx, systemdb.LLMMessage{
			SessionID: sess.ID, ProjectID: projectID, Role: last.Role, Content: last.Content, RequestID: rid,
			TokenInput: int64(resp.Usage.PromptTokens),
		})
	}
	_, _ = h.store.AppendLLMMessage(ctx, systemdb.LLMMessage{
		SessionID: sess.ID, ProjectID: projectID, Role: "assistant", Content: resp.Content, RequestID: rid,
		TokenOutput: int64(resp.Usage.CompletionTokens),
	})
}
