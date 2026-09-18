// Package value 提供解释器运行时的统一值抽象。
//
// Value 接口是对 reflect.Value 的轻量包装，所有 SSA 指令执行过程中的
// 中间值、常量、外部对象统一以 Value 表示。
package value

import (
	"fmt"
	"reflect"
)

// Value 统一值接口
type Value interface {
	// 基本操作
	Interface() interface{}
	String() string
	Type() reflect.Type
	Kind() reflect.Kind
	IsValid() bool
	IsNil() bool

	// 数值操作
	Int() int64
	Uint() uint64
	Float() float64
	Bool() bool

	// 容器操作
	Len() int
	Cap() int
	Index(i int) Value
	MapIndex(key Value) Value

	// 结构体操作
	Field(i int) Value
	Elem() Value

	// 修改操作
	Set(Value)

	// 反射操作
	RValue() reflect.Value
}

// RValue 基于 reflect.Value 的值实现（值类型，方法直接透传）。
//
// 解释器内部大量以复合字面量形式构造值（value.RValue{Value: v}），
// 因此实现为值类型而非指针。
type RValue struct {
	Value reflect.Value
}

// ValueOf 将任意 Go 值包装为 Value
func ValueOf(v interface{}) Value {
	return RValue{Value: reflect.ValueOf(v)}
}

// NewRValueOf 将 reflect.Value 包装为 Value
func NewRValueOf(v reflect.Value) Value {
	return RValue{Value: v}
}

// Interface 返回接口值
func (r RValue) Interface() interface{} {
	if !r.Value.IsValid() {
		return nil
	}
	return r.Value.Interface()
}

// Type 返回反射类型
func (r RValue) Type() reflect.Type {
	return r.Value.Type()
}

// Kind 返回反射种类
func (r RValue) Kind() reflect.Kind {
	return r.Value.Kind()
}

// IsValid 判断值是否有效
func (r RValue) IsValid() bool {
	return r.Value.IsValid()
}

// IsNil 判断值是否为 nil（仅对可 nil 类型有意义）
func (r RValue) IsNil() bool {
	switch r.Value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Ptr, reflect.Slice:
		return r.Value.IsNil()
	default:
		return false
	}
}

// Int 返回有符号整数值
func (r RValue) Int() int64 {
	return r.Value.Int()
}

// Uint 返回无符号整数值
func (r RValue) Uint() uint64 {
	return r.Value.Uint()
}

// Float 返回浮点值
func (r RValue) Float() float64 {
	return r.Value.Float()
}

// Bool 返回布尔值
func (r RValue) Bool() bool {
	return r.Value.Bool()
}

// Len 返回长度
func (r RValue) Len() int {
	return r.Value.Len()
}

// Cap 返回容量
func (r RValue) Cap() int {
	return r.Value.Cap()
}

// Index 返回下标 i 对应的元素
func (r RValue) Index(i int) Value {
	return RValue{Value: r.Value.Index(i)}
}

// MapIndex 返回 map 中 key 对应的值
func (r RValue) MapIndex(key Value) Value {
	return RValue{Value: r.Value.MapIndex(key.RValue())}
}

// Field 返回结构体第 i 个字段
func (r RValue) Field(i int) Value {
	return RValue{Value: r.Value.Field(i)}
}

// Elem 解引用（指针/接口）
func (r RValue) Elem() Value {
	return RValue{Value: r.Value.Elem()}
}

// Set 修改可寻址位置的值
func (r RValue) Set(v Value) {
	r.Value.Set(v.RValue())
}

// RValue 返回底层 reflect.Value
func (r RValue) RValue() reflect.Value {
	return r.Value
}

// String 实现 fmt.Stringer
func (r RValue) String() string {
	if !r.Value.IsValid() {
		return "<invalid>"
	}
	return fmt.Sprint(r.Value)
}
