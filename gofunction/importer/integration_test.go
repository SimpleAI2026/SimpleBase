package importer

import (
	"reflect"
	"strings"
	"testing"
)

// TestFullWorkflow 测试完整的工作流程
func TestFullWorkflow(t *testing.T) {
	// 1. 创建注册器和导入器
	registry := NewRegistry()
	importer := NewImporterWithRegistry(registry)
	completionProvider := NewCompletionProvider(registry)

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
	mathPkg, err := importer.Import("test/math")
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

	// 5. 测试代码补全
	allCompletions := completionProvider.GetCompletionItems()
	if len(allCompletions) != 6 { // 2 math + 3 utils + 1 type
		t.Errorf("expected 6 completion items, got %d", len(allCompletions))
	}

	// 验证函数补全
	addCompletion := findCompletionByLabel(allCompletions, "math.Add")
	if addCompletion == nil {
		t.Error("math.Add completion should exist")
	}

	if addCompletion.Kind != "Function" {
		t.Error("Add should be recognized as Function")
	}

	if !strings.Contains(addCompletion.InsertText, "${1:int}") {
		t.Error("Add function should have parameter placeholders")
	}

	// 验证常量补全
	piCompletion := findCompletionByLabel(allCompletions, "math.PI")
	if piCompletion == nil {
		t.Error("math.PI completion should exist")
	}

	if piCompletion.Kind != "Constant" {
		t.Error("PI should be recognized as Constant")
	}

	// 6. 测试包内补全
	mathCompletions := completionProvider.GetPackageCompletions("math")
	if len(mathCompletions) != 3 {
		t.Errorf("expected 3 math completions, got %d", len(mathCompletions))
	}

	// 7. 测试函数签名
	addSignature := completionProvider.GetFunctionSignature("math", "Add")
	if addSignature == "" {
		t.Error("Add function signature should not be empty")
	}

	// 修正期望的函数签名格式 - 反射类型的字符串表示不包含参数名
	if !strings.Contains(addSignature, "func(int, int) int") {
		t.Errorf("Add function signature should match expected format, got: %s", addSignature)
	}
}

// findCompletionByLabel 辅助函数，用于查找指定标签的补全项
func findCompletionByLabel(completions []*CompletionItem, label string) *CompletionItem {
	for _, completion := range completions {
		if completion.Label == label {
			return completion
		}
	}
	return nil
}
