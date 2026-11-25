// 文件说明：prompt.go — gocloudagent/cloudagent 中 AssembleInstruction 等符号的实现
//
// 用法
//
//	AssembleInstruction(sections ...string) string  // AssembleInstruction joins non-empty prompt sections in the supplied order.
//	同包调用：truncate() → history.go, mcptool.go
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/prompt.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//	[关键引用] AssembleInstruction ← runtime.go, runtime_test.go；truncate ← history.go, mcptool.go
//
// 主要符号
//
//	const maxHistoryChars / maxUserChars
//	AssembleInstruction(sections ...string) string
//	truncate(s string, max int) string

package cloudagent

import (
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

const (
	maxHistoryChars = 12000
	maxUserChars    = 8000
)

// AssembleInstruction joins non-empty prompt sections in the supplied order.
func AssembleInstruction(sections ...string) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		if section = strings.TrimSpace(section); section != "" {
			parts = append(parts, section)
		}
	}
	return agentcore.RedactSecrets(strings.Join(parts, "\n\n"))
}

func truncate(s string, max int) string { return agentcore.Truncate(s, max) }
