// 文件说明：types.go — gofunction/importer 中 ObjectKind、UnknownKind 等符号的实现
//
// 用法
//
//	NewObject(name string, kind ObjectKind, value interface{}, doc string) *Object
//	(k ObjectKind) String() string  // String 返回对象类型的字符串表示
//	(o *Object) GetDoc() string  // 向后兼容的字段别名
//	同包调用：String() → importer.go, registry.go；Package → importer.go, registry.go；Object → importer.go, registry.go
//
// 调用架构
//
//	[上游] gofunction/cmd/gofunction, gofunction/internal/interp, gofunction/packages 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/importer/types.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] importer.go, registry.go
//	[关键引用] String() ← importer.go, registry.go
//
// 主要符号
//
//	type ObjectKind int
//	const UnknownKind
//	const Var
//	const Const
//	const TypeName
//	const Function
//	... 另有 12 个顶层符号

package importer

import (
	"reflect"
)

// ObjectKind 对象类型枚举
type ObjectKind int

const (
	// UnknownKind 未知类型
	UnknownKind ObjectKind = iota
	// VariableKind 变量
	VariableKind
	// ConstantKind 常量
	ConstantKind
	// TypeKind 类型
	TypeKind
	// FunctionKind 函数
	FunctionKind
	// BuiltinFunctionKind 内置函数
	BuiltinFunctionKind
)

// 向后兼容的常量别名
const (
	Var             = VariableKind
	Const           = ConstantKind
	TypeName        = TypeKind
	Function        = FunctionKind
	BuiltinFunction = BuiltinFunctionKind
)

// String 返回对象类型的字符串表示
func (k ObjectKind) String() string {
	switch k {
	case VariableKind:
		return "Variable"
	case ConstantKind:
		return "Constant"
	case TypeKind:
		return "Type"
	case FunctionKind:
		return "Function"
	case BuiltinFunctionKind:
		return "BuiltinFunction"
	default:
		return "Unknown"
	}
}

// Package 外部包描述
type Package struct {
	Path    string    // 包路径
	Name    string    // 包名
	Objects []*Object // 包含的对象
}

// Object 外部对象描述
type Object struct {
	Name          string        // 对象名称
	Kind          ObjectKind    // 对象类型
	Value         reflect.Value // 对象值
	Type          reflect.Type  // 对象类型
	Documentation string        // 文档说明
}

// 向后兼容的字段别名
func (o *Object) GetDoc() string {
	return o.Documentation
}

func (o *Object) SetDoc(doc string) {
	o.Documentation = doc
}

// Doc 字段的 getter/setter（向后兼容）
func (o *Object) Doc() string {
	return o.Documentation
}

// NewObject 创建新的外部对象
func NewObject(name string, kind ObjectKind, value interface{}, doc string) *Object {
	return &Object{
		Name:          name,
		Kind:          kind,
		Value:         reflect.ValueOf(value),
		Type:          reflect.TypeOf(value),
		Documentation: doc,
	}
}

// NewFunction 创建函数对象
func NewFunction(name string, fn interface{}, doc string) *Object {
	return NewObject(name, FunctionKind, fn, doc)
}

// NewVariable 创建变量对象
func NewVariable(name string, valueAddr interface{}, typ reflect.Type, doc string) *Object {
	return &Object{
		Name:          name,
		Kind:          VariableKind,
		Value:         reflect.ValueOf(valueAddr),
		Type:          typ,
		Documentation: doc,
	}
}

// NewConstant 创建常量对象
func NewConstant(name string, value interface{}, doc string) *Object {
	return NewObject(name, ConstantKind, value, doc)
}

// NewType 创建类型对象
func NewType(name string, typ reflect.Type, doc string) *Object {
	return &Object{
		Name:          name,
		Kind:          TypeKind,
		Type:          typ,
		Documentation: doc,
	}
}
