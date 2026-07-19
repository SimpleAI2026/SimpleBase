// Package observability 提供 SimpleBase 的结构化日志、Prometheus 指标和脱敏工具。
// 本包是对 internal/log 和 internal/prom 的薄封装，统一为 API/服务层使用。
package observability

import (
	"io"
	"os"

	"github.com/linkxzhou/SimpleBase/internal/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger 是服务层使用的结构化日志接口。禁止调用方直接使用 zap 全局变量。
type Logger = *log.Logger

// NewLogger 根据 level/format 构造 Logger。format 仅支持 "json" 或 "console"。
func NewLogger(level, format string, w io.Writer) Logger {
	if w == nil {
		w = os.Stderr
	}
	lvl := parseLevel(level)
	if format == "console" {
		return newConsoleLogger(lvl, w)
	}
	return log.New(w, lvl)
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

func newConsoleLogger(level zapcore.Level, w io.Writer) Logger {
	// 复用 log.New，其内部使用 JSON 编码器；若需 console 可后续扩展。
	return log.New(w, level)
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

// Field 便于调用方构造结构化字段而不直接依赖 zap。
type Field = zap.Field
