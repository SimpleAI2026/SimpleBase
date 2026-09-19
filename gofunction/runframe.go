package gofunction

import (
	"fmt"

	"golang.org/x/tools/go/ssa"
)

// debugging 调试开关：设置为 true 时打印每条语句的执行详情
var debugging = false

// step 指令执行后主循环的下一步动作
type step int

const (
	stepNext   step = iota // 继续执行下一条语句
	stepReturn             // 函数返回
	stepJump               // 跳转到另一个block
)

// runFrame 在栈帧上执行程序
func runFrame(fr *frame) {
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

	// stepJump 必须继续外层循环以执行目标 block，而不能 break 出整个 runFrame
BlockLoop:
	for fr.block != nil {
		if !debugging {
			// 进入块前检查取消/超时；空 Instrs 块（如纯跳转目标）也不会漏检导致死循环
			if err := fr.context.Err(); err != nil {
				panic(err)
			}
		}
		for _, instr = range fr.block.Instrs {
			c := visitInstr(fr, instr)
			switch c {
			case stepReturn:
				return
			case stepJump:
				continue BlockLoop
			}
		}
	}
}

// visitInstr 执行一条ssa.Instruction语句，返回值step用于指示下一条语句的位置
func visitInstr(fr *frame, instr ssa.Instruction) step {
	c := stepNext
	// 无法为第三方包中的结构体添加方法，因此通过switch type的方式来找到不同语句的执行方法
	switch instr := instr.(type) {
	case *ssa.DebugRef:
		// no-op
	case *ssa.Alloc:
		c = runAlloc(fr, instr)
	case *ssa.UnOp:
		c = runUnOp(fr, instr)
	case *ssa.BinOp:
		c = runBinOp(fr, instr)
	case *ssa.MakeInterface:
		c = runMakeInterface(fr, instr)
	case *ssa.Return:
		c = runReturn(fr, instr)
	case *ssa.IndexAddr:
		c = runIndexAddr(fr, instr)
	case *ssa.Index:
		c = runIndex(fr, instr)
	case *ssa.Field:
		c = runField(fr, instr)
	case *ssa.FieldAddr:
		c = runFieldAddr(fr, instr)
	case *ssa.Store:
		c = runStore(fr, instr)
	case *ssa.Slice:
		c = runSlice(fr, instr)
	case *ssa.Call:
		c = runCall(fr, instr)
	case *ssa.MakeSlice:
		c = runMakeSlice(fr, instr)
	case *ssa.MakeMap:
		c = runMakeMap(fr, instr)
	case *ssa.MapUpdate:
		c = runMapUpdate(fr, instr)
	case *ssa.Lookup:
		c = runLookup(fr, instr)
	case *ssa.Extract:
		c = runExtract(fr, instr)
	case *ssa.If:
		c = runIf(fr, instr)
	case *ssa.Jump:
		c = runJump(fr, instr)
	case *ssa.Phi:
		c = runPhi(fr, instr)
	case *ssa.Convert:
		c = runConvert(fr, instr)
	case *ssa.Range:
		c = runRange(fr, instr)
	case *ssa.Next:
		c = runNext(fr, instr)
	case *ssa.ChangeType:
		c = runChangeType(fr, instr)
	case *ssa.ChangeInterface:
		c = runChangeInterface(fr, instr)
	case *ssa.MakeClosure:
		c = runMakeClosure(fr, instr)
	case *ssa.Defer:
		c = runDefer(fr, instr)
	case *ssa.RunDefers:
		c = runRunDefers(fr, instr)
	case *ssa.MakeChan:
		c = runMakeChan(fr, instr)
	case *ssa.Send:
		c = runSend(fr, instr)
	case *ssa.TypeAssert:
		c = runTypeAssert(fr, instr)
	case *ssa.Go:
		c = runGo(fr, instr)
	case *ssa.Panic:
		c = runPanic(fr, instr)
	case *ssa.Select:
		c = runSelect(fr, instr)
	default:
		panic(fmt.Sprintf("unexpected instruction: %T", instr))
	}

	if debugging {
		logDebug("run %s: \t%s \t%T", fr.program.mainPkg.Prog.Fset.Position(instr.Pos()), instr.String(), instr)
		if val, ok := instr.(ssa.Value); ok {
			if slot, ok := fr.layout.slotOf(val); ok {
				v := fr.env[slot]
				if v != nil && v.IsValid() {
					logDebug("\t\t\t%#v", v.Interface())
				}
			}
		}
	}

	return c
}
