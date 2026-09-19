package gofunction

import (
	"reflect"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
)

// 常用值单例表（P3-2）：解释器每条指令的结果都要包装成 value.Value，
// 其中 bool 与小整数占比极高（剖析显示比较结果装箱占 13.7% 分配对象数）。
// 这里预构造不可变单例，热路径直接返回，消除 reflect.ValueOf + 接口装箱。
//
// 安全性：reflect.ValueOf 的产物不可寻址（CanAddr()==false），
// 解释器不会对指令结果调用 Set —— 写操作一律经 runStore 走地址值
// （*ssa.Alloc / FreeVar 的 cell 指针）或容器元素，不会落到这些单例上。
// 该不变量由 TestValueCacheImmutable 固化。
var (
	trueValue  value.Value = value.RValue{Value: reflect.ValueOf(true)}
	falseValue value.Value = value.RValue{Value: reflect.ValueOf(false)}
)

// 预声明 int 小整数缓存区间：覆盖循环计数、切片索引、小常量、
// 取模/位运算中间值等绝大多数场景（实测 arith i*2%97+1 的
// 中间值可达 ~2n，区间取 ±4096 平衡内存与命中率）。
const (
	smallIntMin = -4096
	smallIntMax = 4096
)

// smallIntValues 下标 = 值 - smallIntMin，元素类型恒为预声明 int
var smallIntValues [smallIntMax - smallIntMin + 1]value.Value

func init() {
	for i := range smallIntValues {
		smallIntValues[i] = value.RValue{Value: reflect.ValueOf(i + smallIntMin)}
	}
}

// boolValue 返回 bool 单例（类型为预声明 bool，与原
// reflect.ValueOf(b) 完全等价）
func boolValue(b bool) value.Value {
	if b {
		return trueValue
	}
	return falseValue
}

// intValue 返回预声明 int 值；命中缓存区间时零分配。
// 仅用于目标类型确为预声明 int 的场景，调用方须自行保证。
func intValue(n int64) value.Value {
	if n >= smallIntMin && n <= smallIntMax {
		return smallIntValues[n-smallIntMin]
	}
	return value.RValue{Value: reflect.ValueOf(int(n))}
}
