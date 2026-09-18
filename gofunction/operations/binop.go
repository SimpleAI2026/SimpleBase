package operations

import (
	"reflect"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
)

// BinaryOperations 二元运算操作集合
type BinaryOperations struct{}

// NewBinaryOperations 创建新的二元运算操作实例
func NewBinaryOperations() *BinaryOperations {
	return &BinaryOperations{}
}

// Add 加法运算
func (bo *BinaryOperations) Add(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.String:
		result = x.String() + y.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() + y.Int()
	case reflect.Float32, reflect.Float64:
		result = x.Float() + y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() + y.Uint()
	}
	return result
}

// Sub 减法运算
func (bo *BinaryOperations) Sub(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() - y.Int()
	case reflect.Float32, reflect.Float64:
		result = x.Float() - y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() - y.Uint()
	}
	return result
}

// Mul 乘法运算
func (bo *BinaryOperations) Mul(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() * y.Int()
	case reflect.Float32, reflect.Float64:
		result = x.Float() * y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() * y.Uint()
	}
	return result
}

// Quo 除法运算
func (bo *BinaryOperations) Quo(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() / y.Int()
	case reflect.Float32, reflect.Float64:
		result = x.Float() / y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() / y.Uint()
	}
	return result
}

// Rem 取余运算
func (bo *BinaryOperations) Rem(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() % y.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() % y.Uint()
	}
	return result
}

// And 按位与运算
func (bo *BinaryOperations) And(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() & y.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() & y.Uint()
	}
	return result
}

// Or 按位或运算
func (bo *BinaryOperations) Or(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() | y.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() | y.Uint()
	}
	return result
}

// Xor 按位异或运算
func (bo *BinaryOperations) Xor(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() ^ y.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() ^ y.Uint()
	}
	return result
}

// AndNot 按位清除运算
func (bo *BinaryOperations) AndNot(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() &^ y.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() &^ y.Uint()
	}
	return result
}

// Shl 左移运算
func (bo *BinaryOperations) Shl(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() << y.Uint()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() << y.Uint()
	}
	return result
}

// Shr 右移运算
func (bo *BinaryOperations) Shr(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() >> y.Uint()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() >> y.Uint()
	}
	return result
}