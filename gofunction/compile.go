package gofunction

import (
	"fmt"

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
}

// compileInstr 把单条 SSA 指令编译为绑定具体类型的执行闭包。
// 未知指令类型回落到 visitInstr（由其 panic 报错，保持原有语义）。
func compileInstr(instr ssa.Instruction) instrStep {
	switch instr := instr.(type) {
	case *ssa.DebugRef:
		return func(fr *frame) step { return stepNext }
	case *ssa.Alloc:
		return func(fr *frame) step { return runAlloc(fr, instr) }
	case *ssa.UnOp:
		return func(fr *frame) step { return runUnOp(fr, instr) }
	case *ssa.BinOp:
		return func(fr *frame) step { return runBinOp(fr, instr) }
	case *ssa.MakeInterface:
		return func(fr *frame) step { return runMakeInterface(fr, instr) }
	case *ssa.Return:
		return func(fr *frame) step { return runReturn(fr, instr) }
	case *ssa.IndexAddr:
		return func(fr *frame) step { return runIndexAddr(fr, instr) }
	case *ssa.Index:
		return func(fr *frame) step { return runIndex(fr, instr) }
	case *ssa.Field:
		return func(fr *frame) step { return runField(fr, instr) }
	case *ssa.FieldAddr:
		return func(fr *frame) step { return runFieldAddr(fr, instr) }
	case *ssa.Store:
		return func(fr *frame) step { return runStore(fr, instr) }
	case *ssa.Slice:
		return func(fr *frame) step { return runSlice(fr, instr) }
	case *ssa.Call:
		return func(fr *frame) step { return runCall(fr, instr) }
	case *ssa.MakeSlice:
		return func(fr *frame) step { return runMakeSlice(fr, instr) }
	case *ssa.MakeMap:
		return func(fr *frame) step { return runMakeMap(fr, instr) }
	case *ssa.MapUpdate:
		return func(fr *frame) step { return runMapUpdate(fr, instr) }
	case *ssa.Lookup:
		return func(fr *frame) step { return runLookup(fr, instr) }
	case *ssa.Extract:
		return func(fr *frame) step { return runExtract(fr, instr) }
	case *ssa.Phi:
		return compilePhi(instr)
	case *ssa.Convert:
		return func(fr *frame) step { return runConvert(fr, instr) }
	case *ssa.Range:
		return func(fr *frame) step { return runRange(fr, instr) }
	case *ssa.Next:
		return func(fr *frame) step { return runNext(fr, instr) }
	case *ssa.ChangeType:
		return func(fr *frame) step { return runChangeType(fr, instr) }
	case *ssa.ChangeInterface:
		return func(fr *frame) step { return runChangeInterface(fr, instr) }
	case *ssa.MakeClosure:
		return func(fr *frame) step { return runMakeClosure(fr, instr) }
	case *ssa.Defer:
		return func(fr *frame) step { return runDefer(fr, instr) }
	case *ssa.RunDefers:
		return func(fr *frame) step { return runRunDefers(fr, instr) }
	case *ssa.MakeChan:
		return func(fr *frame) step { return runMakeChan(fr, instr) }
	case *ssa.Send:
		return func(fr *frame) step { return runSend(fr, instr) }
	case *ssa.TypeAssert:
		return func(fr *frame) step { return runTypeAssert(fr, instr) }
	case *ssa.Go:
		return func(fr *frame) step { return runGo(fr, instr) }
	case *ssa.Panic:
		return func(fr *frame) step { return runPanic(fr, instr) }
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
