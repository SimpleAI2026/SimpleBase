package operations

import (
	"testing"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
)

func TestBinaryOperations_Add(t *testing.T) {
	bo := NewBinaryOperations()

	tests := []struct {
		name     string
		x        value.Value
		y        value.Value
		expected interface{}
	}{
		{
			name:     "string addition",
			x:        value.ValueOf("hello"),
			y:        value.ValueOf(" world"),
			expected: "hello world",
		},
		{
			name:     "int addition",
			x:        value.ValueOf(10),
			y:        value.ValueOf(20),
			expected: int64(30),
		},
		{
			name:     "float addition",
			x:        value.ValueOf(3.14),
			y:        value.ValueOf(2.86),
			expected: 6.0,
		},
		{
			name:     "uint addition",
			x:        value.ValueOf(uint(5)),
			y:        value.ValueOf(uint(15)),
			expected: uint64(20),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bo.Add(tt.x, tt.y)
			if result != tt.expected {
				t.Errorf("Add() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBinaryOperations_Sub(t *testing.T) {
	bo := NewBinaryOperations()

	tests := []struct {
		name     string
		x        value.Value
		y        value.Value
		expected interface{}
	}{
		{
			name:     "int subtraction",
			x:        value.ValueOf(30),
			y:        value.ValueOf(10),
			expected: int64(20),
		},
		{
			name:     "float subtraction",
			x:        value.ValueOf(5.5),
			y:        value.ValueOf(2.5),
			expected: 3.0,
		},
		{
			name:     "uint subtraction",
			x:        value.ValueOf(uint(25)),
			y:        value.ValueOf(uint(5)),
			expected: uint64(20),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bo.Sub(tt.x, tt.y)
			if result != tt.expected {
				t.Errorf("Sub() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBinaryOperations_Mul(t *testing.T) {
	bo := NewBinaryOperations()

	tests := []struct {
		name     string
		x        value.Value
		y        value.Value
		expected interface{}
	}{
		{
			name:     "int multiplication",
			x:        value.ValueOf(6),
			y:        value.ValueOf(7),
			expected: int64(42),
		},
		{
			name:     "float multiplication",
			x:        value.ValueOf(2.5),
			y:        value.ValueOf(4.0),
			expected: 10.0,
		},
		{
			name:     "uint multiplication",
			x:        value.ValueOf(uint(3)),
			y:        value.ValueOf(uint(4)),
			expected: uint64(12),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bo.Mul(tt.x, tt.y)
			if result != tt.expected {
				t.Errorf("Mul() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBinaryOperations_Quo(t *testing.T) {
	bo := NewBinaryOperations()

	tests := []struct {
		name     string
		x        value.Value
		y        value.Value
		expected interface{}
	}{
		{
			name:     "int division",
			x:        value.ValueOf(20),
			y:        value.ValueOf(4),
			expected: int64(5),
		},
		{
			name:     "float division",
			x:        value.ValueOf(10.0),
			y:        value.ValueOf(2.0),
			expected: 5.0,
		},
		{
			name:     "uint division",
			x:        value.ValueOf(uint(15)),
			y:        value.ValueOf(uint(3)),
			expected: uint64(5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bo.Quo(tt.x, tt.y)
			if result != tt.expected {
				t.Errorf("Quo() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBinaryOperations_Rem(t *testing.T) {
	bo := NewBinaryOperations()

	tests := []struct {
		name     string
		x        value.Value
		y        value.Value
		expected interface{}
	}{
		{
			name:     "int remainder",
			x:        value.ValueOf(17),
			y:        value.ValueOf(5),
			expected: int64(2),
		},
		{
			name:     "uint remainder",
			x:        value.ValueOf(uint(23)),
			y:        value.ValueOf(uint(7)),
			expected: uint64(2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bo.Rem(tt.x, tt.y)
			if result != tt.expected {
				t.Errorf("Rem() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBinaryOperations_BitwiseOperations(t *testing.T) {
	bo := NewBinaryOperations()

	// 测试按位与运算
	t.Run("And operation", func(t *testing.T) {
		x := value.ValueOf(12) // 1100
		y := value.ValueOf(10) // 1010
		result := bo.And(x, y)
		expected := int64(8) // 1000
		if result != expected {
			t.Errorf("And() = %v, want %v", result, expected)
		}
	})

	// 测试按位或运算
	t.Run("Or operation", func(t *testing.T) {
		x := value.ValueOf(12) // 1100
		y := value.ValueOf(10) // 1010
		result := bo.Or(x, y)
		expected := int64(14) // 1110
		if result != expected {
			t.Errorf("Or() = %v, want %v", result, expected)
		}
	})

	// 测试按位异或运算
	t.Run("Xor operation", func(t *testing.T) {
		x := value.ValueOf(12) // 1100
		y := value.ValueOf(10) // 1010
		result := bo.Xor(x, y)
		expected := int64(6) // 0110
		if result != expected {
			t.Errorf("Xor() = %v, want %v", result, expected)
		}
	})

	// 测试按位与非运算
	t.Run("AndNot operation", func(t *testing.T) {
		x := value.ValueOf(12) // 1100
		y := value.ValueOf(10) // 1010
		result := bo.AndNot(x, y)
		expected := int64(4) // 0100
		if result != expected {
			t.Errorf("AndNot() = %v, want %v", result, expected)
		}
	})
}

func TestBinaryOperations_ShiftOperations(t *testing.T) {
	bo := NewBinaryOperations()

	// 测试左移运算
	t.Run("Shl operation", func(t *testing.T) {
		x := value.ValueOf(5)       // 101
		y := value.ValueOf(uint(2)) // 左移2位
		result := bo.Shl(x, y)
		expected := int64(20) // 10100
		if result != expected {
			t.Errorf("Shl() = %v, want %v", result, expected)
		}
	})

	// 测试右移运算
	t.Run("Shr operation", func(t *testing.T) {
		x := value.ValueOf(20)      // 10100
		y := value.ValueOf(uint(2)) // 右移2位
		result := bo.Shr(x, y)
		expected := int64(5) // 101
		if result != expected {
			t.Errorf("Shr() = %v, want %v", result, expected)
		}
	})
}
