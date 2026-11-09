// 文件说明：errors.go — gocloudagent 中 ErrThreadBusy、RunError 等符号的实现
//
// 用法
//
//	(e *RunError) Error() string
//	ClassifyError(err error) *RunError  // ClassifyError 把运行内部错误归一化为可安全展示的分类
//	同包调用：Error() → cli_runner.go, identity.go 等；ErrThreadBusy → runtime.go；RunError → runtime.go
//
// 调用架构
//
//	[上游] gocloudagent/cmd/gocloudagent, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/errors.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access.go, cli_entry.go, cli_runner.go, cli_tool.go, identity.go 等 11 个
//	[关键引用] Error() ← cli_runner.go, identity.go, runtime.go
//
// 主要符号
//
//	var ErrThreadBusy
//	type RunError struct {…}
//	(e *RunError) Error() string
//	ClassifyError(err error) *RunError

package gocloudagent

import (
	"context"
	"errors"
	"net"

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

// ClassifyError 把运行内部错误归一化为可安全展示的分类。
//
// 只处理模块自有语义：调用链上游的 llmgateway 哨兵由宿主的
// internal/agentbridge.ClassifyError 先行匹配，再把剩余错误交给这里。
// 注意：本函数只在最终出口调用（HTTP/SSE/任务落库）。agentcore 的
// native→text 回退与 stream→chat 退化依赖原始错误类型与文案，
// 在运行内部提前归一化会让回退失效。
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
