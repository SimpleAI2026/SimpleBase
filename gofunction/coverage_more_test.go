package gofunction

import (
	"context"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
	"github.com/linkxzhou/SimpleBase/gofunction/operations"
	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

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

func TestRunMakeSliceDirect(t *testing.T) {
	fr := &frame{env: map[ssa.Value]*value.Value{}}
	lenC := ssa.NewConst(constant.MakeInt64(2), types.Typ[types.Int])
	capC := ssa.NewConst(constant.MakeInt64(5), types.Typ[types.Int])
	instr := &ssa.MakeSlice{Len: lenC, Cap: capC}
	setSSAType(instr, types.NewSlice(types.Typ[types.Int]))
	if visitInstr(fr, instr) != _NEXT {
		t.Fatal("expected _NEXT")
	}
	got := fr.get(instr)
	if got.Len() != 2 || got.Cap() != 5 {
		t.Fatalf("make slice len=%d cap=%d", got.Len(), got.Cap())
	}
}

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
	fr := &frame{program: p, context: newCallContext(), env: map[ssa.Value]*value.Value{}}

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

func TestRunFrameNilFnRecoverAndCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fr := &frame{
		block:   &ssa.BasicBlock{Instrs: []ssa.Instruction{&ssa.Panic{}}},
		env:     map[ssa.Value]*value.Value{},
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
	child := &frame{
		program: p,
		fn:      p.mainPkg.Func("test"),
		block:   p.mainPkg.Func("test").Blocks[0],
		env:     map[ssa.Value]*value.Value{},
		context: cctx,
	}
	mustPanic(t, "cancelled context", func() {
		runFrame(child)
	})
}

func TestTypeChangeKindsAndConvNil(t *testing.T) {
	_ = typeChange(types.NewChan(types.RecvOnly, types.Typ[types.Int]))
	_ = typeChange(types.NewChan(types.SendOnly, types.Typ[types.Int]))
	_ = typeChange(types.NewChan(types.SendRecv, types.Typ[types.Int]))

	params := types.NewTuple(types.NewVar(token.NoPos, nil, "x", types.Typ[types.Int]))
	results := types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.String]))
	sig := types.NewSignatureType(nil, nil, nil, params, results, false)
	rt := typeChange(sig)
	if rt.Kind() != reflect.Func || rt.NumIn() != 1 || rt.NumOut() != 1 {
		t.Fatalf("signature type %s", rt)
	}
	variadic := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "a", types.NewSlice(types.Typ[types.Int]))),
		nil, true)
	if !typeChange(variadic).IsVariadic() {
		t.Fatal("expected variadic")
	}

	st := types.NewStruct([]*types.Var{
		types.NewVar(token.NoPos, nil, "A", types.Typ[types.Int]),
	}, []string{`json:"a"`})
	if typeChange(st).NumField() != 1 {
		t.Fatal("struct fields")
	}

	z := conv(nil, types.Typ[types.Int])
	if z.Int() != 0 {
		t.Fatalf("nil conv %v", z.Interface())
	}

	mustPanic(t, "invalid basic", func() {
		_ = typeChange(types.Typ[types.Invalid])
	})
	mustPanic(t, "unsupported type", func() {
		_ = typeChange(types.NewTuple())
	})
}

func TestSetGetGlobalEdgeCases(t *testing.T) {
	p, err := BuildProgram("t", "main", `package main
var S string = "a"
var N int = 1
func test() int { return N }
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SetGlobalValue("S", map[string]int{"x": 1}); err == nil {
		t.Fatal("expected assign error")
	}
	g := p.mainPkg.Members["N"].(*ssa.Global)
	cell := value.ValueOf(9)
	p.globals[g] = &cell
	v, err := p.GetGlobalValue("N")
	if err != nil || v != 9 {
		t.Fatalf("non-ptr cell %v %v", v, err)
	}
	nilCell := value.Value(value.RValue{Value: reflect.Zero(reflect.TypeOf((*int)(nil)))})
	p.globals[g] = &nilCell
	v, err = p.GetGlobalValue("N")
	if err != nil || v != nil {
		t.Fatalf("nil ptr cell %v %v", v, err)
	}
	delete(p.globals, g)
	v, err = p.GetGlobalValue("N")
	if err != nil || v != 0 {
		t.Fatalf("uninitialized %v %v", v, err)
	}

	// convert-compatible SetGlobalValue
	p2, err := BuildProgram("t", "main", `package main
var F float64 = 1
func test() float64 { return F }
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := p2.SetGlobalValue("F", int(3)); err != nil {
		t.Fatal(err)
	}
}

func TestAutoImportAlreadyImported(t *testing.T) {
	spec := importer.GetPackageByName("fmt")
	if spec == nil {
		t.Fatal("fmt not registered")
	}
	f := &ast.File{
		Imports:    []*ast.ImportSpec{spec},
		Unresolved: []*ast.Ident{{Name: "fmt"}},
	}
	_ = autoImport(f)
}

func TestBuildProgramTypeError(t *testing.T) {
	if _, err := BuildProgram("t", "main", "package main\nfunc test() { _ = nosuch }"); err == nil {
		t.Fatal("expected typecheck error")
	}
}

func TestRunWithContextRecover(t *testing.T) {
	p := &Program{}
	_, _, err := p.RunWithContext("t", "x")
	if err == nil {
		t.Fatal("expected recover error")
	}
}

func TestExternalLookups(t *testing.T) {
	p := &Program{}
	mustPanic(t, "externalValue no importer", func() {
		_ = p.externalValue(&ssa.Global{})
	})
	if p.externalFunction(&ssa.Function{}) != nil {
		t.Fatal("nil importer")
	}
	p.importer = importer.NewImporter()
	mustPanic(t, "external object missing", func() {
		g := &ssa.Global{}
		_ = p.externalValue(g)
	})
	if p.importedFunction(nil) != nil {
		t.Fatal("nil fn")
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

func TestFrameGetMissingAndDeferPanic(t *testing.T) {
	fr := &frame{env: map[ssa.Value]*value.Value{}, program: &Program{}}
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

func TestStoreLookupExtractSelectEdges(t *testing.T) {
	src := `package main
func test() []interface{} {
	s := "hi"
	n, err := strconvAtoi("7")
	ch := make(chan int, 1)
	ch <- 5
	v := 0
	select {
	case v = <-ch:
	case <-make(chan int):
	}
	return []interface{}{s[1], n, err == nil, v}
}
func strconvAtoi(s string) (int, error) {
	if s == "7" {
		return 7, nil
	}
	return 0, nil
}
`
	got := runSrc(t, src, "test").([]interface{})
	if got[0] != byte('i') && got[0] != uint8('i') {
		t.Fatalf("string index %#v", got[0])
	}
	if got[1] != 7 || got[3] != 5 {
		t.Fatalf("got %#v", got)
	}

	// extract of a non-[]value.Value tuple
	fr := &frame{env: map[ssa.Value]*value.Value{}}
	key := &ssa.Alloc{}
	tup := value.ValueOf([]int{9, 8})
	fr.env[key] = &tup
	ex := &ssa.Extract{Tuple: key}
	// Index is a field on Extract
	ex.Index = 0
	runExtract(fr, ex)
	if fr.get(ex).Int() != 9 {
		t.Fatalf("extract %v", fr.get(ex).Interface())
	}

	// store panics
	p, err := BuildProgram("t", "main", `package main
var F float64 = 1
var N int = 0
func test() float64 { return F }
`)
	if err != nil {
		t.Fatal(err)
	}
	stFr := &frame{program: p, env: map[ssa.Value]*value.Value{}, context: newCallContext()}
	intConst := ssa.NewConst(constant.MakeInt64(2), types.Typ[types.Int])
	mustPanic(t, "store missing alloc", func() {
		runStore(stFr, &ssa.Store{Addr: &ssa.Alloc{}, Val: intConst})
	})
	g := p.mainPkg.Members["F"].(*ssa.Global)
	runStore(stFr, &ssa.Store{Addr: g, Val: intConst}) // int → float64 convert
	cell := value.ValueOf(1)
	p.globals[g] = &cell
	runStore(stFr, &ssa.Store{Addr: g, Val: intConst}) // non-ptr cell replace

	gN := p.mainPkg.Members["N"].(*ssa.Global)
	delete(p.globals, gN)
	mustPanic(t, "store missing global", func() {
		runStore(stFr, &ssa.Store{Addr: gN, Val: intConst})
	})

	addr := &ssa.FieldAddr{}
	invalid := value.Value(value.RValue{})
	stFr.env[addr] = &invalid
	mustPanic(t, "store invalid addr", func() {
		runStore(stFr, &ssa.Store{Addr: addr, Val: intConst})
	})
	notPtr := value.ValueOf(1)
	stFr.env[addr] = &notPtr
	mustPanic(t, "store non-ptr", func() {
		runStore(stFr, &ssa.Store{Addr: addr, Val: intConst})
	})

	var boxed interface{} = []int{1, 2, 3}
	rv := reflect.ValueOf(&boxed).Elem()
	cv := containerValue(value.RValue{Value: rv})
	if cv.Kind() != reflect.Slice {
		t.Fatalf("container kind %s", cv.Kind())
	}
}

func TestCallFnAdapterInvalidFrame(t *testing.T) {
	mustPanic(t, "callFnAdapter", func() {
		callFnAdapter(stubOpsFrame{}, token.NoPos, nil, nil)
	})
}

type stubOpsFrame struct{}

func (stubOpsFrame) Get(ssa.Value) value.Value { return nil }
func (stubOpsFrame) GetGoroutineCounter() *int32 {
	var n int32
	return &n
}
func (stubOpsFrame) GetOutBuffer() interface{ WriteString(string) (int, error) } {
	return nil
}
func (stubOpsFrame) GetEnv() map[ssa.Value]*value.Value { return nil }

func TestOpsCallInvalidFrameCallbacks(t *testing.T) {
	mustPanic(t, "callSSA adapter", func() {
		opManager.ExecuteCall(stubOpsFrame{}, token.NoPos, &ssa.Function{}, nil,
			func(fr operations.FrameInterface, fn *ssa.Function, args []value.Value, env []*value.Value) value.Value {
				fa, ok := fr.(*frameAdapter)
				if !ok {
					panic("invalid frame type")
				}
				return callSSA(fa.frame, fn, args, env)
			},
			func(fr operations.FrameInterface, pos token.Pos, fn *ssa.Builtin, args []value.Value) value.Value {
				return nil
			},
			func(reflect.Value, []value.Value) value.Value { return nil },
		)
	})
}

func TestExecutorTimeoutAndInterruptProgram(t *testing.T) {
	ex := NewExecutor(nil)
	ex.SetExecutionTimeout(50)
	got, err := ex.Execute("test", `package main
func test() int { return 1 }`)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("got %#v", got)
	}
	ex.SetExecutionTimeout(0)
	ex.StartTimeoutTimer("timeout")
	time.Sleep(5 * time.Millisecond)
	ex.StopTimeoutTimer()

	p, err := BuildProgram("t", "main", `package main
func test() int { return 1 }`)
	if err != nil {
		t.Fatal(err)
	}
	ex.vm.Program = p
	if err := ex.Interrupt("stop"); err != nil {
		t.Fatal(err)
	}
}

func TestStringIndexAndBlockingSelect(t *testing.T) {
	src := `package main
func test() int {
	ch := make(chan int, 1)
	ch <- 3
	select {
	case v := <-ch:
		return v
	}
}
`
	got := runSrc(t, src, "test")
	if got != 3 {
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

func TestAppendNilPointerElems(t *testing.T) {
	src := `package main
func test() int {
	s := []*int{}
	s = append(s, nil)
	return len(s)
}
`
	got := runSrc(t, src, "test")
	if got != 1 {
		t.Fatalf("got %#v", got)
	}
}
