// 文件说明：logger.go — SimpleBase 的结构化日志、Prometheus 指标和脱敏工具
//
// 用法
//
//	NewLogger(level, format string, w io.Writer) Logger
//	WriterFor(output string) io.Writer  // WriterFor 将 observability.log_output 映射到进程 writer
//	Sync(l Logger) error  // Sync 刷新底层日志缓冲
//	同包调用：Logger → zaplog.go；Sync() → zaplog.go
//
// 调用架构
//
//	[上游] internal/api, internal/app, internal/audit 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/observability/logger.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] metrics.go, stage_timing_metrics.go, zaplog.go
//	[关键引用] Logger ← zaplog.go
//
// 主要符号
//
//	type Logger = *zapLogger
//	WriterFor(output string) io.Writer
//	NewLogger(level, format string, w io.Writer) Logger
//	Sync(l Logger) error
//	... 另有 1 个顶层符号

package observability

import (
	"io"
	"os"
	"strings"

	"go.uber.org/zap/zapcore"
)

// Logger 是服务层使用的结构化日志类型。禁止调用方直接使用 zap 全局变量。
type Logger = *zapLogger

// WriterFor 将 observability.log_output 映射到进程 writer。未知值回退 stderr。
func WriterFor(output string) io.Writer {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "stdout":
		return os.Stdout
	default:
		return os.Stderr
	}
}

// NewLogger 根据 level/format 构造 Logger。format 仅支持 "json" 或 "console"。
func NewLogger(level, format string, w io.Writer) Logger {
	if w == nil {
		w = os.Stderr
	}
	lvl := parseLevel(level)
	if format == "console" {
		return newZapConsole(w, lvl)
	}
	return newZapLogger(w, lvl)
}

func parseLevel(s string) zapcore.Level {
	switch s {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// Sync 刷新底层日志缓冲。进程退出前应调用。
// 对 stderr/stdout 等不可 sync 的 writer 返回的错误被忽略。
func Sync(l Logger) error {
	if l == nil {
		return nil
	}
	if err := l.Sync(); err != nil {
		// zap 对非文件 writer（如 stderr）会返回 "sync /dev/stderr: bad file descriptor"，
		// 属于预期行为，不向上传播。
		return nil
	}
	return nil
}
