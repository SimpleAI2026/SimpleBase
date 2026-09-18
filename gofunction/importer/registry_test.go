package importer

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("NewRegistry() returned nil")
	}

	if registry.packages == nil {
		t.Error("packages map should be initialized")
	}

	if registry.packagesByName == nil {
		t.Error("packagesByName map should be initialized")
	}

	if registry.config == nil {
		t.Error("config should be initialized")
	}
}

func TestNewRegistryWithConfig(t *testing.T) {
	customConfig := &RegistryConfig{
		EnableCaching:              false,
		CacheTimeout:               10 * time.Second,
		MaxCacheSize:               500,
		EnableValidation:           false,
		AllowDuplicateRegistration: true,
	}

	registry := NewRegistryWithConfig(customConfig)
	if registry.config != customConfig {
		t.Error("custom config should be set")
	}

	// 测试 nil 配置
	registry2 := NewRegistryWithConfig(nil)
	if registry2.config != DefaultConfig {
		t.Error("should use default config when nil is passed")
	}
}

func TestRegistry_RegisterPackage(t *testing.T) {
	registry := NewRegistry()

	// 创建测试对象
	testFunc := func(a, b int) int { return a + b }
	objects := []*Object{
		CreateFunction("Add", testFunc, "Addition function"),
		CreateConstant("PI", 3.14159, "Pi constant"),
	}

	// 测试正常注册
	err := registry.RegisterPackage("test/pkg", "testpkg", objects...)
	if err != nil {
		t.Errorf("RegisterPackage() error = %v", err)
	}

	// 验证包已注册
	pkg, exists := registry.GetPackage("test/pkg")
	if !exists {
		t.Error("package should exist after registration")
	}

	if pkg.Name != "testpkg" {
		t.Errorf("package name = %s, want testpkg", pkg.Name)
	}

	if len(pkg.Objects) != 2 {
		t.Errorf("package objects count = %d, want 2", len(pkg.Objects))
	}

	// 测试重复注册（应该失败）
	err = registry.RegisterPackage("test/pkg", "testpkg2", objects...)
	if err == nil {
		t.Error("duplicate registration should fail")
	}
}

func TestRegistry_RegisterPackage_AllowDuplicate(t *testing.T) {
	config := &RegistryConfig{
		AllowDuplicateRegistration: true,
	}
	registry := NewRegistryWithConfig(config)

	objects := []*Object{CreateConstant("TEST", "value", "test constant")}

	// 第一次注册
	err := registry.RegisterPackage("test/pkg", "testpkg", objects...)
	if err != nil {
		t.Errorf("first registration failed: %v", err)
	}

	// 第二次注册（应该成功）
	err = registry.RegisterPackage("test/pkg", "testpkg2", objects...)
	if err != nil {
		t.Errorf("duplicate registration should succeed when allowed: %v", err)
	}
}

func TestRegistry_CreateFunction(t *testing.T) {
	registry := NewRegistry()
	testFunc := func(a, b int) int { return a + b }

	obj := registry.CreateFunction("Add", testFunc, "Addition function")

	if obj.Name != "Add" {
		t.Errorf("function name = %s, want Add", obj.Name)
	}

	if obj.Kind != FunctionKind {
		t.Errorf("function kind = %v, want %v", obj.Kind, FunctionKind)
	}

	if obj.Documentation != "Addition function" {
		t.Errorf("function documentation = %s, want 'Addition function'", obj.Documentation)
	}

	if obj.Type != reflect.TypeOf(testFunc) {
		t.Error("function type mismatch")
	}
}

func TestRegistry_CreateVariable(t *testing.T) {
	registry := NewRegistry()
	testVar := 42

	obj := registry.CreateVariable("TestVar", &testVar, reflect.TypeOf(testVar), "Test variable")

	if obj.Name != "TestVar" {
		t.Errorf("variable name = %s, want TestVar", obj.Name)
	}

	if obj.Kind != VariableKind {
		t.Errorf("variable kind = %v, want %v", obj.Kind, VariableKind)
	}

	if obj.Type != reflect.TypeOf(testVar) {
		t.Error("variable type mismatch")
	}
}

func TestRegistry_CreateConstant(t *testing.T) {
	registry := NewRegistry()

	obj := registry.CreateConstant("PI", 3.14159, "Pi constant")

	if obj.Name != "PI" {
		t.Errorf("constant name = %s, want PI", obj.Name)
	}

	if obj.Kind != ConstantKind {
		t.Errorf("constant kind = %v, want %v", obj.Kind, ConstantKind)
	}

	if obj.Value.Float() != 3.14159 {
		t.Errorf("constant value = %f, want 3.14159", obj.Value.Float())
	}
}

func TestRegistry_CreateType(t *testing.T) {
	registry := NewRegistry()
	testType := reflect.TypeOf(struct{ Name string }{})

	obj := registry.CreateType("TestStruct", testType, "Test structure")

	if obj.Name != "TestStruct" {
		t.Errorf("type name = %s, want TestStruct", obj.Name)
	}

	if obj.Kind != TypeKind {
		t.Errorf("type kind = %v, want %v", obj.Kind, TypeKind)
	}

	if obj.Type != testType {
		t.Error("type mismatch")
	}
}

func TestRegistry_GetAllPackages(t *testing.T) {
	registry := NewRegistry()

	// 注册多个包
	obj1 := CreateConstant("CONST1", "value1", "constant 1")
	obj2 := CreateConstant("CONST2", "value2", "constant 2")

	_ = registry.RegisterPackage("pkg1", "package1", obj1)
	_ = registry.RegisterPackage("pkg2", "package2", obj2)

	allPackages := registry.GetAllPackages()

	if len(allPackages) != 2 {
		t.Errorf("expected 2 packages, got %d", len(allPackages))
	}

	if _, exists := allPackages["pkg1"]; !exists {
		t.Error("pkg1 should exist")
	}

	if _, exists := allPackages["pkg2"]; !exists {
		t.Error("pkg2 should exist")
	}
}

func TestRegistry_Config(t *testing.T) {
	registry := NewRegistry()

	// 测试获取配置
	config := registry.GetConfig()
	if config == nil {
		t.Error("config should not be nil")
	}

	// 测试更新配置
	newConfig := &RegistryConfig{
		EnableCaching: false,
		CacheTimeout:  1 * time.Minute,
	}

	registry.UpdateConfig(newConfig)
	updatedConfig := registry.GetConfig()

	if updatedConfig.EnableCaching != false {
		t.Error("config should be updated")
	}

	if updatedConfig.CacheTimeout != 1*time.Minute {
		t.Error("cache timeout should be updated")
	}
}

func TestGlobalFunctions(t *testing.T) {
	// 测试全局便捷函数
	testFunc := func(x int) int { return x * 2 }
	obj := CreateFunction("Double", testFunc, "Double function")

	if obj.Name != "Double" {
		t.Error("global CreateFunction failed")
	}

	// 测试全局注册 - 使用唯一的包路径
	err := RegisterPackage("global/registry-test", "registrytest", obj)
	if err != nil {
		t.Errorf("global RegisterPackage failed: %v", err)
	}

	// 验证全局注册
	allPackages := GetAllPackages()
	if _, exists := allPackages["global/registry-test"]; !exists {
		t.Error("globally registered package should exist")
	}
}

func BenchmarkRegistry_RegisterPackage(b *testing.B) {
	registry := NewRegistry()
	testFunc := func(a, b int) int { return a + b }

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj := CreateFunction("TestFunc", testFunc, "test function")
		_ = registry.RegisterPackage(fmt.Sprintf("test/pkg%d", i), fmt.Sprintf("pkg%d", i), obj)
	}
}

func BenchmarkRegistry_CreateFunction(b *testing.B) {
	registry := NewRegistry()
	testFunc := func(a, b int) int { return a + b }

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.CreateFunction("TestFunc", testFunc, "test function")
	}
}
