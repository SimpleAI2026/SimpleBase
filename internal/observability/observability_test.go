package observability

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap/zapcore"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want zapcore.Level
	}{
		{"debug", zapcore.DebugLevel},
		{"info", zapcore.InfoLevel},
		{"warn", zapcore.WarnLevel},
		{"error", zapcore.ErrorLevel},
		{"", zapcore.InfoLevel},
		{"unknown", zapcore.InfoLevel},
	}
	for _, tc := range cases {
		if got := parseLevel(tc.in); got != tc.want {
			t.Errorf("parseLevel(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}

func TestNewLoggerLevelsAndFormats(t *testing.T) {
	var buf bytes.Buffer
	for _, level := range []string{"debug", "info", "warn", "error", "other"} {
		for _, format := range []string{"json", "console"} {
			l := NewLogger(level, format, &buf)
			if l == nil {
				t.Fatalf("logger nil for %s/%s", level, format)
			}
			l.Info("hello")
		}
	}
	if Sync(NewLogger("info", "json", io.Discard)) != nil {
		t.Fatal("Sync should ignore writer errors")
	}
	if NewLogger("info", "json", nil) == nil {
		t.Fatal("nil writer should default to stderr")
	}
}

func TestConsoleFormatIsNotJSON(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger("info", "console", &buf)
	l.Info("hello-console")
	out := buf.String()
	if out == "" {
		t.Fatal("empty console log")
	}
	if len(out) > 0 && out[0] == '{' {
		t.Fatalf("console format still JSON: %q", out)
	}
	if !bytes.Contains(buf.Bytes(), []byte("hello-console")) {
		t.Fatalf("missing message: %q", out)
	}
	if WriterFor("stdout") != os.Stdout || WriterFor("STDERR") != os.Stderr {
		t.Fatal("WriterFor")
	}
}

func TestSyncNil(t *testing.T) {
	if err := Sync(nil); err != nil {
		t.Fatalf("Sync(nil)=%v", err)
	}
}

func TestNewMetricsAndCacheHelpers(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	if m == nil || m.CacheBytes == nil {
		t.Fatal("metrics")
	}
	m.ObserveCacheBytes(42)
	m.IncCacheEvictions()
	var g dto.Metric
	if err := m.CacheBytes.Write(&g); err != nil || g.GetGauge().GetValue() != 42 {
		t.Fatalf("cache bytes %v %v", g.GetGauge().GetValue(), err)
	}
	g = dto.Metric{}
	if err := m.CacheEvictions.Write(&g); err != nil || g.GetCounter().GetValue() != 1 {
		t.Fatalf("evictions %v %v", g.GetCounter().GetValue(), err)
	}

	// nil registerer uses DefaultRegisterer. Isolate via recover in case
	// another test already registered the same names there.
	func() {
		defer func() { _ = recover() }()
		if NewMetrics(nil) == nil {
			t.Fatal("default registry")
		}
	}()
}
