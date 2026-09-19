package importer

import (
	"reflect"
	"testing"
)

// TestFullWorkflow 测试完整的工作流程：注册包 → 导入器导入 → 作用域查找
func TestFullWorkflow(t *testing.T) {
	// 1. 创建注册器和导入器
	registry := NewRegistry()
	imp := NewImporterWithRegistry(registry)

	// 2. 创建测试对象
	addFunc := func(a, b int) int { return a + b }
	mulFunc := func(a, b int) int { return a * b }
	piConst := 3.14159
	maxSize := 1000

	testVar := "global"
	testType := reflect.TypeOf(struct {
		Name string
		Age  int
	}{})

	// 3. 注册包
	mathObjects := []*Object{
		CreateFunction("Add", addFunc, "Addition function"),
		CreateFunction("Multiply", mulFunc, "Multiplication function"),
		CreateConstant("PI", piConst, "Pi constant"),
	}

	utilsObjects := []*Object{
		CreateConstant("MaxSize", maxSize, "Maximum size constant"),
		CreateVariable("GlobalVar", &testVar, reflect.TypeOf(testVar), "Global variable"),
		CreateType("Person", testType, "Person structure"),
	}

	err := registry.RegisterPackage("test/math", "math", mathObjects...)
	if err != nil {
		t.Fatalf("failed to register math package: %v", err)
	}

	err = registry.RegisterPackage("test/utils", "utils", utilsObjects...)
	if err != nil {
		t.Fatalf("failed to register utils package: %v", err)
	}

	// 4. 测试导入器
	mathPkg, err := imp.Import("test/math")
	if err != nil {
		t.Errorf("failed to import math package: %v", err)
	}

	if mathPkg.Name() != "math" {
		t.Errorf("math package name = %s, want math", mathPkg.Name())
	}

	// 验证包作用域
	mathScope := mathPkg.Scope()
	addObj := mathScope.Lookup("Add")
	if addObj == nil {
		t.Error("Add function should be found in math package scope")
	}

	piObj := mathScope.Lookup("PI")
	if piObj == nil {
		t.Error("PI constant should be found in math package scope")
	}
}
