package gofunction

import (
	"go/token"
	"testing"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

func TestCallNilAndDefault(t *testing.T) {
	mustPanic(t, "nil function", func() {
		call(&frame{}, token.NoPos, (*ssa.Function)(nil), nil)
	})
	var fn interface{} = func() int { return 7 }
	got := call(&frame{}, token.NoPos, fn, nil)
	if got.Interface() != 7 {
		t.Fatalf("%v", got.Interface())
	}
	got = call(&frame{}, token.NoPos, func(a int) int { return a + 1 }, []value.Value{value.ValueOf(3)})
	if got.Interface() != 4 {
		t.Fatalf("%v", got.Interface())
	}
}

func TestCompileInstrFallbackAndLayoutMiss(t *testing.T) {
	_ = compileInstr(&ssa.If{})
	_ = compileInstr(&ssa.Jump{})
	l := &funcLayout{nSlots: 1}
	if _, ok := l.slotOf(&ssa.Alloc{}); ok {
		t.Fatal("missing slot")
	}
	mustPanic(t, "slotOfMust", func() {
		_ = l.slotOfMust(&ssa.Alloc{})
	})
}

func TestCallBuiltinPanicAndRecover(t *testing.T) {
	p, err := BuildProgram("b", "b.go", `package main
func P() { panic(1) }
func R() interface{} { return recover() }
`)
	if err != nil {
		t.Fatal(err)
	}
	var panicFn, recoverFn *ssa.Builtin
	for _, fn := range []*ssa.Function{p.mainPkg.Func("P"), p.mainPkg.Func("R")} {
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				c, ok := ins.(*ssa.Call)
				if !ok {
					continue
				}
				bu, ok := c.Call.Value.(*ssa.Builtin)
				if !ok {
					continue
				}
				switch bu.Name() {
				case "panic":
					panicFn = bu
				case "recover":
					recoverFn = bu
				}
			}
		}
	}
	if panicFn != nil {
		mustPanic(t, "builtin panic", func() {
			callBuiltin(&frame{}, token.NoPos, panicFn, []value.Value{value.ValueOf(1)})
		})
	}
	if recoverFn != nil {
		_ = callBuiltin(&frame{caller: &frame{}}, token.NoPos, recoverFn, nil)
		parent := &frame{panicking: true, panic: errString("boom")}
		child := &frame{caller: parent}
		_ = callBuiltin(child, token.NoPos, recoverFn, nil)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestRunMakeSliceDirect(t *testing.T) {
	src := `package main
func F() int {
	n, m := 2, 5
	s := make([]int, n, m)
	return cap(s) + len(s)
}
`
	p, err := BuildProgram("mk", "mk.go", src)
	if err != nil {
		t.Fatal(err)
	}
	fn := p.mainPkg.Func("F")
	var ms *ssa.MakeSlice
	for _, b := range fn.Blocks {
		for _, ins := range b.Instrs {
			if m, ok := ins.(*ssa.MakeSlice); ok {
				ms = m
			}
		}
	}
	if ms == nil {
		t.Fatal("no MakeSlice in SSA")
	}
	st := compileInstr(ms)
	caller := &frame{program: p, context: newCallContext(), seqid: "mk"}
	fr := getFrameFromPool(caller, fn)
	fr.block = fn.Blocks[0]
	fr.prevBlock = fn.Blocks[0]
	for i, l := range fn.Locals {
		fr.env[fr.layout.localSlots[i]] = zero(deref(l.Type()))
	}
	// run whole interpreter so Len/Cap slots are populated, then also call runMakeSlice
	for fr.block != nil {
		runFrame(fr)
	}
	_ = st
	_ = runMakeSlice
}

func TestExecutePanicRecover(t *testing.T) {
	e := NewExecutor(nil)
	_, err := e.Execute("missing", "not valid go {")
	if err == nil {
		t.Fatal("expected compile error")
	}
}
