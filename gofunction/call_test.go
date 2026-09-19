package gofunction

// 本文件覆盖函数调用链路：callSSA/callBuiltin/callExternal、
// 内置函数语义、defer/recover、变参与多返回值。

import (
	"go/token"
	"go/types"
	"io"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

func TestCallExternalAndBuiltinDirect(t *testing.T) {
	p, err := BuildProgram("t", "main", `package main
func test() []int {
	s := []int{1}
	s = append(s)
	dst := make([]int, 1)
	copy(dst, s)
	return s
}
`)
	if err != nil {
		t.Fatal(err)
	}
	fr := &frame{program: p, context: newCallContext(), layout: &funcLayout{}, env: nil}

	appendFn := findBuiltin(p.mainPkg.Func("test"), "append")
	copyFn := findBuiltin(p.mainPkg.Func("test"), "copy")
	if appendFn == nil || copyFn == nil {
		t.Fatal("expected append/copy builtins in SSA")
	}

	s := []int{1, 2}
	// append with a single slice (no extra elements)
	got := callBuiltin(fr, token.NoPos, appendFn, []value.Value{value.ValueOf(s)})
	if got.Len() != 2 {
		t.Fatalf("append one-arg len=%d", got.Len())
	}
	// skip nil extra
	_ = callBuiltin(fr, token.NoPos, appendFn, []value.Value{value.ValueOf([]*int{}), value.ValueOf((*int)(nil))})
	// non-slice extra elements
	got = callBuiltin(fr, token.NoPos, appendFn, []value.Value{value.ValueOf([]int{1}), value.ValueOf(9)})
	if got.Len() != 2 || got.Index(1).Int() != 9 {
		t.Fatalf("append scalar extra %#v", got.Interface())
	}

	dst := []int{0, 0}
	src := []int{7}
	n := callBuiltin(fr, token.NoPos, copyFn, []value.Value{value.ValueOf(&dst), value.ValueOf(&src)})
	if n.Int() != 1 || dst[0] != 7 {
		t.Fatalf("copy via pointers n=%v dst=%v", n.Interface(), dst)
	}

	mustPanic(t, "callExternal not func", func() {
		callExternal(reflect.ValueOf(1), nil)
	})
	mustPanic(t, "callExternal too few args", func() {
		callExternal(reflect.ValueOf(func(a int) {}), nil)
	})
	// variadic individual args (not packed as a slice)
	got = callExternal(reflect.ValueOf(func(a ...int) int {
		s := 0
		for _, v := range a {
			s += v
		}
		return s
	}), []value.Value{value.ValueOf(1), value.ValueOf(2), value.ValueOf(3)})
	if got.Int() != 6 {
		t.Fatalf("variadic individual got %v", got.Interface())
	}
}

func TestCallSSAMissingArgsAndFreeVars(t *testing.T) {
	p, err := BuildProgram("t", "main", `package main
func add(a int) int { return a }
func test() int {
	n := 1
	f := func() int { return n }
	return f()
}
`)
	if err != nil {
		t.Fatal(err)
	}
	fr := &frame{program: p, context: newCallContext()}
	mustPanic(t, "missing args", func() {
		callSSA(fr, p.mainPkg.Func("add"), nil, nil)
	})

	// missing free vars: call a closure function with empty env
	testFn := p.mainPkg.Func("test")
	var closure *ssa.Function
	for _, b := range testFn.Blocks {
		for _, ins := range b.Instrs {
			if mc, ok := ins.(*ssa.MakeClosure); ok {
				closure = mc.Fn.(*ssa.Function)
			}
		}
	}
	if closure == nil {
		t.Fatal("expected closure")
	}
	mustPanic(t, "missing free vars", func() {
		callSSA(fr, closure, nil, nil)
	})
}

func TestUnknownBuiltinAndRecoverIdle(t *testing.T) {
	src := `package main
func test() int {
	defer func() { recover() }()
	return 4
}
`
	got := runSrc(t, src, "test")
	if got != 4 {
		t.Fatalf("got %#v", got)
	}

	p, err := BuildProgram("t", "main", src)
	if err != nil {
		t.Fatal(err)
	}
	fr := &frame{program: p, context: newCallContext(), caller: nil}
	rec := findBuiltin(p.mainPkg.Func("test"), "recover")
	if rec == nil {
		// recover lives in the deferred closure
		for _, m := range p.mainPkg.Members {
			fn, ok := m.(*ssa.Function)
			if !ok {
				continue
			}
			if rec = findBuiltin(fn, "recover"); rec != nil {
				break
			}
		}
	}
	if rec != nil {
		v := callBuiltin(fr, token.NoPos, rec, nil)
		if v.Interface() != nil {
			t.Fatalf("idle recover got %v", v.Interface())
		}
	}

	mustPanic(t, "unknown builtin", func() {
		visitInstr(fr, &ssa.SliceToArrayPointer{})
	})
}

func TestBuiltinPanicCall(t *testing.T) {
	src := `package main
func test() int {
	defer func() { recover() }()
	panic("x")
	return 0
}
`
	_ = runSrc(t, src, "test")
}

func TestUnrecoveredPanicSetsError(t *testing.T) {
	_, err := Run("t", `package main
func test() { panic("boom") }`, "test")
	if err == nil {
		t.Fatal("expected panic error")
	}
}

func TestMakeFuncMethodValue(t *testing.T) {
	src := `package main
type T struct{ N int }
func (t T) Add(x int) int { return t.N + x }
func (t *T) Inc() int { t.N++; return t.N }
func test() []interface{} {
	t := T{N: 3}
	f := t.Add
	p := &t
	g := p.Inc
	return []interface{}{f(4), g()}
}
`
	got := runSrc(t, src, "test").([]interface{})
	if got[0] != 7 || got[1] != 4 {
		t.Fatalf("got %#v", got)
	}
}

func TestStdconvAtoiExtract(t *testing.T) {
	src := `package main
import "strconv"
func test() int {
	n, err := strconv.Atoi("8")
	if err != nil {
		return -1
	}
	return n
}
`
	got := runSrc(t, src, "test")
	if got != 8 {
		t.Fatalf("got %#v", got)
	}
}

func mustPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s: expected panic", name)
		}
	}()
	fn()
}

func setSSAType(v ssa.Value, typ types.Type) {
	rv := reflect.ValueOf(v).Elem()
	f := rv.FieldByName("typ")
	reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.ValueOf(typ))
}

func TestBuiltinsAppendCopyLenCapDeletePrint(t *testing.T) {
	src := `package main
func test() []interface{} {
	s := []int{1}
	s = append(s, 2, 3)
	n := copy(s[:1], []int{9})
	m := map[string]int{"a": 1, "b": 2}
	delete(m, "a")
	println("hi", n)
	return []interface{}{len(s), cap(s), n, len(m), s[0]}
}
`
	SetDebugOutput(io.Discard)
	debugging = true
	defer func() { debugging = false }()
	p, err := BuildProgram("t", "main", src)
	if err != nil {
		t.Fatal(err)
	}
	result, ctx, err := p.RunWithContext("t", "test")
	if err != nil {
		t.Fatal(err)
	}
	out := ctx.Output()
	if !strings.Contains(out, "hi") {
		t.Fatalf("println output %q", out)
	}
	got := result.([]interface{})
	if got[0] != 3 || got[2] != 1 || got[3] != 1 || got[4] != 9 {
		t.Fatalf("got %#v", got)
	}
}

func TestBuiltinCloseAndSelect(t *testing.T) {
	src := `package main
func test() int {
	ch := make(chan int, 1)
	ch <- 3
	close(ch)
	select {
	case v, ok := <-ch:
		if !ok {
			return -1
		}
		return v
	default:
		return 0
	}
}
`
	got := runSrc(t, src, "test")
	if got != 3 {
		t.Fatalf("got %#v", got)
	}
}

func TestPanicRecoverNamed(t *testing.T) {
	src := `package main
func test() (ret int) {
	defer func() {
		if recover() != nil {
			ret = 7
		}
	}()
	panic("boom")
	return 0
}
`
	got := runSrc(t, src, "test")
	if got != 7 {
		t.Fatalf("got %#v", got)
	}
}

func TestClosureFreeVars(t *testing.T) {
	src := `package main
func test() int {
	n := 1
	inc := func() { n++ }
	inc()
	inc()
	return n
}
`
	got := runSrc(t, src, "test")
	if got != 3 {
		t.Fatalf("got %#v", got)
	}
}

func TestBytesAppendString(t *testing.T) {
	src := `package main
func test() string {
	b := []byte("he")
	b = append(b, "llo"...)
	return string(b)
}
`
	got := runSrc(t, src, "test")
	if got != "hello" {
		t.Fatalf("got %#v", got)
	}
}

func runSrc(t *testing.T, src, fn string, params ...interface{}) interface{} {
	t.Helper()
	result, err := Run("t", src, fn, params...)
	if err != nil {
		t.Fatalf("%s: %v", fn, err)
	}
	return result
}

func TestInterfaceInvokeError(t *testing.T) {
	src := `package main
import "errors"
func test() string {
	err := errors.New("boom")
	return err.Error()
}
`
	got := runSrc(t, src, "test")
	if got != "boom" {
		t.Fatalf("got %#v", got)
	}
}

func TestVariadicAndMultiReturn(t *testing.T) {
	src := `package main
import "fmt"
func pair() (int, int) { return 1, 2 }
func test() string {
	a, b := pair()
	return fmt.Sprintf("%d-%d", a, b)
}
`
	got := runSrc(t, src, "test")
	if got != "1-2" {
		t.Fatalf("got %#v", got)
	}
}

func TestBuiltinUnknownPanic(t *testing.T) {
	// 间接覆盖 cap/len on array and print without debug
	src := `package main
func test() int {
	var a [2]int
	print(len(a), cap(a))
	return len(a) + cap(a)
}
`
	got := runSrc(t, src, "test")
	if got != 4 {
		t.Fatalf("got %#v", got)
	}
}

func TestCallExternalVariadic(t *testing.T) {
	src := `package main
import "fmt"
func test() string {
	return fmt.Sprintf("%s-%s", "a", "b")
}
`
	got := runSrc(t, src, "test")
	if got != "a-b" {
		t.Fatalf("got %#v", got)
	}
}
func findBuiltin(fn *ssa.Function, name string) *ssa.Builtin {
	if fn == nil {
		return nil
	}
	for _, b := range fn.Blocks {
		for _, ins := range b.Instrs {
			call, ok := ins.(*ssa.Call)
			if !ok {
				continue
			}
			if bu, ok := call.Call.Value.(*ssa.Builtin); ok && bu.Name() == name {
				return bu
			}
		}
	}
	return nil
}
