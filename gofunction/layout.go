package gofunction

import (
	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

// funcLayout 函数槽位布局（P3-1）。
//
// 设计要点：
//
//   - 编译期枚举函数内全部 SSA 值（Params/FreeVars/Locals/产值指令），
//     每个值分配一个连续槽位号，运行期 env 为 []value.Value 槽位数组，
//     读写都是一次切片索引，彻底消除 map[ssa.Value]*value.Value 的
//     接口哈希（typehash/efaceeq ~11.7% CPU）与 &val 取址堆分配（~56% alloc）。
//
//   - 指针语义下沉到 reflect 层：Alloc/FreeVar 的槽内存放
//     ptr 类型的 value.Value（reflect.New 的产物），共享与修改通过
//     Elem().Set() 完成 —— 与 Go 原生指针语义完全一致，
//     不再需要 *value.Value 间接层。
//
//   - SSA 保证：被闭包捕获的参数/局部变量会被自动装箱为堆 Alloc，
//     未捕获的参数保持只读 Param —— 因此参数槽位安全地按值存取。
//
//   - 布局不可变：SSA 构建完成后函数结构不再变化，布局随 Program
//     生命周期缓存；并发 Run 只读共享，无锁。
type funcLayout struct {
	slots  map[ssa.Value]int // 全部槽位映射（低频路径 & 编译期使用）
	nSlots int
	// paramSlots/freeVarSlots/localSlots 与 ssa.Function 对应下标一一对应
	paramSlots   []int
	freeVarSlots []int
	localSlots   []int
}

// slotOf 查槽位号；未注册的值返回 ok=false（调用方决定 panic 还是忽略）
func (l *funcLayout) slotOf(v ssa.Value) (int, bool) {
	if l == nil {
		return 0, false
	}
	s, ok := l.slots[v]
	return s, ok
}

// slotOfMust 查槽位号，未注册直接 panic（指令结果理应注册过）
func (l *funcLayout) slotOfMust(v ssa.Value) int {
	if s, ok := l.slotOf(v); ok {
		return s
	}
	panic("slot: no slot for " + v.Name())
}

// buildLayout 为函数构建槽位布局。遍历函数内全部产值指令，
// 按发现顺序分配槽位；Params/FreeVars/Locals 优先注册，
// 保证与 frame.env 初始化顺序解耦。
func buildLayout(fn *ssa.Function) *funcLayout {
	l := &funcLayout{
		slots: make(map[ssa.Value]int, len(fn.Params)+len(fn.Locals)+16),
	}
	alloc := func(v ssa.Value) int {
		s := l.nSlots
		l.nSlots++
		l.slots[v] = s
		return s
	}
	l.paramSlots = make([]int, len(fn.Params))
	for i, p := range fn.Params {
		l.paramSlots[i] = alloc(p)
	}
	l.freeVarSlots = make([]int, len(fn.FreeVars))
	for i, fv := range fn.FreeVars {
		l.freeVarSlots[i] = alloc(fv)
	}
	l.localSlots = make([]int, len(fn.Locals))
	for i, loc := range fn.Locals {
		l.localSlots[i] = alloc(loc)
	}
	for _, b := range fn.Blocks {
		for _, ins := range b.Instrs {
			if v, ok := ins.(ssa.Value); ok {
				if _, exists := l.slots[v]; !exists {
					alloc(v)
				}
			}
		}
	}
	return l
}

// layoutCache Program 级布局缓存（*ssa.Function -> *funcLayout）。
// BuildProgram 后由 compileProgram 预热，运行期只读。
func (p *Program) layoutOf(fn *ssa.Function) *funcLayout {
	if fn == nil {
		return nil
	}
	if l, ok := p.layoutCache.Load(fn); ok {
		return l.(*funcLayout)
	}
	l := buildLayout(fn)
	p.layoutCache.Store(fn, l)
	return l
}

// prepareEnv 把槽位数组扩容到 n 并清零已用部分（池化复用入口）
func (fr *frame) prepareEnv(n int) {
	if cap(fr.env) < n {
		fr.env = make([]value.Value, n)
		return
	}
	fr.env = fr.env[:n]
	clear(fr.env) // 仅清实际长度；接口清零即释放引用
}

// testLayout 为测试手工构造 frame 提供最小布局：注册传入的值到连续槽位。
// 生产路径布局由 Program.layoutOf 编译期构建，此函数仅供测试使用。
func testLayout(vals ...ssa.Value) *funcLayout {
	l := &funcLayout{slots: make(map[ssa.Value]int, len(vals))}
	for _, v := range vals {
		if _, ok := l.slots[v]; !ok {
			l.slots[v] = l.nSlots
			l.nSlots++
		}
	}
	return l
}
