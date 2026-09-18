package importer

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestNewCompletionProvider(t *testing.T) {
	registry := NewRegistry()
	provider := NewCompletionProvider(registry)

	if provider == nil {
		t.Fatal("NewCompletionProvider() returned nil")
	}

	if provider.registry != registry {
		t.Error("registry should be set")
	}

	// 测试 nil 注册器
	provider2 := NewCompletionProvider(nil)
	if provider2.registry != GlobalRegistry {
		t.Error("should use global registry when nil is passed")
	}
}

func TestCompletionProvider_GetCompletionItems(t *testing.T) {
	registry := NewRegistry()
	provider := NewCompletionProvider(registry)

	// 注册测试包
	testFunc := func(a, b int) int { return a + b }
	objects := []*Object{
		CreateFunction("Add", testFunc, "Addition function"),
		CreateConstant("PI", 3.14159, "Pi constant"),
		CreateVariable("GlobalVar", new(string), reflect.TypeOf(""), "Global variable"),
	}

	err := registry.RegisterPackage("test/math", "math", objects...)
	if err != nil {
		t.Fatalf("failed to register package: %v", err)
	}

	// 获取补全项
	items := provider.GetCompletionItems()

	if len(items) != 3 {
		t.Errorf("expected 3 completion items, got %d", len(items))
	}

	// 验证函数补全项
	funcItem := findCompletionItem(items, "math.Add")
	if funcItem == nil {
		t.Error("function completion item not found")
	}

	if funcItem.Kind != "Function" {
		t.Errorf("function kind = %s, want Function", funcItem.Kind)
	}

	if funcItem.InsertTextRules != "InsertAsSnippet" {
		t.Error("function should have snippet insert rules")
	}

	if !strings.Contains(funcItem.InsertText, "${1:int}") {
		t.Error("function snippet should contain parameter placeholders")
	}

	// 验证常量补全项
	constItem := findCompletionItem(items, "math.PI")
	if constItem == nil {
		t.Error("constant completion item not found")
	}

	if constItem.Kind != "Constant" {
		t.Errorf("constant kind = %s, want Constant", constItem.Kind)
	}

	if constItem.InsertText != "math.PI" {
		t.Errorf("constant insert text = %s, want math.PI", constItem.InsertText)
	}
}

func TestCompletionProvider_GetPackageCompletions(t *testing.T) {
	registry := NewRegistry()
	provider := NewCompletionProvider(registry)

	// 注册测试包
	testFunc := func(x int) int { return x * 2 }
	objects := []*Object{
		CreateFunction("Double", testFunc, "Double function"),
		CreateConstant("MAX_SIZE", 1000, "Maximum size"),
	}

	err := registry.RegisterPackage("test/utils", "utils", objects...)
	if err != nil {
		t.Fatalf("failed to register package: %v", err)
	}

	// 获取包内补全项
	items := provider.GetPackageCompletions("utils")

	if len(items) != 2 {
		t.Errorf("expected 2 completion items, got %d", len(items))
	}

	// 验证函数补全项（不包含包前缀）
	funcItem := findCompletionItem(items, "Double")
	if funcItem == nil {
		t.Error("function completion item not found")
	}

	if !strings.Contains(funcItem.InsertText, "Double(") {
		t.Error("function snippet should contain function call")
	}

	// 测试不存在的包
	emptyItems := provider.GetPackageCompletions("nonexistent")
	if len(emptyItems) != 0 {
		t.Error("nonexistent package should return empty items")
	}
}

func TestCompletionProvider_GetFunctionSignature(t *testing.T) {
	registry := NewRegistry()
	provider := NewCompletionProvider(registry)

	// 注册测试函数
	testFunc := func(a int, b string) (int, error) { return 0, nil }
	obj := CreateFunction("TestFunc", testFunc, "Test function")

	err := registry.RegisterPackage("test/funcs", "funcs", obj)
	if err != nil {
		t.Fatalf("failed to register package: %v", err)
	}

	// 获取函数签名
	signature := provider.GetFunctionSignature("funcs", "TestFunc")

	if signature == "" {
		t.Error("function signature should not be empty")
	}

	if !strings.Contains(signature, "func") {
		t.Error("signature should contain 'func'")
	}

	// 测试不存在的函数
	emptySignature := provider.GetFunctionSignature("funcs", "NonExistent")
	if emptySignature != "" {
		t.Error("nonexistent function should return empty signature")
	}
}

func TestCompletionProvider_GenerateFunctionSnippet(t *testing.T) {
	registry := NewRegistry()
	provider := NewCompletionProvider(registry)

	tests := []struct {
		name           string
		fn             interface{}
		packagePrefix  string
		expectedPrefix string
		expectedParams int
	}{
		{
			name:           "no params",
			fn:             func() {},
			packagePrefix:  "pkg",
			expectedPrefix: "pkg.TestFunc",
			expectedParams: 0,
		},
		{
			name:           "with params",
			fn:             func(a int, b string) {},
			packagePrefix:  "pkg",
			expectedPrefix: "pkg.TestFunc",
			expectedParams: 2,
		},
		{
			name:           "no package prefix",
			fn:             func(x float64) {},
			packagePrefix:  "",
			expectedPrefix: "TestFunc",
			expectedParams: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := CreateFunction("TestFunc", tt.fn, "test function")
			snippet := provider.generateFunctionSnippet(tt.packagePrefix, obj)

			if !strings.HasPrefix(snippet, tt.expectedPrefix) {
				t.Errorf("snippet should start with %s, got %s", tt.expectedPrefix, snippet)
			}

			if !strings.Contains(snippet, "(") || !strings.Contains(snippet, ")") {
				t.Error("snippet should contain parentheses")
			}

			// 计算参数占位符数量
			paramCount := strings.Count(snippet, "${")
			if paramCount != tt.expectedParams {
				t.Errorf("expected %d parameter placeholders, got %d", tt.expectedParams, paramCount)
			}
		})
	}
}

func TestCompletionProvider_GetTypeDisplayName(t *testing.T) {
	registry := NewRegistry()
	provider := NewCompletionProvider(registry)

	tests := []struct {
		typeName string
		expected string
	}{
		{"int", "int"},
		{"int32", "int"},
		{"int64", "int"},
		{"uint", "uint"},
		{"uint32", "uint"},
		{"float32", "float"},
		{"float64", "float"},
		{"string", "string"},
		{"bool", "bool"},
		{"custom.Type", "custom.Type"},
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			mockType := &mockStringType{name: tt.typeName}
			result := provider.getTypeDisplayName(mockType)
			if result != tt.expected {
				t.Errorf("getTypeDisplayName(%s) = %s, want %s", tt.typeName, result, tt.expected)
			}
		})
	}
}

func TestGlobalCompletionFunctions(t *testing.T) {
	// 注册全局测试包 - 使用唯一的包路径
	testFunc := func(a, b int) int { return a + b }
	obj := CreateFunction("GlobalAdd", testFunc, "Global addition function")

	err := RegisterPackage("global/completion-test", "completiontest", obj)
	if err != nil {
		t.Fatalf("failed to register global package: %v", err)
	}

	// 测试全局补全函数
	items := GetCompletionItems()
	if len(items) == 0 {
		t.Error("global completion items should not be empty")
	}

	// 查找全局注册的函数
	found := false
	for _, item := range items {
		if strings.Contains(item.Label, "GlobalAdd") {
			found = true
			break
		}
	}
	if !found {
		t.Error("globally registered function should be found in completion items")
	}

	// 测试包内补全 - 修正包名为 "completiontest"
	packageItems := GetPackageCompletions("completiontest")
	if len(packageItems) == 0 {
		t.Error("package completion items should not be empty")
	}

	// 测试函数签名 - 修正包名为 "completiontest"
	signature := GetFunctionSignature("completiontest", "GlobalAdd")
	if signature == "" {
		t.Error("function signature should not be empty")
	}
}

// 辅助函数
func findCompletionItem(items []*CompletionItem, label string) *CompletionItem {
	for _, item := range items {
		if item.Label == label {
			return item
		}
	}
	return nil
}

// 模拟类型，用于测试
type mockStringType struct {
	name string
}

func (m *mockStringType) String() string {
	return m.name
}

func BenchmarkCompletionProvider_GetCompletionItems(b *testing.B) {
	registry := NewRegistry()
	provider := NewCompletionProvider(registry)

	// 注册多个包用于基准测试
	for i := 0; i < 10; i++ {
		testFunc := func(a, b int) int { return a + b }
		obj := CreateFunction("TestFunc", testFunc, "test function")
		_ = registry.RegisterPackage(fmt.Sprintf("bench/pkg%d", i), fmt.Sprintf("pkg%d", i), obj)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = provider.GetCompletionItems()
	}
}
