package gofunction

// 本文件覆盖 P3 优化引入的不变量：常用值单例表的只读性、
// conv*Typed 全宽度快路径的类型保真、Phi 预解析的多前驱正确性。

import (
	"reflect"
	"testing"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
)

// TestValueCacheImmutable 固化单例表的核心安全前提：
// 单例底层 reflect.Value 不可寻址，任何 Set 尝试都会 panic 而非
// 静默污染全局状态。解释器写路径一律经 runStore 走地址值，
// 不会落到这些单例上。
func TestValueCacheImmutable(t *testing.T) {
	for name, v := range map[string]interface{}{
		"true":  trueValue,
		"false": falseValue,
		"int0":  smallIntValues[-smallIntMin],
	} {
		rv := v.(interface{ RValue() reflect.Value }).RValue()
		if rv.CanAddr() {
			t.Errorf("%s: 单例不应可寻址，否则可能被 Set 污染", name)
		}
		if rv.CanSet() {
			t.Errorf("%s: 单例不应可设置", name)
		}
	}
}

// TestBoolValueSemantics 校验 bool 单例与直接构造完全等价
func TestBoolValueSemantics(t *testing.T) {
	for _, b := range []bool{true, false} {
		got := boolValue(b)
		if got.Bool() != b {
			t.Errorf("boolValue(%v).Bool() = %v", b, got.Bool())
		}
		if got.Type() != reflect.TypeOf(b) {
			t.Errorf("boolValue(%v).Type() = %v, want bool", b, got.Type())
		}
		if got.Interface() != interface{}(b) {
			t.Errorf("boolValue(%v).Interface() mismatch", b)
		}
	}
}

// TestIntValueBoundary 校验小整数缓存边界内外行为一致
func TestIntValueBoundary(t *testing.T) {
	for _, n := range []int64{
		smallIntMin - 1, smallIntMin, smallIntMin + 1,
		-1, 0, 1, 42,
		smallIntMax - 1, smallIntMax, smallIntMax + 1,
		1 << 40, -(1 << 40),
	} {
		got := intValue(n)
		if got.Int() != n {
			t.Errorf("intValue(%d).Int() = %d", n, got.Int())
		}
		if got.Type().Kind() != reflect.Int || got.Type().PkgPath() != "" {
			t.Errorf("intValue(%d).Type() = %v, want 预声明 int", n, got.Type())
		}
	}
}

// truncInt32/16/8 通过变量绕开编译期常量溢出检查，
// 得到与 Go 运行时转换完全一致的截断结果
func truncInt32(v int64) int32    { return int32(v) }
func truncInt16(v int64) int16    { return int16(v) }
func truncInt8(v int64) int8      { return int8(v) }
func truncUint32(v uint64) uint32 { return uint32(v) }
func truncUint16(v uint64) uint16 { return uint16(v) }
func truncUint8(v uint64) uint8   { return uint8(v) }

// TestConvTypedWidthFidelity 校验 P3-4 扩展后各宽度快路径的类型与数值保真，
// 特别是溢出截断行为须与 Go 原生转换一致。
func TestConvTypedWidthFidelity(t *testing.T) {
	intCases := []struct {
		rt   reflect.Type
		in   int64
		want interface{}
	}{
		{reflect.TypeOf(int(0)), 1 << 40, int(1 << 40)},
		{reflect.TypeOf(int64(0)), -1 << 40, int64(-1 << 40)},
		{reflect.TypeOf(int32(0)), 1 << 40, truncInt32(1 << 40)}, // 截断
		{reflect.TypeOf(int16(0)), 70000, truncInt16(70000)},
		{reflect.TypeOf(int8(0)), 300, truncInt8(300)},
	}
	for _, c := range intCases {
		got := convIntTyped(c.in, c.rt).Interface()
		if got != c.want {
			t.Errorf("convIntTyped(%d, %v) = %v(%T), want %v(%T)",
				c.in, c.rt, got, got, c.want, c.want)
		}
	}

	uintCases := []struct {
		rt   reflect.Type
		in   uint64
		want interface{}
	}{
		{reflect.TypeOf(uint(0)), 1 << 40, uint(1 << 40)},
		{reflect.TypeOf(uint64(0)), 1 << 63, uint64(1 << 63)},
		{reflect.TypeOf(uint32(0)), 1 << 40, truncUint32(1 << 40)},
		{reflect.TypeOf(uint16(0)), 70000, truncUint16(70000)},
		{reflect.TypeOf(uint8(0)), 300, truncUint8(300)},
		{reflect.TypeOf(uintptr(0)), 4096, uintptr(4096)},
	}
	for _, c := range uintCases {
		got := convUintTyped(c.in, c.rt).Interface()
		if got != c.want {
			t.Errorf("convUintTyped(%d, %v) = %v(%T), want %v(%T)",
				c.in, c.rt, got, got, c.want, c.want)
		}
	}

	floatCases := []struct {
		rt   reflect.Type
		in   float64
		want interface{}
	}{
		{reflect.TypeOf(float64(0)), 1.5, float64(1.5)},
		{reflect.TypeOf(float32(0)), 1.5, float32(1.5)},
	}
	for _, c := range floatCases {
		got := convFloatTyped(c.in, c.rt).Interface()
		if got != c.want {
			t.Errorf("convFloatTyped(%v, %v) = %v(%T), want %v(%T)",
				c.in, c.rt, got, got, c.want, c.want)
		}
	}
}

// myDuration 模拟 time.Duration 这类命名整型
type myDuration int64

// myWeekday 模拟 time.Weekday（命名 int）
type myWeekday int

// TestConvTypedNamedTypeFidelity 回归守卫：命名类型绝不能被快路径
// 降级为预声明类型（P2 阶段在 time.Duration / time.Weekday 上踩坑两次）。
func TestConvTypedNamedTypeFidelity(t *testing.T) {
	durType := reflect.TypeOf(myDuration(0))
	got := convIntTyped(5, durType)
	if got.Type() != durType {
		t.Fatalf("命名 int64 类型被降级: got %v, want %v", got.Type(), durType)
	}
	if _, ok := got.Interface().(myDuration); !ok {
		t.Fatalf("命名 int64 类型标识丢失: %T", got.Interface())
	}

	wdType := reflect.TypeOf(myWeekday(0))
	got = convIntTyped(6, wdType)
	if got.Type() != wdType {
		t.Fatalf("命名 int 类型被降级: got %v, want %v", got.Type(), wdType)
	}
	if v, ok := got.Interface().(myWeekday); !ok || v != 6 {
		t.Fatalf("命名 int 类型标识丢失: %T %v", got.Interface(), got.Interface())
	}
}

// TestPhiMultiPred 覆盖 compilePhi 的多前驱分支（>2 个前驱），
// 确保预解析后取边值仍与 prevBlock 正确对应。
func TestPhiMultiPred(t *testing.T) {
	// switch 产生 3+ 前驱汇聚到同一后继块
	src := `package main
func classify(n int) string {
	var s string
	switch n {
	case 1:
		s = "one"
	case 2:
		s = "two"
	case 3:
		s = "three"
	default:
		s = "many"
	}
	return s + "!"
}`
	p, err := BuildProgram("phi", "main", src)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		in   int
		want string
	}{{1, "one!"}, {2, "two!"}, {3, "three!"}, {9, "many!"}} {
		got, err := p.Run("", "classify", c.in)
		if err != nil {
			t.Fatalf("classify(%d): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("classify(%d) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestRangeStringSliceMap 覆盖 ssa.Range/Next 对三种序列的迭代语义
// （runRange 曾只实现 map 路径，string range 误走 MapKeys panic，
// 由 examples/gofunction 性能示例冒烟暴露）。
func TestRangeStringSliceMap(t *testing.T) {
	p, err := BuildProgram("range", "main", `package main
func sumStrRunes(s string) (int, int) {
	sum, n := 0, 0
	for _, c := range s {
		sum += int(c)
		n++
	}
	return sum, n
}
func sumSlice(xs []int) int {
	s := 0
	for _, x := range xs {
		s += x
	}
	return s
}
func sumMap(m map[string]int) int {
	s := 0
	for _, v := range m {
		s += v
	}
	return s
}
func idxPositions(xs []int) []int {
	var out []int
	for i, x := range xs {
		if x == 0 {
			out = append(out, i)
		}
	}
	return out
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if sum, n, err := func() (int, int, error) {
		a, err := p.Run("", "sumStrRunes", "héllo")
		if err != nil {
			return 0, 0, err
		}
		pair := a.([]value.Value)
		return int(pair[0].Int()), int(pair[1].Int()), nil
	}(); err != nil || sum != 664 || n != 5 {
		t.Errorf("sumStrRunes(héllo) = %v, %v, %v; want 664, 5 (é=U+00E9 按 rune 计)", sum, n, err)
	}
	if got, err := p.Run("", "sumSlice", []int{1, 2, 3, 4}); err != nil || got != 10 {
		t.Errorf("sumSlice = %v, %v; want 10", got, err)
	}
	if got, err := p.Run("", "sumMap", map[string]int{"a": 1, "b": 2, "c": 3}); err != nil || got != 6 {
		t.Errorf("sumMap = %v, %v; want 6", got, err)
	}
	if got, err := p.Run("", "idxPositions", []int{3, 0, 5, 0}); err != nil {
		t.Errorf("idxPositions: %v", err)
	} else {
		pos := got.([]int)
		if len(pos) != 2 || pos[0] != 1 || pos[1] != 3 {
			t.Errorf("idxPositions = %v; want [1 3]", pos)
		}
	}
}

// TestPhiLoopCarried 覆盖循环携带的 Phi（双前驱：入口边 + 回边）
func TestPhiLoopCarried(t *testing.T) {
	p, err := BuildProgram("phi2", "main", `package main
func sum(n int) int {
	s := 0
	for i := 0; i < n; i++ {
		s += i
	}
	return s
}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run("", "sum", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got != 45 {
		t.Errorf("sum(10) = %v, want 45", got)
	}
}
