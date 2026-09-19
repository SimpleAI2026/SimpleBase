package gofunction

import (
	"context"
	"fmt"
	"go/types"
	"io"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

const defaultTimeout = 10 * time.Second

// logger 解释器内部日志，可通过 SetLogger 注入
var logger = slog.Default()

// stderrWriter 调试输出目标（SSA 转储等）
var stderrWriter io.Writer = os.Stderr

// logDebug 调试日志输出
func logDebug(format string, args ...interface{}) {
	logger.Debug(fmt.Sprintf(format, args...))
}

// Context 函数执行的上下文环境
type Context struct {
	context.Context
	outBuffer  strings.Builder
	goroutines int32
	cancelFunc context.CancelFunc
}

// Output 返回内置函数print和println打印的内容
func (p *Context) Output() string {
	return p.outBuffer.String()
}

// waitGoroutines 等待脚本内 go 语句启动的协程结束（B8）
func (p *Context) waitGoroutines(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for atomic.LoadInt32(&p.goroutines) > 0 {
		if time.Now().After(deadline) || p.Err() != nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

func newCallContext() *Context {
	ctx, cancelFunc := context.WithTimeout(context.Background(), defaultTimeout)
	return &Context{
		Context:    ctx,
		cancelFunc: cancelFunc,
	}
}

// frame 函数栈帧，每次调用函数都会生成一个新的栈帧，用于存储该函数返回地址、参数、局部变量等信息
type frame struct {
	program          *Program
	caller           *frame
	fn               *ssa.Function
	layout           *funcLayout // 当前函数的槽位布局（P3-1）
	block, prevBlock *ssa.BasicBlock
	env              []value.Value // 槽位数组：参数/自由变量/局部变量/指令结果，按 layout.slotOf 索引
	defers           []*ssa.Defer
	result           value.Value
	panicking        bool
	panic            interface{}
	seqid            string

	context *Context
}

// makeFunc 定义函数或创建闭包，bindings为闭包中关联的外部变量。
// 闭包体内以调用时的 caller frame 身份执行 callSSA；由于 callSSA 现在
// 从池中借用子 frame，此处的 fr 必须是「逃逸出本次执行的稳定 frame」——
// 即创建闭包时的逻辑帧。为避免池化归还后闭包引用悬空，
// makeFunc 为闭包构造一个不参与池化的独立调用帧。
// caller 保留创建时的帧引用：recover builtin 需沿 caller 链向上查找
// panicking 帧（defer/recover 语义）。该帧此刻处于活跃调用中
// （runDefers/runFrame 内），尚未归还池，不存在悬空窗口。
func (fr *frame) makeFunc(f *ssa.Function, bindings []ssa.Value) value.Value {
	layout := fr.program.layoutOf(f)
	// 闭包捕获：绑定值在 SSA 中已装箱为 Alloc/FreeVar（ptr 类型），
	// 槽内存放的就是共享地址值；callSSA 接口保持 *value.Value
	// 以承载地址共享语义（解引用后写入被调帧槽位，Elem 写回共享）
	env := make([]*value.Value, len(bindings))
	for i, binding := range bindings {
		v := fr.get(binding)
		cell := new(value.Value)
		*cell = v
		env[i] = cell
	}
	// 闭包捕获创建时的帧信息（program/context/seqid），不复用池化 frame
	closureFrame := &frame{
		program: fr.program,
		context: fr.context,
		caller:  fr,
		seqid:   fr.seqid,
		layout:  layout,
		env:     make([]value.Value, layout.nSlots),
	}
	sig := f.Signature
	nIn := sig.Params().Len()
	if sig.Recv() != nil {
		nIn++
	}
	in := make([]reflect.Type, 0, nIn)
	if sig.Recv() != nil {
		in = append(in, typeChange(sig.Recv().Type()))
	}
	for i := 0; i < sig.Params().Len(); i++ {
		in = append(in, typeChange(sig.Params().At(i).Type()))
	}
	out := make([]reflect.Type, 0)
	results := sig.Results()
	for i := 0; i < results.Len(); i++ {
		out = append(out, typeChange(results.At(i).Type()))
	}
	funcType := reflect.FuncOf(in, out, sig.Variadic())
	fn := func(in []reflect.Value) (results []reflect.Value) {
		// 无泄漏切片：callSSA 按值拷入槽位后不再引用，
		// 逃逸分析可栈分配（P3-3）
		args := make([]value.Value, len(in))
		for i, arg := range in {
			args[i] = value.RValue{Value: arg}
		}
		ret := callSSA(closureFrame, f, args, env)
		if ret != nil {
			return value.Unpackage(ret)
		}
		return nil
	}
	return value.RValue{Value: reflect.MakeFunc(funcType, fn)}
}

// get 通用取值入口（走槽位映射，一次切片索引）。
// 预编译闭包内已把热路径槽位号直接编译进指令闭包，
// 此方法仅供 runframe 兜底、callOp 动态调用目标等低频路径使用。
func (fr *frame) get(key ssa.Value) value.Value {
	switch key := key.(type) {
	case nil:
		return nil
	case *ssa.Const:
		return fr.constValueCached(key)
	case *ssa.Global:
		if r, ok := fr.program.globals[key]; ok && r != nil {
			return *r
		}
		// 外部包全局变量走注册表
		if key.Pkg != nil && key.Pkg != fr.program.mainPkg {
			return fr.program.externalValue(key)
		}
	case *ssa.Function:
		if v, ok := fr.program.funcCache.Load(key); ok {
			return v.(value.Value)
		}
		v := fr.makeFunc(key, nil)
		fr.program.funcCache.Store(key, v)
		return v
	}
	if slot, ok := fr.layout.slotOf(key); ok {
		return fr.env[slot]
	}
	panic(fmt.Sprintf("get: no Value for %T: %v", key, key.Name()))
}

// constValueCached 常量求值缓存：SSA 中同一 *ssa.Const 单例且值恒定，
// 基础类型缓存复用；复合零值（结构体/数组）每次新建以防共享可变内存。
func (fr *frame) constValueCached(c *ssa.Const) value.Value {
	if v, ok := fr.program.constCache.Load(c); ok {
		return v.(value.Value)
	}
	v := constValue(c)
	if isBasicType(c.Type()) {
		fr.program.constCache.Store(c, v)
	}
	return v
}

// isBasicType 判断 types.Type 底层是否为基础类型
func isBasicType(t types.Type) bool {
	_, ok := t.Underlying().(*types.Basic)
	return ok
}

func (fr *frame) set(instr ssa.Value, val value.Value) {
	fr.env[fr.layout.slotOfMust(instr)] = val
}

func (fr *frame) runDefers() {
	for i := len(fr.defers) - 1; i >= 0; i-- {
		fr.runDefer(fr.defers[i])
	}
	fr.defers = nil
	if fr.panicking {
		panic(fr.panic)
	}
}

func (fr *frame) runDefer(d *ssa.Defer) {
	var ok bool
	defer func() {
		if !ok {
			fr.panicking = true
			fr.panic = recover()
		}
	}()
	callOp(fr, d.Common())
	ok = true
}
