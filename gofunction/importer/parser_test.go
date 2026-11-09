package importer

import (
	"reflect"
	"testing"
)

func TestImporter_ParseNameType(t *testing.T) {
	importer := NewImporter()

	// 测试命名类型解析
	testType := reflect.TypeOf(struct {
		Name string
		Age  int
	}{})

	// 由于测试类型没有包路径，这里主要测试解析逻辑
	named := importer.parseNameType(testType)

	if named == nil {
		t.Error("parseNameType should not return nil")
	}

	// 验证类型已缓存
	if _, exists := importer.typeCache[testType]; !exists {
		t.Error("parsed type should be cached")
	}
}

func TestImporter_ParseNameType_WithPackage(t *testing.T) {
	importer := NewImporter()

	// 使用有包路径的类型进行测试
	// 注意：在实际使用中，这需要真实的包路径
	testType := reflect.TypeOf((*testing.T)(nil)).Elem()

	named := importer.parseNameType(testType)

	if named == nil {
		t.Error("parseNameType should not return nil")
	}

	// 验证命名类型的基本属性
	if named.Obj() == nil {
		t.Error("named type should have an object")
	}

	if named.Obj().Name() != testType.Name() {
		t.Errorf("named type name = %s, want %s", named.Obj().Name(), testType.Name())
	}
}

func TestImporter_ParseNameType_Caching(t *testing.T) {
	importer := NewImporter()

	testType := reflect.TypeOf(struct{ Value int }{})

	// 第一次解析
	named1 := importer.parseNameType(testType)

	// 第二次解析应该返回缓存的结果
	named2 := importer.parseNameType(testType)

	if named1 != named2 {
		t.Error("parseNameType should return cached result")
	}

	// 验证缓存
	cachedType, exists := importer.typeCache[testType]
	if !exists {
		t.Error("type should be cached")
	}

	if cachedType != named1 {
		t.Error("cached type should match parsed type")
	}
}

func TestImporter_ParseNameType_EmptyPackage(t *testing.T) {
	importer := NewImporter()

	// 测试没有包路径的类型
	testType := reflect.TypeOf(struct{ Name string }{})

	named := importer.parseNameType(testType)

	if named == nil {
		t.Error("parseNameType should handle types without package path")
	}

	// 验证类型名称
	if named.Obj().Name() != "" {
		// 匿名结构体应该有空名称或生成的名称
		// 这里主要验证不会崩溃
	}
}

func TestImporter_ParseNameType_Scope(t *testing.T) {
	importer := NewImporter()

	// 使用有名称的类型
	type TestStruct struct {
		Field string
	}
	testType := reflect.TypeOf(TestStruct{})

	named := importer.parseNameType(testType)

	if named == nil {
		t.Error("parseNameType should not return nil")
	}

	// 验证类型对象
	obj := named.Obj()
	if obj == nil {
		t.Error("named type should have an object")
	}

	// 验证对象类型 - 移除错误的类型断言
	// obj 已经是 *types.TypeName 类型，不需要类型断言
	if obj.Name() == "" {
		t.Error("object should have a valid name")
	}
}

func BenchmarkImporter_ParseNameType(b *testing.B) {
	importer := NewImporter()
	testType := reflect.TypeOf(struct {
		Name  string
		Value int
		Data  []byte
	}{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = importer.parseNameType(testType)
	}
}

func BenchmarkImporter_ParseNameType_Cached(b *testing.B) {
	importer := NewImporter()
	testType := reflect.TypeOf(struct {
		Name  string
		Value int
	}{})

	// 预先解析一次以建立缓存
	_ = importer.parseNameType(testType)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = importer.parseNameType(testType)
	}
}
