package gofunction

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"sync/atomic"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

// skipFastEval is a test-only seam so coverage can exercise the generic
// unop/binop fallbacks that production keeps behind the typed fast paths.
var skipFastEval bool

// unop 一元表达式求值
func unop(instr *ssa.UnOp, x value.Value) value.Value {
	if instr.Op == token.MUL {
		// 解引用：直接包装 Elem() 的 reflect.Value，
		// 跳过 Interface() 装箱 + ValueOf 反射再拆箱（剖析双重装箱 ~8% alloc）
		return value.RValue{Value: x.RValue().Elem()}
	}
	// 数值取负/取反/布尔取非快路径（免 interface{} 装箱）
	if !skipFastEval {
		switch x.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			switch instr.Op {
			case token.SUB:
				return convTyped(-x.Int(), instr.Type())
			case token.XOR:
				return convTyped(^x.Int(), instr.Type())
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			switch instr.Op {
			case token.SUB:
				return convTyped(-x.Uint(), instr.Type())
			case token.XOR:
				return convTyped(^x.Uint(), instr.Type())
			}
		case reflect.Float32, reflect.Float64:
			if instr.Op == token.SUB {
				return convTyped(-x.Float(), instr.Type())
			}
		case reflect.Bool:
			if instr.Op == token.NOT {
				return boolValue(!x.Bool())
			}
		}
	}
	var result interface{}
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch instr.Op {
		case token.SUB:
			result = -x.Int()
		case token.XOR:
			result = ^x.Int()
		default:
			panic(fmt.Sprintf("invalid unary op %s %T", instr.Op, x))
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		switch instr.Op {
		case token.SUB:
			result = -x.Uint()
		case token.XOR:
			result = ^x.Uint()
		default:
			panic(fmt.Sprintf("invalid unary op %s %T", instr.Op, x))
		}
	case reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		switch instr.Op {
		case token.SUB:
			result = -x.Float()
		default:
			panic(fmt.Sprintf("invalid unary op %s %T", instr.Op, x))
		}
	case reflect.Bool:
		switch instr.Op {
		case token.NOT:
			result = !x.Bool()
		default:
			panic(fmt.Sprintf("invalid unary op %s %T", instr.Op, x))
		}
	case reflect.Chan: // receive
		v, ok := x.RValue().Recv()
		if !ok {
			v = reflect.Zero(x.Type().Elem())
		}
		if instr.CommaOk {
			return value.ValueOf([]value.Value{value.NewRValueOf(v), value.ValueOf(ok)})
		}
		return value.NewRValueOf(v)
	}
	return conv(result, instr.Type())
}

// binop 二元表达式求值
func binop(instr *ssa.BinOp, x, y value.Value) value.Value {
	// 数值快路径：直接按目标类型产出，避免 interface{} 装箱 + 反射转换。
	// SSA 保证 BinOp 两操作数同类型，且运算结果类型即 instr.Type()。
	if !skipFastEval {
		if v, ok := binopFast(instr, x, y); ok {
			return v
		}
	}
	var result interface{}
	switch instr.Op {
	case token.ADD: // +
		switch x.Kind() {
		case reflect.String:
			result = x.String() + y.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() + y.Int()
		case reflect.Float32, reflect.Float64:
			result = x.Float() + y.Float()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() + y.Uint()
		}
	case token.SUB: // -
		switch x.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() - y.Int()
		case reflect.Float32, reflect.Float64:
			result = x.Float() - y.Float()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() - y.Uint()
		}
	case token.MUL: // *
		switch x.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() * y.Int()
		case reflect.Float32, reflect.Float64:
			result = x.Float() * y.Float()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() * y.Uint()
		}
	case token.QUO: // /
		switch x.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() / y.Int()
		case reflect.Float32, reflect.Float64:
			result = x.Float() / y.Float()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() / y.Uint()
		}
	case token.REM: // %
		switch x.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() % y.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() % y.Uint()
		}
	case token.AND: // &
		switch x.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() & y.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() & y.Uint()
		}
	case token.OR: // |
		switch x.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() | y.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() | y.Uint()
		}
	case token.XOR: // ^
		switch x.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() ^ y.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() ^ y.Uint()
		}
	case token.AND_NOT: // &^
		switch x.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() &^ y.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() &^ y.Uint()
		}
	case token.SHL: // <<
		switch x.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() << y.Uint()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() << y.Uint()
		}
	case token.SHR: // >>
		switch x.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() >> y.Uint()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() >> y.Uint()
		}
	case token.LSS: // <
		switch x.Kind() {
		case reflect.String:
			result = x.String() < y.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() < y.Int()
		case reflect.Float32, reflect.Float64:
			result = x.Float() < y.Float()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() < y.Uint()
		}
	case token.LEQ: // <=
		switch x.Kind() {
		case reflect.String:
			result = x.String() <= y.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() <= y.Int()
		case reflect.Float32, reflect.Float64:
			result = x.Float() <= y.Float()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() <= y.Uint()
		}
	case token.EQL: // ==
		if x.IsNil() || y.IsNil() {
			result = x.IsNil() && y.IsNil()
		} else {
			result = x.Interface() == y.Interface()
		}
	case token.NEQ: // !=
		if x.IsNil() || y.IsNil() {
			result = x.IsNil() != y.IsNil()
		} else {
			result = x.Interface() != y.Interface()
		}
	case token.GTR: // >
		switch x.Kind() {
		case reflect.String:
			result = x.String() > y.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() > y.Int()
		case reflect.Float32, reflect.Float64:
			result = x.Float() > y.Float()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() > y.Uint()
		}
	case token.GEQ: // >=
		switch x.Kind() {
		case reflect.String:
			result = x.String() >= y.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result = x.Int() >= y.Int()
		case reflect.Float32, reflect.Float64:
			result = x.Float() >= y.Float()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result = x.Uint() >= y.Uint()
		}
	}
	return conv(result, instr.Type())
}

// binopFast 数值运算免装箱快路径。
// 仅处理整型/浮点/无符号的四则与位运算及比较，结果类型严格按 instr.Type()
// 产出；不支持的组合返回 ok=false 交由通用路径。
func binopFast(instr *ssa.BinOp, x, y value.Value) (value.Value, bool) {
	// Go 移位语义：左操作数决定结果类型，右操作数可为任意整数类型（int 或 uint），
	// 因此 SHL/SHR 不能按 x.Kind() 统一取 y，须单独处理
	if instr.Op == token.SHL || instr.Op == token.SHR {
		return binopShiftFast(instr, x, y)
	}
	k := x.Kind()
	rt := typeChangeCached(instr.Type())
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		a, b := x.Int(), y.Int()
		var r int64
		switch instr.Op {
		case token.ADD:
			r = a + b
		case token.SUB:
			r = a - b
		case token.MUL:
			r = a * b
		case token.QUO:
			r = a / b
		case token.REM:
			r = a % b
		case token.AND:
			r = a & b
		case token.OR:
			r = a | b
		case token.XOR:
			r = a ^ b
		case token.AND_NOT:
			r = a &^ b
		default:
			return binopCompareFast(instr, x, y)
		}
		return convIntTyped(r, rt), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		a, b := x.Uint(), y.Uint()
		var r uint64
		switch instr.Op {
		case token.ADD:
			r = a + b
		case token.SUB:
			r = a - b
		case token.MUL:
			r = a * b
		case token.QUO:
			r = a / b
		case token.REM:
			r = a % b
		case token.AND:
			r = a & b
		case token.OR:
			r = a | b
		case token.XOR:
			r = a ^ b
		case token.AND_NOT:
			r = a &^ b
		default:
			return binopCompareFast(instr, x, y)
		}
		return convUintTyped(r, rt), true
	case reflect.Float32, reflect.Float64:
		a, b := x.Float(), y.Float()
		var r float64
		switch instr.Op {
		case token.ADD:
			r = a + b
		case token.SUB:
			r = a - b
		case token.MUL:
			r = a * b
		case token.QUO:
			r = a / b
		default:
			return binopCompareFast(instr, x, y)
		}
		return convFloatTyped(r, rt), true
	}
	return nil, false
}

// binopShiftFast 移位快路径：按左操作数类型产出，按右操作数实际类型取移位数
func binopShiftFast(instr *ssa.BinOp, x, y value.Value) (value.Value, bool) {
	shiftBy := func() uint64 {
		switch y.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return y.Uint() & 63
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return uint64(y.Int()) & 63
		}
		return 0
	}()
	rt := typeChangeCached(instr.Type())
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var r int64
		if instr.Op == token.SHL {
			r = x.Int() << shiftBy
		} else {
			r = x.Int() >> shiftBy
		}
		return convIntTyped(r, rt), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		var r uint64
		if instr.Op == token.SHL {
			r = x.Uint() << shiftBy
		} else {
			r = x.Uint() >> shiftBy
		}
		return convUintTyped(r, rt), true
	}
	return nil, false
}

// binopCompareFast 整型/浮点比较快路径（结果必为 bool）
func binopCompareFast(instr *ssa.BinOp, x, y value.Value) (value.Value, bool) {
	var r bool
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		a, b := x.Int(), y.Int()
		switch instr.Op {
		case token.LSS:
			r = a < b
		case token.LEQ:
			r = a <= b
		case token.GTR:
			r = a > b
		case token.GEQ:
			r = a >= b
		default:
			return nil, false
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		a, b := x.Uint(), y.Uint()
		switch instr.Op {
		case token.LSS:
			r = a < b
		case token.LEQ:
			r = a <= b
		case token.GTR:
			r = a > b
		case token.GEQ:
			r = a >= b
		default:
			return nil, false
		}
	case reflect.Float32, reflect.Float64:
		a, b := x.Float(), y.Float()
		switch instr.Op {
		case token.LSS:
			r = a < b
		case token.LEQ:
			r = a <= b
		case token.GTR:
			r = a > b
		case token.GEQ:
			r = a >= b
		default:
			return nil, false
		}
	default:
		return nil, false
	}
	return boolValue(r), true
}

// convTyped 将具体类型数值直接转换为指定 SSA 类型的 Value。
// 目标类型与天然类型一致时零转换开销；跨宽度（如 int32 存储 int64 运算结果）
// 时走一次 Convert。类型解析经缓存，热路径无重复分配。
func convTyped(v interface{}, typ types.Type) value.Value {
	rt := typeChangeCached(typ)
	rv := reflect.ValueOf(v)
	if rv.Type() == rt {
		return value.RValue{Value: rv}
	}
	return value.RValue{Value: rv.Convert(rt)}
}

// convIntTyped 把 int64 运算结果按目标 reflect.Kind 直接产出，避免
// interface{} 装箱与 reflect.Value.Convert 的额外分配（P3-4 覆盖全宽度）。
//
// 守卫 `rt.PkgPath() == ""` 不可省略：Go 中不存在「无名整型」，
// PkgPath 为空即唯一对应预声明类型；命名类型（time.Weekday/Duration 等，
// Kind 可能同为 Int/Int64）必须走 Convert 保持类型标识，
// 否则 switch 比较、方法集、接口断言全部失配（P2 阶段已踩坑两次）。
func convIntTyped(r int64, rt reflect.Type) value.Value {
	if rt.PkgPath() == "" {
		switch rt.Kind() {
		case reflect.Int:
			return intValue(r) // 小整数走单例缓存（P3-2）
		case reflect.Int64:
			return value.RValue{Value: reflect.ValueOf(r)}
		case reflect.Int32:
			return value.RValue{Value: reflect.ValueOf(int32(r))}
		case reflect.Int16:
			return value.RValue{Value: reflect.ValueOf(int16(r))}
		case reflect.Int8:
			return value.RValue{Value: reflect.ValueOf(int8(r))}
		}
	}
	return value.RValue{Value: reflect.ValueOf(r).Convert(rt)}
}

// convUintTyped 同 convIntTyped，用于无符号结果
func convUintTyped(r uint64, rt reflect.Type) value.Value {
	if rt.PkgPath() == "" {
		switch rt.Kind() {
		case reflect.Uint:
			return value.RValue{Value: reflect.ValueOf(uint(r))}
		case reflect.Uint64:
			return value.RValue{Value: reflect.ValueOf(r)}
		case reflect.Uint32:
			return value.RValue{Value: reflect.ValueOf(uint32(r))}
		case reflect.Uint16:
			return value.RValue{Value: reflect.ValueOf(uint16(r))}
		case reflect.Uint8:
			return value.RValue{Value: reflect.ValueOf(uint8(r))}
		case reflect.Uintptr:
			return value.RValue{Value: reflect.ValueOf(uintptr(r))}
		}
	}
	return value.RValue{Value: reflect.ValueOf(r).Convert(rt)}
}

// convFloatTyped 同 convIntTyped，用于浮点结果
func convFloatTyped(r float64, rt reflect.Type) value.Value {
	if rt.PkgPath() == "" {
		switch rt.Kind() {
		case reflect.Float64:
			return value.RValue{Value: reflect.ValueOf(r)}
		case reflect.Float32:
			return value.RValue{Value: reflect.ValueOf(float32(r))}
		}
	}
	return value.RValue{Value: reflect.ValueOf(r).Convert(rt)}
}

// constValue 常量表达式求值
func constValue(c *ssa.Const) value.Value {
	if c.IsNil() {
		return zero(c.Type()).Elem() // typed nil
	}
	basic, ok := c.Type().Underlying().(*types.Basic)
	if !ok {
		// 结构体/数组等复合类型的零值常量（如 Vertex{}）
		return zero(c.Type()).Elem()
	}
	var val interface{}
	switch basic.Kind() {
	case types.Bool, types.UntypedBool:
		val = constant.BoolVal(c.Value)
	case types.Int, types.UntypedInt, types.Int8, types.Int16, types.Int32, types.UntypedRune, types.Int64:
		val = c.Int64()
	case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
		val = c.Uint64()
	case types.Float32, types.Float64, types.UntypedFloat:
		val = c.Float64()
	case types.Complex64, types.Complex128, types.UntypedComplex:
		val = c.Complex128()
	case types.String, types.UntypedString:
		if c.Value != nil && c.Value.Kind() == constant.String {
			val = constant.StringVal(c.Value)
		} else {
			val = string(rune(c.Int64()))
		}
	default:
		panic(fmt.Sprintf("constValue: %s", c))
	}
	return conv(val, c.Type())
}

// callOp 函数调用语句执行
func callOp(fr *frame, instr *ssa.CallCommon) value.Value {
	sig := instr.Signature()
	if sig.Recv() == nil {
		// 参数切片必须就地 make：抽出辅助函数会触发「返回值逃逸」
		// 判定，使每次调用恒堆分配（P3-3 实测教训）。
		// 就地写法下逃逸分析可将其栈分配（fib18 全程 11 allocs）。
		args := make([]value.Value, len(instr.Args))
		for i, arg := range instr.Args {
			args[i] = fr.get(arg)
		}
		return call(fr, instr.Pos(), instr.Value, args)
	}

	// invoke Method
	if instr.IsInvoke() {
		recv := fr.get(instr.Value)
		args := make([]value.Value, len(instr.Args))
		for i := range args {
			args[i] = fr.get(instr.Args[i])
		}
		m := lookupMethod(recv.RValue(), instr.Method.Name())
		if !m.IsValid() {
			panic(fmt.Sprintf("method %s not found on %s", instr.Method.Name(), recv.Type()))
		}
		return callExternal(m, args)
	}

	args := make([]value.Value, len(instr.Args))
	for i, arg := range instr.Args {
		args[i] = fr.get(arg)
	}
	if len(args) == 0 {
		return call(fr, instr.Pos(), instr.Value, args)
	}
	m := lookupMethod(args[0].RValue(), instr.Value.Name())
	if m.IsValid() {
		return callExternal(m, args[1:])
	}
	return call(fr, instr.Pos(), instr.Value, args)
}

// goCall go语句执行
func goCall(fr *frame, instr *ssa.CallCommon) {
	if instr.Signature().Recv() != nil {
		recv := fr.get(instr.Args[0])
		m := lookupMethod(recv.RValue(), instr.Value.Name())
		if m.IsValid() {
			args := make([]value.Value, len(instr.Args)-1)
			for i := range args {
				args[i] = fr.get(instr.Args[i+1])
			}
			go callExternal(m, args)
			return
		}
	}

	args := make([]value.Value, len(instr.Args))
	for i, arg := range instr.Args {
		args[i] = fr.get(arg)
	}

	atomic.AddInt32(&fr.context.goroutines, 1)

	// Go 语义：goroutine 通过闭包捕获变量，与父帧的后续写入互不可见。
	// SSA 已将被捕获变量装箱（Alloc，槽位为地址值），仅需拷贝 env
	// 快照切片，子协程与父帧并发读写各自副本，无数据竞争。
	gf := *fr
	gf.env = make([]value.Value, len(fr.env))
	copy(gf.env, fr.env)

	go func(caller *frame, fn ssa.Value, args []value.Value) {
		defer func() {
			// 启动协程前添加recover语句，避免协程panic影响其他协程
			if re := recover(); re != nil {
				_, _ = caller.context.outBuffer.WriteString(fmt.Sprintf("goroutine panic: %v", re))
			}
			atomic.AddInt32(&caller.context.goroutines, -1)
		}()
		call(caller, instr.Pos(), fn, args)
	}(&gf, instr.Value, args)
}

// lookupMethod 在值或其指针上查找方法（指针接收者方法对 T 值也可用）
func lookupMethod(recv reflect.Value, name string) reflect.Value {
	if !recv.IsValid() {
		return reflect.Value{}
	}
	if m := recv.MethodByName(name); m.IsValid() {
		return m
	}
	if recv.Kind() != reflect.Ptr {
		if recv.CanAddr() {
			if m := recv.Addr().MethodByName(name); m.IsValid() {
				return m
			}
		}
		ptr := reflect.New(recv.Type())
		ptr.Elem().Set(recv)
		if m := ptr.MethodByName(name); m.IsValid() {
			return m
		}
	}
	return reflect.Value{}
}
