package interp

import (
	"fmt"

	"golang.org/x/tools/go/ssa"
)

// runFrame 在栈帧上执行程序（遗留通用主循环）。
// C3 标注：生产路径唯一入口是 runFrameCompiled（预编译模式，call.go）；
// 本函数仅测试直接引用（runframe_test.go 等构造裸 frame 验证
// panic/恢复/跳转语义），不在生产调用链上，勿在新增代码中使用。
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
