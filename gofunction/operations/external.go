package operations

import (
	"fmt"
	"reflect"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
)

// ExternalValue 外部值包装器
type ExternalValue struct {
	Object struct {
		Value reflect.Value
	}
}

// NewExternalValue 创建新的外部值
func NewExternalValue(v reflect.Value) *ExternalValue {
	ev := &ExternalValue{}
	ev.Object.Value = v
	return ev
}

// Interface 实现 Value 接口
func (ev *ExternalValue) Interface() interface{} {
	return ev.Object.Value.Interface()
}

// String 实现 Value 接口
func (ev *ExternalValue) String() string {
	val := ev.Object.Value
	switch val.Kind() {
	case reflect.String:
		return val.String()
	case reflect.Bool:
		if val.Bool() {
			return "true"
		}
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", val.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", val.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%g", val.Float())
	case reflect.Complex64, reflect.Complex128:
		return fmt.Sprintf("%g", val.Complex())
	default:
		return val.String()
	}
}

// Type 实现 Value 接口
func (ev *ExternalValue) Type() reflect.Type {
	return ev.Object.Value.Type()
}

// Kind 实现 Value 接口
func (ev *ExternalValue) Kind() reflect.Kind {
	return ev.Object.Value.Kind()
}

// IsValid 实现 Value 接口
func (ev *ExternalValue) IsValid() bool {
	return ev.Object.Value.IsValid()
}

// IsNil 实现 Value 接口
func (ev *ExternalValue) IsNil() bool {
	if !ev.IsValid() {
		return true
	}
	switch ev.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return ev.Object.Value.IsNil()
	default:
		return false
	}
}

// Int 实现 Value 接口
func (ev *ExternalValue) Int() int64 {
	return ev.Object.Value.Int()
}

// Uint 实现 Value 接口
func (ev *ExternalValue) Uint() uint64 {
	return ev.Object.Value.Uint()
}

// Float 实现 Value 接口
func (ev *ExternalValue) Float() float64 {
	return ev.Object.Value.Float()
}

// Bool 实现 Value 接口
func (ev *ExternalValue) Bool() bool {
	return ev.Object.Value.Bool()
}

// Len 实现 Value 接口
func (ev *ExternalValue) Len() int {
	return ev.Object.Value.Len()
}

// Cap 实现 Value 接口
func (ev *ExternalValue) Cap() int {
	return ev.Object.Value.Cap()
}

// Index 实现 Value 接口
func (ev *ExternalValue) Index(i int) value.Value {
	return value.NewRValueOf(ev.Object.Value.Index(i))
}

// MapIndex 实现 Value 接口
func (ev *ExternalValue) MapIndex(key value.Value) value.Value {
	return value.NewRValueOf(ev.Object.Value.MapIndex(key.RValue()))
}

// Field 实现 Value 接口
func (ev *ExternalValue) Field(i int) value.Value {
	return value.NewRValueOf(ev.Object.Value.Field(i))
}

// Elem 实现 Value 接口
func (ev *ExternalValue) Elem() value.Value {
	return value.NewRValueOf(ev.Object.Value.Elem())
}

// Set 实现 Value 接口
func (ev *ExternalValue) Set(v value.Value) {
	ev.Object.Value.Set(v.RValue())
}

// Next 实现 Value 接口
func (ev *ExternalValue) Next() value.Value {
	panic("ExternalValue does not support iteration")
}

// RValue 实现 Value 接口
func (ev *ExternalValue) RValue() reflect.Value {
	return ev.Object.Value
}
