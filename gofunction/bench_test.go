package gofunction

// 性能基准测试：覆盖编译、解释执行热路径、外部函数调用与并发场景。
//
// 全量运行：
//	go test -run='^$' -bench=. -benchmem .
// CPU/内存剖析（选单个基准）：
//	go test -run='^$' -bench=BenchmarkExecFib -cpuprofile=cpu.prof -memprofile=mem.prof .
//	go tool pprof -top -nodecount=30 cpu.prof
//	go tool pprof -top -nodecount=20 -sample_index=alloc_space mem.prof

import (
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"testing"
)

const benchSeqID = "bench"

// ---------- 基准脚本 ----------

// fibSrc 递归函数调用（测 callSSA / 栈帧创建开销）
const fibSrc = `package main
func fib(n int) int {
	if n < 2 { return n }
	return fib(n-1) + fib(n-2)
}`

// arithSrc 算术循环（测 BinOp/If/Jump/Phi 解释开销）
const arithSrc = `package main
func sumN(n int) int {
	sum := 0
	for i := 0; i < n; i++ {
		sum += i*2%97 + 1
	}
	return sum
}`

// callSrc 脚本内平凡函数调用
const callSrc = `package main
func add(a, b int) int { return a + b }
func benchCall(n int) int {
	s := 0
	for i := 0; i < n; i++ { s = add(s, i) }
	return s
}`

// sliceSrc 切片 append / 索引读写
const sliceSrc = `package main
func benchSlice(n int) int {
	s := make([]int, 0, n)
	for i := 0; i < n; i++ { s = append(s, i) }
	sum := 0
	for i := 0; i < n; i++ { sum += s[i] }
	return sum
}`

// mapSrc map 写入与查找
const mapSrc = `package main
func benchMap(n int) int {
	m := make(map[int]int)
	for i := 0; i < n; i++ { m[i] = i * 3 }
	sum := 0
	for i := 0; i < n; i++ { sum += m[i] }
	return sum
}`

// stringSrc 字符串拼接
const stringSrc = `package main
func benchString(n int) string {
	s := ""
	for i := 0; i < n; i++ { s += "ab" }
	return s
}`

// externalSrc 外部（宿主）函数反射调用
const externalSrc = `package main
import "strings"
func benchExternal(n int) string {
	s := "hello"
	for i := 0; i < n; i++ { s = strings.ToUpper(s) }
	return s
}`

// closureSrc 闭包创建与调用（MakeClosure/FreeVar）
const closureSrc = `package main
func benchClosure(n int) int {
	total := 0
	f := func(x int) { total += x }
	for i := 0; i < n; i++ { f(i) }
	return total
}`

// methodSrc 结构体指针方法调用（Store/FieldAddr + 方法分派）。
// 注意：字段必须导出——解释器经 reflect.StructOf 构造结构体类型，
// 未导出字段缺少 PkgPath 会被 reflect 拒绝（引擎已知限制）。
const methodSrc = `package main
type Counter struct{ N int }
func (c *Counter) Inc() { c.N++ }
func benchMethod(n int) int {
	c := &Counter{}
	for i := 0; i < n; i++ { c.Inc() }
	return c.N
}`

// workSrc 端到端综合场景（编译 + 执行）
const workSrc = `package main
func work() int {
	sum := 0
	for i := 0; i < 1000; i++ { sum += i }
	return sum
}`

// ---------- 辅助 ----------

func mustBuild(b *testing.B, src string) *Program {
	b.Helper()
	p, err := BuildProgram(benchSeqID, "main", src)
	if err != nil {
		b.Fatal(err)
	}
	return p
}

func runBench(b *testing.B, src, fn string, args ...interface{}) {
	b.Helper()
	p := mustBuild(b, src)
	b.ResetTimer()
	for b.Loop() {
		if _, err := p.Run(benchSeqID, fn, args...); err != nil {
			b.Fatal(err)
		}
	}
}

// ---------- 编译阶段 ----------

func BenchmarkBuildProgram(b *testing.B) {
	for b.Loop() {
		if _, err := BuildProgram(benchSeqID, "main", workSrc); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildProgramWithImports(b *testing.B) {
	for b.Loop() {
		if _, err := BuildProgram(benchSeqID, "main", externalSrc); err != nil {
			b.Fatal(err)
		}
	}
}

// ---------- 解释执行（预编译后仅测执行） ----------

func BenchmarkExecArithLoop(b *testing.B) {
	runBench(b, arithSrc, "sumN", 1000)
}

func BenchmarkExecFib18(b *testing.B) {
	runBench(b, fibSrc, "fib", 18)
}

func BenchmarkExecCall100(b *testing.B) {
	runBench(b, callSrc, "benchCall", 100)
}

func BenchmarkExecSlice100(b *testing.B) {
	runBench(b, sliceSrc, "benchSlice", 100)
}

func BenchmarkExecMap100(b *testing.B) {
	runBench(b, mapSrc, "benchMap", 100)
}

func BenchmarkExecString100(b *testing.B) {
	runBench(b, stringSrc, "benchString", 100)
}

func BenchmarkExecExternal100(b *testing.B) {
	runBench(b, externalSrc, "benchExternal", 100)
}

func BenchmarkExecClosure100(b *testing.B) {
	runBench(b, closureSrc, "benchClosure", 100)
}

func BenchmarkExecMethod100(b *testing.B) {
	runBench(b, methodSrc, "benchMethod", 100)
}

// ---------- 端到端（编译 + 执行） ----------

func BenchmarkRunEndToEnd(b *testing.B) {
	for b.Loop() {
		if _, err := Run(benchSeqID, workSrc, "work"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExecutorExecute(b *testing.B) {
	ex := NewExecutor(nil)
	defer ex.Close()
	for b.Loop() {
		if _, err := ex.Execute("work", workSrc); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPoolParallelExecute(b *testing.B) {
	// MaxActive 需 ≥ 并行度（GOMAXPROCS），否则并发取用会耗尽池
	pool := NewExecutorPool(ExecutorPoolConfig{MaxIdle: 16, MaxActive: 256})
	defer pool.Close()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ex, err := pool.GetExecutor()
			if err != nil {
				b.Error(err)
				return
			}
			if _, err := ex.Execute("work", workSrc); err != nil {
				b.Error(err)
				return
			}
			ex.Close()
		}
	})
}

// 共享同一 Program 的并发解释执行（只读脚本，测解释器线程安全性下的吞吐）
func BenchmarkProgramRunParallel(b *testing.B) {
	p := mustBuild(b, fibSrc)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := p.Run(benchSeqID, "fib", 14); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

// ---------- 原生 Go 基线（对照） ----------

func BenchmarkNativeFib18(b *testing.B) {
	for b.Loop() {
		_ = nativeFib(18)
	}
}

func BenchmarkNativeSumN1000(b *testing.B) {
	for b.Loop() {
		_ = nativeSumN(1000)
	}
}

func BenchmarkNativeCall100(b *testing.B) {
	for b.Loop() {
		_ = nativeBenchCall(100)
	}
}

func nativeFib(n int) int {
	if n < 2 {
		return n
	}
	return nativeFib(n-1) + nativeFib(n-2)
}

func nativeSumN(n int) int {
	sum := 0
	for i := 0; i < n; i++ {
		sum += i*2%97 + 1
	}
	return sum
}

func nativeBenchCall(n int) int {
	s := 0
	for i := 0; i < n; i++ {
		s = s + i
	}
	return s
}
