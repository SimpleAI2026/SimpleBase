package gofunction

import (
	"fmt"
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

// 调试开关：设置为 true 时打印每条语句的执行详情
var debugging = false

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

	fr := caller.newChild(fn)
	fr.env = make(map[ssa.Value]*value.Value)
	if len(fn.Blocks) > 0 {
		fr.block = fn.Blocks[0]
		// 入口基本块作为 prevBlock，保证首个 phi 能从入口边取值
		fr.prevBlock = fn.Blocks[0]
	}
	fr.locals = make([]value.Value, len(fn.Locals))
	for i, l := range fn.Locals {
		fr.locals[i] = zero(deref(l.Type()))
		fr.env[l] = &fr.locals[i]
	}
	for i, p := range fn.Params {
		if i >= len(args) {
			panic(fmt.Sprintf("call %s: missing argument %d", fn.String(), i))
		}
		fr.env[p] = &args[i]
	}
	for i, fv := range fn.FreeVars {
		if i >= len(env) {
			panic(fmt.Sprintf("call %s: missing free var %s", fn.String(), fv.Name()))
		}
		fr.env[fv] = env[i]
	}
	// 与 x/tools SSA interp 一致：panic 恢复后可能进入 Recover 块，需再次执行
	for fr.block != nil {
		runFrame(fr)
	}
	// 返回时释放所有局部变量
	for i := range fn.Locals {
		fr.locals[i] = nil
	}
	return fr.result
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

// runFrame 在栈帧上执行程序
func runFrame(fr *frame) {
	var instr ssa.Instruction
	defer func() {
		if fr.block == nil {
			return // 正常返回
		}
		fr.panicking = true
		re := recover()
		pos := "-"
		if instr != nil && fr.program != nil && fr.program.mainPkg != nil {
			pos = fr.program.mainPkg.Prog.Fset.Position(instr.Pos()).String()
		}
		fr.panic = fmt.Errorf("panic: %s: %v", pos, re)
		fr.runDefers()
		if fr.fn != nil {
			fr.block = fr.fn.Recover
		} else {
			fr.block = nil
		}
	}()

	// B9：_JUMP 必须继续外层循环以执行目标 block，而不能 break 出整个 runFrame
BlockLoop:
	for fr.block != nil {
		for _, instr = range fr.block.Instrs {
			c := visitInstr(fr, instr)
			if !debugging {
				if err := fr.context.Err(); err != nil {
					panic(err)
				}
			}
			switch c {
			case _Return:
				return
			case _JUMP:
				continue BlockLoop
			}
		}
	}
}

// 下一条执行指令的状态
type nextInstr int

const (
	_NEXT   nextInstr = iota // 继续执行下一条语句
	_Return                  // 函数返回
	_JUMP                    // 跳转到另一个block
)

// visitInstr 执行一条ssa.Instruction语句，返回值nextInstr用于指示下一条语句的位置
func visitInstr(fr *frame, instr ssa.Instruction) nextInstr {
	c := _NEXT
	// 无法为第三方包中的结构体添加方法，因此通过switch type的方式来找到不同语句的执行方法
	switch instr := instr.(type) {
	case *ssa.DebugRef:
		// no-op
	case *ssa.Alloc:
		c = runAlloc(fr, instr)
	case *ssa.UnOp:
		c = runUnOp(fr, instr)
	case *ssa.BinOp:
		c = runBinOp(fr, instr)
	case *ssa.MakeInterface:
		c = runMakeInterface(fr, instr)
	case *ssa.Return:
		c = runReturn(fr, instr)
	case *ssa.IndexAddr:
		c = runIndexAddr(fr, instr)
	case *ssa.Index:
		c = runIndex(fr, instr)
	case *ssa.Field:
		c = runField(fr, instr)
	case *ssa.FieldAddr:
		c = runFieldAddr(fr, instr)
	case *ssa.Store:
		c = runStore(fr, instr)
	case *ssa.Slice:
		c = runSlice(fr, instr)
	case *ssa.Call:
		c = runCall(fr, instr)
	case *ssa.MakeSlice:
		c = runMakeSlice(fr, instr)
	case *ssa.MakeMap:
		c = runMakeMap(fr, instr)
	case *ssa.MapUpdate:
		c = runMapUpdate(fr, instr)
	case *ssa.Lookup:
		c = runLookup(fr, instr)
	case *ssa.Extract:
		c = runExtract(fr, instr)
	case *ssa.If:
		c = runIf(fr, instr)
	case *ssa.Jump:
		c = runJump(fr, instr)
	case *ssa.Phi:
		c = runPhi(fr, instr)
	case *ssa.Convert:
		c = runConvert(fr, instr)
	case *ssa.Range:
		c = runRange(fr, instr)
	case *ssa.Next:
		c = runNext(fr, instr)
	case *ssa.ChangeType:
		c = runChangeType(fr, instr)
	case *ssa.ChangeInterface:
		c = runChangeInterface(fr, instr)
	case *ssa.MakeClosure:
		c = runMakeClosure(fr, instr)
	case *ssa.Defer:
		c = runDefer(fr, instr)
	case *ssa.RunDefers:
		c = runRunDefers(fr, instr)
	case *ssa.MakeChan:
		c = runMakeChan(fr, instr)
	case *ssa.Send:
		c = runSend(fr, instr)
	case *ssa.TypeAssert:
		c = runTypeAssert(fr, instr)
	case *ssa.Go:
		c = runGo(fr, instr)
	case *ssa.Panic:
		c = runPanic(fr, instr)
	case *ssa.Select:
		c = runSelect(fr, instr)
	default:
		panic(fmt.Sprintf("unexpected instruction: %T", instr))
	}

	if debugging {
		logDebug("run %s: \t%s \t%T", fr.program.mainPkg.Prog.Fset.Position(instr.Pos()), instr.String(), instr)
		if val, ok := instr.(ssa.Value); ok {
			if pv, ok := fr.env[val]; ok && pv != nil {
				v := *pv
				if v != nil && v.IsValid() {
					logDebug("\t\t\t%#v", v.Interface())
				}
			}
		}
	}

	return c
}
