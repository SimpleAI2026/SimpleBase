package gofunction

import (
	"go/token"

	"github.com/linkxzhou/SimpleBase/gofunction/operations"
	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

// opManager 运算管理器单例
var opManager = operations.NewOperationManager()

// frameAdapter 将 frame 适配为 operations.FrameInterface，
// 使运算层（operations）无需反向依赖解释器的 frame 类型。
type frameAdapter struct {
	frame *frame
}

func (fa *frameAdapter) Get(v ssa.Value) value.Value {
	return fa.frame.get(v)
}

func (fa *frameAdapter) GetGoroutineCounter() *int32 {
	return &fa.frame.context.goroutines
}

func (fa *frameAdapter) GetOutBuffer() interface{ WriteString(string) (int, error) } {
	return &fa.frame.context.outBuffer
}

func (fa *frameAdapter) GetEnv() map[ssa.Value]*value.Value {
	return fa.frame.env
}

// unop 一元表达式求值
func unop(instr *ssa.UnOp, x value.Value) value.Value {
	return opManager.ExecuteUnaryOp(instr, x, conv)
}

// constValue 常量表达式求值
func constValue(c *ssa.Const) value.Value {
	return opManager.EvaluateConstant(c, zero, conv)
}

// binop 二元表达式求值
func binop(instr *ssa.BinOp, x, y value.Value) value.Value {
	return opManager.ExecuteBinaryOp(instr, x, y, conv)
}

// goCall go语句执行
func goCall(fr *frame, instr *ssa.CallCommon) {
	opManager.ExecuteGoCall(&frameAdapter{frame: fr}, instr, callExternal, callFnAdapter)
}

// callOp 函数调用语句执行
func callOp(fr *frame, instr *ssa.CallCommon) value.Value {
	return opManager.ExecuteCallOp(&frameAdapter{frame: fr}, instr, callExternal, callFnAdapter)
}

// callFnAdapter 统一的 call 回调适配，避免每处重复构造闭包
func callFnAdapter(fr operations.FrameInterface, callpos token.Pos, fn interface{}, args []value.Value) value.Value {
	fa, ok := fr.(*frameAdapter)
	if !ok {
		panic("invalid frame type")
	}
	return call(fa.frame, callpos, fn, args)
}

// call 函数调用（分发到 ssa 函数 / 内置函数 / 外部函数）
func call(caller *frame, callpos token.Pos, fn interface{}, args []value.Value) value.Value {
	return opManager.ExecuteCall(&frameAdapter{frame: caller}, callpos, fn, args,
		func(fr operations.FrameInterface, fn *ssa.Function, args []value.Value, env []*value.Value) value.Value {
			fa, ok := fr.(*frameAdapter)
			if !ok {
				panic("invalid frame type")
			}
			return callSSA(fa.frame, fn, args, env)
		},
		func(fr operations.FrameInterface, callpos token.Pos, fn *ssa.Builtin, args []value.Value) value.Value {
			fa, ok := fr.(*frameAdapter)
			if !ok {
				panic("invalid frame type")
			}
			return callBuiltin(fa.frame, callpos, fn, args)
		},
		callExternal)
}
