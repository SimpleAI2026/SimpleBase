package api

import (
	"context"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"go.uber.org/zap/zapcore"
)

func TestRequestIDContext(t *testing.T) {
	if RequestIDFromContext(context.Background()) != "" {
		t.Fatal("empty context should yield empty rid")
	}
	ctx := WithRequestID(context.Background(), "abc-123")
	if got := RequestIDFromContext(ctx); got != "abc-123" {
		t.Fatalf("got %q", got)
	}
	// wrong type stored under key is treated as missing
	wrong := context.WithValue(context.Background(), requestIDKey, 99)
	if RequestIDFromContext(wrong) != "" {
		t.Fatal("non-string value should yield empty")
	}
}

func TestProjectContextRoundTrip(t *testing.T) {
	if _, ok := ProjectFromContext(context.Background()); ok {
		t.Fatal("expected missing")
	}
	want := ProjectContext{ID: "proj-1", TenantID: "ten-1"}
	ctx := WithProject(context.Background(), want)
	got, ok := ProjectFromContext(ctx)
	if !ok || got != want {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
}

func TestPrincipalContextRoundTrip(t *testing.T) {
	if _, ok := PrincipalFromContext(context.Background()); ok {
		t.Fatal("expected missing")
	}
	p := auth.Principal{APIKeyID: "key-1", TenantID: "t1"}
	ctx := WithPrincipal(context.Background(), p)
	got, ok := PrincipalFromContext(ctx)
	if !ok || got.APIKeyID != "key-1" || got.TenantID != "t1" {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
}

func TestItoaAndZapFields(t *testing.T) {
	if itoa(0) != "0" || itoa(-7) != "-7" || itoa(2048) != "2048" {
		t.Fatalf("itoa failed")
	}
	sf := fieldString("k", "v")
	if sf.Key != "k" || sf.Type != zapcore.StringType || sf.String != "v" {
		t.Fatalf("fieldString=%+v", sf)
	}
	df := fieldDuration("d", 250*time.Millisecond)
	if df.Key != "d" || df.Type != zapcore.DurationType {
		t.Fatalf("fieldDuration=%+v", df)
	}
}

func TestNullWriter(t *testing.T) {
	var w nullWriter
	n, err := w.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	n, err = w.Write(nil)
	if err != nil || n != 0 {
		t.Fatalf("Write(nil) = %d, %v", n, err)
	}
}
