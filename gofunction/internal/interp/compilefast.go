// 文件说明：compilefast.go — A1 槽位固化：指令操作数/结果槽位号编译期
// 捕获进执行闭包，运行期直接 env 切片索引，消除 fr.get/fr.set 的
// map[ssa.Value]int 哈希查找。
//
// 设计约束：
//   - 编译期解析 slot 的前提是「该值在本函数布局中注册过」。布局由
//     compileProgram → layoutOf 预热，函数结构此后不可变。
//   - 操作数可能是 *ssa.Const / *ssa.Global / *ssa.Function（不在布局内），
//     此时编译期整体回落慢路径（run 前缀通用函数），不做运行期判断。
//   - 结果槽位：产值指令必在布局中（slotOfMust 语义），编译期确定。
//   - 与 run 前缀通用实现语义完全一致；panic 位置信息仍由外层主循环
//     从 compiledBlock.instrs 取，不受此处拆分影响。
package interp

import (
	"fmt"
	"go/token"
	"go/types"
	"reflect"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

// slotGetter 是运行期取值的最小抽象：成功固化为切片索引读取，
// 失败（Const/Global/Function 等）整体走慢路径。
type slotGetter func(fr *frame) value.Value

// trySlot 返回 v 的槽位号；未注册（ok=false）时调用方回落慢路径。
func trySlot(l *funcLayout, v ssa.Value) (int, bool) {
	if l == nil || v == nil {
		return 0, false
	}
	s, ok := l.slots[v]
	return s, ok
}

// slotGetterOf 把槽位号固化为读取闭包。
func slotGetterOf(s int) func(fr *frame) value.Value {
	return func(fr *frame) value.Value { return fr.env[s] }
}

// constGetter 为常量操作数生成读取闭包（A3 常量烤入）：
// 基础类型编译期求值捕获（SSA 常量不可变）；复合零值每次新建
// （保持 constValueCached 语义——复合常量共享可变内存是语义变化）。
func constGetter(c *ssa.Const) func(fr *frame) value.Value {
	if isBasicType(c.Type()) {
		v := constValue(c)
		return func(*frame) value.Value { return v }
	}
	return func(fr *frame) value.Value { return fr.constValueCached(c) }
}

// operandGetter 解析单操作数：槽位 → 常量 → 慢路径（Global/Function
// 及其他跨函数值）。第三返回值表示生成了慢路径闭包。
func operandGetter(l *funcLayout, v ssa.Value) func(fr *frame) value.Value {
	if s, ok := trySlot(l, v); ok {
		return slotGetterOf(s)
	}
	if c, ok := v.(*ssa.Const); ok && c != nil {
		return constGetter(c)
	}
	key := v
	return func(fr *frame) value.Value { return fr.get(key) }
}

// compileUnOp 同 runUnOp：操作数经 getter 固化，结果槽位编译期解析。
func compileUnOp(instr *ssa.UnOp) instrStep {
	l := layoutOfInstr(instr)
	gx := operandGetter(l, instr.X)
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, unop(instr, gx(fr)))
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = unop(instr, gx(fr))
		return stepNext
	}
}

// compileBinOp A2 运算特化：编译期按 (Op, 操作数底层类型) 生成特化
// 闭包，运行期零 Kind×Op 双重分发，直达 convIntTyped/convUintTyped/
// convFloatTyped/boolValue。操作数读取经 A1 getter 固化；命名类型
// （PkgPath != ""）或 string/complex/移位回落通用 binop 链。
func compileBinOp(instr *ssa.BinOp) instrStep {
	l := layoutOfInstr(instr)
	gx := operandGetter(l, instr.X)
	gy := operandGetter(l, instr.Y)
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, binop(instr, gx(fr), gy(fr)))
			return stepNext
		}
	}
	if fast, ok := binopSpecialized(instr); ok {
		return func(fr *frame) step {
			if v, ok := fast(gx(fr), gy(fr)); ok {
				fr.env[s] = v
				return stepNext
			}
			fr.env[s] = binop(instr, gx(fr), gy(fr))
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = binop(instr, gx(fr), gy(fr))
		return stepNext
	}
}

// binopSpecialized 按 (Op, X 底层类型) 返回免分派的二元运算闭包。
// 仅覆盖预声明数值类型（含命名类型的底层为基础类型时仍走通用链，
// 保持类型标识语义——见 convIntTyped 的 PkgPath 守卫说明）。
func binopSpecialized(instr *ssa.BinOp) (func(x, y value.Value) (value.Value, bool), bool) {
	if instr.X == nil {
		return nil, false
	}
	bt, ok := instr.X.Type().Underlying().(*types.Basic)
	if !ok || bt.Info()&types.IsUntyped != 0 {
		return nil, false
	}
	rt := typeChangeCached(instr.Type())
	if rt.PkgPath() != "" {
		return nil, false
	}
	isInt := func() bool {
		switch bt.Kind() {
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
			return true
		}
		return false
	}
	isUint := func() bool {
		switch bt.Kind() {
		case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
			return true
		}
		return false
	}
	isFloat := func() bool {
		return bt.Kind() == types.Float32 || bt.Kind() == types.Float64
	}
	switch instr.Op {
	case token.ADD, token.SUB, token.MUL, token.QUO, token.REM,
		token.AND, token.OR, token.XOR, token.AND_NOT:
		if isInt() {
			op := instr.Op
			return func(x, y value.Value) (value.Value, bool) {
				a, b := x.Int(), y.Int()
				switch op {
				case token.ADD:
					return convIntTyped(a+b, rt), true
				case token.SUB:
					return convIntTyped(a-b, rt), true
				case token.MUL:
					return convIntTyped(a*b, rt), true
				case token.QUO:
					return convIntTyped(a/b, rt), true
				case token.REM:
					return convIntTyped(a%b, rt), true
				case token.AND:
					return convIntTyped(a&b, rt), true
				case token.OR:
					return convIntTyped(a|b, rt), true
				case token.XOR:
					return convIntTyped(a^b, rt), true
				case token.AND_NOT:
					return convIntTyped(a&^b, rt), true
				}
				return nil, false
			}, true
		}
		if isUint() {
			op := instr.Op
			return func(x, y value.Value) (value.Value, bool) {
				a, b := x.Uint(), y.Uint()
				switch op {
				case token.ADD:
					return convUintTyped(a+b, rt), true
				case token.SUB:
					return convUintTyped(a-b, rt), true
				case token.MUL:
					return convUintTyped(a*b, rt), true
				case token.QUO:
					return convUintTyped(a/b, rt), true
				case token.REM:
					return convUintTyped(a%b, rt), true
				case token.AND:
					return convUintTyped(a&b, rt), true
				case token.OR:
					return convUintTyped(a|b, rt), true
				case token.XOR:
					return convUintTyped(a^b, rt), true
				case token.AND_NOT:
					return convUintTyped(a&^b, rt), true
				}
				return nil, false
			}, true
		}
		if isFloat() {
			op := instr.Op
			return func(x, y value.Value) (value.Value, bool) {
				a, b := x.Float(), y.Float()
				switch op {
				case token.ADD:
					return convFloatTyped(a+b, rt), true
				case token.SUB:
					return convFloatTyped(a-b, rt), true
				case token.MUL:
					return convFloatTyped(a*b, rt), true
				case token.QUO:
					return convFloatTyped(a/b, rt), true
				}
				return nil, false
			}, true
		}
	case token.LSS, token.LEQ, token.GTR, token.GEQ:
		if isInt() {
			op := instr.Op
			return func(x, y value.Value) (value.Value, bool) {
				a, b := x.Int(), y.Int()
				switch op {
				case token.LSS:
					return boolValue(a < b), true
				case token.LEQ:
					return boolValue(a <= b), true
				case token.GTR:
					return boolValue(a > b), true
				case token.GEQ:
					return boolValue(a >= b), true
				}
				return nil, false
			}, true
		}
		if isUint() {
			op := instr.Op
			return func(x, y value.Value) (value.Value, bool) {
				a, b := x.Uint(), y.Uint()
				switch op {
				case token.LSS:
					return boolValue(a < b), true
				case token.LEQ:
					return boolValue(a <= b), true
				case token.GTR:
					return boolValue(a > b), true
				case token.GEQ:
					return boolValue(a >= b), true
				}
				return nil, false
			}, true
		}
		if isFloat() {
			op := instr.Op
			return func(x, y value.Value) (value.Value, bool) {
				a, b := x.Float(), y.Float()
				switch op {
				case token.LSS:
					return boolValue(a < b), true
				case token.LEQ:
					return boolValue(a <= b), true
				case token.GTR:
					return boolValue(a > b), true
				case token.GEQ:
					return boolValue(a >= b), true
				}
				return nil, false
			}, true
		}
	case token.SHL, token.SHR:
		// 移位：左操作数决定结果类型，右操作数可为任意整数。
		// SSA 保证 instr.Type() == X 类型，结果类型特化安全。
		if isInt() {
			op := instr.Op
			return func(x, y value.Value) (value.Value, bool) {
				a := x.Int()
				b := shiftBy(y)
				if op == token.SHL {
					return convIntTyped(a<<b, rt), true
				}
				return convIntTyped(a>>b, rt), true
			}, true
		}
		if isUint() {
			op := instr.Op
			return func(x, y value.Value) (value.Value, bool) {
				a := x.Uint()
				b := shiftBy(y)
				if op == token.SHL {
					return convUintTyped(a<<b, rt), true
				}
				return convUintTyped(a>>b, rt), true
			}, true
		}
	}
	return nil, false
}

// shiftBy 按右操作数实际类型取移位数（Go 语义：可为任意整数类型）。
func shiftBy(y value.Value) uint64 {
	switch y.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return y.Uint() & 63
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return uint64(y.Int()) & 63
	}
	return 0
}

func compileMakeInterface(instr *ssa.MakeInterface) instrStep {
	l := layoutOfInstr(instr)
	gx := operandGetter(l, instr.X)
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, gx(fr))
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = gx(fr)
		return stepNext
	}
}

func compileReturn(instr *ssa.Return) instrStep {
	switch len(instr.Results) {
	case 0:
		return func(fr *frame) step {
			fr.block = nil
			return stepReturn
		}
	case 1:
		l := layoutOfInstr(instr)
		g := operandGetter(l, instr.Results[0])
		return func(fr *frame) step {
			fr.result = g(fr)
			fr.block = nil
			return stepReturn
		}
	default:
		l := layoutOfInstr(instr)
		gs := make([]func(fr *frame) value.Value, len(instr.Results))
		for i, r := range instr.Results {
			gs[i] = operandGetter(l, r)
		}
		return func(fr *frame) step {
			res := make([]value.Value, len(gs))
			for i, g := range gs {
				res[i] = g(fr)
			}
			fr.result = value.ValueOf(res)
			fr.block = nil
			return stepReturn
		}
	}
}

func compileIndexAddr(instr *ssa.IndexAddr) instrStep {
	l := layoutOfInstr(instr)
	gx := operandGetter(l, instr.X)
	gi := operandGetter(l, instr.Index)
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, value.RValue{Value: containerValue(gx(fr)).Index(int(gi(fr).Int())).Addr()})
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = value.RValue{Value: containerValue(gx(fr)).Index(int(gi(fr).Int())).Addr()}
		return stepNext
	}
}

func compileIndex(instr *ssa.Index) instrStep {
	l := layoutOfInstr(instr)
	gx := operandGetter(l, instr.X)
	gi := operandGetter(l, instr.Index)
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, value.RValue{Value: containerValue(gx(fr)).Index(int(gi(fr).Int()))})
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = value.RValue{Value: containerValue(gx(fr)).Index(int(gi(fr).Int()))}
		return stepNext
	}
}

func compileField(instr *ssa.Field) instrStep {
	l := layoutOfInstr(instr)
	gx := operandGetter(l, instr.X)
	idx := instr.Field
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, gx(fr).Field(idx))
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = gx(fr).Field(idx)
		return stepNext
	}
}

func compileFieldAddr(instr *ssa.FieldAddr) instrStep {
	l := layoutOfInstr(instr)
	gx := operandGetter(l, instr.X)
	idx := instr.Field
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, value.RValue{Value: gx(fr).Elem().RValue().Field(idx).Addr()})
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = value.RValue{Value: gx(fr).Elem().RValue().Field(idx).Addr()}
		return stepNext
	}
}

// compileStore 槽位固化 Store。Addr 为 Alloc/FreeVar（局部变量地址）
// 时读取与写入都走切片索引；Global/IndexAddr 保持原分支逻辑，
// 仅 Val 经 getter 固化。
func compileStore(instr *ssa.Store) instrStep {
	l := layoutOfInstr(instr)
	gv := operandGetter(l, instr.Val)
	switch addr := instr.Addr.(type) {
	case *ssa.Alloc, *ssa.FreeVar:
		ps, ok := trySlot(l, instr.Addr)
		if !ok {
			return func(fr *frame) step { return runStore(fr, instr) }
		}
		gp := slotGetterOf(ps)
		return func(fr *frame) step {
			p := gp(fr)
			if p == nil || !p.IsValid() {
				panic(fmt.Sprintf("store: no address for %T: %v", addr, addr.Name()))
			}
			ptr := p.RValue()
			elem := ptr.Elem()
			src := gv(fr).RValue()
			if src.Type().AssignableTo(elem.Type()) {
				elem.Set(src)
			} else {
				elem.Set(src.Convert(elem.Type()))
			}
			return stepNext
		}
	default:
		key := instr.Addr
		return func(fr *frame) step {
			val := gv(fr)
			switch addr := key.(type) {
			case *ssa.Global:
				fr.program.globalsMu.RLock()
				cell, ok := fr.program.globals[addr]
				fr.program.globalsMu.RUnlock()
				if !ok || cell == nil {
					panic(fmt.Sprintf("store: global %s not initialized", addr.Name()))
				}
				dst := (*cell).RValue()
				src := val.RValue()
				if dst.Kind() == reflect.Ptr {
					if src.Type().AssignableTo(dst.Elem().Type()) {
						dst.Elem().Set(src)
					} else {
						dst.Elem().Set(src.Convert(dst.Elem().Type()))
					}
				} else {
					fr.program.globalsMu.Lock()
					fr.program.globals[addr] = &val
					fr.program.globalsMu.Unlock()
				}
			case *ssa.IndexAddr:
				ga := operandGetter(l, addr.Index)
				gx := operandGetter(l, addr.X)
				index := int(ga(fr).Int())
				x := containerValue(gx(fr))
				x.Index(index).Set(val.RValue())
			default:
				addrVal := fr.get(key)
				if addrVal == nil || !addrVal.IsValid() {
					panic(fmt.Sprintf("store: no address for %T: %v", key, key.Name()))
				}
				rv := addrVal.RValue()
				if rv.Kind() != reflect.Ptr {
					panic(fmt.Sprintf("store: address is not pointer: %s", rv.Kind()))
				}
				rv.Elem().Set(val.RValue())
			}
			return stepNext
		}
	}
}

func compileMakeSlice(instr *ssa.MakeSlice) instrStep {
	l := layoutOfInstr(instr)
	gl := operandGetter(l, instr.Len)
	gc := operandGetter(l, instr.Cap)
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, value.RValue{Value: reflect.MakeSlice(typeChange(instr.Type()), int(gl(fr).Int()), int(gc(fr).Int()))})
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = value.RValue{Value: reflect.MakeSlice(typeChange(instr.Type()), int(gl(fr).Int()), int(gc(fr).Int()))}
		return stepNext
	}
}

func compileMakeMap(instr *ssa.MakeMap) instrStep {
	l := layoutOfInstr(instr)
	s, ok := trySlot(l, instr)
	if instr.Reserve == nil {
		if !ok {
			return func(fr *frame) step {
				fr.set(instr, value.RValue{Value: reflect.MakeMap(typeChange(instr.Type()))})
				return stepNext
			}
		}
		return func(fr *frame) step {
			fr.env[s] = value.RValue{Value: reflect.MakeMap(typeChange(instr.Type()))}
			return stepNext
		}
	}
	gr := operandGetter(l, instr.Reserve)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, value.RValue{Value: reflect.MakeMapWithSize(typeChange(instr.Type()), int(gr(fr).Int()))})
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = value.RValue{Value: reflect.MakeMapWithSize(typeChange(instr.Type()), int(gr(fr).Int()))}
		return stepNext
	}
}

func compileMapUpdate(instr *ssa.MapUpdate) instrStep {
	l := layoutOfInstr(instr)
	gm := operandGetter(l, instr.Map)
	gk := operandGetter(l, instr.Key)
	gv := operandGetter(l, instr.Value)
	return func(fr *frame) step {
		m := containerValue(gm(fr))
		m.SetMapIndex(gk(fr).RValue(), gv(fr).RValue())
		return stepNext
	}
}

func compileExtract(instr *ssa.Extract) instrStep {
	l := layoutOfInstr(instr)
	gt := operandGetter(l, instr.Tuple)
	idx := instr.Index
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			item := gt(fr).Index(idx)
			if v, ok := item.Interface().(value.Value); ok {
				fr.set(instr, v)
				return stepNext
			}
			fr.set(instr, item)
			return stepNext
		}
	}
	return func(fr *frame) step {
		item := gt(fr).Index(idx)
		if v, ok := item.Interface().(value.Value); ok {
			fr.env[s] = v
			return stepNext
		}
		fr.env[s] = item
		return stepNext
	}
}

func compileConvert(instr *ssa.Convert) instrStep {
	l := layoutOfInstr(instr)
	gx := operandGetter(l, instr.X)
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, convSlow(gx(fr).Interface(), instr.Type()))
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = convSlow(gx(fr).Interface(), instr.Type())
		return stepNext
	}
}

func compileNext(instr *ssa.Next) instrStep {
	l := layoutOfInstr(instr)
	gi := operandGetter(l, instr.Iter)
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, gi(fr).(*value.MapIter).Next())
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = gi(fr).(*value.MapIter).Next()
		return stepNext
	}
}

func compileChangeType(instr *ssa.ChangeType) instrStep {
	l := layoutOfInstr(instr)
	gx := operandGetter(l, instr.X)
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, gx(fr))
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = gx(fr)
		return stepNext
	}
}

func compileChangeInterface(instr *ssa.ChangeInterface) instrStep {
	l := layoutOfInstr(instr)
	gx := operandGetter(l, instr.X)
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, gx(fr))
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = gx(fr)
		return stepNext
	}
}

func compileMakeChan(instr *ssa.MakeChan) instrStep {
	l := layoutOfInstr(instr)
	gs := operandGetter(l, instr.Size)
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step {
			fr.set(instr, value.RValue{Value: reflect.MakeChan(typeChange(instr.Type()), int(gs(fr).Int()))})
			return stepNext
		}
	}
	return func(fr *frame) step {
		fr.env[s] = value.RValue{Value: reflect.MakeChan(typeChange(instr.Type()), int(gs(fr).Int()))}
		return stepNext
	}
}

func compileSend(instr *ssa.Send) instrStep {
	l := layoutOfInstr(instr)
	gc := operandGetter(l, instr.Chan)
	gx := operandGetter(l, instr.X)
	return func(fr *frame) step {
		containerValue(gc(fr)).Send(gx(fr).RValue())
		return stepNext
	}
}

func compilePanic(instr *ssa.Panic) instrStep {
	l := layoutOfInstr(instr)
	gx := operandGetter(l, instr.X)
	return func(fr *frame) step {
		panic(gx(fr).Interface())
	}
}

// compileAlloc 同 runAlloc（仅一条指令体，直接复用逻辑并固化槽位写入）。
func compileAlloc(instr *ssa.Alloc) instrStep {
	l := layoutOfInstr(instr)
	s, ok := trySlot(l, instr)
	if !ok {
		return func(fr *frame) step { return runAlloc(fr, instr) }
	}
	elemType := deref(instr.Type())
	return func(fr *frame) step {
		fr.env[s] = zero(elemType)
		return stepNext
	}
}

// layoutOfInstr 取指令所属函数的布局。compileInstr 在 compileProgram
// 预热阶段被调用（此前 layoutOf 已完成），惰性编译路径同样先建布局；
// 无所属函数（测试手工构造）时返回 nil，调用方整体走慢路径。
func layoutOfInstr(instr ssa.Instruction) *funcLayout {
	if instr == nil || instr.Parent() == nil {
		return nil
	}
	prog := programOf(instr.Parent())
	if prog == nil {
		return nil
	}
	return prog.layoutOf(instr.Parent())
}

// programOf 返回 SSA 函数所属的 Program（经进程级注册表反查，
// 见 compile.go 的 programRegistry）。查不到返回 nil，调用方走慢路径。
func programOf(fn *ssa.Function) *Program {
	return lookupProgram(fn)
}
