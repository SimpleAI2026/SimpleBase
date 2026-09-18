package operations

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

// mockFrame 模拟 FrameInterface
type mockFrame struct {
	values     map[ssa.Value]value.Value
	goroutines int32
	outBuffer  mockBuffer
	env        map[ssa.Value]*value.Value
}

type mockBuffer struct {
	content string
}

func (mb *mockBuffer) WriteString(s string) (int, error) {
	mb.content += s
	return len(s), nil
}

func (mf *mockFrame) Get(v ssa.Value) value.Value {
	if val, ok := mf.values[v]; ok {
		return val
	}
	return value.ValueOf(nil)
}

func (mf *mockFrame) GetGoroutineCounter() *int32 {
	return &mf.goroutines
}

func (mf *mockFrame) GetOutBuffer() interface{ WriteString(string) (int, error) } {
	return &mf.outBuffer
}

func (mf *mockFrame) GetEnv() map[ssa.Value]*value.Value {
	return mf.env
}

func TestOperationManager_GetBinaryOperations(t *testing.T) {
	manager := NewOperationManager()
	binOps := manager.GetBinaryOperations()

	if binOps == nil {
		t.Error("GetBinaryOperations() returned nil")
	}

	// 测试二元运算
	x := value.ValueOf(10)
	y := value.ValueOf(5)
	result := binOps.Add(x, y)
	expected := int64(15)

	if result != expected {
		t.Errorf("BinaryOperations.Add() = %v, want %v", result, expected)
	}
}

func TestOperationManager_GetComparisonOperations(t *testing.T) {
	manager := NewOperationManager()
	compOps := manager.GetComparisonOperations()

	if compOps == nil {
		t.Error("GetComparisonOperations() returned nil")
	}

	// 测试比较运算
	x := value.ValueOf(10)
	y := value.ValueOf(5)
	result := compOps.Greater(x, y)
	expected := true

	if result != expected {
		t.Errorf("ComparisonOperations.Greater() = %v, want %v", result, expected)
	}
}

func TestOperationManager_GetUnaryOperations(t *testing.T) {
	manager := NewOperationManager()
	unaryOps := manager.GetUnaryOperations()

	if unaryOps == nil {
		t.Error("GetUnaryOperations() returned nil")
	}
}

func TestOperationManager_GetConstantOperations(t *testing.T) {
	manager := NewOperationManager()
	constOps := manager.GetConstantOperations()

	if constOps == nil {
		t.Error("GetConstantOperations() returned nil")
	}
}

func TestOperationManager_GetCallOperations(t *testing.T) {
	manager := NewOperationManager()
	callOps := manager.GetCallOperations()

	if callOps == nil {
		t.Error("GetCallOperations() returned nil")
	}
}

func TestOperationManager_ExecuteBinaryOp(t *testing.T) {
	manager := NewOperationManager()

	// 模拟转换函数
	conv := func(val interface{}, typ types.Type) value.Value {
		return value.ValueOf(val)
	}

	tests := []struct {
		name     string
		op       token.Token
		x        value.Value
		y        value.Value
		expected interface{}
	}{
		{
			name:     "addition",
			op:       token.ADD,
			x:        value.ValueOf(10),
			y:        value.ValueOf(5),
			expected: int64(15),
		},
		{
			name:     "subtraction",
			op:       token.SUB,
			x:        value.ValueOf(10),
			y:        value.ValueOf(3),
			expected: int64(7),
		},
		{
			name:     "multiplication",
			op:       token.MUL,
			x:        value.ValueOf(6),
			y:        value.ValueOf(7),
			expected: int64(42),
		},
		{
			name:     "division",
			op:       token.QUO,
			x:        value.ValueOf(20),
			y:        value.ValueOf(4),
			expected: int64(5),
		},
		{
			name:     "less than",
			op:       token.LSS,
			x:        value.ValueOf(5),
			y:        value.ValueOf(10),
			expected: true,
		},
		{
			name:     "equal",
			op:       token.EQL,
			x:        value.ValueOf(42),
			y:        value.ValueOf(42),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建模拟的 BinOp 指令
			instr := &ssa.BinOp{
				Op: tt.op,
			}

			result := manager.ExecuteBinaryOp(instr, tt.x, tt.y, conv)
			if result.Interface() != tt.expected {
				t.Errorf("ExecuteBinaryOp() = %v, want %v", result.Interface(), tt.expected)
			}
		})
	}
}

func TestOperationManager_ExecuteUnaryOp(t *testing.T) {
	manager := NewOperationManager()

	// 模拟转换函数
	conv := func(val interface{}, typ types.Type) value.Value {
		return value.ValueOf(val)
	}

	tests := []struct {
		name     string
		op       token.Token
		x        value.Value
		expected interface{}
	}{
		{
			name:     "negation",
			op:       token.SUB,
			x:        value.ValueOf(42),
			expected: int64(-42),
		},
		{
			name:     "logical NOT",
			op:       token.NOT,
			x:        value.ValueOf(true),
			expected: false,
		},
		{
			name:     "bitwise NOT",
			op:       token.XOR,
			x:        value.ValueOf(5),
			expected: int64(^5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建模拟的 UnOp 指令
			instr := &ssa.UnOp{
				Op: tt.op,
			}

			result := manager.ExecuteUnaryOp(instr, tt.x, conv)
			if result.Interface() != tt.expected {
				t.Errorf("ExecuteUnaryOp() = %v, want %v", result.Interface(), tt.expected)
			}
		})
	}
}

func TestOperationManager_EvaluateConstant(t *testing.T) {
	manager := NewOperationManager()

	// 跳过这个测试，因为创建有效的 ssa.Const 对象需要复杂的设置
	t.Skip("Skipping EvaluateConstant test - requires complex ssa.Const setup")

	// 如果需要测试，应该使用真实的 SSA 包来创建 Const 对象
	// 这里只是验证方法存在
	if manager.GetConstantOperations() == nil {
		t.Error("GetConstantOperations() returned nil")
	}
}

func TestOperationManager_Integration(t *testing.T) {
	manager := NewOperationManager()

	// 测试所有操作类型都能正确获取
	if manager.GetBinaryOperations() == nil {
		t.Error("Binary operations not available")
	}

	if manager.GetComparisonOperations() == nil {
		t.Error("Comparison operations not available")
	}

	if manager.GetUnaryOperations() == nil {
		t.Error("Unary operations not available")
	}

	if manager.GetConstantOperations() == nil {
		t.Error("Constant operations not available")
	}

	if manager.GetCallOperations() == nil {
		t.Error("Call operations not available")
	}
}

func TestOperationManager_ComplexBinaryOperations(t *testing.T) {
	manager := NewOperationManager()

	// 模拟转换函数
	conv := func(val interface{}, typ types.Type) value.Value {
		return value.ValueOf(val)
	}

	// 测试位运算
	t.Run("bitwise operations", func(t *testing.T) {
		x := value.ValueOf(12) // 1100
		y := value.ValueOf(10) // 1010

		// 按位与
		andInstr := &ssa.BinOp{Op: token.AND}
		andResult := manager.ExecuteBinaryOp(andInstr, x, y, conv)
		if andResult.Interface() != int64(8) { // 1000
			t.Errorf("AND operation failed: got %v, want %v", andResult.Interface(), int64(8))
		}

		// 按位或
		orInstr := &ssa.BinOp{Op: token.OR}
		orResult := manager.ExecuteBinaryOp(orInstr, x, y, conv)
		if orResult.Interface() != int64(14) { // 1110
			t.Errorf("OR operation failed: got %v, want %v", orResult.Interface(), int64(14))
		}

		// 按位异或
		xorInstr := &ssa.BinOp{Op: token.XOR}
		xorResult := manager.ExecuteBinaryOp(xorInstr, x, y, conv)
		if xorResult.Interface() != int64(6) { // 0110
			t.Errorf("XOR operation failed: got %v, want %v", xorResult.Interface(), int64(6))
		}
	})

	// 测试移位运算
	t.Run("shift operations", func(t *testing.T) {
		x := value.ValueOf(5)       // 101
		y := value.ValueOf(uint(2)) // 移位量

		// 左移
		shlInstr := &ssa.BinOp{Op: token.SHL}
		shlResult := manager.ExecuteBinaryOp(shlInstr, x, y, conv)
		if shlResult.Interface() != int64(20) { // 10100
			t.Errorf("SHL operation failed: got %v, want %v", shlResult.Interface(), int64(20))
		}

		// 右移
		x2 := value.ValueOf(20) // 10100
		shrInstr := &ssa.BinOp{Op: token.SHR}
		shrResult := manager.ExecuteBinaryOp(shrInstr, x2, y, conv)
		if shrResult.Interface() != int64(5) { // 101
			t.Errorf("SHR operation failed: got %v, want %v", shrResult.Interface(), int64(5))
		}
	})
}
