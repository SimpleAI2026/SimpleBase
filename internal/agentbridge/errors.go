// 文件说明：errors.go — internal/agentbridge 中 ClassifyError 等符号的实现
//
// 用法
//
//	ClassifyError(err error) *gocloudagent.RunError  // ClassifyError 是最终出口的统一错误分类
//	同包调用：ClassifyError() → cron_agent.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/errors.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, internal/llmgateway
//	[同包] cli.go, command.go, cron_agent.go, delegation.go, principal.go 等 6 个
//	[关键引用] ClassifyError() ← cron_agent.go
//
// 主要符号
//
//	ClassifyError(err error) *gocloudagent.RunError

package agentbridge

import (
	"errors"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
)

// ClassifyError 是最终出口的统一错误分类。
//
// 顺序有讲究：llmgateway 的哨兵只在宿主侧可见，必须先匹配；剩余错误（供应商
// 错误、上下文、网络超时、线程忙）交给 gocloudagent.ClassifyError。两处都只
// 产出固定文案，不透传上游原文。
//
// 本函数只用于「错误已经终结」的位置：HTTP 响应、SSE 终态、任务落库。运行内部
// （agentcore 的 native→text 回退、stream→chat 退化）必须保留原始错误类型，
// 否则回退判定与「不得重试」判定都会失效。
func ClassifyError(err error) *gocloudagent.RunError {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, llmgateway.ErrModelNotAllowed):
		return &gocloudagent.RunError{Code: "llm_model_not_allowed", Message: "模型不在允许列表中"}
	case errors.Is(err, llmgateway.ErrNoProviders),
		errors.Is(err, llmgateway.ErrProviderNotFound),
		errors.Is(err, llmgateway.ErrModelNotConfigured):
		return &gocloudagent.RunError{Code: "llm_not_configured", Message: "模型服务未配置"}
	}
	return gocloudagent.ClassifyError(err)
}
