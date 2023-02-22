package goscript

import (
	"fmt"
	"go/constant"
	"go/types"
	"reflect"
	"unsafe"

	"golang.org/x/tools/go/ssa"
)

// asInt converts x, which must be an integer, to an int suitable for
// use as a slice or array index or operand to make().
func asInt(x value) int {
	switch x := x.(type) {
	case int:
		return x
	case int8:
		return int(x)
	case int16:
		return int(x)
	case int32:
		return int(x)
	case int64:
		return int(x)
	case uint:
		return int(x)
	case uint8:
		return int(x)
	case uint16:
		return int(x)
	case uint32:
		return int(x)
	case uint64:
		return int(x)
	case uintptr:
		return int(x)
	default:
		v := reflect.ValueOf(x)
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int(v.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return int(v.Uint())
		}
	}
	panic(fmt.Sprintf("cannot convert %T to int", x))
}

func xtypeValue(c *ssa.Const, kind types.BasicKind) value {
	switch kind {
	case types.Bool, types.UntypedBool:
		return constant.BoolVal(c.Value)
	case types.Int, types.UntypedInt:
		return int(c.Int64()) // Assume sizeof(int) is same on host and target.
	case types.Int8:
		return int8(c.Int64())
	case types.Int16:
		return int16(c.Int64())
	case types.Int32, types.UntypedRune:
		return int32(c.Int64())
	case types.Int64:
		return c.Int64()
	case types.Uint:
		return uint(c.Uint64()) // Assume sizeof(uint) is same on host and target.
	case types.Uint8:
		return uint8(c.Uint64())
	case types.Uint16:
		return uint16(c.Uint64())
	case types.Uint32:
		return uint32(c.Uint64())
	case types.Uint64:
		return c.Uint64()
	case types.Uintptr:
		return uintptr(c.Uint64()) // Assume sizeof(uintptr) is same on host and target.
	case types.Float32:
		return float32(c.Float64())
	case types.Float64, types.UntypedFloat:
		return c.Float64()
	case types.Complex64:
		return complex64(c.Complex128())
	case types.Complex128, types.UntypedComplex:
		return c.Complex128()
	case types.String, types.UntypedString:
		if c.Value.Kind() == constant.String {
			return constant.StringVal(c.Value)
		}
		return string(rune(c.Int64()))
	case types.UnsafePointer:
		return unsafe.Pointer(uintptr(c.Uint64()))
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", kind))
}

// go:nocheckptr
func toUnsafePointer(v uintptr) unsafe.Pointer {
	return unsafe.Pointer(v)
}

func setValue(v reflect.Value, x reflect.Value) {
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(x.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(x.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(x.Uint())
	case reflect.Uintptr:
		v.SetUint(x.Uint())
	case reflect.Float32, reflect.Float64:
		v.SetFloat(x.Float())
	case reflect.Complex64, reflect.Complex128:
		v.SetComplex(x.Complex())
	case reflect.String:
		v.SetString(x.String())
	case reflect.UnsafePointer:
		v.SetPointer(unsafe.Pointer(x.Pointer()))
	default:
		v.Set(x)
	}
}
