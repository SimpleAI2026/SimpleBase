package gofunction

// 本文件覆盖 runFrame 主循环：跳转、panic 恢复、上下文取消。

import (
	"context"
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
	"testing"
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

func TestRunFrameNilFnRecoverAndCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fr := &frame{
		block:   &ssa.BasicBlock{Instrs: []ssa.Instruction{&ssa.Panic{}}},
		context: &Context{Context: ctx, cancelFunc: cancel},
	}
	func() {
		defer func() { _ = recover() }()
		runFrame(fr)
	}()

	p, err := BuildProgram("t", "main", `package main
func test() int {
	for {
	}
	return 0
}
`)
	if err != nil {
		t.Fatal(err)
	}
	cctx := newCallContext()
	cctx.cancelFunc()
	fnTest := p.mainPkg.Func("test")
	child := &frame{
		program: p,
		fn:      fnTest,
		block:   fnTest.Blocks[0],
		layout:  p.layoutOf(fnTest),
		env:     make([]value.Value, p.layoutOf(fnTest).nSlots),
		context: cctx,
	}
	mustPanic(t, "cancelled context", func() {
		runFrame(child)
	})
}

func TestFrameGetMissingAndDeferPanic(t *testing.T) {
	fr := &frame{layout: &funcLayout{}, program: &Program{}}
	mustPanic(t, "get missing", func() {
		_ = fr.get(&ssa.Alloc{})
	})

	src := `package main
func test() (r int) {
	defer func() {
		recover()
		r = 9
	}()
	defer func() { panic("inner") }()
	return 0
}
`
	got := runSrc(t, src, "test")
	if got != 9 {
		t.Fatalf("got %#v", got)
	}
}
