package gocloudagent

import (
	"bytes"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// planv6.0 v2 §5.6：内容层日志是 internal/AGENTS.md 脱敏约定的显式例外，
// 必须验证「默认关闭不落正文」与「打开后正文全部脱敏 + 截断」两侧。

func newTestTracer(t *testing.T, trace bool) (*bytes.Buffer, tracer) {
	t.Helper()
	buf := &bytes.Buffer{}
	enc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(enc, zapcore.AddSync(buf), zap.DebugLevel)
	return buf, tracer{logger: zap.New(core), payload: trace}
}

func TestTracerPayloadOffByDefault(t *testing.T) {
	buf, tr := newTestTracer(t, false)
	tr.Meta("meta line", zap.String("run_id", "r1"))
	tr.Payload("payload line", "content", "password=hunter2 正文")
	out := buf.String()
	if !strings.Contains(out, "meta line") {
		t.Fatalf("metadata layer must still log: %s", out)
	}
	if strings.Contains(out, "payload line") || strings.Contains(out, "hunter2") || strings.Contains(out, "正文") {
		t.Fatalf("payload leaked while disabled: %s", out)
	}
}

func TestTracerPayloadRedactsAndTruncates(t *testing.T) {
	buf, tr := newTestTracer(t, true)
	long := strings.Repeat("x", traceMaxChars+500)
	tr.Payload("payload", "content", "api_key=sk-abcdefghijklmnopqrstuvwxyz012345 AKIAIOSFODNN7EXAMPLE password=hunter2\n"+long)
	out := buf.String()
	for _, secret := range []string{"sk-abcdefghijklmnopqrstuvwxyz012345", "AKIAIOSFODNN7EXAMPLE", "hunter2"} {
		if strings.Contains(out, secret) {
			t.Fatalf("secret leaked %q: %s", secret, out)
		}
	}
	if !strings.Contains(out, "[truncated]") {
		t.Fatalf("expected truncation marker: %s", out)
	}
}

// JSON 形态是工具参数与结果的常见形态（planv6.0 v2 §5.3 记录的缺口）。
func TestTracerRedactsJSONSecretShape(t *testing.T) {
	_, tr := newTestTracer(t, true)
	for _, in := range []string{
		`{"password":"hunter2"}`,
		`{"api_key":"sk-abcdefghijklmnopqrstuvwxyz012345"}`,
		`{"headers":{"authorization":"Bearer abc.def"}}`,
	} {
		got := tr.Redact(in)
		if strings.Contains(got, "hunter2") || strings.Contains(got, "sk-abcdefghijklmnopqrstuvwxyz012345") || strings.Contains(got, "abc.def") {
			t.Fatalf("JSON secret leaked: %s -> %s", in, got)
		}
	}
}

func TestHashSummaryShape(t *testing.T) {
	if got := hashSummary(""); got != "0:-" {
		t.Fatalf("empty summary=%q", got)
	}
	got := hashSummary("hello")
	if !strings.HasPrefix(got, "5:") || len(got) != len("5:")+16 {
		t.Fatalf("summary=%q", got)
	}
}

func TestArgvSummaryHidesValues(t *testing.T) {
	if got := argvSummary([]string{"sql", "exec", "--statement", "INSERT INTO t VALUES (1)"}); got != "sql exec" {
		t.Fatalf("summary=%q", got)
	}
	if got := argvSummary(nil); got != "" {
		t.Fatalf("empty summary=%q", got)
	}
	if got := argvSummary([]string{"database"}); got != "database" {
		t.Fatalf("single summary=%q", got)
	}
}
