// 文件说明：redact.go — gocloudagent/agentcore 中 RedactSecrets、Truncate 等符号的实现
//
// 用法
//
//	RedactSecrets(s string) string  // RedactSecrets 清理文本里的凭据样式子串
//	Truncate(s string, max int) string  // Truncate 按字符（rune）截断并加标记，避免切断多字节字符
//
// 调用架构
//
//	[上游] gocloudagent, gocloudagent/cli
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/agentcore/redact.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] messages.go, model.go, stream.go, types.go
//
// 主要符号
//
//	RedactSecrets(s string) string
//	Truncate(s string, max int) string
//	... 另有 6 个顶层符号

package agentcore

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// redact.go 从 gocloudagent/prompt.go 平移而来（planv6.0 §4.1）：
// 脱敏与截断是「与运行时无关」的纯函数，云端与本地 CLI 共用同一份实现，
// 避免 CLI 另写一套而漏掉某类密钥。
var (
	// secretKeyCore 是敏感键名的公共片段（不含两侧引号）。
	secretKeyCore = `(?:aws_)?(?:secret|access)[_-]?key(?:[_-]?id)?|api[_-]?key|password|credential_ref|authorization`
	// jsonSecretRE 匹配 JSON 形态 "key":"value"，值可含空格（如 "Bearer abc"）。
	jsonSecretRE = regexp.MustCompile(`(?i)"(` + secretKeyCore + `)"\s*:\s*"(?:[^"\\]|\\.)*"`)
	// secretLineRE 匹配 key: value / key=value 的非 JSON 形态（值到空白或分隔符为止）。
	secretLineRE = regexp.MustCompile(`(?i)(` + secretKeyCore + `)"?\s*[:=]\s*"?[^\s,}\]]+`)
	awsKeyRE     = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
	bearerRE     = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-._~+/]+=*`)
	apiTokenRE   = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`)
)

// RedactSecrets 清理文本里的凭据样式子串。提示词、工具结果、落盘内容都必须经过它。
func RedactSecrets(s string) string {
	if s == "" {
		return s
	}
	// JSON 引号值优先整体替换，避免值内含空格时只截掉第一段。
	out := jsonSecretRE.ReplaceAllStringFunc(s, func(m string) string {
		key := m[:strings.Index(m, ":")]
		return key + `:"[REDACTED]"`
	})
	out = secretLineRE.ReplaceAllString(out, "$1=[REDACTED]")
	out = awsKeyRE.ReplaceAllString(out, "[REDACTED_AWS_KEY]")
	out = bearerRE.ReplaceAllString(out, "Bearer [REDACTED]")
	out = apiTokenRE.ReplaceAllString(out, "[REDACTED_API_KEY]")
	return out
}

// Truncate 按字符（rune）截断并加标记，避免切断多字节字符。
func Truncate(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "\n…[truncated]"
}
