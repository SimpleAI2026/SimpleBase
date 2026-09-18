package operations

import (
	"go/token"
	"go/types"
	"reflect"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

// OperationManager 操作管理器，统一管理所有运算操作
type OperationManager struct {
	binaryOps     *BinaryOperations
	comparisonOps *ComparisonOperations
	unaryOps      *UnaryOperations
	constantOps   *ConstantOperations
	callOps       *CallOperations
}

// NewOperationManager 创建新的操作管理器
func NewOperationManager() *OperationManager {
	return &OperationManager{
		binaryOps:     NewBinaryOperations(),
		comparisonOps: NewComparisonOperations(),
		unaryOps:      NewUnaryOperations(),
		constantOps:   NewConstantOperations(),
		callOps:       NewCallOperations(),
	}
}

// ExecuteBinaryOp 执行二元运算
func (om *OperationManager) ExecuteBinaryOp(instr *ssa.BinOp, x, y value.Value, conv func(interface{}, types.Type) value.Value) value.Value {
	var result interface{}
	switch instr.Op {
	case token.ADD: // +
		result = om.binaryOps.Add(x, y)
	case token.SUB: // -
		result = om.binaryOps.Sub(x, y)
	case token.MUL: // *
		result = om.binaryOps.Mul(x, y)
	case token.QUO: // /
		result = om.binaryOps.Quo(x, y)
	case token.REM: // %
		result = om.binaryOps.Rem(x, y)
	case token.AND: // &
		result = om.binaryOps.And(x, y)
	case token.OR: // |
		result = om.binaryOps.Or(x, y)
	case token.XOR: // ^
		result = om.binaryOps.Xor(x, y)
	case token.AND_NOT: // &^
		result = om.binaryOps.AndNot(x, y)
	case token.SHL: // <<
		result = om.binaryOps.Shl(x, y)
	case token.SHR: // >>
		result = om.binaryOps.Shr(x, y)
	case token.LSS: // <
		result = om.comparisonOps.Less(x, y)
	case token.LEQ: // <=
		result = om.comparisonOps.LessEqual(x, y)
	case token.EQL: // ==
		result = om.comparisonOps.Equal(x, y)
	case token.NEQ: // !=
		result = om.comparisonOps.NotEqual(x, y)
	case token.GTR: // >
		result = om.comparisonOps.Greater(x, y)
	case token.GEQ: // >=
		result = om.comparisonOps.GreaterEqual(x, y)
	}
	return conv(result, instr.Type())
}

// ExecuteUnaryOp 执行一元运算
func (om *OperationManager) ExecuteUnaryOp(instr *ssa.UnOp, x value.Value, conv func(interface{}, types.Type) value.Value) value.Value {
	return om.unaryOps.Execute(instr, x, conv)
}

// EvaluateConstant 常量表达式求值
func (om *OperationManager) EvaluateConstant(c *ssa.Const, zero func(types.Type) value.Value, conv func(interface{}, types.Type) value.Value) value.Value {
	return om.constantOps.Evaluate(c, zero, conv)
}

// ExecuteGoCall 执行go语句
func (om *OperationManager) ExecuteGoCall(fr FrameInterface, instr *ssa.CallCommon, callExternal func(reflect.Value, []value.Value) value.Value, call func(FrameInterface, token.Pos, interface{}, []value.Value) value.Value) {
	om.callOps.GoCall(fr, instr, callExternal, call)
}

// ExecuteCallOp 执行函数调用
func (om *OperationManager) ExecuteCallOp(fr FrameInterface, instr *ssa.CallCommon, callExternal func(reflect.Value, []value.Value) value.Value, call func(FrameInterface, token.Pos, interface{}, []value.Value) value.Value) value.Value {
	return om.callOps.CallOp(fr, instr, callExternal, call)
}

// ExecuteCall 执行函数调用
func (om *OperationManager) ExecuteCall(caller FrameInterface, callpos token.Pos, fn interface{}, args []value.Value, callSSA func(FrameInterface, *ssa.Function, []value.Value, []*value.Value) value.Value, callBuiltin func(FrameInterface, token.Pos, *ssa.Builtin, []value.Value) value.Value, callExternal func(reflect.Value, []value.Value) value.Value) value.Value {
	return om.callOps.Call(caller, callpos, fn, args, callSSA, callBuiltin, callExternal)
}

// GetBinaryOperations 获取二元运算操作
func (om *OperationManager) GetBinaryOperations() *BinaryOperations {
	return om.binaryOps
}

// GetComparisonOperations 获取比较运算操作
func (om *OperationManager) GetComparisonOperations() *ComparisonOperations {
	return om.comparisonOps
}

// GetUnaryOperations 获取一元运算操作
func (om *OperationManager) GetUnaryOperations() *UnaryOperations {
	return om.unaryOps
}

// GetConstantOperations 获取常量运算操作
func (om *OperationManager) GetConstantOperations() *ConstantOperations {
	return om.constantOps
}

// GetCallOperations 获取函数调用操作
func (om *OperationManager) GetCallOperations() *CallOperations {
	return om.callOps
}
