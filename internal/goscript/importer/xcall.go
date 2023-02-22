package importer

import (
	"errors"
	"reflect"
)

func FieldAddrX(v interface{}, index int) (interface{}, error) {
	x := reflect.ValueOf(v).Elem()
	if !x.IsValid() {
		return nil, errors.New("invalid memory address or nil pointer dereference")
	}
	return x.Field(index).Addr().Interface(), nil
}

func FieldX(v interface{}, index int) (interface{}, error) {
	x := reflect.ValueOf(v)
	for x.Kind() == reflect.Ptr {
		x = x.Elem()
	}
	if !x.IsValid() {
		return nil, errors.New("invalid memory address or nil pointer dereference")
	}
	return x.Field(index).Interface(), nil
}

func AllMethodX(typ reflect.Type) []reflect.Method {
	n := typ.NumMethod()
	if n == 0 {
		return nil
	}
	ms := make([]reflect.Method, n)
	for i := 0; i < n; i++ {
		ms[i] = typ.Method(i)
	}
	return ms
}
