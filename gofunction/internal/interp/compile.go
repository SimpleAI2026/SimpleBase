// 文件说明：compile.go — gofunction/internal/interp 的内部实现（instrStep、compiledBlock 等）
//
// 用法
//
//	type instrStep（包内使用）
//	type compiledBlock（包内使用）
//	同包调用：compileProgram() → layout.go, program.go；runFrameCompiled() → call.go；compiledBlock → program.go
//
// 调用架构
//
//	[上游] gofunction
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/internal/interp/compile.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] call.go, eval.go, executor.go, frame.go, gofunction.go 等 13 个
//	[关键引用] compileProgram() ← layout.go, program.go
//
// 主要符号
//
//	type instrStep func(fr *frame) step
//	type compiledBlock struct {…}
//	compileBlock(b *ssa.BasicBlock) *compiledBlock
//	compileInstr(instr ssa.Instruction) instrStep
//	compilePhi(instr *ssa.Phi) instrStep
//	compileProgram(p *Program)
//	... 另有 2 个顶层符号

package interp

import (
	"fmt"
	"sync"

	"golang.org/x/tools/go/ssa"
)

// instrStep 预编译指令执行器：构建期把 ssa.Instruction 绑定为闭包，
// 运行期直接调用，消除 visitInstr 类型 switch 的线性分发开销（P2-1）。
type instrStep func(fr *frame) step

// compiledBlock 预编译基本块：指令闭包序列 + 终结指令直通（P2-2）。
// SSA 保证每个非空块以终结指令（If/Jump/Return/Panic…）结尾；
// If/Jump 只做块切换，编译期单独识别，执行期直通处理，
// 不再经过指令分发。
type compiledBlock struct {
	steps     []instrStep       // 非终结指令的预编译闭包（含终结型 Return/Panic 等）
	instrs    []ssa.Instruction // 与 steps 并行的原指令（panic 时报告源码位置）
	hasIf     bool              // 末指令为 *ssa.If
	ifInstr   *ssa.If
	hasJump   bool // 末指令为 *ssa.Jump
	jumpInstr *ssa.Jump
}

// compileBlock 预编译一个基本块（纯函数，结果由调用方缓存）
func compileBlock(b *ssa.BasicBlock) *compiledBlock {
	cb := &compiledBlock{instrs: b.Instrs}
	n := len(b.Instrs)
	if n == 0 {
		return cb
	}
	body := b.Instrs
	switch term := b.Instrs[n-1].(type) {
	case *ssa.If:
		cb.hasIf = true
		cb.ifInstr = term
		body = body[:n-1]
	case *ssa.Jump:
		cb.hasJump = true
		cb.jumpInstr = term
		body = body[:n-1]
	}
	cb.steps = make([]instrStep, len(body))
	for i, ins := range body {
		cb.steps[i] = compileInstr(ins)
	}
	return cb
}// compileInstr 把单条 SSA 指令编译为绑定具体类型的执行闭包。
// 未知指令类型回落到 visitInstr（由其 panic 报错，保持原有语义）。
//
// A1（槽位固化）：两三操作数指令在编译期解析槽位号并捕获进闭包，
// 运行期直接 env 切片索引，消除 fr.get/fr.set 的 map[ssa.Value]int
// 哈希查找（此前每次执行 2–3 次）。操作数不是本函数 SSA 值
// （Const/Global/Function/外部 FreeVar）时编译期回落 fr.get 慢路径，
// 不在运行期做类型判断。
func compileInstr(instr ssa.Instruction) instrStep {
	switch instr := instr.(type) {
	case *ssa.DebugRef:
		return func(fr *frame) step { return stepNext }
	case *ssa.Alloc:
		return compileAlloc(instr)
	case *ssa.UnOp:
		return compileUnOp(instr)
	case *ssa.BinOp:
		return compileBinOp(instr)
	case *ssa.MakeInterface:
		return compileMakeInterface(instr)
	case *ssa.Return:
		return compileReturn(instr)
	case *ssa.IndexAddr:
		return compileIndexAddr(instr)
	case *ssa.Index:
		return compileIndex(instr)
	case *ssa.Field:
		return compileField(instr)
	case *ssa.FieldAddr:
		return compileFieldAddr(instr)
	case *ssa.Store:
		return compileStore(instr)
	case *ssa.Slice:
		return func(fr *frame) step { return runSlice(fr, instr) }
	case *ssa.Call:
		return func(fr *frame) step { return runCall(fr, instr) }
	case *ssa.MakeSlice:
		return compileMakeSlice(instr)
	case *ssa.MakeMap:
		return compileMakeMap(instr)
	case *ssa.MapUpdate:
		return compileMapUpdate(instr)
	case *ssa.Lookup:
		return func(fr *frame) step { return runLookup(fr, instr) }
	case *ssa.Extract:
		return compileExtract(instr)
	case *ssa.Phi:
		return compilePhi(instr)
	case *ssa.Convert:
		return compileConvert(instr)
	case *ssa.Range:
		return func(fr *frame) step { return runRange(fr, instr) }
	case *ssa.Next:
		return compileNext(instr)
	case *ssa.ChangeType:
		return compileChangeType(instr)
	case *ssa.ChangeInterface:
		return compileChangeInterface(instr)
	case *ssa.MakeClosure:
		return func(fr *frame) step { return runMakeClosure(fr, instr) }
	case *ssa.Defer:
		return func(fr *frame) step { return runDefer(fr, instr) }
	case *ssa.RunDefers:
		return func(fr *frame) step { return runRunDefers(fr, instr) }
	case *ssa.MakeChan:
		return compileMakeChan(instr)
	case *ssa.Send:
		return compileSend(instr)
	case *ssa.TypeAssert:
		return func(fr *frame) step { return runTypeAssert(fr, instr) }
	case *ssa.Go:
		return func(fr *frame) step { return runGo(fr, instr) }
	case *ssa.Panic:
		return compilePanic(instr)
	case *ssa.Select:
		return func(fr *frame) step { return runSelect(fr, instr) }
	default:
		in := instr
		return func(fr *frame) step { return visitInstr(fr, in) }
	}
}

// compilePhi 预解析 Phi 节点（P3-5）。
// runPhi 原实现每次执行都线性遍历 instr.Block().Preds 查找 prevBlock，
// 这里在编译期把 (前驱块 -> 边值) 固化进闭包：
//   - 单前驱：退化为直接取值，无分支
//   - 双前驱（if/for 最常见）：两次指针比较
//   - 多前驱：线性比较预抽取的 preds 切片（比原来少一层 Block() 调用与边界判断）
//
// 语义与 runPhi 一致：prevBlock 不匹配任何前驱时不写值（保持原行为）。
func compilePhi(instr *ssa.Phi) instrStep {
	preds := instr.Block().Preds
	edges := instr.Edges
	// SSA 保证 len(Edges) == len(Preds)；防御性截断避免越界
	n := len(preds)
	if len(edges) < n {
		n = len(edges)
	}
	switch n {
	case 0:
		return func(fr *frame) step { return stepNext }
	case 1:
		p0, e0 := preds[0], edges[0]
		return func(fr *frame) step {
			if fr.prevBlock == p0 {
				fr.set(instr, fr.get(e0))
			}
			return stepNext
		}
	case 2:
		p0, e0 := preds[0], edges[0]
		p1, e1 := preds[1], edges[1]
		return func(fr *frame) step {
			switch fr.prevBlock {
			case p0:
				fr.set(instr, fr.get(e0))
			case p1:
				fr.set(instr, fr.get(e1))
			}
			return stepNext
		}
	default:
		preds, edges := preds[:n], edges[:n]
		return func(fr *frame) step {
			for i, pred := range preds {
				if fr.prevBlock == pred {
					fr.set(instr, fr.get(edges[i]))
					break
				}
			}
			return stepNext
		}
	}
}

// compileProgram 预编译 Program 内全部函数的基本块（BuildProgram 后调用，
// 并发 Run 时缓存已预热，运行期只读）。漏网的函数（如外部导入包的 SSA）
// 由 getCompiledBlock 惰性编译兜底。
func compileProgram(p *Program) {
	if p == nil || p.mainPkg == nil {
		return
	}
	registerProgram(p)
	var walk func(fn *ssa.Function)
	walk = func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		// 槽位布局（P3-1）：先建布局，指令闭包编译时槽位号直接绑定
		p.layoutOf(fn)
		for _, b := range fn.Blocks {
			getCompiledBlock(p, b)
		}
		for _, anon := range fn.AnonFuncs {
			walk(anon)
		}
	}
	for _, member := range p.mainPkg.Members {
		if fn, ok := member.(*ssa.Function); ok {
			walk(fn)
		}
	}
}

// programRegistry Program ↔ ssa.Package 的进程级注册表。
// A1 槽位固化需要在编译期从 ssa.Function 反查所属 Program 的布局；
// mainPkg 之外的导入包函数由 Importer 构建，同样注册（BuildProgram 时）。
// map 只增不删（Program 由调用方持有引用期间有效；SSA 对象生命周期
// 与 Program 一致，进程内常驻的脚本 Program 数量有限）。
var programRegistry = map[*ssa.Package]*Program{}

var programRegistryMu sync.Mutex

func registerProgram(p *Program) {
	programRegistryMu.Lock()
	defer programRegistryMu.Unlock()
	if p.mainPkg != nil {
		programRegistry[p.mainPkg] = p
	}
	if p.importer != nil {
		for _, pkg := range p.importer.LoadedSSAPackages() {
			programRegistry[pkg] = p
		}
	}
}

func lookupProgram(fn *ssa.Function) *Program {
	if fn == nil || fn.Pkg == nil {
		return nil
	}
	programRegistryMu.Lock()
	p := programRegistry[fn.Pkg]
	programRegistryMu.Unlock()
	return p
}

// getCompiledBlock 取预编译块；miss 时惰性编译并写回 Program 缓存。
// Program 缓存随 Program 生命周期回收，避免包级 map 无限增长。
// frame 无 program（测试手工构造）时不缓存，直接现编译。
func getCompiledBlock(p *Program, b *ssa.BasicBlock) *compiledBlock {
	if p == nil {
		return compileBlock(b)
	}
	if cb, ok := p.blockCache.Load(b); ok {
		return cb.(*compiledBlock)
	}
	cb := compileBlock(b)
	p.blockCache.Store(b, cb)
	return cb
}

// runFrameCompiled 预编译模式主循环（P2-1 + P2-2）。
// 与解释模式语义一致：块边界检查取消、If/Jump 直通切换块、
// panic 恢复进入 Recover 块；panic 位置仍取自当前指令。
func runFrameCompiled(fr *frame) {
	var instr ssa.Instruction
	defer func() {
		if fr.block == nil {
			return // 正常返回
		}
		fr.panicking = true
		re := recover()
		pos := "-"
		if instr != nil && fr.program != nil && fr.program.mainPkg != nil {
			pos = fr.program.mainPkg.Prog.Fset.Position(instr.Pos()).String()
		}
		fr.panic = fmt.Errorf("panic: %s: %v", pos, re)
		fr.runDefers()
		if fr.fn != nil {
			fr.block = fr.fn.Recover
		} else {
			fr.block = nil
		}
	}()

BlockLoop:
	for fr.block != nil {
		if err := fr.context.Err(); err != nil {
			panic(err)
		}
		cb := getCompiledBlock(fr.program, fr.block)
		for i, st := range cb.steps {
			instr = cb.instrs[i]
			switch st(fr) {
			case stepReturn:
				return
			case stepJump:
				continue BlockLoop
			}
		}
		// 终结指令直通（P2-2）：If/Jump 已知目标块，直接切换，
		// 不再经指令分发
		switch {
		case cb.hasIf:
			if fr.get(cb.ifInstr.Cond).Bool() {
				fr.prevBlock, fr.block = fr.block, fr.block.Succs[0]
			} else {
				fr.prevBlock, fr.block = fr.block, fr.block.Succs[1]
			}
		case cb.hasJump:
			fr.prevBlock, fr.block = fr.block, fr.block.Succs[0]
		default:
			// 空块或终结型指令已在 steps 内处理（Return/Panic）：
			// 空块保持原语义（回到外层重检 context，避免死循环）
		}
	}
}
