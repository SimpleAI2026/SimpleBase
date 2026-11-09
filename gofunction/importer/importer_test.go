package importer

import (
	"go/types"
	"reflect"
	"testing"
)

func TestNewImporter(t *testing.T) {
	importer := NewImporter()

	if importer == nil {
		t.Fatal("NewImporter() returned nil")
	}

	if importer.typeCache == nil {
		t.Error("typeCache should be initialized")
	}

	if importer.registry == nil {
		t.Error("registry should be initialized")
	}

	if importer.ssaPackages == nil {
		t.Error("ssaPackages should be initialized")
	}

	if importer.packageCache == nil {
		t.Error("packageCache should be initialized")
	}

	if importer.externalObjects == nil {
		t.Error("externalObjects should be initialized")
	}
}

func TestNewImporterWithRegistry(t *testing.T) {
	customRegistry := NewRegistry()
	importer := NewImporterWithRegistry(customRegistry)

	if importer.registry != customRegistry {
		t.Error("custom registry should be set")
	}
}

func TestImporter_TypeCache(t *testing.T) {
	importer := NewImporter()

	// 测试基本类型缓存
	tests := []struct {
		name         string
		reflectType  reflect.Type
		expectedType types.Type
	}{
		{"bool", reflect.TypeOf(true), types.Typ[types.Bool]},
		{"int", reflect.TypeOf(int(0)), types.Typ[types.Int]},
		{"string", reflect.TypeOf(""), types.Typ[types.String]},
		{"float64", reflect.TypeOf(float64(0)), types.Typ[types.Float64]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cachedType, exists := importer.typeCache[tt.reflectType]
			if !exists {
				t.Errorf("type %s should be cached", tt.name)
			}
			if cachedType != tt.expectedType {
				t.Errorf("cached type mismatch for %s", tt.name)
			}
		})
	}
}

func TestImporter_Import(t *testing.T) {
	importer := NewImporter()

	// 注册一个测试包
	testFunc := func(a, b int) int { return a + b }
	obj := CreateFunction("Add", testFunc, "Addition function")
	err := RegisterPackage("test/math", "math", obj)
	if err != nil {
		t.Fatalf("failed to register package: %v", err)
	}

	// 测试导入
	pkg, err := importer.Import("test/math")
	if err != nil {
		t.Errorf("Import() error = %v", err)
	}

	if pkg == nil {
		t.Error("imported package should not be nil")
	}

	if pkg.Name() != "math" {
		t.Errorf("package name = %s, want math", pkg.Name())
	}
}

func TestImporter_Package(t *testing.T) {
	importer := NewImporter()

	// 注册测试包
	testConst := CreateConstant("PI", 3.14159, "Pi constant")
	err := RegisterPackage("test/constants", "constants", testConst)
	if err != nil {
		t.Fatalf("failed to register package: %v", err)
	}

	// 测试获取包
	pkg := importer.Package("test/constants")
	if pkg == nil {
		t.Error("package should not be nil")
	}

	if pkg.Name() != "constants" {
		t.Errorf("package name = %s, want constants", pkg.Name())
	}

	// 测试包作用域
	scope := pkg.Scope()
	if scope == nil {
		t.Error("package scope should not be nil")
	}

	// 查找常量
	obj := scope.Lookup("PI")
	if obj == nil {
		t.Error("PI constant should be found in scope")
	}

	if obj.Name() != "PI" {
		t.Errorf("object name = %s, want PI", obj.Name())
	}
}

func TestImporter_NewObject(t *testing.T) {
	importer := NewImporter()
	pkg := types.NewPackage("test", "test")

	tests := []struct {
		name     string
		obj      *Object
		wantType string
	}{
		{
			name:     "function",
			obj:      CreateFunction("TestFunc", func(int) int { return 0 }, "test function"),
			wantType: "*types.Func",
		},
		{
			name:     "constant",
			obj:      CreateConstant("TestConst", 42, "test constant"),
			wantType: "*types.Const",
		},
		{
			name:     "variable",
			obj:      CreateVariable("TestVar", new(int), reflect.TypeOf(int(0)), "test variable"),
			wantType: "*types.Var",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := importer.newObject(pkg, tt.obj)
			if result == nil {
				t.Error("newObject should not return nil")
			}
			if result.Name() != tt.obj.Name {
				t.Errorf("object name = %s, want %s", result.Name(), tt.obj.Name)
			}
		})
	}
}

func TestImporter_TypeOf(t *testing.T) {
	importer := NewImporter()
	pkg := types.NewPackage("test", "test")

	tests := []struct {
		name        string
		reflectType reflect.Type
		wantKind    string
	}{
		{"int", reflect.TypeOf(int(0)), "int"},
		{"string", reflect.TypeOf(""), "string"},
		{"slice", reflect.TypeOf([]int{}), "[]int"},
		{"map", reflect.TypeOf(map[string]int{}), "map[string]int"},
		{"struct", reflect.TypeOf(struct{ Name string }{}), "struct"},
		{"pointer", reflect.TypeOf((*int)(nil)), "*int"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := importer.typeOf(tt.reflectType, pkg)
			if result == nil {
				t.Error("typeOf should not return nil")
			}
			// 基本验证类型转换成功
			if result.String() == "invalid type" {
				t.Errorf("typeOf returned invalid type for %s", tt.name)
			}
		})
	}
}

func TestImporter_FuncSignature(t *testing.T) {
	importer := NewImporter()
	pkg := types.NewPackage("test", "test")

	// 测试函数签名生成
	funcType := reflect.TypeOf(func(a int, b string) (int, error) { return 0, nil })
	sig := importer.funcSignature(funcType, nil, pkg)

	if sig == nil {
		t.Error("function signature should not be nil")
	}

	if sig.Params().Len() != 2 {
		t.Errorf("expected 2 parameters, got %d", sig.Params().Len())
	}

	if sig.Results().Len() != 2 {
		t.Errorf("expected 2 results, got %d", sig.Results().Len())
	}
}

func TestPathToName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"github.com/user/repo", "repo"},
		{"std/fmt", "fmt"},
		{"simple", "simple"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := pathToName(tt.path)
			if got != tt.want {
				t.Errorf("pathToName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func BenchmarkImporter_TypeOf(b *testing.B) {
	importer := NewImporter()
	pkg := types.NewPackage("test", "test")
	testType := reflect.TypeOf(struct {
		Name string
		Age  int
	}{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = importer.typeOf(testType, pkg)
	}
}

func BenchmarkImporter_Import(b *testing.B) {
	importer := NewImporter()

	// 预注册包
	obj := CreateFunction("TestFunc", func() {}, "test function")
	_ = RegisterPackage("bench/test", "test", obj)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = importer.Import("bench/test")
	}
}
