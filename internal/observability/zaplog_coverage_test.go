package observability

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewNilWriterPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = newZapLogger(nil, InfoLevel)
}

func TestLoggerMethodsAndSync(t *testing.T) {
	var buf bytes.Buffer
	l := newZapLogger(&buf, DebugLevel)
	l.Warn("warn msg", String("k", "v"))
	l.DPanic("dpanic msg")
	require.NoError(t, l.Sync())
	out := buf.String()
	assert.Contains(t, out, "warn msg")
	assert.Contains(t, out, "dpanic msg")

	original := std
	t.Cleanup(func() { ResetDefault(original) })

	var gbuf bytes.Buffer
	ResetDefault(newZapLogger(&gbuf, DebugLevel))
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
	require.NoError(t, syncDefault())

	got := gbuf.String()
	for _, want := range []string{"g-debug", "g-dpanic", "s-debug", "s-warn", "s-error", "s-debugf", "s-warnf", "s-errorf"} {
		assert.Contains(t, got, want)
	}

	std = nil
	require.NoError(t, syncDefault())
	std = newZapLogger(io.Discard, InfoLevel)
}

func TestPanicRecoverable(t *testing.T) {
	var buf bytes.Buffer
	l := newZapLogger(&buf, DebugLevel)
	require.Panics(t, func() { l.Panic("boom") })

	original := std
	t.Cleanup(func() { ResetDefault(original) })
	ResetDefault(newZapLogger(&buf, DebugLevel))
	require.Panics(t, func() { Panic("g-boom") })
	require.Panics(t, func() { SugarPanic("s-boom") })
	require.Panics(t, func() { SugarPanicf("s-boomf %s", "x") })
	require.Panics(t, func() { SugarPanicw("s-boomw", "k", 1) })
}

func TestFieldAliases(t *testing.T) {
	// 别名绑定契约：导出的字段构造器必须与底层 zap 构造器产出一致。
	cases := []struct {
		name string
		got  Field
		want Field
	}{
		{"Skip", Skip(), zap.Skip()},
		{"Binary", Binary("b", []byte{1}), zap.Binary("b", []byte{1})},
		{"Bool", Bool("b", true), zap.Bool("b", true)},
		{"ByteString", ByteString("bs", []byte("x")), zap.ByteString("bs", []byte("x"))},
		{"Int", Int("i", 1), zap.Int("i", 1)},
		{"Int64", Int64("i64", 1), zap.Int64("i64", 1)},
		{"String", String("s", "v"), zap.String("s", "v")},
		{"Any", Any("a", 1), zap.Any("a", 1)},
	}
	for _, c := range cases {
		if c.got.Key != c.want.Key || c.got.Type != c.want.Type {
			t.Errorf("%s: alias mismatch: got {Key:%s Type:%v}, want {Key:%s Type:%v}",
				c.name, c.got.Key, c.got.Type, c.want.Key, c.want.Type)
		}
	}
}
