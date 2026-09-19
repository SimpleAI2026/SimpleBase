package gofunction

import (
	"testing"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

func TestVisitInstrInterpreterPath(t *testing.T) {
	debugging = true
	defer func() { debugging = false }()

	src := `package main
func test() int {
	s := make([]int, 2, 4)
	ln, capn := 2, 4
	s2 := make([]int, ln, capn)
	_ = s2
	s[0] = 1
	s = append(s, 3)
	m := map[string]int{"a": 1}
	m["b"] = 2
	v, ok := m["a"]
	sum := 0
	for i := 0; i < 3; i++ {
		sum += i + v
	}
	if !ok || sum < 0 {
		return -1
	}
	ch := make(chan int, 1)
	ch <- 4
	x := <-ch
	var iface interface{} = x
	n, ok2 := iface.(int)
	_ = ok2
	go func() { _ = n }()
	defer func() { _ = sum }()
	switch n {
	case 1, 2, 3, 4:
		sum++
	default:
		sum += 2
	}
	type S struct{ A int }
	p := &S{A: 1}
	p.A++
	byVal := S{A: 7}
	_ = byVal.A
	sl := s[0:1:2]
	var wide int64 = 9
	narrow := int(wide)
	var anyv interface{} = narrow
	var any2 any = anyv
	_, _ = any2.(int)
	sel := 0
	sch := make(chan int, 1)
	sch <- 1
	select {
	case sel = <-sch:
	default:
		sel = -1
	}
	return sum + len(s) + p.A + len(sl) + sel
}
`
	p, err := BuildProgram("interp", "interp.go", src)
	if err != nil {
		t.Fatal(err)
	}
	fn := p.mainPkg.Func("test")
	if fn == nil {
		t.Fatal("missing test")
	}
	caller := &frame{program: p, context: newCallContext(), seqid: "interp"}
	fr := getFrameFromPool(caller, fn)
	if len(fn.Blocks) > 0 {
		fr.block = fn.Blocks[0]
		fr.prevBlock = fn.Blocks[0]
	}
	for i, l := range fn.Locals {
		fr.env[fr.layout.localSlots[i]] = zero(deref(l.Type()))
	}
	for fr.block != nil {
		runFrame(fr)
	}
	if fr.result == nil {
		t.Fatal("no result")
	}

	debugging = false
	mustPanic(t, "unexpected instruction", func() {
		var unknown ssa.Instruction
		_ = visitInstr(fr, unknown)
	})
}

func TestUnopUintFloatAndChan(t *testing.T) {
	src := `package main
func Uxor() uint { var x uint = 3; return ^x }
func Uneg() uint { var x uint = 3; return -x }
func Fneg() float64 { var x float64 = 1.5; return -x }
func Bnot() bool { return !false }
func Recv() int {
	ch := make(chan int)
	close(ch)
	v, ok := <-ch
	if ok { return v }
	return -2
}
func RecvOne() int {
	ch := make(chan int, 1)
	ch <- 8
	return <-ch
}
func Deref() int {
	x := 5
	p := &x
	return *p
}
`
	if v := runExtraSrc(t, src, "Uxor"); v == nil {
		t.Fatal("uxor")
	}
	if v := runExtraSrc(t, src, "Uneg"); v == nil {
		t.Fatal("uneg")
	}
	if v := runExtraSrc(t, src, "Fneg"); v != -1.5 {
		t.Fatalf("fneg=%v", v)
	}
	if v := runExtraSrc(t, src, "Bnot"); v != true {
		t.Fatalf("bnot=%v", v)
	}
	if v := runExtraSrc(t, src, "Recv"); v != -2 {
		t.Fatalf("recv=%v", v)
	}
	if v := runExtraSrc(t, src, "RecvOne"); v != 8 {
		t.Fatalf("recv1=%v", v)
	}
	if v := runExtraSrc(t, src, "Deref"); v != 5 {
		t.Fatalf("deref=%v", v)
	}
}

func TestBinopStringAndNilCompare(t *testing.T) {
	src := `package main
func S() string { return "a" + "b" }
func Cmp() int {
	if "a" < "b" && "a" <= "a" && "b" > "a" && "b" >= "b" && "a" != "b" && "a" == "a" {
		return 1
	}
	return 0
}
func NilEq() bool {
	var p *int
	return p == nil && p != &[]int{1}[0]
}
func Ucmp() bool {
	return uint(1) < uint(2) && uint(2) <= uint(2) && uint(3) > uint(1) && uint(3) >= uint(3)
}
func Fcmp() bool {
	return 1.0 < 2.0 && 2.0 <= 2.0 && 3.0 > 1.0 && 3.0 >= 3.0
}
`
	if v := runExtraSrc(t, src, "S"); v != "ab" {
		t.Fatalf("%v", v)
	}
	if v := runExtraSrc(t, src, "Cmp"); v != 1 {
		t.Fatalf("%v", v)
	}
	_ = runExtraSrc(t, src, "NilEq")
	if v := runExtraSrc(t, src, "Ucmp"); v != true {
		t.Fatalf("%v", v)
	}
	if v := runExtraSrc(t, src, "Fcmp"); v != true {
		t.Fatalf("%v", v)
	}
	_ = value.ValueOf(1)
}

func TestCompilePhiManyPreds(t *testing.T) {
	src := `package main
func Phi() int {
	x := 0
	switch 2 {
	case 0:
		x = 10
	case 1:
		x = 11
	case 2:
		x = 12
	case 3:
		x = 13
	default:
		x = 14
	}
	return x
}
`
	if v := runExtraSrc(t, src, "Phi"); v != 12 {
		t.Fatalf("%v", v)
	}
}
