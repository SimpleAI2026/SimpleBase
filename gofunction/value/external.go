package value

import (
	"fmt"
	"reflect"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
	"golang.org/x/tools/go/ssa"
)

// ExternalValue 外部（宿主）对象值，包装通过 importer 注册的
// 函数、变量、常量等宿主程序对象，供脚本侧访问。
type ExternalValue struct {
	Object struct {
		Value reflect.Value
	}
}

// NewExternalValue 创建外部值包装
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
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Ptr, reflect.Slice:
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
func (ev *ExternalValue) Index(i int) Value {
	return RValue{Value: ev.Object.Value.Index(i)}
}

// MapIndex 实现 Value 接口
func (ev *ExternalValue) MapIndex(key Value) Value {
	return RValue{Value: ev.Object.Value.MapIndex(key.RValue())}
}

// Field 实现 Value 接口
func (ev *ExternalValue) Field(i int) Value {
	return RValue{Value: ev.Object.Value.Field(i)}
}

// Elem 实现 Value 接口
func (ev *ExternalValue) Elem() Value {
	return RValue{Value: ev.Object.Value.Elem()}
}

// RValue 实现 Value 接口
func (ev *ExternalValue) RValue() reflect.Value {
	return ev.Object.Value
}

// ToValue 将外部对象还原为普通 Value（取址语义：变量返回其指针指向）
func (ev *ExternalValue) ToValue() Value {
	v := ev.Object.Value
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return RValue{Value: v}
}

// Store 向外部变量写入值
func (ev *ExternalValue) Store(val Value) {
	target := ev.Object.Value
	if target.Kind() != reflect.Ptr {
		if target.CanAddr() {
			target = target.Addr()
		} else {
			panic(fmt.Sprintf("external value %s is not addressable", ev.String()))
		}
	}
	target.Elem().Set(val.RValue())
}

// Set 实现 Value 接口（等价 Store）
func (ev *ExternalValue) Set(val Value) {
	ev.Store(val)
}

// ExternalValueWrap 预热外部对象：遍历 importer 已加载的外部对象，
// 确保其 reflect.Value 就绪，并返回 "pkgPath.ObjectName" -> *ExternalValue 映射。
// 脚本 import 外部包时，frame.get 会通过该映射找到宿主对象。
func ExternalValueWrap(imp *importer.Importer, mainPkg *ssa.Package) map[string]*ExternalValue {
	wrapped := make(map[string]*ExternalValue)
	for key, obj := range imp.ExternalObjects() {
		if obj.Value.IsValid() {
			wrapped[key] = NewExternalValue(obj.Value)
		}
	}
	return wrapped
}
