package operations

import (
	"fmt"
	"go/token"
	"reflect"
	"sync/atomic"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

// CallOperations 函数调用操作集合
type CallOperations struct{}

// NewCallOperations 创建新的函数调用操作实例
func NewCallOperations() *CallOperations {
	return &CallOperations{}
}

// GoCall go语句执行
func (co *CallOperations) GoCall(fr FrameInterface, instr *ssa.CallCommon, callExternal func(reflect.Value, []value.Value) value.Value, call func(FrameInterface, token.Pos, interface{}, []value.Value) value.Value) {
	if instr.Signature().Recv() != nil {
		recv := fr.Get(instr.Args[0])
		if recv.RValue().NumMethod() > 0 { // external method
			args := make([]value.Value, len(instr.Args)-1)
			for i := range args {
				args[i] = fr.Get(instr.Args[i+1])
			}
			go callExternal(recv.RValue().MethodByName(instr.Value.Name()), args)
			return
		}
	}

	args := make([]value.Value, len(instr.Args))
	for i, arg := range instr.Args {
		args[i] = fr.Get(arg)
	}

	atomic.AddInt32(fr.GetGoroutineCounter(), 1)

	go func(caller FrameInterface, fn ssa.Value, args []value.Value) {
		defer func() {
			// 启动协程前添加recover语句，避免协程panic影响其他协程
			if re := recover(); re != nil {
				caller.GetOutBuffer().WriteString(fmt.Sprintf("goroutine panic: %v", re))
			}
			atomic.AddInt32(fr.GetGoroutineCounter(), -1)
		}()
		call(caller, instr.Pos(), fn, args)
	}(fr, instr.Value, args)
}

// CallOp 函数调用语句执行
func (co *CallOperations) CallOp(fr FrameInterface, instr *ssa.CallCommon, callExternal func(reflect.Value, []value.Value) value.Value, call func(FrameInterface, token.Pos, interface{}, []value.Value) value.Value) value.Value {
	if instr.Signature().Recv() == nil {
		// call func
		args := make([]value.Value, len(instr.Args))
		for i, arg := range instr.Args {
			args[i] = fr.Get(arg)
		}
		return call(fr, instr.Pos(), instr.Value, args)
	}

	// invoke Method
	if instr.IsInvoke() {
		recv := fr.Get(instr.Value)
		args := make([]value.Value, len(instr.Args))
		for i := range args {
			args[i] = fr.Get(instr.Args[i])
		}
		return callExternal(recv.RValue().MethodByName(instr.Method.Name()), args)
	}

	args := make([]value.Value, len(instr.Args))
	for i, arg := range instr.Args {
		args[i] = fr.Get(arg)
	}
	if args[0].Type().NumMethod() == 0 {
		return call(fr, instr.Pos(), instr.Value, args)
	}
	return callExternal(args[0].RValue().MethodByName(instr.Value.Name()), args[1:])
}

// Call 函数调用
func (co *CallOperations) Call(caller FrameInterface, callpos token.Pos, fn interface{}, args []value.Value, callSSA func(FrameInterface, *ssa.Function, []value.Value, []*value.Value) value.Value, callBuiltin func(FrameInterface, token.Pos, *ssa.Builtin, []value.Value) value.Value, callExternal func(reflect.Value, []value.Value) value.Value) value.Value {
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
	case ssa.Value:
		p := caller.GetEnv()[fun]
		f := (*p).Interface()
		return co.Call(caller, callpos, f, args, callSSA, callBuiltin, callExternal)
	default:
		return callExternal(reflect.ValueOf(fun), args)
	}
}

// FrameInterface 定义frame接口，避免循环依赖
type FrameInterface interface {
	Get(ssa.Value) value.Value
	GetGoroutineCounter() *int32
	GetOutBuffer() interface{ WriteString(string) (int, error) }
	GetEnv() map[ssa.Value]*value.Value
}
