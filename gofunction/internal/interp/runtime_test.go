package interp

import (
	"bytes"
	"io"
	"log/slog"
	"testing"
)

func TestRuntimeInitAndReset(t *testing.T) {
	r := NewRuntime()
	r.Initialize("")
	if r.SequenceID == "" {
		t.Fatal("expected generated sequence id")
	}
	r.Initialize("fixed")
	if r.SequenceID != "fixed" {
		t.Fatalf("got %s", r.SequenceID)
	}
	r.Reset()
	if r.Program != nil {
		t.Fatal("reset should clear program")
	}
	r.Cleanup()
}

func TestSetLoggerAndDebugOutput(t *testing.T) {
	SetLogger(nil) // no-op
	SetLogger(slog.Default())
	SetDebugOutput(nil)
	SetDebugOutput(io.Discard)
	var buf bytes.Buffer
	SetDebugOutput(&buf)
	logDebug("hello %s", "world")
}
