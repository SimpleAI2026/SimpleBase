// 文件说明：zaplog_test.go — internal/observability 的单元测试（TestDefaultLogger、TestNewLogger 等）
//
// 用法
//
//	go test ./internal/observability -run 'TestDefaultLogger|TestNewLogger|TestNewConsoleLogger|TestLogLevels'
//	同包调用：syncDefault() → logger_edge_test.go
//
// 调用架构
//
//	[上游] internal/alerts, internal/api, internal/app, internal/audit 等 10 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/observability/zaplog_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] logger.go, logger_edge_test.go, metrics.go, observability_test.go, stage_timing_metrics.go 等 6 个
//	[关键引用] ResetDefault ← logger_edge_test.go；syncDefault ← logger_edge_test.go
//
// 主要符号
//
//	TestDefaultLogger(t *testing.T)
//	TestNewLogger(t *testing.T)
//	TestNewConsoleLogger(t *testing.T)
//	TestLogLevels(t *testing.T)
//	TestGlobalFunctions(t *testing.T)
//	TestSugarFunctions(t *testing.T)
//	TestZapLevels(t *testing.T)
//	Default() *zapLogger
//	... 另有 2 个顶层符号

package observability

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

func TestDefaultLogger(t *testing.T) {
	l := Default()
	assert.NotNil(t, l)
	assert.NotNil(t, l.l)
}

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	l := newZapLogger(&buf, DebugLevel)

	l.Info("info message", String("key", "value"))
	l.Debug("debug message")
	l.Error("error message")

	output := buf.String()
	assert.Contains(t, output, "info message")
	assert.Contains(t, output, "debug message")
	assert.Contains(t, output, "error message")
	assert.Contains(t, output, "value")
}

func TestNewConsoleLogger(t *testing.T) {
	var buf bytes.Buffer
	l := newZapConsole(&buf, InfoLevel)
	l.Info("console-line", String("k", "v"))
	out := buf.String()
	assert.Contains(t, out, "console-line")
	assert.NotEqual(t, '{', out[0])
}

func TestLogLevels(t *testing.T) {
	var buf bytes.Buffer
	l := newZapLogger(&buf, ErrorLevel)

	l.Info("info message")   // Should be ignored
	l.Debug("debug message") // Should be ignored
	l.Error("error message") // Should be logged

	output := buf.String()
	assert.NotContains(t, output, "info message")
	assert.NotContains(t, output, "debug message")
	assert.Contains(t, output, "error message")
}

func TestGlobalFunctions(t *testing.T) {
	originalStd := std
	defer ResetDefault(originalStd)

	var buf bytes.Buffer
	l := newZapLogger(&buf, InfoLevel)
	ResetDefault(l)

	Info("global info")
	Warn("global warn")
	Error("global error")

	output := buf.String()
	assert.Contains(t, output, "global info")
	assert.Contains(t, output, "global warn")
	assert.Contains(t, output, "global error")
}

func TestSugarFunctions(t *testing.T) {
	originalStd := std
	defer ResetDefault(originalStd)

	var buf bytes.Buffer
	l := newZapLogger(&buf, InfoLevel)
	ResetDefault(l)

	SugarInfo("sugar info")
	SugarInfof("sugar info %s", "formatted")
	SugarInfow("sugar info w", "key", "val")

	output := buf.String()
	assert.Contains(t, output, "sugar info")
	assert.Contains(t, output, "formatted")
	assert.Contains(t, output, "sugar info w")
	assert.Contains(t, output, "val")
}

func TestZapLevels(t *testing.T) {
	assert.True(t, ErrorLevel > InfoLevel)
	assert.True(t, InfoLevel > DebugLevel)
	assert.Equal(t, zapcore.InfoLevel, Level(0))
	assert.Equal(t, zapcore.ErrorLevel, Level(2))
}

// Default / ResetDefault / syncDefault 只服务测试（planv6.6 §1.3 T-5）：
// 生产代码用 Logger 接口 + Sync(l)，不需要替换包级默认 logger。

func Default() *zapLogger {
	return std
}

// ResetDefault 替换包级默认 logger，并重新绑定 Info / Sugar* 等函数变量。
func ResetDefault(l *zapLogger) {
	std = l
	Info = std.l.Info
	Warn = std.l.Warn
	Error = std.l.Error
	DPanic = std.l.DPanic
	Panic = std.l.Panic
	Fatal = std.l.Fatal
	Debug = std.l.Debug

	sugarStd = std.l.Sugar()
	SugarDebug = sugarStd.Debug
	SugarInfo = sugarStd.Info
	SugarWarn = sugarStd.Warn
	SugarError = sugarStd.Error
	SugarDPanic = sugarStd.DPanic
	SugarPanic = sugarStd.Panic
	SugarFatal = sugarStd.Fatal

	SugarDebugf = sugarStd.Debugf
	SugarInfof = sugarStd.Infof
	SugarWarnf = sugarStd.Warnf
	SugarErrorf = sugarStd.Errorf
	SugarDPanicf = sugarStd.DPanicf
	SugarPanicf = sugarStd.Panicf
	SugarFatalf = sugarStd.Fatalf

	SugarDebugw = sugarStd.Debugw
	SugarInfow = sugarStd.Infow
	SugarWarnw = sugarStd.Warnw
	SugarErrorw = sugarStd.Errorw
	SugarDPanicw = sugarStd.DPanicw
	SugarPanicw = sugarStd.Panicw
	SugarFatalw = sugarStd.Fatalw
}

func syncDefault() error {
	if std != nil {
		return std.Sync()
	}
	return nil
}
