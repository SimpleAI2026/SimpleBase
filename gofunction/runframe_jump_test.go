package gofunction

import (
	"testing"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

func TestRunFrameJumpContinues(t *testing.T) {
	debugging = true
	defer func() { debugging = false }()

	src := `package test
func test() int {
	sum := 0
	for i := 0; i < 3; i++ {
		sum += i
	}
	return sum
}`
	p, err := BuildProgram("dbg", "t", src)
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Run("dbg", "test")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result != 3 {
		t.Fatalf("expected 3, got %#v", result)
	}
}
