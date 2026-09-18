package gofunction

import (
	"context"
	"fmt"
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

// SetLogger 注入解释器内部使用的日志器（默认 slog.Default()）
func SetLogger(l *slog.Logger) {
	if l != nil {
		logger = l
	}
}

// SetDebugOutput 设置调试输出目标
func SetDebugOutput(w io.Writer) {
	if w != nil {
		stderrWriter = w
	}
}

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
	block, prevBlock *ssa.BasicBlock
	env              map[ssa.Value]*value.Value
	locals           []value.Value
	defers           []*ssa.Defer
	result           value.Value
	panicking        bool
	panic            interface{}
	seqid            string

	context *Context
}

// makeFunc 定义函数或创建闭包，bindings为闭包中关联的外部变量
func (fr *frame) makeFunc(f *ssa.Function, bindings []ssa.Value) value.Value {
	env := make([]*value.Value, len(bindings))
	for i, binding := range bindings {
		env[i] = fr.env[binding]
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
		args := make([]value.Value, len(in))
		for i, arg := range in {
			args[i] = value.RValue{Value: arg}
		}
		ret := callSSA(fr, f, args, env)
		if ret != nil {
			return value.Unpackage(ret)
		}
		return nil
	}
	return value.RValue{Value: reflect.MakeFunc(funcType, fn)}
}

func (fr *frame) get(key ssa.Value) value.Value {
	switch key := key.(type) {
	case nil:
		return nil
	case *ssa.Const:
		return constValue(key)
	case *ssa.Global:
		if r, ok := fr.program.globals[key]; ok && r != nil {
			return *r
		}
		// 外部包全局变量走注册表
		if key.Pkg != nil && key.Pkg != fr.program.mainPkg {
			return fr.program.externalValue(key)
		}
	case *ssa.Function:
		return fr.makeFunc(key, nil)
	}
	if r, ok := fr.env[key]; ok && r != nil {
		return *r
	}
	panic(fmt.Sprintf("get: no Value for %T: %v", key, key.Name()))
}

func (fr *frame) set(instr ssa.Value, val value.Value) {
	fr.env[instr] = &val
}

func (fr *frame) newChild(fn *ssa.Function) *frame {
	return &frame{
		program: fr.program,
		context: fr.context,
		caller:  fr, // for panic/recover
		fn:      fn,
		seqid:   fr.seqid,
	}
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
