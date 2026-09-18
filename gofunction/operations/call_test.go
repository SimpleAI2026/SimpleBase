package operations

import (
	"go/token"
	"go/types"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

// mockFrameForCall 模拟 FrameInterface 用于调用测试
type mockFrameForCall struct {
	values map[ssa.Value]value.Value
	goroutines int32
	outBuffer mockBufferForCall
	env map[ssa.Value]*value.Value
}

type mockBufferForCall struct {
	content string
}

func (mb *mockBufferForCall) WriteString(s string) (int, error) {
	mb.content += s
	return len(s), nil
}

func (mf *mockFrameForCall) Get(v ssa.Value) value.Value {
	if val, ok := mf.values[v]; ok {
		return val
	}
	return value.ValueOf(nil)
}

func (mf *mockFrameForCall) GetGoroutineCounter() *int32 {
	return &mf.goroutines
}

func (mf *mockFrameForCall) GetOutBuffer() interface{ WriteString(string) (int, error) } {
	return &mf.outBuffer
}

func (mf *mockFrameForCall) GetEnv() map[ssa.Value]*value.Value {
	return mf.env
}

// 创建模拟的 SSA 值
func createMockSSAValue(name string) ssa.Value {
	// 这里我们使用一个简单的实现
	return &mockSSAValue{name: name}
}

type mockSSAValue struct {
	name string
}

func (m *mockSSAValue) Name() string {
	return m.name
}

func (m *mockSSAValue) String() string {
	return m.name
}

func (m *mockSSAValue) Type() types.Type {
	// 返回函数类型而不是基本类型
	sig := types.NewSignature(nil, nil, nil, false)
	return sig
}

func (m *mockSSAValue) Parent() *ssa.Function {
	return nil
}

func (m *mockSSAValue) Pos() token.Pos {
	return token.NoPos
}

func (m *mockSSAValue) Referrers() *[]ssa.Instruction {
	return nil
}

// 创建模拟的 CallCommon
func createMockCallCommon(value ssa.Value, args []ssa.Value, hasReceiver bool) *ssa.CallCommon {
	cc := &ssa.CallCommon{
		Value: value,
		Args:  args,
	}
	// 注意：ssa.CallCommon 的 Signature 字段需要通过其他方式设置
	// 这里我们创建一个简单的模拟
	return cc
}

// 创建简单的模拟 CallCommon，直接使用 ssa.CallCommon 并设置正确的类型
func createSimpleCallCommon(value ssa.Value, args []ssa.Value) *ssa.CallCommon {
	cc := &ssa.CallCommon{
		Value: value,
		Args:  args,
	}
	return cc
}

// 创建带接收者的 CallCommon
func createCallCommonWithReceiver(value ssa.Value, args []ssa.Value) *ssa.CallCommon {
	cc := &ssa.CallCommon{
		Value: value,
		Args:  args,
	}
	return cc
}

func TestCallOperations_GoCall(t *testing.T) {
	co := NewCallOperations()
	frame := &mockFrameForCall{
		values: make(map[ssa.Value]value.Value),
		env:    make(map[ssa.Value]*value.Value),
	}
	
	// 模拟外部调用函数
	callExternal := func(rv reflect.Value, args []value.Value) value.Value {
		return value.ValueOf("external_result")
	}
	
	// 模拟调用函数
	call := func(fr FrameInterface, pos token.Pos, fn interface{}, args []value.Value) value.Value {
		return value.ValueOf("call_result")
	}
	
	t.Run("go call without receiver", func(t *testing.T) {
		// 创建模拟的 CallCommon
		instr := createSimpleCallCommon(
			createMockSSAValue("testFunc"),
			[]ssa.Value{createMockSSAValue("arg1")},
		)
		
		// 设置参数值
		frame.values[instr.Args[0]] = value.ValueOf(42)
		
		initialCount := atomic.LoadInt32(&frame.goroutines)
		
		co.GoCall(frame, instr, callExternal, call)
		
		// 等待一小段时间让 goroutine 启动
		time.Sleep(10 * time.Millisecond)
		
		// 验证 goroutine 计数器被正确管理
		finalCount := atomic.LoadInt32(&frame.goroutines)
		if finalCount != initialCount {
			t.Errorf("Goroutine count not properly managed: initial=%d, final=%d", initialCount, finalCount)
		}
	})
}

func TestCallOperations_CallOp(t *testing.T) {
	co := NewCallOperations()
	frame := &mockFrameForCall{
		values: make(map[ssa.Value]value.Value),
		env:    make(map[ssa.Value]*value.Value),
	}
	
	// 模拟外部调用函数
	callExternal := func(rv reflect.Value, args []value.Value) value.Value {
		return value.ValueOf("external_result")
	}
	
	// 模拟调用函数
	call := func(fr FrameInterface, pos token.Pos, fn interface{}, args []value.Value) value.Value {
		return value.ValueOf("call_result")
	}
	
	t.Run("call function without receiver", func(t *testing.T) {
		// 创建模拟的 CallCommon
		instr := createSimpleCallCommon(
			createMockSSAValue("testFunc"),
			[]ssa.Value{createMockSSAValue("arg1")},
		)
		
		// 设置参数值
		frame.values[instr.Args[0]] = value.ValueOf(42)
		
		result := co.CallOp(frame, instr, callExternal, call)
		
		if result.Interface() != "call_result" {
			t.Errorf("CallOp() = %v, want %v", result.Interface(), "call_result")
		}
	})
	
	t.Run("call method with receiver", func(t *testing.T) {
		// 创建模拟的 CallCommon with receiver
		instr := createCallCommonWithReceiver(
			createMockSSAValue("testMethod"),
			[]ssa.Value{createMockSSAValue("receiver"), createMockSSAValue("arg1")},
		)
		
		// 创建一个有方法的类型
		type TestStruct struct {
			Value int
		}
		testObj := TestStruct{Value: 42}
		
		// 设置参数值
		frame.values[instr.Args[0]] = value.ValueOf(testObj)
		frame.values[instr.Args[1]] = value.ValueOf(10)
		
		result := co.CallOp(frame, instr, callExternal, call)
		
		if result.Interface() != "call_result" {
			t.Errorf("CallOp() = %v, want %v", result.Interface(), "call_result")
		}
	})
}

func TestCallOperations_Call(t *testing.T) {
	co := NewCallOperations()
	frame := &mockFrameForCall{
		values: make(map[ssa.Value]value.Value),
		env:    make(map[ssa.Value]*value.Value),
	}
	
	// 模拟 SSA 调用函数
	callSSA := func(fr FrameInterface, fn *ssa.Function, args []value.Value, closure []*value.Value) value.Value {
		return value.ValueOf("ssa_result")
	}
	
	// 模拟内置函数调用
	callBuiltin := func(fr FrameInterface, pos token.Pos, builtin *ssa.Builtin, args []value.Value) value.Value {
		return value.ValueOf("builtin_result")
	}
	
	// 模拟外部调用函数
	callExternal := func(rv reflect.Value, args []value.Value) value.Value {
		return value.ValueOf("external_result")
	}
	
	t.Run("call SSA function", func(t *testing.T) {
		// 创建模拟的 SSA 函数
		fn := &ssa.Function{}
		args := []value.Value{value.ValueOf(42)}
		
		result := co.Call(frame, token.NoPos, fn, args, callSSA, callBuiltin, callExternal)
		
		if result.Interface() != "ssa_result" {
			t.Errorf("Call() with SSA function = %v, want %v", result.Interface(), "ssa_result")
		}
	})
	
	t.Run("call builtin function", func(t *testing.T) {
		// 创建模拟的内置函数
		builtin := &ssa.Builtin{}
		args := []value.Value{value.ValueOf(42)}
		
		result := co.Call(frame, token.NoPos, builtin, args, callSSA, callBuiltin, callExternal)
		
		if result.Interface() != "builtin_result" {
			t.Errorf("Call() with builtin function = %v, want %v", result.Interface(), "builtin_result")
		}
	})
	
	t.Run("call external value", func(t *testing.T) {
		// 创建模拟的外部值
		extVal := NewExternalValue(reflect.ValueOf(func(x int) int { return x * 2 }))
		args := []value.Value{value.ValueOf(21)}
		
		result := co.Call(frame, token.NoPos, extVal, args, callSSA, callBuiltin, callExternal)
		
		if result.Interface() != "external_result" {
			t.Errorf("Call() with external value = %v, want %v", result.Interface(), "external_result")
		}
	})
	
	t.Run("call with reflect value", func(t *testing.T) {
		// 创建一个普通的函数
		fn := func(x int) int { return x * 3 }
		args := []value.Value{value.ValueOf(14)}
		
		result := co.Call(frame, token.NoPos, fn, args, callSSA, callBuiltin, callExternal)
		
		if result.Interface() != "external_result" {
			t.Errorf("Call() with reflect value = %v, want %v", result.Interface(), "external_result")
		}
	})
	
	t.Run("call nil function should panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Call() with nil function should panic")
			}
		}()
		
		var fn *ssa.Function = nil
		args := []value.Value{value.ValueOf(42)}
		
		co.Call(frame, token.NoPos, fn, args, callSSA, callBuiltin, callExternal)
	})
}

func TestNewCallOperations(t *testing.T) {
	co := NewCallOperations()
	
	if co == nil {
		t.Error("NewCallOperations() returned nil")
	}
	
	// 验证返回的是 CallOperations 类型
		if _, ok := interface{}(co).(*CallOperations); !ok {
			t.Error("NewCallOperations() did not return *CallOperations")
		}
}

func TestCallOperations_Integration(t *testing.T) {
	co := NewCallOperations()
	frame := &mockFrameForCall{
		values: make(map[ssa.Value]value.Value),
		env:    make(map[ssa.Value]*value.Value),
	}
	
	// 集成测试：测试所有方法都能正常工作
	t.Run("all methods work together", func(t *testing.T) {
		// 模拟函数
		callExternal := func(rv reflect.Value, args []value.Value) value.Value {
			return value.ValueOf("integrated_external")
		}
		
		call := func(fr FrameInterface, pos token.Pos, fn interface{}, args []value.Value) value.Value {
			return value.ValueOf("integrated_call")
		}
		
		callSSA := func(fr FrameInterface, fn *ssa.Function, args []value.Value, closure []*value.Value) value.Value {
			return value.ValueOf("integrated_ssa")
		}
		
		callBuiltin := func(fr FrameInterface, pos token.Pos, builtin *ssa.Builtin, args []value.Value) value.Value {
			return value.ValueOf("integrated_builtin")
		}
		
		// 测试 CallOp
		instr := createSimpleCallCommon(
			createMockSSAValue("testFunc"),
			[]ssa.Value{createMockSSAValue("arg1")},
		)
		frame.values[instr.Args[0]] = value.ValueOf(42)
		
		result1 := co.CallOp(frame, instr, callExternal, call)
		if result1.Interface() != "integrated_call" {
			t.Errorf("Integrated CallOp failed: got %v", result1.Interface())
		}
		
		// 测试 Call with SSA function
		fn := &ssa.Function{}
		args := []value.Value{value.ValueOf(42)}
		result2 := co.Call(frame, token.NoPos, fn, args, callSSA, callBuiltin, callExternal)
		if result2.Interface() != "integrated_ssa" {
			t.Errorf("Integrated Call with SSA failed: got %v", result2.Interface())
		}
	})
}