package cloudagent

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// traceMaxChars 是内容层日志的截断上限（planv6.0 v2 §5.3）。
const traceMaxChars = 2000

// tracer 集中云助手的运行日志（planv6.0 v2 §5）。
//
// 两级输出：
//   - 元数据层（Meta/Warn）：只有标识、长度与 hash，不落正文，默认可用；
//   - 内容层（Payload）：仅在开关打开时输出，且先 RedactSecrets 再截断。
//
// 内容层是 internal/AGENTS.md「不得在日志/审计中输出 SQL 参数或 LLM 正文」
// 的显式例外，边界见该文件与 planv6.0 v2 §5.2。禁止写入系统库 logs 表：
// 助手自身的 search_logs 工具会把它读回 LLM 上下文，造成跨用户泄漏。
type tracer struct {
	logger observability.Logger
	// payload 为真时才输出内容层（agent.trace_log）。
	payload bool
}

// trace 返回本 Runtime 的 tracer。logger 为 nil 时返回零值 tracer（全部为 no-op）。
func (r *Runtime) trace() tracer {
	if r == nil {
		return tracer{}
	}
	return tracer{logger: r.Logger, payload: r.TraceLog}
}

// enabled 报告是否可以写日志。
func (t tracer) enabled() bool { return t.logger != nil }

// Meta 输出元数据层日志（info）。正文必须已经在调用点转成摘要。
func (t tracer) Meta(msg string, fields ...zap.Field) {
	if !t.enabled() {
		return
	}
	t.logger.Info(msg, fields...)
}

// Warn 输出元数据层告警（warn），用于降级与退化路径。
func (t tracer) Warn(msg string, fields ...zap.Field) {
	if !t.enabled() {
		return
	}
	t.logger.Warn(msg, fields...)
}

// Payload 输出内容层日志（debug）：开关关闭时直接返回，不构造字段。
// 正文先 RedactSecrets 再截断，调用点不要重复处理。
func (t tracer) Payload(msg, label, content string) {
	if !t.enabled() || !t.payload {
		return
	}
	t.logger.Debug(msg, zap.String(label, t.Redact(content)))
}

// Redact 是内容层的统一处理入口：先脱敏再按 traceMaxChars 截断。
func (t tracer) Redact(s string) string {
	return truncate(RedactSecrets(s), traceMaxChars)
}

// hashSummary 生成「长度 + sha256 前缀」摘要，用于不落正文的元数据层。
// 空串返回 "0:-"。
func hashSummary(s string) string {
	if s == "" {
		return "0:-"
	}
	sum := sha256.Sum256([]byte(s))
	return itoa(len([]rune(s))) + ":" + hex.EncodeToString(sum[:8])
}

// finish 记录一轮 run 的收尾摘要（元数据层 + 内容层正文汇总）。
func (t tracer) finish(req RunRequest, res RunResult) {
	if !t.enabled() {
		return
	}
	t.Meta("cloudagent: run finish",
		zap.String("run_id", req.RunID),
		zap.String("reason", res.Reason),
		zap.Int64("duration_ms", res.DurationMS),
		zap.Int("prompt_tokens", res.PromptTokens),
		zap.Int("completion_tokens", res.CompletionTokens),
		zap.Int("reasoning_tokens", res.ReasoningTokens),
		zap.Int("tool_calls", res.ToolCalls),
		zap.String("assistant", hashSummary(res.Content)),
	)
	// 内容层：整轮汇总一次，不逐 token 输出。
	t.Payload("cloudagent: assistant output", "content", res.Content)
}

// itoa 避免为单个整数引入 strconv 依赖噪音。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// argvSummary 生成命令行摘要：只留资源与动作（前两段），不落参数值。
func argvSummary(argv []string) string {
	switch len(argv) {
	case 0:
		return ""
	case 1:
		return argv[0]
	default:
		return argv[0] + " " + argv[1]
	}
}

// argvRedacted 返回可用于内容层的 argv 文本（脱敏后拼接）。
func argvRedacted(argv []string) string {
	return strings.Join(redactArgv(argv), " ")
}
