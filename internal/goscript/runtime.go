package goscript

import (
	"fmt"
	"go/types"
	"reflect"
	"runtime"
	"sync/atomic"
	"unsafe"

	"github.com/linkxzhou/goedge/internal/goscript/ctype"
	"github.com/linkxzhou/goedge/internal/goscript/loader"
	"github.com/petermattis/goid"
	"golang.org/x/tools/go/ssa"
)

var (
	maxMemLen    int
	externValues = make(map[string]reflect.Value)
)

const intSize = 32 << (^uint(0) >> 63)

func init() {
	if intSize == 32 {
		maxMemLen = 1<<31 - 1
	} else {
		v := int64(1) << 59
		maxMemLen = int(v)
	}
}

type plainError string

func (e plainError) Error() string {
	return string(e)
}

type runtimeError string

func (e runtimeError) RuntimeError() {}

func (e runtimeError) Error() string {
	pc, _, _, _ := runtime.Caller(2)
	return "runtime error: " + string(e) + ", FuncForPC: " + runtime.FuncForPC(pc).Name()
}

// If the target program panics, the interpreter panics with this type.
type targetPanic struct {
	v value
}

func (p targetPanic) Error() string {
	return fmt.Sprintf("%v", p.v)
}

type panicking struct {
	value interface{}
}

// If the target program calls exit, the interpreter panics with this type.
type exitPanic int
type goexitPanic int

func registerExternal(key string, i interface{}) {
	externValues[key] = reflect.ValueOf(i)
}

type deferred struct {
	fn      value
	instr   *ssa.Defer
	tail    *deferred
	args    []value
	ssaArgs []ssa.Value
}

type goVm struct {
	caller    *goVm
	pfn       *function
	defers    *deferred
	panicking *panicking
	block     *ssa.BasicBlock
	stack     []value // result args env datas
	pc        int
	pred      int
	deferid   int64
}

func (vm *goVm) setReg(ir register, v value) {
	vm.stack[ir] = v
}

func (vm *goVm) reg(ir register) value {
	return vm.stack[ir]
}

func (vm *goVm) bytes(ir register) []byte {
	return ctype.Bytes(vm.stack[ir])
}

func (vm *goVm) runes(ir register) []rune {
	return ctype.Runes(vm.stack[ir])
}

func (vm *goVm) bool(ir register) bool {
	return ctype.Bool(vm.stack[ir])
}

func (vm *goVm) int(ir register) int {
	return ctype.Int(vm.stack[ir])
}

func (vm *goVm) int8(ir register) int8 {
	return ctype.Int8(vm.stack[ir])
}

func (vm *goVm) int16(ir register) int16 {
	return ctype.Int16(vm.stack[ir])
}

func (vm *goVm) int32(ir register) int32 {
	return ctype.Int32(vm.stack[ir])
}

func (vm *goVm) int64(ir register) int64 {
	return ctype.Int64(vm.stack[ir])
}

func (vm *goVm) uint(ir register) uint {
	return ctype.Uint(vm.stack[ir])
}

func (vm *goVm) uint8(ir register) uint8 {
	return ctype.Uint8(vm.stack[ir])
}

func (vm *goVm) uint16(ir register) uint16 {
	return ctype.Uint16(vm.stack[ir])
}

func (vm *goVm) uint32(ir register) uint32 {
	return ctype.Uint32(vm.stack[ir])
}

func (vm *goVm) uint64(ir register) uint64 {
	return ctype.Uint64(vm.stack[ir])
}

func (vm *goVm) uintptr(ir register) uintptr {
	return ctype.Uintptr(vm.stack[ir])
}

func (vm *goVm) float32(ir register) float32 {
	return ctype.Float32(vm.stack[ir])
}

func (vm *goVm) float64(ir register) float64 {
	return ctype.Float64(vm.stack[ir])
}

func (vm *goVm) complex64(ir register) complex64 {
	return ctype.Complex64(vm.stack[ir])
}

func (vm *goVm) complex128(ir register) complex128 {
	return ctype.Complex128(vm.stack[ir])
}

func (vm *goVm) string(ir register) string {
	return ctype.String(vm.stack[ir])
}

func (vm *goVm) pointer(ir register) unsafe.Pointer {
	return ctype.Pointer(vm.stack[ir])
}

func (vm *goVm) copyReg(dst register, src register) {
	vm.stack[dst] = vm.stack[src]
}

// runDefer deferred call created a new state of panic.
func (vm *goVm) runDefer(d *deferred) {
	var noPanic bool
	defer func() {
		if !noPanic {
			vm.panicking = &panicking{recover()}
		}
	}()
	vm.pfn.Interp.callDiscardsResult(vm, d.fn, d.args, d.ssaArgs)
	noPanic = true
}

// runDefers executes fr's deferred function calls in LIFO order.
//
// On entry, fr.panicking indicates a state of panic; if
// true, fr.panic contains the panic value.
//
// On completion, if a deferred call started a panic, or if no
// deferred call recovered from a previous state of panic, then
// runDefers itself panics after the last deferred call has run.
//
// If there was no initial state of panic, or it was recovered from,
// runDefers returns normally.
func (vm *goVm) runDefers() {
	interp := vm.pfn.Interp
	atomic.AddInt32(&interp.deferCount, 1)
	vm.deferid = goroutineID()
	interp.deferMap.Store(vm.deferid, vm)
	for d := vm.defers; d != nil; d = d.tail {
		vm.runDefer(d)
	}
	interp.deferMap.Delete(vm.deferid)
	atomic.AddInt32(&interp.deferCount, -1)
	vm.deferid = 0
	if vm.panicking != nil {
		panic(vm.panicking.value) // new panic, or still panicking
	}
}

func (vm *goVm) run() {
	if vm.pfn.Recover != nil {
		defer func() {
			if vm.pc == -1 {
				return // normal return
			}
			vm.panicking = &panicking{recover()}
			vm.runDefers()
			for _, fn := range vm.pfn.Recover {
				fn(vm)
			}
		}()
	}

	for vm.pc != -1 && vm.pc < vm.pfn.InstrsLen {
		fn := vm.pfn.Instrs[vm.pc]
		vm.pc++
		fn(vm)
	}
}

// slice returns x[lo:hi:max].  Any of lo, hi and max may be nil.
func slice(vm *goVm, instr *ssa.Slice, makesliceCheck bool, ix, ih, il, im register) reflect.Value {
	x := vm.reg(ix)
	var ilen, icap int
	v := reflect.ValueOf(x)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	kind := v.Kind()
	switch kind {
	case reflect.String:
		ilen = v.Len()
		icap = ilen
	case reflect.Slice, reflect.Array:
		ilen = v.Len()
		icap = v.Cap()
	}
	lo := 0
	hi := ilen
	max := icap
	var isSlice3 bool
	if instr.Low != nil {
		lo = asInt(vm.reg(il))
	}
	if instr.High != nil {
		hi = asInt(vm.reg(ih))
	}
	if instr.Max != nil {
		max = asInt(vm.reg(im))
		isSlice3 = true
	}
	if makesliceCheck {
		if hi < 0 {
			panic(runtimeError("makeslice: len out of range"))
		} else if hi > max {
			panic(runtimeError("makeslice: cap out of range"))
		}
	} else {
		if isSlice3 {
			if max < 0 {
				panic(runtimeError(fmt.Sprintf("slice bounds out of range [::%v]", max)))
			} else if max > icap {
				if kind == reflect.Slice {
					panic(runtimeError(fmt.Sprintf("slice bounds out of range [::%v] with capacity %v", max, icap)))
				} else {
					panic(runtimeError(fmt.Sprintf("slice bounds out of range [::%v] with length %v", max, icap)))
				}
			} else if hi < 0 {
				panic(runtimeError(fmt.Sprintf("slice bounds out of range [:%v:]", hi)))
			} else if hi > max {
				panic(runtimeError(fmt.Sprintf("slice bounds out of range [:%v:%v]", hi, max)))
			} else if lo < 0 {
				panic(runtimeError(fmt.Sprintf("slice bounds out of range [%v::]", lo)))
			} else if lo > hi {
				panic(runtimeError(fmt.Sprintf("slice bounds out of range [%v:%v:]", lo, hi)))
			}
		} else {
			if hi < 0 {
				panic(runtimeError(fmt.Sprintf("slice bounds out of range [:%v]", hi)))
			} else if hi > icap {
				if kind == reflect.Slice {
					panic(runtimeError(fmt.Sprintf("slice bounds out of range [:%v] with capacity %v", hi, icap)))
				} else {
					panic(runtimeError(fmt.Sprintf("slice bounds out of range [:%v] with length %v", hi, icap)))
				}
			} else if lo < 0 {
				panic(runtimeError(fmt.Sprintf("slice bounds out of range [%v:]", lo)))
			} else if lo > hi {
				panic(runtimeError(fmt.Sprintf("slice bounds out of range [%v:%v]", lo, hi)))
			}
		}
	}
	switch kind {
	case reflect.String:
		if lo == hi { // optimization x[len(x):], see $GOROOT/test/slicecap.go
			return v.Slice(0, 0)
		}
		return v.Slice(lo, hi)
	case reflect.Slice, reflect.Array:
		return v.Slice3(lo, hi, max)
	}
	panic(fmt.Sprintf("slice: unexpected X type: %T", x))
}

// typeAssert checks whether dynamic type of itf is instr.AssertedType.
// It returns the extracted value on success, and panics on failure,
// unless instr.CommaOk, in which case it always returns a "value, ok" tuple.
func typeAssert(i *Interp, instr *ssa.TypeAssert, typ reflect.Type, iv interface{}) value {
	var v value
	var err error
	if iv == nil {
		err = plainError(fmt.Sprintf("interface conversion: interface is nil, not %v", typ))
	} else {
		rv := reflect.ValueOf(iv)
		rt := rv.Type()
		if typ == rt {
			v = iv
		} else {
			if !rt.AssignableTo(typ) {
				err = runtimeError(fmt.Sprintf("interface conversion: %v is %v, not %v", instr.X.Type(), rt, typ))
				if itype, ok := instr.AssertedType.Underlying().(*types.Interface); ok {
					if it, ok := i.findType(rt, false); ok {
						if meth, _ := types.MissingMethod(it, itype, true); meth != nil {
							err = runtimeError(fmt.Sprintf("interface conversion: %v is not %v: missing method %s", rt, instr.AssertedType, meth.Name()))
						}
					}
				} else if typ.PkgPath() == rt.PkgPath() && typ.Name() == rt.Name() {
					t1, typOk := i.findType(typ, false)
					t2, typeRt := i.findType(rt, false)
					if typOk && typeRt {
						n1, ok1 := t1.(*types.Named)
						n2, ok2 := t2.(*types.Named)
						if ok1 && ok2 && n1.Obj().Parent() != n2.Obj().Parent() {
							err = runtimeError(fmt.Sprintf("interface conversion: %v is %v, not %v (types from different scopes)", instr.X.Type(), rt, typ))
						}
					}
				}
			} else {
				v = rv.Convert(typ).Interface()
			}
		}
	}
	if err != nil {
		if !instr.CommaOk {
			panic(err)
		}
		return tuple{reflect.New(typ).Elem().Interface(), false}
	}
	if instr.CommaOk {
		return tuple{v, true}
	}
	return v
}

// doRecover implements the recover() built-in.
func doRecover(caller *goVm) value {
	// recover() must be exactly one level beneath the deferred
	// function (two levels beneath the panicking function) to
	// have any effect.  Thus we ignore both "defer recover()" and
	// "defer f() -> g() -> recover()".
	if caller.pfn.Interp.ctx.Mode&loader.DisableRecover == 0 &&
		caller.panicking == nil &&
		caller.caller != nil && caller.caller.panicking != nil {
		p := caller.caller.panicking.value
		caller.caller.panicking = nil
		switch p := p.(type) {
		case targetPanic:
			return p.v
		case runtime.Error, string, plainError, *reflect.ValueError:
			return p
		default:
			panic(fmt.Sprintf("unexpected panic type %T in target call to recover()", p))
		}
	}

	return nil //iface{}
}

func deref(typ types.Type) types.Type {
	if p, ok := typ.Underlying().(*types.Pointer); ok {
		return p.Elem()
	}
	return typ
}

func goroutineID() int64 {
	return goid.Get()
}
