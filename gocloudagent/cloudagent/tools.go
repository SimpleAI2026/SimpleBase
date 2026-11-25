// 文件说明：tools.go — gocloudagent/cloudagent 的内部实现（wrapToolErrors、errorTolerantTool 等）
//
// 用法
//
//	同包调用：wrapToolErrors() → runtime.go, runtime_test.go；toolFailure() → mcptool.go, mcptool_test.go
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/tools.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//	[关键引用] wrapToolErrors ← runtime.go, runtime_test.go；toolFailure ← mcptool.go, mcptool_test.go
//
// 主要符号
//
//	wrapToolErrors(tools []tool.BaseTool) []tool.BaseTool
//	type errorTolerantTool struct{…}
//	(w *errorTolerantTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error)
//	toolFailure(code, message string) string

package cloudagent

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/components/tool"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

func wrapToolErrors(tools []tool.BaseTool) []tool.BaseTool {
	out := make([]tool.BaseTool, 0, len(tools))
	for _, t := range tools {
		inv, ok := t.(tool.InvokableTool)
		if !ok {
			out = append(out, t)
			continue
		}
		out = append(out, &errorTolerantTool{InvokableTool: inv})
	}
	return out
}

type errorTolerantTool struct {
	tool.InvokableTool
}

func (w *errorTolerantTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	out, err := w.InvokableTool.InvokableRun(ctx, args, opts...)
	if err == nil {
		return out, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", err
	}
	return agentcore.MarshalToolJSON(map[string]any{"error": err.Error(), "is_error": true})
}

// toolFailure 构造失败信封；message 先脱敏（MCP 错误文本来自外部 Server）。
func toolFailure(code, message string) string {
	return agentcore.ErrorEnvelope(code, agentcore.RedactSecrets(message))
}
