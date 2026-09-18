package operations

import (
	"testing"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
)

func TestComparisonOperations_Less(t *testing.T) {
	co := NewComparisonOperations()

	tests := []struct {
		name     string
		x        value.Value
		y        value.Value
		expected interface{}
	}{
		{
			name:     "string less than",
			x:        value.ValueOf("apple"),
			y:        value.ValueOf("banana"),
			expected: true,
		},
		{
			name:     "string not less than",
			x:        value.ValueOf("zebra"),
			y:        value.ValueOf("apple"),
			expected: false,
		},
		{
			name:     "int less than",
			x:        value.ValueOf(5),
			y:        value.ValueOf(10),
			expected: true,
		},
		{
			name:     "int not less than",
			x:        value.ValueOf(15),
			y:        value.ValueOf(10),
			expected: false,
		},
		{
			name:     "float less than",
			x:        value.ValueOf(3.14),
			y:        value.ValueOf(3.15),
			expected: true,
		},
		{
			name:     "uint less than",
			x:        value.ValueOf(uint(5)),
			y:        value.ValueOf(uint(10)),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := co.Less(tt.x, tt.y)
			if result != tt.expected {
				t.Errorf("Less() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestComparisonOperations_LessEqual(t *testing.T) {
	co := NewComparisonOperations()

	tests := []struct {
		name     string
		x        value.Value
		y        value.Value
		expected interface{}
	}{
		{
			name:     "string less equal",
			x:        value.ValueOf("apple"),
			y:        value.ValueOf("apple"),
			expected: true,
		},
		{
			name:     "int less equal",
			x:        value.ValueOf(10),
			y:        value.ValueOf(10),
			expected: true,
		},
		{
			name:     "float less equal",
			x:        value.ValueOf(3.14),
			y:        value.ValueOf(3.15),
			expected: true,
		},
		{
			name:     "uint not less equal",
			x:        value.ValueOf(uint(15)),
			y:        value.ValueOf(uint(10)),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := co.LessEqual(tt.x, tt.y)
			if result != tt.expected {
				t.Errorf("LessEqual() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestComparisonOperations_Equal(t *testing.T) {
	co := NewComparisonOperations()

	tests := []struct {
		name     string
		x        value.Value
		y        value.Value
		expected interface{}
	}{
		{
			name:     "string equal",
			x:        value.ValueOf("hello"),
			y:        value.ValueOf("hello"),
			expected: true,
		},
		{
			name:     "string not equal",
			x:        value.ValueOf("hello"),
			y:        value.ValueOf("world"),
			expected: false,
		},
		{
			name:     "int equal",
			x:        value.ValueOf(42),
			y:        value.ValueOf(42),
			expected: true,
		},
		{
			name:     "float equal",
			x:        value.ValueOf(3.14),
			y:        value.ValueOf(3.14),
			expected: true,
		},
		{
			name:     "uint equal",
			x:        value.ValueOf(uint(100)),
			y:        value.ValueOf(uint(100)),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := co.Equal(tt.x, tt.y)
			if result != tt.expected {
				t.Errorf("Equal() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestComparisonOperations_NotEqual(t *testing.T) {
	co := NewComparisonOperations()

	tests := []struct {
		name     string
		x        value.Value
		y        value.Value
		expected interface{}
	}{
		{
			name:     "string not equal",
			x:        value.ValueOf("hello"),
			y:        value.ValueOf("world"),
			expected: true,
		},
		{
			name:     "int not equal",
			x:        value.ValueOf(10),
			y:        value.ValueOf(20),
			expected: true,
		},
		{
			name:     "equal values",
			x:        value.ValueOf(42),
			y:        value.ValueOf(42),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := co.NotEqual(tt.x, tt.y)
			if result != tt.expected {
				t.Errorf("NotEqual() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestComparisonOperations_Greater(t *testing.T) {
	co := NewComparisonOperations()

	tests := []struct {
		name     string
		x        value.Value
		y        value.Value
		expected interface{}
	}{
		{
			name:     "string greater",
			x:        value.ValueOf("zebra"),
			y:        value.ValueOf("apple"),
			expected: true,
		},
		{
			name:     "int greater",
			x:        value.ValueOf(20),
			y:        value.ValueOf(10),
			expected: true,
		},
		{
			name:     "float not greater",
			x:        value.ValueOf(3.14),
			y:        value.ValueOf(3.15),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := co.Greater(tt.x, tt.y)
			if result != tt.expected {
				t.Errorf("Greater() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestComparisonOperations_GreaterEqual(t *testing.T) {
	co := NewComparisonOperations()

	tests := []struct {
		name     string
		x        value.Value
		y        value.Value
		expected interface{}
	}{
		{
			name:     "string greater equal",
			x:        value.ValueOf("apple"),
			y:        value.ValueOf("apple"),
			expected: true,
		},
		{
			name:     "int greater equal",
			x:        value.ValueOf(20),
			y:        value.ValueOf(10),
			expected: true,
		},
		{
			name:     "float equal",
			x:        value.ValueOf(3.14),
			y:        value.ValueOf(3.14),
			expected: true,
		},
		{
			name:     "uint not greater equal",
			x:        value.ValueOf(uint(5)),
			y:        value.ValueOf(uint(10)),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := co.GreaterEqual(tt.x, tt.y)
			if result != tt.expected {
				t.Errorf("GreaterEqual() = %v, want %v", result, tt.expected)
			}
		})
	}
}
