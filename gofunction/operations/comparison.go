package operations

import (
	"reflect"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
)

// ComparisonOperations 比较运算操作集合
type ComparisonOperations struct{}

// NewComparisonOperations 创建新的比较运算操作实例
func NewComparisonOperations() *ComparisonOperations {
	return &ComparisonOperations{}
}

// Less 小于比较
func (co *ComparisonOperations) Less(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.String:
		result = x.String() < y.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() < y.Int()
	case reflect.Float32, reflect.Float64:
		result = x.Float() < y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() < y.Uint()
	}
	return result
}

// LessEqual 小于等于比较
func (co *ComparisonOperations) LessEqual(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.String:
		result = x.String() <= y.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() <= y.Int()
	case reflect.Float32, reflect.Float64:
		result = x.Float() <= y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() <= y.Uint()
	}
	return result
}

// Equal 等于比较
func (co *ComparisonOperations) Equal(x, y value.Value) interface{} {
	var result interface{}
	if x.IsNil() || y.IsNil() {
		result = x.IsNil() && y.IsNil()
	} else {
		result = x.Interface() == y.Interface()
	}
	return result
}

// NotEqual 不等于比较
func (co *ComparisonOperations) NotEqual(x, y value.Value) interface{} {
	var result interface{}
	if x.IsNil() || y.IsNil() {
		result = x.IsNil() != y.IsNil()
	} else {
		result = x.Interface() != y.Interface()
	}
	return result
}

// Greater 大于比较
func (co *ComparisonOperations) Greater(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.String:
		result = x.String() > y.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() > y.Int()
	case reflect.Float32, reflect.Float64:
		result = x.Float() > y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() > y.Uint()
	}
	return result
}

// GreaterEqual 大于等于比较
func (co *ComparisonOperations) GreaterEqual(x, y value.Value) interface{} {
	var result interface{}
	switch x.Kind() {
	case reflect.String:
		result = x.String() >= y.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result = x.Int() >= y.Int()
	case reflect.Float32, reflect.Float64:
		result = x.Float() >= y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		result = x.Uint() >= y.Uint()
	}
	return result
}
