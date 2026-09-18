package gofunction

import (
	"bytes"
	"io"
	"log/slog"
	"testing"
)

func TestRuntimeUserDataAndInit(t *testing.T) {
	r := NewRuntime()
	r.Initialize("")
	if r.SequenceID == "" {
		t.Fatal("expected generated sequence id")
	}
	r.Initialize("fixed")
	if r.SequenceID != "fixed" {
		t.Fatalf("got %s", r.SequenceID)
	}
	if _, ok := r.GetUserData("k"); ok {
		t.Fatal("missing key should be absent")
	}
	r.SetUserData("k", 1)
	v, ok := r.GetUserData("k")
	if !ok || v != 1 {
		t.Fatalf("got %v %v", v, ok)
	}
	r.ConsoleLog(1, "tag", "data")
	r.Reset()
	if _, ok := r.GetUserData("k"); ok {
		t.Fatal("reset should clear user data")
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
