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

type tracer = Tracer

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
	return truncate(agentcore.RedactSecrets(s), traceMaxChars)
}

// HashSummary returns a length and SHA-256 prefix without retaining content.
func HashSummary(s string) string {
	if s == "" {
		return "0:-"
	}
	sum := sha256.Sum256([]byte(s))
	return strconv.Itoa(len([]rune(s))) + ":" + hex.EncodeToString(sum[:8])
}

func hashSummary(s string) string { return HashSummary(s) }

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
