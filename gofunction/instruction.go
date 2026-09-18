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

func runAlloc(fr *frame, instr *ssa.Alloc) nextInstr {
	var addr *value.Value
	if instr.Heap {
		// 堆分配
		addr = new(value.Value)
		fr.env[instr] = addr
	} else {
		// 栈分配
		addr = fr.env[instr]
	}
	*addr = zero(deref(instr.Type()))
	return _NEXT
}

func runUnOp(fr *frame, instr *ssa.UnOp) nextInstr {
	v := unop(instr, fr.get(instr.X))
	fr.set(instr, v)
	return _NEXT
}

func runBinOp(fr *frame, instr *ssa.BinOp) nextInstr {
	v := binop(instr, fr.get(instr.X), fr.get(instr.Y))
	fr.set(instr, v)
	return _NEXT
}

func runMakeInterface(fr *frame, instr *ssa.MakeInterface) nextInstr {
	v := fr.get(instr.X)
	fr.set(instr, v)
	return _NEXT
}

func runReturn(fr *frame, instr *ssa.Return) nextInstr {
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
	return _Return
}

func runIndexAddr(fr *frame, instr *ssa.IndexAddr) nextInstr {
	x := containerValue(fr.get(instr.X))
	idx := int(fr.get(instr.Index).Int())
	fr.set(instr, value.RValue{Value: x.Index(idx).Addr()})
	return _NEXT
}

func runIndex(fr *frame, instr *ssa.Index) nextInstr {
	x := containerValue(fr.get(instr.X))
	idx := int(fr.get(instr.Index).Int())
	fr.set(instr, value.RValue{Value: x.Index(idx)})
	return _NEXT
}

func runField(fr *frame, instr *ssa.Field) nextInstr {
	x := fr.get(instr.X)
	fr.set(instr, x.Field(instr.Field))
	return _NEXT
}

func runFieldAddr(fr *frame, instr *ssa.FieldAddr) nextInstr {
	x := fr.get(instr.X).Elem()
	v := x.RValue().Field(instr.Field).Addr()
	fr.set(instr, value.RValue{Value: v})
	return _NEXT
}

func runStore(fr *frame, instr *ssa.Store) nextInstr {
	val := fr.get(instr.Val)
	switch addr := instr.Addr.(type) {
	case *ssa.Alloc, *ssa.FreeVar:
		p, ok := fr.env[addr]
		if !ok || p == nil {
			panic(fmt.Sprintf("store: no address for %T: %v", addr, addr.Name()))
		}
		(*p).Elem().Set(val)
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
	return _NEXT
}

func runSlice(fr *frame, instr *ssa.Slice) nextInstr {
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
	return _NEXT
}

func runCall(fr *frame, instr *ssa.Call) nextInstr {
	v := callOp(fr, instr.Common())
	fr.env[instr] = &v
	return _NEXT
}

func runMakeSlice(fr *frame, instr *ssa.MakeSlice) nextInstr {
	sliceLen := int(fr.get(instr.Len).Int())
	sliceCap := int(fr.get(instr.Cap).Int())
	fr.set(instr, value.RValue{Value: reflect.MakeSlice(typeChange(instr.Type()), sliceLen, sliceCap)})
	return _NEXT
}

func runMakeMap(fr *frame, instr *ssa.MakeMap) nextInstr {
	size := 0
	if instr.Reserve != nil {
		size = int(fr.get(instr.Reserve).Int())
	}
	fr.set(instr, value.RValue{Value: reflect.MakeMapWithSize(typeChange(instr.Type()), size)})
	return _NEXT
}

func runMapUpdate(fr *frame, instr *ssa.MapUpdate) nextInstr {
	m := containerValue(fr.get(instr.Map))
	key := fr.get(instr.Key)
	v := fr.get(instr.Value)
	m.SetMapIndex(key.RValue(), v.RValue())
	return _NEXT
}

func runLookup(fr *frame, instr *ssa.Lookup) nextInstr {
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
		return _NEXT
	}
	fr.set(instr, value.RValue{Value: x.Index(int(index.Int()))})
	return _NEXT
}

func runExtract(fr *frame, instr *ssa.Extract) nextInstr {
	tuple := fr.get(instr.Tuple)
	item := tuple.Index(instr.Index)
	if v, ok := item.Interface().(value.Value); ok {
		fr.set(instr, v)
		return _NEXT
	}
	fr.set(instr, item)
	return _NEXT
}

func runIf(fr *frame, instr *ssa.If) nextInstr {
	succ := 1
	if fr.get(instr.Cond).Bool() {
		succ = 0
	}
	fr.prevBlock, fr.block = fr.block, fr.block.Succs[succ]
	return _JUMP
}

func runJump(fr *frame, instr *ssa.Jump) nextInstr {
	fr.prevBlock, fr.block = fr.block, fr.block.Succs[0]
	return _JUMP
}

func runPhi(fr *frame, instr *ssa.Phi) nextInstr {
	for i, pred := range instr.Block().Preds {
		if fr.prevBlock == pred {
			fr.set(instr, fr.get(instr.Edges[i]))
			break
		}
	}
	return _NEXT
}

func runConvert(fr *frame, instr *ssa.Convert) nextInstr {
	fr.set(instr, conv(fr.get(instr.X).Interface(), instr.Type()))
	return _NEXT
}

func runRange(fr *frame, instr *ssa.Range) nextInstr {
	v := fr.get(instr.X)
	rv := containerValue(v)
	fr.set(instr, &value.MapIter{
		I:     0,
		Value: value.NewRValueOf(rv),
		Keys:  rv.MapKeys(),
	})
	return _NEXT
}

func runNext(fr *frame, instr *ssa.Next) nextInstr {
	fr.set(instr, fr.get(instr.Iter).(*value.MapIter).Next())
	return _NEXT
}

func runChangeType(fr *frame, instr *ssa.ChangeType) nextInstr {
	fr.set(instr, fr.get(instr.X))
	return _NEXT
}

func runChangeInterface(fr *frame, instr *ssa.ChangeInterface) nextInstr {
	fr.set(instr, fr.get(instr.X))
	return _NEXT
}

func runMakeClosure(fr *frame, instr *ssa.MakeClosure) nextInstr {
	closure := fr.makeFunc(instr.Fn.(*ssa.Function), instr.Bindings)
	fr.set(instr, closure)
	return _NEXT
}

func runDefer(fr *frame, instr *ssa.Defer) nextInstr {
	fr.defers = append(fr.defers, instr)
	return _NEXT
}

func runRunDefers(fr *frame, instr *ssa.RunDefers) nextInstr {
	fr.runDefers()
	return _NEXT
}

func runMakeChan(fr *frame, instr *ssa.MakeChan) nextInstr {
	fr.set(instr, value.RValue{Value: reflect.MakeChan(typeChange(instr.Type()), int(fr.get(instr.Size).Int()))})
	return _NEXT
}

func runSend(fr *frame, instr *ssa.Send) nextInstr {
	ch := containerValue(fr.get(instr.Chan))
	ch.Send(fr.get(instr.X).RValue())
	return _NEXT
}

func runTypeAssert(fr *frame, instr *ssa.TypeAssert) nextInstr {
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
	return _NEXT
}

func runGo(fr *frame, instr *ssa.Go) nextInstr {
	goCall(fr, instr.Common())
	return _NEXT
}

func runPanic(fr *frame, instr *ssa.Panic) nextInstr {
	panic(fr.get(instr.X).Interface())
}

func runSelect(fr *frame, instr *ssa.Select) nextInstr {
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
	return _NEXT
}
