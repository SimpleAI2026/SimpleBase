// llm_handler.go 实现 LLM Gateway 的 OpenAI 兼容 HTTP handler
// （planv6.0 openai-compatible-llm-api-plan）。
//
// 路由：
//
//	POST /v1/projects/:projectID/v1/chat/completions   （stream:true 时为 SSE）
//	GET  /v1/projects/:projectID/v1/models
//
// 路径里的两段 v1 都是刻意保留的，不是笔误：前一段是 SimpleBase 自身的 API 版本前缀，
// 后一段是 OpenAI SDK 约定的资源前缀。只有保留 base_url 尾部这个 /v1，
// 官方 SDK 才能把 base_url 指向 ".../projects/<id>/v1" 并原样拼出 /chat/completions。
// 请勿"顺手修正"成单段 v1（路由测试 TestOpenAIRoutePaths 会锁定这一点）。
//
// 旧的 /llm/chat、/llm/stream、/llm/providers 已删除，不保留兼容期。
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
