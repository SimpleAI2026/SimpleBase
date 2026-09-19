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
