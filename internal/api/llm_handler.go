// llm_handler.go 实现 LLM Gateway 的 HTTP handler（plan8.md）。
//
// 路由：
//   POST /v1/projects/:projectID/llm/chat
//   POST /v1/projects/:projectID/llm/stream  (SSE)
//   GET  /v1/projects/:projectID/llm/providers
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

// LLMHandler 依赖 LLMService、UsageService、AuditService。
type LLMHandler struct {
	svc   LLMService
	usage UsageService
	audit AuditService
}

type llmChatRequest struct {
	Model       string           `json:"model,omitempty"`
	Messages    []llmMessageDTO `json:"messages"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
}

type llmMessageDTO struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type llmChatResponse struct {
	Content      string         `json:"content"`
	Usage        LLMTokenUsage  `json:"usage"`
	Model        string         `json:"model"`
	Provider     string         `json:"provider"`
	FinishReason string         `json:"finish_reason,omitempty"`
}

// Chat 处理非流式对话。
func (h *LLMHandler) Chat(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	var req llmChatRequest
	if err := c.Bind(&req); err != nil {
		return WriteError(c, err)
	}
	if len(req.Messages) == 0 {
		return WriteError(c, errors.New("messages required"))
	}
	if h.usage != nil {
		if err := h.usage.CheckQuota(c.Request().Context(), pc.ID, "llm"); err != nil {
			return WriteError(c, err)
		}
	}
	msgs := make([]LLMMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = LLMMessage{Role: m.Role, Content: m.Content}
	}
	resp, err := h.svc.Chat(c.Request().Context(), pc.ID, LLMRequest{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	})
	if err != nil {
		return WriteError(c, err)
	}
	h.recordAudit(c, pc.ID, "llm_chat", "ok")
	return c.JSON(http.StatusOK, llmChatResponse{
		Content:      resp.Content,
		Usage:        resp.Usage,
		Model:        resp.Model,
		Provider:     resp.Provider,
		FinishReason: resp.FinishReason,
	})
}

// Stream 处理流式对话，使用 SSE 转发。
func (h *LLMHandler) Stream(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	var req llmChatRequest
	if err := c.Bind(&req); err != nil {
		return WriteError(c, err)
	}
	if len(req.Messages) == 0 {
		return WriteError(c, errors.New("messages required"))
	}
	if h.usage != nil {
		if err := h.usage.CheckQuota(c.Request().Context(), pc.ID, "llm"); err != nil {
			return WriteError(c, err)
		}
	}
	msgs := make([]LLMMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = LLMMessage{Role: m.Role, Content: m.Content}
	}
	reader, err := h.svc.Stream(c.Request().Context(), pc.ID, LLMRequest{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	})
	if err != nil {
		return WriteError(c, err)
	}
	defer reader.Close()

	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().WriteHeader(http.StatusOK)
	flusher, _ := c.Response().Writer.(http.Flusher)
	for {
		chunk, err := reader.Next()
		if err != nil {
			end := map[string]any{"type": "end"}
			data, _ := json.Marshal(end)
			_, _ = c.Response().Write([]byte("data: " + string(data) + "\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
			break
		}
		data, _ := json.Marshal(chunk)
		_, _ = c.Response().Write([]byte("data: " + string(data) + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}
	return nil
}

// ListProviders 返回可用供应商名。
func (h *LLMHandler) ListProviders(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, errors.New("project context missing"))
	}
	names, err := h.svc.ListProviders(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"providers": names})
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