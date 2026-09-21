package log

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
	l := New(&buf, DebugLevel)

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
	l := NewConsole(&buf, InfoLevel)
	l.Info("console-line", String("k", "v"))
	out := buf.String()
	assert.Contains(t, out, "console-line")
	assert.NotEqual(t, '{', out[0])
}

func TestLogLevels(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, ErrorLevel)

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
	l := New(&buf, InfoLevel)
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
	l := New(&buf, InfoLevel)
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
