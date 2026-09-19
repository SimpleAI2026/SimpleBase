package log

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNilWriterPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = New(nil, InfoLevel)
}

func TestLoggerMethodsAndSync(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, DebugLevel)
	l.Warn("warn msg", String("k", "v"))
	l.DPanic("dpanic msg")
	require.NoError(t, l.Sync())
	out := buf.String()
	assert.Contains(t, out, "warn msg")
	assert.Contains(t, out, "dpanic msg")

	original := std
	t.Cleanup(func() { ResetDefault(original) })

	var gbuf bytes.Buffer
	ResetDefault(New(&gbuf, DebugLevel))
	Debug("g-debug")
	DPanic("g-dpanic")
	SugarDebug("s-debug")
	SugarWarn("s-warn")
	SugarError("s-error")
	SugarDPanic("s-dpanic")
	SugarDebugf("s-debugf %d", 1)
	SugarWarnf("s-warnf %d", 2)
	SugarErrorf("s-errorf %d", 3)
	SugarDPanicf("s-dpanicf %d", 4)
	SugarDebugw("s-debugw", "a", 1)
	SugarWarnw("s-warnw", "a", 1)
	SugarErrorw("s-errorw", "a", 1)
	SugarDPanicw("s-dpanicw", "a", 1)
	require.NoError(t, Sync())

	got := gbuf.String()
	for _, want := range []string{"g-debug", "g-dpanic", "s-debug", "s-warn", "s-error", "s-debugf", "s-warnf", "s-errorf"} {
		assert.Contains(t, got, want)
	}

	std = nil
	require.NoError(t, Sync())
	std = New(io.Discard, InfoLevel)
}

func TestPanicRecoverable(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, DebugLevel)
	require.Panics(t, func() { l.Panic("boom") })

	original := std
	t.Cleanup(func() { ResetDefault(original) })
	ResetDefault(New(&buf, DebugLevel))
	require.Panics(t, func() { Panic("g-boom") })
	require.Panics(t, func() { SugarPanic("s-boom") })
	require.Panics(t, func() { SugarPanicf("s-boomf %s", "x") })
	require.Panics(t, func() { SugarPanicw("s-boomw", "k", 1) })
}

func TestFieldAliases(t *testing.T) {
	_ = Skip()
	_ = Binary("b", []byte{1})
	_ = Bool("b", true)
	_ = ByteString("bs", []byte("x"))
	_ = Int("i", 1)
	_ = Int64("i64", 1)
	_ = String("s", "v")
	_ = Any("a", 1)
}
