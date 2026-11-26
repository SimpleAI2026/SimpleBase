// 文件说明：trace.go — gocloudagent/cloudagent 中 Tracer、NewTracer 等符号的实现
//
// 用法
//
//	type Tracer struct{…}  // Tracer emits metadata by default and redacted payloads only when enabled.
//	NewTracer(logger Logger, payload bool) Tracer  // NewTracer constructs a tracer for a provider or runtime.
//	HashSummary(s string) string  // HashSummary returns a length and SHA-256 prefix without retaining content.
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/trace.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//	[关键引用] Tracer ← mcptool.go, runtime.go；HashSummary ← mcptool.go, runtime.go
//
// 主要符号
//
//	const traceMaxChars
//	type Tracer struct{…}
//	NewTracer(logger Logger, payload bool) Tracer
//	(r *Runtime) trace() Tracer
//	(t Tracer) enabled() bool
//	(t Tracer) Meta(msg string, fields ...zap.Field)
//	(t Tracer) Warn(msg string, fields ...zap.Field)
//	(t Tracer) Payload(msg, label, content string)
//	... 另有 3 个顶层符号

package cloudagent

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

const traceMaxChars = 2000

// Tracer emits metadata by default and redacted payloads only when enabled.
type Tracer struct {
	logger  Logger
	payload bool
}

// NewTracer constructs a tracer for a provider or runtime.
func NewTracer(logger Logger, payload bool) Tracer {
	return Tracer{logger: logger, payload: payload}
}

func (r *Runtime) trace() Tracer {
	if r == nil {
		return Tracer{}
	}
	return NewTracer(r.Logger, r.TraceLog)
}

func (t Tracer) enabled() bool { return t.logger != nil }

func (t Tracer) Meta(msg string, fields ...zap.Field) {
	if t.enabled() {
		t.logger.Info(msg, fields...)
	}
}

func (t Tracer) Warn(msg string, fields ...zap.Field) {
	if t.enabled() {
		t.logger.Warn(msg, fields...)
	}
}

func (t Tracer) Payload(msg, label, content string) {
	if t.enabled() && t.payload {
		t.logger.Debug(msg, zap.String(label, t.Redact(content)))
	}
}

func (t Tracer) Redact(s string) string {
	return agentcore.Truncate(agentcore.RedactSecrets(s), traceMaxChars)
}

// HashSummary returns a length and SHA-256 prefix without retaining content.
func HashSummary(s string) string {
	if s == "" {
		return "0:-"
	}
	sum := sha256.Sum256([]byte(s))
	return strconv.Itoa(len([]rune(s))) + ":" + hex.EncodeToString(sum[:8])
}

func (t Tracer) finish(req RunRequest, res RunResult) {
	if !t.enabled() {
		return
	}
	t.Meta("gocloudagent: run finish",
		zap.String("run_id", req.RunID),
		zap.String("reason", res.Reason),
		zap.Int64("duration_ms", res.DurationMS),
		zap.Int("prompt_tokens", res.PromptTokens),
		zap.Int("completion_tokens", res.CompletionTokens),
		zap.Int("reasoning_tokens", res.ReasoningTokens),
		zap.Int("tool_calls", res.ToolCalls),
		zap.String("assistant", HashSummary(res.Content)),
	)
	t.Payload("gocloudagent: assistant output", "content", res.Content)
}
