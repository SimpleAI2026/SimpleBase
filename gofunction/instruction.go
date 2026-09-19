package gofunction

import (
	"fmt"
	"go/types"
	"reflect"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

// containerValue 取出容器（slice/array/map/string/chan）的 reflect.Value。
// SSA 中部分指令的操作数是指针（*array、*map 局部变量），需要解引用一次。
func containerValue(v value.Value) reflect.Value {
	rv := v.RValue()
	for rv.IsValid() && rv.Kind() == reflect.Interface {
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	return rv
}

func runAlloc(fr *frame, instr *ssa.Alloc) step {
	// 槽内存放 *T 地址值（reflect.New 产物）：栈/堆统一，
	// 共享与修改经 Elem() 完成（P3-1：指针语义下沉 reflect 层）
	fr.env[fr.layout.slotOfMust(instr)] = zero(deref(instr.Type()))
	return stepNext
}

func runUnOp(fr *frame, instr *ssa.UnOp) step {
	v := unop(instr, fr.get(instr.X))
	fr.set(instr, v)
	return stepNext
}

func runBinOp(fr *frame, instr *ssa.BinOp) step {
	v := binop(instr, fr.get(instr.X), fr.get(instr.Y))
	fr.set(instr, v)
	return stepNext
}

func runMakeInterface(fr *frame, instr *ssa.MakeInterface) step {
	v := fr.get(instr.X)
	fr.set(instr, v)
	return stepNext
}

func runReturn(fr *frame, instr *ssa.Return) step {
	switch len(instr.Results) {
	case 0:
	case 1:
		fr.result = fr.get(instr.Results[0])
	default:
		// 返回多值时，对返回值进行打包
		var res []value.Value
		for _, r := range instr.Results {
			res = append(res, fr.get(r))
		}
		fr.result = value.ValueOf(res)
	}
	fr.block = nil
	return stepReturn
}

func runIndexAddr(fr *frame, instr *ssa.IndexAddr) step {
	x := containerValue(fr.get(instr.X))
	idx := int(fr.get(instr.Index).Int())
	fr.set(instr, value.RValue{Value: x.Index(idx).Addr()})
	return stepNext
}

func runIndex(fr *frame, instr *ssa.Index) step {
	x := containerValue(fr.get(instr.X))
	idx := int(fr.get(instr.Index).Int())
	fr.set(instr, value.RValue{Value: x.Index(idx)})
	return stepNext
}

func runField(fr *frame, instr *ssa.Field) step {
	x := fr.get(instr.X)
	fr.set(instr, x.Field(instr.Field))
	return stepNext
}

func runFieldAddr(fr *frame, instr *ssa.FieldAddr) step {
	x := fr.get(instr.X).Elem()
	v := x.RValue().Field(instr.Field).Addr()
	fr.set(instr, value.RValue{Value: v})
	return stepNext
}

func runStore(fr *frame, instr *ssa.Store) step {
	val := fr.get(instr.Val)
	switch addr := instr.Addr.(type) {
	case *ssa.Alloc, *ssa.FreeVar:
		// 槽内存放 *T 地址值；直接走 reflect 写入，
		// 避免 p.Elem() 的接口包装分配（剖析 ~8% alloc）
		p := fr.get(addr)
		if p == nil || !p.IsValid() {
			panic(fmt.Sprintf("store: no address for %T: %v", addr, addr.Name()))
		}
		ptr := p.RValue()
		elem := ptr.Elem()
		src := val.RValue()
		if src.Type().AssignableTo(elem.Type()) {
			elem.Set(src)
		} else {
			elem.Set(src.Convert(elem.Type()))
		}
	case *ssa.Global:
		cell, ok := fr.program.globals[addr]
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
			fr.program.globals[addr] = &val
		}
	case *ssa.IndexAddr:
		index := int(fr.get(addr.Index).Int())
		x := containerValue(fr.get(addr.X))
		x.Index(index).Set(val.RValue())
	default:
		// 按指针写入（FieldAddr、UnOp 取址等）
		addrVal := fr.get(instr.Addr)
		if addrVal == nil || !addrVal.IsValid() {
			panic(fmt.Sprintf("store: no address for %T: %v", addr, addr.Name()))
		}
		rv := addrVal.RValue()
		if rv.Kind() != reflect.Ptr {
			panic(fmt.Sprintf("store: address is not pointer: %s", rv.Kind()))
		}
		rv.Elem().Set(val.RValue())
	}
	return stepNext
}

func runSlice(fr *frame, instr *ssa.Slice) step {
	x := containerValue(fr.get(instr.X))
	l, h := 0, x.Len()
	low := fr.get(instr.Low)
	if low != nil {
		l = int(low.Int())
	}
	high := fr.get(instr.High)
	if high != nil {
		h = int(high.Int())
	}
	max := fr.get(instr.Max)
	if max != nil {
		fr.set(instr, value.RValue{Value: x.Slice3(l, h, int(max.Int()))})
	} else {
		fr.set(instr, value.RValue{Value: x.Slice(l, h)})
	}
	return stepNext
}

func runCall(fr *frame, instr *ssa.Call) step {
	v := callOp(fr, instr.Common())
	fr.set(instr, v)
	return stepNext
}

func runMakeSlice(fr *frame, instr *ssa.MakeSlice) step {
	sliceLen := int(fr.get(instr.Len).Int())
	sliceCap := int(fr.get(instr.Cap).Int())
	fr.set(instr, value.RValue{Value: reflect.MakeSlice(typeChange(instr.Type()), sliceLen, sliceCap)})
	return stepNext
}

func runMakeMap(fr *frame, instr *ssa.MakeMap) step {
	size := 0
	if instr.Reserve != nil {
		size = int(fr.get(instr.Reserve).Int())
	}
	fr.set(instr, value.RValue{Value: reflect.MakeMapWithSize(typeChange(instr.Type()), size)})
	return stepNext
}

func runMapUpdate(fr *frame, instr *ssa.MapUpdate) step {
	m := containerValue(fr.get(instr.Map))
	key := fr.get(instr.Key)
	v := fr.get(instr.Value)
	m.SetMapIndex(key.RValue(), v.RValue())
	return stepNext
}

func runLookup(fr *frame, instr *ssa.Lookup) step {
	x := containerValue(fr.get(instr.X))
	index := fr.get(instr.Index)
	if x.Kind() == reflect.Map {
		var v value.Value = value.RValue{Value: x.MapIndex(index.RValue())}
		ok := true
		if !v.IsValid() {
			v = value.RValue{Value: reflect.Zero(x.Type().Elem())}
			ok = false
		}
		if instr.CommaOk {
			v = value.ValueOf([]value.Value{v, value.ValueOf(ok)})
		}
		fr.set(instr, v)
		return stepNext
	}
	fr.set(instr, value.RValue{Value: x.Index(int(index.Int()))})
	return stepNext
}

func runExtract(fr *frame, instr *ssa.Extract) step {
	tuple := fr.get(instr.Tuple)
	item := tuple.Index(instr.Index)
	if v, ok := item.Interface().(value.Value); ok {
		fr.set(instr, v)
		return stepNext
	}
	fr.set(instr, item)
	return stepNext
}

func runIf(fr *frame, instr *ssa.If) step {
	succ := 1
	if fr.get(instr.Cond).Bool() {
		succ = 0
	}
	fr.prevBlock, fr.block = fr.block, fr.block.Succs[succ]
	return stepJump
}

func runJump(fr *frame, instr *ssa.Jump) step {
	fr.prevBlock, fr.block = fr.block, fr.block.Succs[0]
	return stepJump
}

func runPhi(fr *frame, instr *ssa.Phi) step {
	for i, pred := range instr.Block().Preds {
		if fr.prevBlock == pred {
			fr.set(instr, fr.get(instr.Edges[i]))
			break
		}
	}
	return stepNext
}

func runConvert(fr *frame, instr *ssa.Convert) step {
	fr.set(instr, conv(fr.get(instr.X).Interface(), instr.Type()))
	return stepNext
}

func runRange(fr *frame, instr *ssa.Range) step {
	v := fr.get(instr.X)
	rv := containerValue(v)
	switch rv.Kind() {
	case reflect.Map:
		fr.set(instr, &value.MapIter{
			Value: value.NewRValueOf(rv),
			Keys:  rv.MapKeys(),
		})
	case reflect.String:
		// string 按 rune 解码迭代（Go range 语义）
		fr.set(instr, value.NewStringIter(rv.String()))
	default:
		// slice/array 走通用序列迭代器
		fr.set(instr, value.NewSliceIter(v))
	}
	return stepNext
}

func runNext(fr *frame, instr *ssa.Next) step {
	fr.set(instr, fr.get(instr.Iter).(*value.MapIter).Next())
	return stepNext
}

func runChangeType(fr *frame, instr *ssa.ChangeType) step {
	fr.set(instr, fr.get(instr.X))
	return stepNext
}

func runChangeInterface(fr *frame, instr *ssa.ChangeInterface) step {
	fr.set(instr, fr.get(instr.X))
	return stepNext
}

func runMakeClosure(fr *frame, instr *ssa.MakeClosure) step {
	closure := fr.makeFunc(instr.Fn.(*ssa.Function), instr.Bindings)
	fr.set(instr, closure)
	return stepNext
}

func runDefer(fr *frame, instr *ssa.Defer) step {
	fr.defers = append(fr.defers, instr)
	return stepNext
}

func runRunDefers(fr *frame, instr *ssa.RunDefers) step {
	fr.runDefers()
	return stepNext
}

func runMakeChan(fr *frame, instr *ssa.MakeChan) step {
	fr.set(instr, value.RValue{Value: reflect.MakeChan(typeChange(instr.Type()), int(fr.get(instr.Size).Int()))})
	return stepNext
}

func runSend(fr *frame, instr *ssa.Send) step {
	ch := containerValue(fr.get(instr.Chan))
	ch.Send(fr.get(instr.X).RValue())
	return stepNext
}

func runTypeAssert(fr *frame, instr *ssa.TypeAssert) step {
	v := fr.get(instr.X)
	for v.IsValid() && v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	destType := typeChange(instr.AssertedType)

	var assignable bool
	if !v.IsValid() || v.Kind() == reflect.Invalid {
		assignable = false
	} else {
		assignable = v.Type().AssignableTo(destType)
	}

	switch {
	case instr.CommaOk && assignable:
		fr.set(instr, value.ValueOf([]value.Value{v, value.ValueOf(true)}))

	case instr.CommaOk && !assignable:
		fr.set(instr, value.ValueOf([]value.Value{value.RValue{Value: reflect.Zero(destType)}, value.ValueOf(false)}))

	case !instr.CommaOk && assignable:
		fr.set(instr, v)

	case !instr.CommaOk && !assignable:
		if !v.IsValid() || v.Kind() == reflect.Invalid {
			panic(fmt.Errorf("interface conversion: interface is nil, not %s", destType.String()))
		} else {
			panic(fmt.Errorf("interface conversion: interface is %s, not %s", v.Type().String(), destType.String()))
		}
	}
	return stepNext
}

func runGo(fr *frame, instr *ssa.Go) step {
	goCall(fr, instr.Common())
	return stepNext
}

func runPanic(fr *frame, instr *ssa.Panic) step {
	panic(fr.get(instr.X).Interface())
}

func runSelect(fr *frame, instr *ssa.Select) step {
	var cases []reflect.SelectCase
	if !instr.Blocking {
		cases = append(cases, reflect.SelectCase{
			Dir: reflect.SelectDefault,
		})
	}
	for _, state := range instr.States {
		var dir reflect.SelectDir
		if state.Dir == types.RecvOnly {
			dir = reflect.SelectRecv
		} else {
			dir = reflect.SelectSend
		}
		var send reflect.Value
		if state.Send != nil {
			send = fr.get(state.Send).RValue()
		}
		cases = append(cases, reflect.SelectCase{
			Dir:  dir,
			Chan: containerValue(fr.get(state.Chan)),
			Send: send,
		})
	}
	chosen, recv, recvOk := reflect.Select(cases)
	if !instr.Blocking {
		chosen-- // default case should have index -1.
	}
	r := []value.Value{value.ValueOf(chosen), value.ValueOf(recvOk)}
	for i, st := range instr.States {
		if st.Dir == types.RecvOnly {
			var v value.Value
			if i == chosen && recvOk {
				v = value.RValue{Value: recv}
			} else {
				v = zero(st.Chan.Type().Underlying().(*types.Chan).Elem())
			}
			r = append(r, v)
		}
	}
	fr.set(instr, value.ValueOf(r))
	return stepNext
}
