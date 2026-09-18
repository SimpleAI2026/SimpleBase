package operations

import (
	"reflect"
	"testing"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
)

func TestExternalValue_NewExternalValue(t *testing.T) {
	val := reflect.ValueOf(42)
	extVal := NewExternalValue(val)

	if extVal.Object.Value != val {
		t.Errorf("NewExternalValue() Object.Value = %v, want %v", extVal.Object.Value, val)
	}
}

func TestExternalValue_Interface(t *testing.T) {
	val := reflect.ValueOf("hello world")
	extVal := NewExternalValue(val)

	result := extVal.Interface()
	expected := "hello world"

	if result != expected {
		t.Errorf("Interface() = %v, want %v", result, expected)
	}
}

func TestExternalValue_String(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{
			name:     "string value",
			value:    "test string",
			expected: "test string",
		},
		{
			name:     "int value",
			value:    42,
			expected: "42",
		},
		{
			name:     "bool value",
			value:    true,
			expected: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val := reflect.ValueOf(tt.value)
			extVal := NewExternalValue(val)

			result := extVal.String()
			if result != tt.expected {
				t.Errorf("String() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExternalValue_Type(t *testing.T) {
	val := reflect.ValueOf(3.14)
	extVal := NewExternalValue(val)

	result := extVal.Type()
	expected := reflect.TypeOf(3.14)

	if result != expected {
		t.Errorf("Type() = %v, want %v", result, expected)
	}
}

func TestExternalValue_Kind(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected reflect.Kind
	}{
		{
			name:     "int kind",
			value:    42,
			expected: reflect.Int,
		},
		{
			name:     "string kind",
			value:    "hello",
			expected: reflect.String,
		},
		{
			name:     "slice kind",
			value:    []int{1, 2, 3},
			expected: reflect.Slice,
		},
		{
			name:     "map kind",
			value:    map[string]int{"key": 1},
			expected: reflect.Map,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val := reflect.ValueOf(tt.value)
			extVal := NewExternalValue(val)

			result := extVal.Kind()
			if result != tt.expected {
				t.Errorf("Kind() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExternalValue_IsValid(t *testing.T) {
	// 测试有效值
	val := reflect.ValueOf(42)
	extVal := NewExternalValue(val)

	if !extVal.IsValid() {
		t.Errorf("IsValid() = false, want true for valid value")
	}

	// 测试无效值
	invalidVal := reflect.Value{}
	invalidExtVal := NewExternalValue(invalidVal)

	if invalidExtVal.IsValid() {
		t.Errorf("IsValid() = true, want false for invalid value")
	}
}

func TestExternalValue_IsNil(t *testing.T) {
	// 测试非 nil 值
	val := reflect.ValueOf(42)
	extVal := NewExternalValue(val)

	if extVal.IsNil() {
		t.Errorf("IsNil() = true, want false for non-nil value")
	}

	// 测试 nil 指针
	var ptr *int
	nilVal := reflect.ValueOf(ptr)
	nilExtVal := NewExternalValue(nilVal)

	if !nilExtVal.IsNil() {
		t.Errorf("IsNil() = false, want true for nil pointer")
	}
}

func TestExternalValue_NumericMethods(t *testing.T) {
	// 测试 Int() 方法
	t.Run("Int method", func(t *testing.T) {
		val := reflect.ValueOf(int64(42))
		extVal := NewExternalValue(val)

		result := extVal.Int()
		expected := int64(42)

		if result != expected {
			t.Errorf("Int() = %v, want %v", result, expected)
		}
	})

	// 测试 Uint() 方法
	t.Run("Uint method", func(t *testing.T) {
		val := reflect.ValueOf(uint64(42))
		extVal := NewExternalValue(val)

		result := extVal.Uint()
		expected := uint64(42)

		if result != expected {
			t.Errorf("Uint() = %v, want %v", result, expected)
		}
	})

	// 测试 Float() 方法
	t.Run("Float method", func(t *testing.T) {
		val := reflect.ValueOf(3.14)
		extVal := NewExternalValue(val)

		result := extVal.Float()
		expected := 3.14

		if result != expected {
			t.Errorf("Float() = %v, want %v", result, expected)
		}
	})

	// 测试 Bool() 方法
	t.Run("Bool method", func(t *testing.T) {
		val := reflect.ValueOf(true)
		extVal := NewExternalValue(val)

		result := extVal.Bool()
		expected := true

		if result != expected {
			t.Errorf("Bool() = %v, want %v", result, expected)
		}
	})
}

func TestExternalValue_CollectionMethods(t *testing.T) {
	// 测试 Len() 方法
	t.Run("Len method", func(t *testing.T) {
		slice := []int{1, 2, 3, 4, 5}
		val := reflect.ValueOf(slice)
		extVal := NewExternalValue(val)

		result := extVal.Len()
		expected := 5

		if result != expected {
			t.Errorf("Len() = %v, want %v", result, expected)
		}
	})

	// 测试 Cap() 方法
	t.Run("Cap method", func(t *testing.T) {
		slice := make([]int, 3, 10)
		val := reflect.ValueOf(slice)
		extVal := NewExternalValue(val)

		result := extVal.Cap()
		expected := 10

		if result != expected {
			t.Errorf("Cap() = %v, want %v", result, expected)
		}
	})

	// 测试 Index() 方法
	t.Run("Index method", func(t *testing.T) {
		slice := []string{"a", "b", "c"}
		val := reflect.ValueOf(slice)
		extVal := NewExternalValue(val)

		result := extVal.Index(1)
		expected := value.NewRValueOf(reflect.ValueOf("b"))

		if result.Interface() != expected.Interface() {
			t.Errorf("Index(1) = %v, want %v", result.Interface(), expected.Interface())
		}
	})
}

func TestExternalValue_MapIndex(t *testing.T) {
	m := map[string]int{"key1": 10, "key2": 20}
	val := reflect.ValueOf(m)
	extVal := NewExternalValue(val)

	keyVal := value.ValueOf("key1")
	result := extVal.MapIndex(keyVal)
	expected := value.NewRValueOf(reflect.ValueOf(10))

	if result.Interface() != expected.Interface() {
		t.Errorf("MapIndex() = %v, want %v", result.Interface(), expected.Interface())
	}
}

func TestExternalValue_RValue(t *testing.T) {
	originalVal := reflect.ValueOf(42)
	extVal := NewExternalValue(originalVal)

	result := extVal.RValue()

	if result != originalVal {
		t.Errorf("RValue() = %v, want %v", result, originalVal)
	}
}
