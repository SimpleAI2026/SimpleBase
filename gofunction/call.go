package gofunction

import (
	"fmt"
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

// callExternal 调用宿主函数（通过反射）
func callExternal(fn reflect.Value, args []value.Value) value.Value {
	if !fn.IsValid() || fn.Kind() != reflect.Func {
		panic(fmt.Sprintf("callExternal: not a function: %s", fn.String()))
	}
	fnType := fn.Type()
	numIn := fnType.NumIn()
	isVariadic := fnType.IsVariadic()
	if isVariadic {
		numIn--
	}
	if len(args) < numIn {
		panic(fmt.Sprintf("callExternal %s: got %d args, want %d", fnType, len(args), fnType.NumIn()))
	}
	in := make([]reflect.Value, numIn)
	for i := 0; i < numIn; i++ {
		in[i] = args[i].RValue().Convert(fnType.In(i))
	}
	if isVariadic {
		variadicType := fnType.In(numIn).Elem()
		if len(args) == numIn+1 && args[numIn].Kind() == reflect.Slice {
			variadicArgs := args[numIn]
			variadicLen := variadicArgs.Len()
			for i := 0; i < variadicLen; i++ {
				in = append(in, variadicArgs.Index(i).RValue().Convert(variadicType))
			}
		} else {
			for i := numIn; i < len(args); i++ {
				in = append(in, args[i].RValue().Convert(variadicType))
			}
		}
	}
	out := fn.Call(in)
	return value.Package(out)
}

// call 函数调用（分发到 ssa 函数 / 内置函数 / 外部函数）
func call(caller *frame, callpos token.Pos, fn interface{}, args []value.Value) value.Value {
	switch fun := fn.(type) {
	case *ssa.Function:
		if fun == nil {
			panic("call of nil function") // nil of func type
		}
		return callSSA(caller, fun, args, nil)
	case *ssa.Builtin:
		return callBuiltin(caller, callpos, fun, args)
	case *value.ExternalValue:
		return callExternal(fun.Object.Value, args)
	case reflect.Value:
		return callExternal(fun, args)
	case ssa.Value:
		// 动态函数值（存于槽位/全局/常量等），统一走 fr.get 取值
		f := caller.get(fun).Interface()
		if rv, ok := f.(reflect.Value); ok {
			return callExternal(rv, args)
		}
		return call(caller, callpos, f, args)
	default:
		rv := reflect.ValueOf(fun)
		if rv.Kind() == reflect.Interface {
			rv = rv.Elem()
		}
		return callExternal(rv, args)
	}
}

// framePool 复用已执行完的 frame 及其 env 槽位数组。
// frame 生命周期与单次函数调用一致且不再被引用后归还，池化可消除
// 每次调用的 frame{} + env 分配。
// 注意：goCall 对 env 做快照拷贝、子协程引用拷贝而非原 frame，
// 归还时机由 waitGoroutines 保证在脚本协程结束后，无悬挂引用风险。
var framePool sync.Pool

// getFrameFromPool 取出并重置一个可复用 frame
func getFrameFromPool(caller *frame, fn *ssa.Function) *frame {
	fr, _ := framePool.Get().(*frame)
	if fr == nil {
		fr = &frame{}
	}
	fr.program = caller.program
	fr.context = caller.context
	fr.caller = caller
	fr.fn = fn
	fr.seqid = caller.seqid
	fr.layout = caller.program.layoutOf(fn)
	// env 槽位数组：按布局大小复用容量（P3-1）
	fr.prepareEnv(fr.layout.nSlots)
	fr.block = nil
	fr.prevBlock = nil
	fr.defers = nil
	fr.result = nil
	fr.panicking = false
	fr.panic = nil
	return fr
}

// putFrameToPool 归还 frame。调用方须确保之后不再访问该 frame
// （当前唯一读取返回值的路径 fr.result 已在归还前拷贝）。
func putFrameToPool(fr *frame) {
	fr.program = nil
	fr.context = nil
	fr.caller = nil
	fr.fn = nil
	fr.layout = nil
	fr.block = nil
	fr.prevBlock = nil
	fr.defers = nil
	fr.result = nil
	clear(fr.env) // 释放槽位持有的引用
	framePool.Put(fr)
}

// callSSA 调用脚本内函数
func callSSA(caller *frame, fn *ssa.Function, args []value.Value, env []*value.Value) value.Value {
	// 外部函数（无函数体，来自宿主注册包）：从注册表查找实现并调用
	if len(fn.Blocks) == 0 {
		if ext := caller.program.externalFunction(fn); ext != nil {
			return callExternal(*ext, args)
		}
		if imported := caller.program.importedFunction(fn); imported != nil && imported != fn {
			return callSSA(caller, imported, args, env)
		}
		panic(fmt.Sprintf("no implementation for external function %s", fn.String()))
	}

	fr := getFrameFromPool(caller, fn)
	if len(fn.Blocks) > 0 {
		fr.block = fn.Blocks[0]
		// 入口基本块作为 prevBlock，保证首个 phi 能从入口边取值
		fr.prevBlock = fn.Blocks[0]
	}
	// 局部变量：SSA 中 Locals 均为 *T（Alloc），槽位存放 ptr 值
	// （reflect.New 产物），读写经 Elem() 共享 —— 指针语义天然保留
	for i, l := range fn.Locals {
		fr.env[fr.layout.localSlots[i]] = zero(deref(l.Type()))
	}
	for i := range fn.Params {
		if i >= len(args) {
			panic(fmt.Sprintf("call %s: missing argument %d", fn.String(), i))
		}
		fr.env[fr.layout.paramSlots[i]] = args[i]
	}
	for i, fv := range fn.FreeVars {
		if i >= len(env) {
			panic(fmt.Sprintf("call %s: missing free var %s", fn.String(), fv.Name()))
		}
		fr.env[fr.layout.freeVarSlots[i]] = *env[i]
	}
	// 与 x/tools SSA interp 一致：panic 恢复后可能进入 Recover 块，需再次执行
	for fr.block != nil {
		runFrameCompiled(fr)
	}
	result := fr.result
	putFrameToPool(fr)
	return result
}

// callBuiltin 调用内置函数
func callBuiltin(caller *frame, callPos token.Pos, fn *ssa.Builtin, args []value.Value) value.Value {
	switch fn.Name() {
	case "append":
		slice := args[0].RValue()
		if slice.Kind() == reflect.Ptr {
			slice = slice.Elem()
		}
		if len(args) == 1 {
			return value.RValue{Value: slice}
		}
		elems := make([]reflect.Value, 0)
		for i := 1; i < len(args); i++ {
			arg := args[i]
			if arg == nil || !arg.IsValid() || arg.IsNil() {
				continue
			}
			rv := arg.RValue()
			// SSA 将 variadic 参数打包为切片
			if rv.Kind() == reflect.Slice {
				for j := 0; j < rv.Len(); j++ {
					elems = append(elems, rv.Index(j))
				}
				continue
			}
			// append([]byte, string...) 特殊情况
			if slice.Kind() == reflect.Slice && slice.Type().Elem().Kind() == reflect.Uint8 && rv.Kind() == reflect.String {
				for _, b := range []byte(rv.String()) {
					elems = append(elems, reflect.ValueOf(b))
				}
				continue
			}
			elems = append(elems, rv)
		}
		return value.RValue{Value: reflect.Append(slice, elems...)}

	case "copy":
		dst := args[0].RValue()
		src := args[1].RValue()
		if dst.Kind() == reflect.Ptr {
			dst = dst.Elem()
		}
		if src.Kind() == reflect.Ptr {
			src = src.Elem()
		}
		n := reflect.Copy(dst, src)
		return value.ValueOf(n)

	case "close": // close(chan T)
		args[0].RValue().Close()
		return nil

	case "delete": // delete(map[K]V, K)
		args[0].RValue().SetMapIndex(args[1].RValue(), reflect.Value{})
		return nil

	case "print", "println": // print(any, ...)
		s := make([]string, len(args))
		for i, arg := range args {
			s[i] = fmt.Sprint(arg.Interface())
		}
		pos := caller.program.mainPkg.Prog.Fset.Position(callPos)
		msg := strings.Join(s, " ")
		if debugging {
			caller.context.outBuffer.WriteString(fmt.Sprintf("[%s %s:%d] %s\n",
				time.Now().Format("15:04:05"),
				pos.Filename,
				pos.Line,
				msg,
			))
		}
		// print/println 输出统一写入上下文缓冲区，调用方通过 Context.Output() 获取
		caller.context.outBuffer.WriteString(msg + "\n")
		return nil

	case "len":
		return value.ValueOf(args[0].Len())

	case "cap":
		return value.ValueOf(args[0].Cap())

	case "panic":
		panic(args[0].Interface())

	case "recover":
		// recover 只对调用链上处于 panic 状态的栈帧生效（通常是 deferred 闭包的父帧）
		for p := caller.caller; p != nil; p = p.caller {
			if p.panicking {
				p.panicking = false
				return value.ValueOf(p.panic)
			}
		}
		return value.ValueOf((interface{})(nil))
	}
	panic("unknown built-in: " + fn.Name())
}

// deref 解引用，若typ为指针类型，返回其指向的类型，否则返回原类型。
func deref(typ types.Type) types.Type {
	if p, ok := typ.Underlying().(*types.Pointer); ok {
		return p.Elem()
	}
	return typ
}

// zero 返回指定类型的零值（指针包装，调用方通过 Elem() 取值）
func zero(t types.Type) value.Value {
	v := reflect.New(typeChange(t))
	return value.RValue{Value: v}
}
