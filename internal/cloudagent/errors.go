package cloudagent

import (
	"context"
	"errors"
	"net"

	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/voocel/litellm/providers"
)

// ErrThreadBusy 表示同一会话已有运行中的 Agent。
var ErrThreadBusy = errors.New("cloud agent: thread busy")

// RunError 不包含上游原始错误内容，避免 SSE 与审计泄漏凭据。
type RunError struct {
	Code    string
	Message string
}

func (e *RunError) Error() string { return e.Message }

// ClassifyError 将上游错误归一化为稳定、可安全展示的错误码。
func ClassifyError(err error) *RunError {
	if err == nil {
		return nil
	}
	var re *RunError
	if errors.As(err, &re) {
		return re
	}
	var le *providers.LiteLLMError
	switch {
	case errors.Is(err, ErrThreadBusy):
		return &RunError{Code: "agent_thread_busy", Message: "当前会话仍在运行"}
	case errors.Is(err, llmgateway.ErrModelNotAllowed):
		return &RunError{Code: "llm_model_not_allowed", Message: "模型不在允许列表中"}
	case errors.Is(err, llmgateway.ErrNoProviders), errors.Is(err, llmgateway.ErrProviderNotFound):
		return &RunError{Code: "llm_not_configured", Message: "模型服务未配置"}
	case errors.Is(err, context.DeadlineExceeded):
		return &RunError{Code: "llm_timeout", Message: "模型响应超时"}
	case errors.As(err, &le):
		switch {
		case le.StatusCode == 401 || le.StatusCode == 403 || le.Type == providers.ErrorTypeAuth:
			return &RunError{Code: "llm_auth_failed", Message: "模型服务鉴权失败"}
		case le.StatusCode == 429 || le.Type == providers.ErrorTypeRateLimit:
			return &RunError{Code: "llm_rate_limited", Message: "模型服务限流，请稍后重试"}
		case le.Type == providers.ErrorTypeTimeout:
			return &RunError{Code: "llm_timeout", Message: "模型响应超时"}
		}
	default:
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return &RunError{Code: "llm_timeout", Message: "模型响应超时"}
		}
	}
	return &RunError{Code: "llm_upstream_error", Message: "模型服务暂时不可用"}
}
