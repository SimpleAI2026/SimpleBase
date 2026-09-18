package importer

import (
	"reflect"
	"testing"
)

func TestObjectKind_String(t *testing.T) {
	tests := []struct {
		kind ObjectKind
		want string
	}{
		{VariableKind, "Variable"},
		{ConstantKind, "Constant"},
		{TypeKind, "Type"},
		{FunctionKind, "Function"},
		{BuiltinFunctionKind, "BuiltinFunction"},
		{UnknownKind, "Unknown"},
		{ObjectKind(999), "Unknown"}, // 测试未知类型
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.kind.String()
			if got != tt.want {
				t.Errorf("ObjectKind.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewObject(t *testing.T) {
	testValue := 42
	obj := NewObject("TestObject", ConstantKind, testValue, "Test object")

	if obj.Name != "TestObject" {
		t.Errorf("Name = %s, want TestObject", obj.Name)
	}

	if obj.Kind != ConstantKind {
		t.Errorf("Kind = %v, want %v", obj.Kind, ConstantKind)
	}

	if obj.Documentation != "Test object" {
		t.Errorf("Documentation = %s, want 'Test object'", obj.Documentation)
	}

	if obj.Value.Interface() != testValue {
		t.Errorf("Value = %v, want %v", obj.Value.Interface(), testValue)
	}

	if obj.Type != reflect.TypeOf(testValue) {
		t.Error("Type mismatch")
	}
}

func TestNewFunction(t *testing.T) {
	testFunc := func(a, b int) int { return a + b }
	obj := NewFunction("Add", testFunc, "Addition function")

	if obj.Name != "Add" {
		t.Errorf("Name = %s, want Add", obj.Name)
	}

	if obj.Kind != FunctionKind {
		t.Errorf("Kind = %v, want %v", obj.Kind, FunctionKind)
	}

	if obj.Documentation != "Addition function" {
		t.Errorf("Documentation = %s, want 'Addition function'", obj.Documentation)
	}

	if obj.Type != reflect.TypeOf(testFunc) {
		t.Error("Type mismatch")
	}
}

func TestNewVariable(t *testing.T) {
	testVar := 42
	obj := NewVariable("TestVar", &testVar, reflect.TypeOf(testVar), "Test variable")

	if obj.Name != "TestVar" {
		t.Errorf("Name = %s, want TestVar", obj.Name)
	}

	if obj.Kind != VariableKind {
		t.Errorf("Kind = %v, want %v", obj.Kind, VariableKind)
	}

	if obj.Type != reflect.TypeOf(testVar) {
		t.Error("Type mismatch")
	}

	// 验证值是指针
	if obj.Value.Kind() != reflect.Ptr {
		t.Error("Variable value should be a pointer")
	}
}

func TestNewConstant(t *testing.T) {
	testConst := 3.14159
	obj := NewConstant("PI", testConst, "Pi constant")

	if obj.Name != "PI" {
		t.Errorf("Name = %s, want PI", obj.Name)
	}

	if obj.Kind != ConstantKind {
		t.Errorf("Kind = %v, want %v", obj.Kind, ConstantKind)
	}

	if obj.Value.Float() != testConst {
		t.Errorf("Value = %f, want %f", obj.Value.Float(), testConst)
	}
}

func TestNewType(t *testing.T) {
	testType := reflect.TypeOf(struct{ Name string }{})
	obj := NewType("TestStruct", testType, "Test structure")

	if obj.Name != "TestStruct" {
		t.Errorf("Name = %s, want TestStruct", obj.Name)
	}

	if obj.Kind != TypeKind {
		t.Errorf("Kind = %v, want %v", obj.Kind, TypeKind)
	}

	if obj.Type != testType {
		t.Error("Type mismatch")
	}
}

func TestPackage(t *testing.T) {
	obj1 := NewConstant("CONST1", "value1", "constant 1")
	obj2 := NewFunction("Func1", func() {}, "function 1")

	pkg := &Package{
		Path:    "test/package",
		Name:    "testpkg",
		Objects: []*Object{obj1, obj2},
	}

	if pkg.Path != "test/package" {
		t.Errorf("Path = %s, want test/package", pkg.Path)
	}

	if pkg.Name != "testpkg" {
		t.Errorf("Name = %s, want testpkg", pkg.Name)
	}

	if len(pkg.Objects) != 2 {
		t.Errorf("Objects count = %d, want 2", len(pkg.Objects))
	}
}

func TestObjectCompatibility(t *testing.T) {
	// 测试向后兼容性
	obj := &Object{
		Name:          "TestObj",
		Kind:          FunctionKind,
		Documentation: "Test documentation",
	}

	// 测试 Doc 方法
	if obj.Doc() != "Test documentation" {
		t.Error("Doc() method should return documentation")
	}

	// 测试 GetDoc 方法
	if obj.GetDoc() != "Test documentation" {
		t.Error("GetDoc() method should return documentation")
	}

	// 测试 SetDoc 方法
	obj.SetDoc("New documentation")
	if obj.Documentation != "New documentation" {
		t.Error("SetDoc() should update documentation")
	}
}

func TestConstantCompatibility(t *testing.T) {
	// 测试向后兼容的常量
	if Var != VariableKind {
		t.Error("Var constant should equal VariableKind")
	}

	if Const != ConstantKind {
		t.Error("Const constant should equal ConstantKind")
	}

	if TypeName != TypeKind {
		t.Error("TypeName constant should equal TypeKind")
	}

	if Function != FunctionKind {
		t.Error("Function constant should equal FunctionKind")
	}

	if BuiltinFunction != BuiltinFunctionKind {
		t.Error("BuiltinFunction constant should equal BuiltinFunctionKind")
	}
}

func BenchmarkNewObject(b *testing.B) {
	testValue := 42
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewObject("TestObject", ConstantKind, testValue, "Test object")
	}
}

func BenchmarkNewFunction(b *testing.B) {
	testFunc := func(a, b int) int { return a + b }
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewFunction("Add", testFunc, "Addition function")
	}
}
