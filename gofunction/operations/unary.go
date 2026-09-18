package operations

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

// UnaryOperations 一元运算操作集合
type UnaryOperations struct{}

// NewUnaryOperations 创建新的一元运算操作实例
func NewUnaryOperations() *UnaryOperations {
	return &UnaryOperations{}
}

// Execute 执行一元运算
func (uo *UnaryOperations) Execute(instr *ssa.UnOp, x value.Value, conv func(interface{}, types.Type) value.Value) value.Value {
	if instr.Op == token.MUL {
		return value.ValueOf(x.Elem().Interface())
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

// ConstantOperations 常量运算操作集合
type ConstantOperations struct{}

// NewConstantOperations 创建新的常量运算操作实例
func NewConstantOperations() *ConstantOperations {
	return &ConstantOperations{}
}

// Evaluate 常量表达式求值
func (co *ConstantOperations) Evaluate(c *ssa.Const, zero func(types.Type) value.Value, conv func(interface{}, types.Type) value.Value) value.Value {
	if c.IsNil() {
		return zero(c.Type()).Elem() // typed nil
	}
	var val interface{}
	t := c.Type().Underlying().(*types.Basic)
	switch t.Kind() {
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
		if c.Value.Kind() == constant.String {
			val = constant.StringVal(c.Value)
		} else {
			val = string(rune(c.Int64()))
		}
	default:
		panic(fmt.Sprintf("constValue: %s", c))
	}
	return conv(val, c.Type())
}
