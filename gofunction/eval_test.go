package gofunction

import (
	"testing"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

// TestEvalArithmetic 通过完整脚本验证二元算术运算
func TestEvalArithmetic(t *testing.T) {
	tests := []struct{ name, expr, expected string }{
		{"string add", `"hello" + " world"`, "hello world"},
		{"int add", "1 + 2", "3"},
		{"float add", "1.5 + 2.5", "4"},
		{"int sub", "30 - 10", "20"},
		{"float sub", "5.5 - 2.5", "3"},
		{"int mul", "6 * 7", "42"},
		{"float mul", "2.5 * 4", "10"},
		{"int quo", "20 / 4", "5"},
		{"float quo", "7.0 / 2", "3.5"},
		{"int rem", "10 % 3", "1"},
	}
	for _, tt := range tests {
		src := "package main\nimport \"fmt\"\nfunc test() string { return fmt.Sprint(" + tt.expr + ") }"
		res, err := Run("t", src, "test")
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if res != tt.expected {
			t.Errorf("%s = %v, want %v", tt.name, res, tt.expected)
		}
	}
}

// TestEvalBitwise 通过完整脚本验证位运算与移位
func TestEvalBitwise(t *testing.T) {
	tests := []struct{ name, expr, expected string }{
		{"and", "12 & 10", "8"},
		{"or", "12 | 10", "14"},
		{"xor", "12 ^ 10", "6"},
		{"andnot", "12 &^ 10", "4"},
		{"shl", "3 << 2", "12"},
		{"shr", "12 >> 2", "3"},
		{"uint and", "uint(8) & uint(2)", "0"},
		{"uint shl", "uint(8) << uint(1)", "16"},
	}
	for _, tt := range tests {
		src := "package main\nimport \"fmt\"\nfunc test() string { return fmt.Sprint(" + tt.expr + ") }"
		res, err := Run("t", src, "test")
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if res != tt.expected {
			t.Errorf("%s = %v, want %v", tt.name, res, tt.expected)
		}
	}
}

// TestEvalComparison 通过完整脚本验证比较运算
func TestEvalComparison(t *testing.T) {
	tests := []struct{ name, expr, expected string }{
		{"string less", `"a" < "b"`, "true"},
		{"int less", "1 < 2", "true"},
		{"float less", "1.5 < 2.5", "true"},
		{"int leq", "2 <= 2", "true"},
		{"float leq", "2.0 <= 2.0", "true"},
		{"equal", "1 == 1", "true"},
		{"not equal", "1 != 2", "true"},
		{"nil equal", "*new(*int) == nil", "true"},
		{"string greater", `"b" > "a"`, "true"},
		{"int greater", "2 > 1", "true"},
		{"int geq", "2 >= 2", "true"},
		{"float geq", "2.5 >= 1.0", "true"},
	}
	for _, tt := range tests {
		src := "package main\nimport \"fmt\"\nfunc test() string { return fmt.Sprint(" + tt.expr + ") }"
		res, err := Run("t", src, "test")
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if res != tt.expected {
			t.Errorf("%s = %v, want %v", tt.name, res, tt.expected)
		}
	}
}

// TestEvalShiftMixedOperands 移位运算右操作数类型与左操作数不同（Go 语义允许），
// 回归 binopFast 曾按左操作数类型统一取值导致的 uint 上调 Int panic
func TestEvalShiftMixedOperands(t *testing.T) {
	tests := []struct {
		name, src string
		expected  interface{}
	}{
		{"int shl by uint var", `package main
func test(x int) int { return 1 << uint(x) }`, 8},
		{"int shr by uint var", `package main
func test(x int) int { return 256 >> uint(x) }`, 32},
		{"uint shl by int var", `package main
func test(x int) uint { return uint(1) << x }`, uint(8)},
		{"range shift loop", `package main
func test() int {
	pow := make([]int, 10)
	for i := range pow {
		pow[i] = 1 << uint(i)
	}
	return pow[3]
}`, 8},
	}
	for _, tt := range tests {
		p, err := BuildProgram("t", "main", tt.src)
		if err != nil {
			t.Errorf("%s: build: %v", tt.name, err)
			continue
		}
		res, err := p.Run("t", "test", 3)
		if err != nil {
			t.Errorf("%s: run: %v", tt.name, err)
			continue
		}
		if res != tt.expected {
			t.Errorf("%s = %#v, want %#v", tt.name, res, tt.expected)
		}
	}
}

// TestEvalUnary 通过完整脚本验证一元运算
func TestEvalUnary(t *testing.T) {
	tests := []struct{ name, expr, expected string }{
		{"int neg", "-5", "-5"},
		{"int not", "^5", "-6"},
		{"float neg", "-1.5", "-1.5"},
		{"bool not", "!true", "false"},
	}
	for _, tt := range tests {
		src := "package main\nimport \"fmt\"\nfunc test() string { return fmt.Sprint(" + tt.expr + ") }"
		res, err := Run("t", src, "test")
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if res != tt.expected {
			t.Errorf("%s = %v, want %v", tt.name, res, tt.expected)
		}
	}
}
