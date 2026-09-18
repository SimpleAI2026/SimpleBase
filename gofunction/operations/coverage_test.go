package operations

import (
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

type ptrMeth struct{ n int }

func (p *ptrMeth) Inc() int { return p.n + 1 }

func TestLookupMethodPointerReceiver(t *testing.T) {
	v := reflect.ValueOf([]int{1, 2, 3})
	m := lookupMethod(v, "Len")
	if m.IsValid() {
		t.Fatal("unexpected method")
	}
	m = lookupMethod(reflect.ValueOf(ptrMeth{}), "Inc")
	if !m.IsValid() {
		t.Fatal("expected pointer method on value")
	}
	if lookupMethod(reflect.Value{}, "X").IsValid() {
		t.Fatal("invalid recv")
	}
}

func TestBinaryOpsUintBranches(t *testing.T) {
	bo := NewBinaryOperations()
	x := value.ValueOf(uint(8))
	y := value.ValueOf(uint(2))
	if bo.And(x, y).(uint64) != 0 {
		t.Fatal("and")
	}
	if bo.Or(x, y).(uint64) != 10 {
		t.Fatal("or")
	}
	if bo.Xor(x, y).(uint64) != 10 {
		t.Fatal("xor")
	}
	if bo.AndNot(x, y).(uint64) != 8 {
		t.Fatal("andnot")
	}
	sh := value.ValueOf(uint(1))
	if bo.Shl(x, sh).(uint64) != 16 {
		t.Fatal("shl")
	}
	if bo.Shr(x, sh).(uint64) != 4 {
		t.Fatal("shr")
	}
}

func TestComparisonNilAndUint(t *testing.T) {
	co := NewComparisonOperations()
	if co.Equal(value.ValueOf((*int)(nil)), value.ValueOf((*int)(nil))) != true {
		t.Fatal("nil equal")
	}
	if co.NotEqual(value.ValueOf((*int)(nil)), value.ValueOf((*int)(nil))) != false {
		t.Fatal("nil neq")
	}
	if co.Greater(value.ValueOf(uint(3)), value.ValueOf(uint(1))) != true {
		t.Fatal("uint gtr")
	}
	if co.Equal(value.ValueOf(1), value.ValueOf(1)) != true {
		t.Fatal("eq")
	}
	if co.NotEqual(value.ValueOf(1), value.ValueOf(2)) != true {
		t.Fatal("neq")
	}
}

func TestUnaryChanAndInvalid(t *testing.T) {
	uo := NewUnaryOperations()
	ch := make(chan int, 1)
	ch <- 5
	close(ch)
	conv := func(v interface{}, typ types.Type) value.Value { return value.ValueOf(v) }
	instr := &ssa.UnOp{Op: token.ARROW}
	got := uo.Execute(instr, value.ValueOf(ch), conv)
	if got.Int() != 5 {
		t.Fatalf("recv %v", got.Interface())
	}
	instr.CommaOk = true
	ch2 := make(chan int)
	close(ch2)
	got = uo.Execute(instr, value.ValueOf(ch2), conv)
	tuple := got.RValue()
	ok := tuple.Index(1).Interface().(value.Value).Bool()
	if ok {
		t.Fatal("expected closed")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	uo.Execute(&ssa.UnOp{Op: token.ADD}, value.ValueOf(1), conv)
}

func TestEvaluateConstantKinds(t *testing.T) {
	co := NewConstantOperations()
	zero := func(t types.Type) value.Value {
		return value.RValue{Value: reflect.New(reflect.TypeOf(0))}
	}
	conv := func(v interface{}, typ types.Type) value.Value { return value.ValueOf(v) }

	mk := func(kind types.BasicKind, val constant.Value) *ssa.Const {
		return ssa.NewConst(val, types.Typ[kind])
	}
	if co.Evaluate(mk(types.Bool, constant.MakeBool(true)), zero, conv).Bool() != true {
		t.Fatal("bool")
	}
	if co.Evaluate(mk(types.Int, constant.MakeInt64(3)), zero, conv).Int() != 3 {
		t.Fatal("int")
	}
	if co.Evaluate(mk(types.Uint, constant.MakeUint64(4)), zero, conv).Uint() != 4 {
		t.Fatal("uint")
	}
	if co.Evaluate(mk(types.Float64, constant.MakeFloat64(1.5)), zero, conv).Float() != 1.5 {
		t.Fatal("float")
	}
	if co.Evaluate(mk(types.String, constant.MakeString("a")), zero, conv).String() != "a" {
		t.Fatal("string")
	}
	// struct zero const
	st := types.NewStruct([]*types.Var{types.NewVar(token.NoPos, nil, "A", types.Typ[types.Int])}, []string{""})
	c := ssa.NewConst(nil, st)
	_ = co.Evaluate(c, func(t types.Type) value.Value {
		return value.RValue{Value: reflect.New(reflect.TypeOf(struct{ A int }{}))}
	}, conv)
}

func TestManagerExecuteAll(t *testing.T) {
	om := NewOperationManager()
	conv := func(v interface{}, typ types.Type) value.Value { return value.ValueOf(v) }
	bin := func(op token.Token) *ssa.BinOp {
		return &ssa.BinOp{Op: op}
	}
	x, y := value.ValueOf(6), value.ValueOf(3)
	ops := []token.Token{
		token.ADD, token.SUB, token.MUL, token.QUO, token.REM,
		token.AND, token.OR, token.XOR, token.AND_NOT,
		token.SHL, token.SHR,
		token.LSS, token.LEQ, token.EQL, token.NEQ, token.GTR, token.GEQ,
	}
	yShift := value.ValueOf(uint(1))
	for _, op := range ops {
		rhs := y
		if op == token.SHL || op == token.SHR {
			rhs = yShift
		}
		_ = om.ExecuteBinaryOp(bin(op), x, rhs, conv)
	}
	_ = om.ExecuteUnaryOp(&ssa.UnOp{Op: token.SUB}, x, conv)
	_ = om.EvaluateConstant(ssa.NewConst(constant.MakeInt64(1), types.Typ[types.Int]), func(t types.Type) value.Value {
		return value.RValue{Value: reflect.New(reflect.TypeOf(0))}
	}, conv)

	fr := &mockFrame{
		values: map[ssa.Value]value.Value{},
		env:    map[ssa.Value]*value.Value{},
	}
	callExt := func(reflect.Value, []value.Value) value.Value { return value.ValueOf(1) }
	callFn := func(FrameInterface, token.Pos, interface{}, []value.Value) value.Value { return value.ValueOf(2) }
	cc := &ssa.CallCommon{}
	// Signature() on empty CallCommon may panic; guard
	func() {
		defer func() { _ = recover() }()
		om.ExecuteCallOp(fr, cc, callExt, callFn)
	}()
	func() {
		defer func() { _ = recover() }()
		om.ExecuteGoCall(fr, cc, callExt, callFn)
	}()
	om.ExecuteCall(fr, token.NoPos, value.ValueOf(func() {}), nil,
		func(FrameInterface, *ssa.Function, []value.Value, []*value.Value) value.Value {
			return nil
		},
		func(FrameInterface, token.Pos, *ssa.Builtin, []value.Value) value.Value {
			return nil
		},
		callExt,
	)
}

func TestGoCallAndCallVariants(t *testing.T) {
	co := NewCallOperations()
	fn := func(a int) int { return a + 1 }
	rv := reflect.ValueOf(fn)
	ext := func(f reflect.Value, args []value.Value) value.Value {
		in := make([]reflect.Value, len(args))
		for i := range args {
			in[i] = args[i].RValue()
		}
		out := f.Call(in)
		return value.Package(out)
	}
	got := co.Call(nil, token.NoPos, rv, []value.Value{value.ValueOf(3)}, nil, nil, ext)
	if got.Int() != 4 {
		t.Fatalf("got %v", got.Interface())
	}
	got = co.Call(nil, token.NoPos, fn, []value.Value{value.ValueOf(5)}, nil, nil, ext)
	if got.Int() != 6 {
		t.Fatalf("got %v", got.Interface())
	}
	ev := value.NewExternalValue(rv)
	got = co.Call(nil, token.NoPos, ev, []value.Value{value.ValueOf(7)}, nil, nil, ext)
	if got.Int() != 8 {
		t.Fatalf("got %v", got.Interface())
	}
	defer func() {
		if recover() == nil {
			t.Fatal("nil function")
		}
	}()
	var nilFn *ssa.Function
	co.Call(nil, token.NoPos, nilFn, nil, nil, nil, ext)
}

func TestExternalValueRemaining(t *testing.T) {
	ev := NewExternalValue(reflect.ValueOf(false))
	if ev.String() != "false" {
		t.Fatal(ev.String())
	}
	ev = NewExternalValue(reflect.ValueOf([]int{1}))
	_ = ev.String()
	if !NewExternalValue(reflect.Value{}).IsNil() {
		t.Fatal("invalid")
	}
	st := NewExternalValue(reflect.ValueOf(struct{ A int }{3}))
	if st.Field(0).Int() != 3 {
		t.Fatal("field")
	}
	x := 1
	p := NewExternalValue(reflect.ValueOf(&x).Elem())
	p.Set(value.ValueOf(9))
	if x != 9 {
		t.Fatal(x)
	}
	ptr := NewExternalValue(reflect.ValueOf(&x))
	if ptr.Elem().Int() != 9 {
		t.Fatal("elem")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("next")
		}
	}()
	p.Next()
}

type invokeRecv struct{}

func (invokeRecv) M() int { return 42 }

func TestCallOpInvokeAndGoCall(t *testing.T) {
	co := NewCallOperations()
	recv := createMockSSAValue("recv")
	fr := &mockFrameForCall{
		values: map[ssa.Value]value.Value{recv: value.ValueOf(invokeRecv{})},
		env:    map[ssa.Value]*value.Value{},
	}
	recvVar := types.NewVar(token.NoPos, nil, "r", types.Typ[types.Int])
	sig := types.NewSignatureType(recvVar, nil, nil, types.NewTuple(), types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.Int])), false)
	meth := types.NewFunc(token.NoPos, nil, "M", sig)
	cc := &ssa.CallCommon{Value: recv, Method: meth}

	ext := func(f reflect.Value, args []value.Value) value.Value {
		out := f.Call(nil)
		return value.Package(out)
	}
	call := func(FrameInterface, token.Pos, interface{}, []value.Value) value.Value {
		return value.ValueOf(0)
	}
	got := co.CallOp(fr, cc, ext, call)
	if got.Int() != 42 {
		t.Fatalf("invoke got %v", got.Interface())
	}

	// GoCall method path
	arg0 := createMockSSAValue("arg0")
	fr.values[arg0] = value.ValueOf(invokeRecv{})
	gcc := &ssa.CallCommon{Value: recv, Args: []ssa.Value{arg0}}
	// Signature() uses Value.Type() which is a function sig from mock; Recv may be nil.
	func() {
		defer func() { _ = recover() }()
		co.GoCall(fr, gcc, ext, call)
		for i := 0; i < 50; i++ {
			if atomic.LoadInt32(&fr.goroutines) == 0 {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// Call ssa.Value from env
	key := createMockSSAValue("stored")
	fv := value.ValueOf(func() int { return 9 })
	fr.env[key] = &fv
	got = co.Call(fr, token.NoPos, key, nil,
		func(FrameInterface, *ssa.Function, []value.Value, []*value.Value) value.Value { return nil },
		func(FrameInterface, token.Pos, *ssa.Builtin, []value.Value) value.Value { return nil },
		func(f reflect.Value, args []value.Value) value.Value {
			out := f.Call(nil)
			return value.Package(out)
		},
	)
	if got.Int() != 9 {
		t.Fatalf("env func %v", got.Interface())
	}
}

type methodHost struct{ n int }

func (m methodHost) Add(x int) int { return m.n + x }
func (m *methodHost) Inc() int     { return m.n + 1 }

type mockRecvValue struct {
	mockSSAValue
	recv bool
}

func (m *mockRecvValue) Type() types.Type {
	if !m.recv {
		return m.mockSSAValue.Type()
	}
	recv := types.NewVar(token.NoPos, nil, "r", types.Typ[types.Int])
	return types.NewSignatureType(recv, nil, nil, types.NewTuple(), types.NewTuple(), false)
}

func TestCallOpGoCallRemaining(t *testing.T) {
	co := NewCallOperations()
	recvArg := createMockSSAValue("recv")
	extraArg := createMockSSAValue("x")
	fr := &mockFrameForCall{
		values: map[ssa.Value]value.Value{
			recvArg:  value.ValueOf(methodHost{n: 10}),
			extraArg: value.ValueOf(3),
		},
		env: map[ssa.Value]*value.Value{},
	}
	recvVar := types.NewVar(token.NoPos, nil, "r", types.Typ[types.Int])
	param := types.NewVar(token.NoPos, nil, "x", types.Typ[types.Int])
	sig := types.NewSignatureType(recvVar, nil, nil, types.NewTuple(param), types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.Int])), false)
	meth := types.NewFunc(token.NoPos, nil, "Add", sig)

	ext := func(f reflect.Value, args []value.Value) value.Value {
		in := make([]reflect.Value, len(args))
		for i := range args {
			in[i] = args[i].RValue()
		}
		out := f.Call(in)
		return value.Package(out)
	}
	call := func(FrameInterface, token.Pos, interface{}, []value.Value) value.Value {
		return value.ValueOf("fallback")
	}

	// GoCall with receiver + extra args (method found via Value.Name)
	fnVal := createMockSSAValue("Add")
	gcc := &ssa.CallCommon{Value: fnVal, Method: meth, Args: []ssa.Value{recvArg, extraArg}}
	co.GoCall(fr, gcc, ext, call)
	time.Sleep(15 * time.Millisecond)

	// GoCall receiver but method not found → fallback goroutine path
	miss := createMockSSAValue("Nope")
	gcc2 := &ssa.CallCommon{Value: miss, Method: meth, Args: []ssa.Value{recvArg}}
	co.GoCall(fr, gcc2, ext, call)
	for i := 0; i < 50; i++ {
		if atomic.LoadInt32(&fr.goroutines) == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// GoCall panic recovered in goroutine
	panicCall := func(FrameInterface, token.Pos, interface{}, []value.Value) value.Value {
		panic("bg")
	}
	plain := createSimpleCallCommon(createMockSSAValue("p"), []ssa.Value{createMockSSAValue("a")})
	fr.values[plain.Args[0]] = value.ValueOf(1)
	co.GoCall(fr, plain, ext, panicCall)
	for i := 0; i < 50; i++ {
		if atomic.LoadInt32(&fr.goroutines) == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if fr.outBuffer.content == "" {
		t.Fatal("expected goroutine panic log")
	}

	// lookupMethod CanAddr
	host := methodHost{n: 1}
	addr := reflect.ValueOf(&host).Elem()
	if !lookupMethod(addr, "Inc").IsValid() {
		t.Fatal("addressable pointer method")
	}

	// CallOp invoke with extra args
	arg := createMockSSAValue("arg")
	fr.values[arg] = value.ValueOf(3)
	cc := &ssa.CallCommon{Value: recvArg, Method: meth, Args: []ssa.Value{arg}}
	got := co.CallOp(fr, cc, ext, call)
	if got.Int() != 13 {
		t.Fatalf("invoke Add got %v", got.Interface())
	}

	// CallOp invoke method missing
	badMeth := types.NewFunc(token.NoPos, nil, "Missing", sig)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected missing method panic")
			}
		}()
		co.CallOp(fr, &ssa.CallCommon{Value: recvArg, Method: badMeth}, ext, call)
	}()

	// CallOp Recv but not invoke (Method nil, Value.Type has Recv)
	recvFn := &mockRecvValue{mockSSAValue: mockSSAValue{name: "Add"}, recv: true}
	got = co.CallOp(fr, &ssa.CallCommon{Value: recvFn, Args: []ssa.Value{recvArg, extraArg}}, ext, call)
	if got.Int() != 13 && got.Interface() != "fallback" {
		t.Fatalf("method call-mode got %v", got.Interface())
	}

	// empty args + Recv
	got = co.CallOp(fr, &ssa.CallCommon{Value: recvFn}, ext, call)
	if got.Interface() != "fallback" {
		t.Fatalf("empty args %v", got.Interface())
	}

	// Recv, args present, method not found
	recvFn.name = "Nope"
	got = co.CallOp(fr, &ssa.CallCommon{Value: recvFn, Args: []ssa.Value{recvArg}}, ext, call)
	if got.Interface() != "fallback" {
		t.Fatalf("missing method fallback %v", got.Interface())
	}

	// Call ssa.Value env nil
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected missing env panic")
			}
		}()
		co.Call(fr, token.NoPos, createMockSSAValue("absent"), nil, nil, nil, ext)
	}()

	// Call ssa.Value whose Interface is reflect.Value
	key := createMockSSAValue("rv")
	inner := reflect.ValueOf(func() int { return 11 })
	wrapped := value.Value(value.RValue{Value: reflect.ValueOf(inner)})
	fr.env[key] = &wrapped
	got = co.Call(fr, token.NoPos, key, nil, nil, nil, ext)
	if got.Int() != 11 {
		t.Fatalf("reflect.Value env %v", got.Interface())
	}
}

func TestUnaryEvaluateRemaining(t *testing.T) {
	uo := NewUnaryOperations()
	conv := func(v interface{}, typ types.Type) value.Value { return value.ValueOf(v) }
	mustPanicOp := func(name string, op token.Token, x value.Value) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatalf("%s: expected panic", name)
			}
		}()
		uo.Execute(&ssa.UnOp{Op: op}, x, conv)
	}
	mustPanicOp("uint not", token.NOT, value.ValueOf(uint(1)))
	mustPanicOp("float xor", token.XOR, value.ValueOf(1.5))
	mustPanicOp("bool sub", token.SUB, value.ValueOf(true))

	co := NewConstantOperations()
	zero := func(t types.Type) value.Value {
		return value.RValue{Value: reflect.New(typeOfTypes(t))}
	}
	// typed nil
	c := ssa.NewConst(nil, types.NewPointer(types.Typ[types.Int]))
	_ = co.Evaluate(c, zero, conv)

	cx := constant.BinaryOp(constant.MakeFloat64(1), token.ADD, constant.MakeImag(constant.MakeFloat64(2)))
	_ = co.Evaluate(ssa.NewConst(cx, types.Typ[types.Complex128]), zero, conv)

	// string from rune (non-string constant payload)
	_ = co.Evaluate(ssa.NewConst(constant.MakeInt64(65), types.Typ[types.String]), zero, conv)

	defer func() {
		if recover() == nil {
			t.Fatal("expected const panic")
		}
	}()
	_ = co.Evaluate(ssa.NewConst(constant.MakeInt64(1), types.Typ[types.UnsafePointer]), zero, conv)
}

func typeOfTypes(t types.Type) reflect.Type {
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		return reflect.PointerTo(typeOfTypes(u.Elem()))
	case *types.Basic:
		switch u.Kind() {
		case types.Int:
			return reflect.TypeOf(0)
		case types.String:
			return reflect.TypeOf("")
		case types.Complex128:
			return reflect.TypeOf(complex128(0))
		}
	}
	return reflect.TypeOf((*interface{})(nil)).Elem()
}
