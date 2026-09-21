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

func TestRedactString(t *testing.T) {
	t.Parallel()
	if RedactString("") != "" {
		t.Fatal("empty")
	}
	if got := RedactString("ab"); got != "**" {
		t.Fatalf("short: %q", got)
	}
	if got := RedactString("abcdefghij"); got != "********" {
		t.Fatalf("long: %q", got)
	}
}

func TestIsSensitiveField(t *testing.T) {
	t.Parallel()
	yes := []string{"api_key", "API_KEY", "  Token ", "password", "secret_key", "dsn", "prompt", "messages", "sql_args"}
	for _, n := range yes {
		if !IsSensitiveField(n) {
			t.Errorf("expected sensitive: %q", n)
		}
	}
	if IsSensitiveField("database_id") || IsSensitiveField("") {
		t.Fatal("unexpected sensitive")
	}
}

func TestRedactMap(t *testing.T) {
	t.Parallel()
	if RedactMap(nil) != nil {
		t.Fatal("nil in => nil out")
	}
	in := map[string]any{
		"api_key": "secret",
		"nested":  map[string]any{"password": "p", "ok": "v"},
		"short":   "abc",
		"long":    "0123456789abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz",
		"count":   3,
		"token":   "should-hide",
	}
	out := RedactMap(in)
	if out["api_key"] != "***" || out["token"] != "***" {
		t.Fatalf("sensitive: %#v", out)
	}
	nested := out["nested"].(map[string]any)
	if nested["password"] != "***" || nested["ok"] != "v" {
		t.Fatalf("nested: %#v", nested)
	}
	if out["short"] != "abc" {
		t.Fatalf("short: %v", out["short"])
	}
	long, _ := out["long"].(string)
	if long != "012345...(redacted)" {
		t.Fatalf("long: %q", long)
	}
	if out["count"] != 3 {
		t.Fatalf("passthrough: %v", out["count"])
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
