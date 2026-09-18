package value

import (
	"reflect"
)

// Package 将多个返回值打包为元组值，
// 用于 SSA 多返回值指令的 Extract 提取。
func Package(results []reflect.Value) Value {
	if len(results) == 1 {
		return RValue{Value: results[0]}
	}
	vals := make([]Value, len(results))
	for i, r := range results {
		vals[i] = RValue{Value: r}
	}
	return RValue{Value: reflect.ValueOf(vals)}
}

// Unpackage 将元组值解包为 reflect.Value 切片，
// 是 Package 的逆操作，用于闭包调用返回值还原。
func Unpackage(v Value) []reflect.Value {
	tuple := v.RValue()
	if tuple.Kind() != reflect.Slice {
		return []reflect.Value{tuple}
	}
	results := make([]reflect.Value, tuple.Len())
	for i := range results {
		results[i] = tuple.Index(i).Interface().(Value).RValue()
	}
	return results
}
