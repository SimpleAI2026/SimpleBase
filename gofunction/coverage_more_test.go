package gofunction

import (
	"context"
	"errors"
	"testing"
)

func TestNewSequenceIDAndParseInit(t *testing.T) {
	id := newSequenceID()
	if len(id) < 5 || id[:4] != "seq-" {
		t.Fatalf("id=%q", id)
	}
	src := `package main
func init() {}
func Exported() {}
`
	pub, err := ParseFuncList(src, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range pub {
		if n == "init" {
			t.Fatal("init must not be exported")
		}
	}
}

func TestIsDeadlineExceeded(t *testing.T) {
	if isDeadlineExceeded(nil) {
		t.Fatal("nil")
	}
	if !isDeadlineExceeded(context.DeadlineExceeded) {
		t.Fatal("sentinel")
	}
	if !isDeadlineExceeded(errors.New("recover: context deadline exceeded")) {
		t.Fatal("wrapped string")
	}
	if isDeadlineExceeded(errors.New("other")) {
		t.Fatal("other")
	}
}

func TestRunCompileError(t *testing.T) {
	_, err := Run("t", "not go", "F")
	if err == nil {
		t.Fatal("expected compile error")
	}
}
