package gointerp

import (
	"fmt"
	"go/constant"
	"go/types"
	"reflect"
	"runtime"
	"sync/atomic"
	"unsafe"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)

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

// If the target program calls exit, the interpreter panics with this type.
type exitPanic int
type goexitPanic int

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
	return xtype.Bytes(vm.stack[ir])
}

func (vm *goVm) runes(ir register) []rune {
	return xtype.Runes(vm.stack[ir])
}

func (vm *goVm) bool(ir register) bool {
	return xtype.Bool(vm.stack[ir])
}

func (vm *goVm) int(ir register) int {
	return xtype.Int(vm.stack[ir])
}

func (vm *goVm) int8(ir register) int8 {
	return xtype.Int8(vm.stack[ir])
}

func (vm *goVm) int16(ir register) int16 {
	return xtype.Int16(vm.stack[ir])
}

func (vm *goVm) int32(ir register) int32 {
	return xtype.Int32(vm.stack[ir])
}

func (vm *goVm) int64(ir register) int64 {
	return xtype.Int64(vm.stack[ir])
}

func (vm *goVm) uint(ir register) uint {
	return xtype.Uint(vm.stack[ir])
}

func (vm *goVm) uint8(ir register) uint8 {
	return xtype.Uint8(vm.stack[ir])
}

func (vm *goVm) uint16(ir register) uint16 {
	return xtype.Uint16(vm.stack[ir])
}

func (vm *goVm) uint32(ir register) uint32 {
	return xtype.Uint32(vm.stack[ir])
}

func (vm *goVm) uint64(ir register) uint64 {
	return xtype.Uint64(vm.stack[ir])
}

func (vm *goVm) uintptr(ir register) uintptr {
	return xtype.Uintptr(vm.stack[ir])
}

func (vm *goVm) float32(ir register) float32 {
	return xtype.Float32(vm.stack[ir])
}

func (vm *goVm) float64(ir register) float64 {
	return xtype.Float64(vm.stack[ir])
}

func (vm *goVm) complex64(ir register) complex64 {
	return xtype.Complex64(vm.stack[ir])
}

func (vm *goVm) complex128(ir register) complex128 {
	return xtype.Complex128(vm.stack[ir])
}

func (vm *goVm) string(ir register) string {
	return xtype.String(vm.stack[ir])
}

func (vm *goVm) pointer(ir register) unsafe.Pointer {
	return xtype.Pointer(vm.stack[ir])
}

func (vm *goVm) copyReg(dst register, src register) {
	vm.stack[dst] = vm.stack[src]
}

type panicking struct {
	value interface{}
}

// runDefer runs a deferred call d.
// It always returns normally, but may set or clear fr.panic.
func (vm *goVm) runDefer(d *deferred) {
	var ok bool
	defer func() {
		if !ok {
			vm.panicking = &panicking{recover()} // Deferred call created a new state of panic.
		}
	}()
	vm.pfn.Interp.callDiscardsResult(vm, d.fn, d.args, d.ssaArgs)
	ok = true
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

func xtypeValue(c *ssa.Const, kind types.BasicKind) value {
	switch kind {
	case types.Bool, types.UntypedBool:
		return constant.BoolVal(c.Value)
	case types.Int, types.UntypedInt:
		// Assume sizeof(int) is same on host and target.
		return int(c.Int64())
	case types.Int8:
		return int8(c.Int64())
	case types.Int16:
		return int16(c.Int64())
	case types.Int32, types.UntypedRune:
		return int32(c.Int64())
	case types.Int64:
		return c.Int64()
	case types.Uint:
		// Assume sizeof(uint) is same on host and target.
		return uint(c.Uint64())
	case types.Uint8:
		return uint8(c.Uint64())
	case types.Uint16:
		return uint16(c.Uint64())
	case types.Uint32:
		return uint32(c.Uint64())
	case types.Uint64:
		return c.Uint64()
	case types.Uintptr:
		// Assume sizeof(uintptr) is same on host and target.
		return uintptr(c.Uint64())
	case types.Float32:
		return float32(c.Float64())
	case types.Float64, types.UntypedFloat:
		return c.Float64()
	case types.Complex64:
		return complex64(c.Complex128())
	case types.Complex128, types.UntypedComplex:
		return c.Complex128()
	case types.String, types.UntypedString:
		if c.Value.Kind() == constant.String {
			return constant.StringVal(c.Value)
		}
		return string(rune(c.Int64()))
	case types.UnsafePointer:
		return unsafe.Pointer(uintptr(c.Uint64()))
	}
	panic("unreachable")
}

// constValue returns the value of the constant with the
// dynamic type tag appropriate for c.Type().
func constToValue(i *Interp, c *ssa.Const) value {
	typ := c.Type()
	if c.IsNil() {
		if xtype, ok := typ.(*types.Basic); ok && xtype.Kind() == types.UntypedNil {
			return nil
		}
		return reflect.Zero(i.preToType(typ)).Interface()
	}
	if xtype, ok := typ.(*types.Basic); ok {
		return xtypeValue(c, xtype.Kind())
	} else if xtype, ok := typ.Underlying().(*types.Basic); ok {
		v := xtypeValue(c, xtype.Kind())
		nv := reflect.New(i.preToType(typ)).Elem()
		SetValue(nv, reflect.ValueOf(v))
		return nv.Interface()
	}
	panic(fmt.Sprintf("unparser constValue: %s", c))
}

func globalToValue(i *Interp, key *ssa.Global) (interface{}, bool) {
	if key.Pkg != nil {
		// TODO: fix by linkxzhou
		// pkgpath := key.Pkg.Pkg.Path()
		// if pkg, ok := i.Installed(pkgpath); ok {
		// 	if ext, ok := pkg.Vars[key.Name()]; ok {
		// 		return ext.Interface(), true
		// 	}
		// }
	}
	if v, ok := i.globals[key]; ok {
		return v, true
	}
	return nil, false
}

// asInt converts x, which must be an integer, to an int suitable for
// use as a slice or array index or operand to make().
func asInt(x value) int {
	switch x := x.(type) {
	case int:
		return x
	case int8:
		return int(x)
	case int16:
		return int(x)
	case int32:
		return int(x)
	case int64:
		return int(x)
	case uint:
		return int(x)
	case uint8:
		return int(x)
	case uint16:
		return int(x)
	case uint32:
		return int(x)
	case uint64:
		return int(x)
	case uintptr:
		return int(x)
	default:
		v := reflect.ValueOf(x)
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int(v.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return int(v.Uint())
		}
	}
	panic(fmt.Sprintf("cannot convert %T to int", x))
}

// slice returns x[lo:hi:max].  Any of lo, hi and max may be nil.
func slice(vm *goVm, instr *ssa.Slice, makesliceCheck bool, ix, ih, il, im register) reflect.Value {
	x := vm.reg(ix)
	var Len, Cap int
	v := reflect.ValueOf(x)
	// *array
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	kind := v.Kind()
	switch kind {
	case reflect.String:
		Len = v.Len()
		Cap = Len
	case reflect.Slice, reflect.Array:
		Len = v.Len()
		Cap = v.Cap()
	}

	lo := 0
	hi := Len
	max := Cap
	var slice3 bool
	if instr.Low != nil {
		lo = asInt(vm.reg(il))
	}

	if instr.High != nil {
		hi = asInt(vm.reg(ih))
	}

	if instr.Max != nil {
		max = asInt(vm.reg(im))
		slice3 = true
	}

	if makesliceCheck {
		if hi < 0 {
			panic(runtimeError("makeslice: len out of range"))
		} else if hi > max {
			panic(runtimeError("makeslice: cap out of range"))
		}
	} else {
		if slice3 {
			if max < 0 {
				panic(runtimeError(fmt.Sprintf("slice bounds out of range [::%v]", max)))
			} else if max > Cap {
				if kind == reflect.Slice {
					panic(runtimeError(fmt.Sprintf("slice bounds out of range [::%v] with capacity %v", max, Cap)))
				} else {
					panic(runtimeError(fmt.Sprintf("slice bounds out of range [::%v] with length %v", max, Cap)))
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
			} else if hi > Cap {
				if kind == reflect.Slice {
					panic(runtimeError(fmt.Sprintf("slice bounds out of range [:%v] with capacity %v", hi, Cap)))
				} else {
					panic(runtimeError(fmt.Sprintf("slice bounds out of range [:%v] with length %v", hi, Cap)))
				}
			} else if lo < 0 {
				panic(runtimeError(fmt.Sprintf("slice bounds out of range [%v:]", lo)))
			} else if lo > hi {
				panic(runtimeError(fmt.Sprintf("slice bounds out of range [%v:%v]", lo, hi)))
			} else {
				//
			}
		}
	}
	switch kind {
	case reflect.String:
		// optimization x[len(x):], see $GOROOT/test/slicecap.go
		if lo == hi {
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
// unless instr.CommaOk, in which case it always returns a "value,ok" tuple.
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
							err = runtimeError(fmt.Sprintf("interface conversion: %v is not %v: missing method %s",
								rt, instr.AssertedType, meth.Name()))
						}
					}
				} else if typ.PkgPath() == rt.PkgPath() && typ.Name() == rt.Name() {
					t1, ok1 := i.findType(typ, false)
					t2, ok2 := i.findType(rt, false)
					if ok1 && ok2 {
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

// callBuiltin interprets a call to builtin fn with arguments args,
// returning its result.
func (inter *Interp) callBuiltin(caller *goVm, fn *ssa.Builtin, args []value, ssaArgs []ssa.Value) value {
	switch fnName := fn.Name(); fnName {
	case "append":
		if len(args) == 1 {
			return args[0]
		}
		v0 := reflect.ValueOf(args[0])
		v1 := reflect.ValueOf(args[1])
		if v1.Kind() == reflect.String {
			v1 = reflect.ValueOf([]byte(v1.String()))
		}
		i0 := v0.Len()
		i1 := v1.Len()
		if i0+i1 < i0 {
			panic(runtimeError("growslice: cap out of range"))
		}
		return reflect.AppendSlice(v0, v1).Interface()

	case "copy": // copy([]T, []T) int or copy([]byte, string) int
		return reflect.Copy(reflect.ValueOf(args[0]), reflect.ValueOf(args[1]))

	case "close": // close(chan T)
		reflect.ValueOf(args[0]).Close()
		return nil

	case "delete": // delete(map[K]value, K)
		reflect.ValueOf(args[0]).SetMapIndex(reflect.ValueOf(args[1]), reflect.Value{})
		return nil

	case "len":
		return reflect.ValueOf(args[0]).Len()

	case "cap":
		return reflect.ValueOf(args[0]).Cap()

	case "panic":
		// ssa.Panic handles most cases; this is only for "go
		// panic" or "defer panic".
		panic(targetPanic{args[0]})

	case "recover":
		return doRecover(caller)

	default:
		panic("unknown built-in: " + fnName)
	}
}

// callBuiltinDiscardsResult interprets a call to builtin fn with arguments args,
// discards its result.
func (inter *Interp) callBuiltinDiscardsResult(caller *goVm, fn *ssa.Builtin, args []value, ssaArgs []ssa.Value) {
	switch fnName := fn.Name(); fnName {
	case "append":
		panic("discards result of " + fnName)

	case "copy": // copy([]T, []T) int or copy([]byte, string) int
		reflect.Copy(reflect.ValueOf(args[0]), reflect.ValueOf(args[1]))

	case "close": // close(chan T)
		reflect.ValueOf(args[0]).Close()

	case "delete": // delete(map[K]value, K)
		reflect.ValueOf(args[0]).SetMapIndex(reflect.ValueOf(args[1]), reflect.Value{})

	case "len":
		panic("discards result of " + fnName)

	case "cap":
		panic("discards result of " + fnName)

	case "panic":
		// ssa.Panic handles most cases; this is only for "go
		// panic" or "defer panic".
		panic(targetPanic{args[0]})

	case "recover":
		doRecover(caller)

	default:
		panic("unknown built-in: " + fnName)
	}
}

// callBuiltin interprets a call to builtin fn with arguments args,
// returning its result.
func (inter *Interp) callBuiltinByStack(caller *goVm, fn string, ssaArgs []ssa.Value, ir register, ia []register) {
	switch fn {
	case "append":
		if len(ia) == 1 {
			caller.copyReg(ir, ia[0])
			return
		}
		arg0 := caller.reg(ia[0])
		arg1 := caller.reg(ia[1])
		v0 := reflect.ValueOf(arg0)
		v1 := reflect.ValueOf(arg1)
		// append([]byte, ...string) []byte
		if v1.Kind() == reflect.String {
			v1 = reflect.ValueOf([]byte(v1.String()))
		}
		i0 := v0.Len()
		i1 := v1.Len()
		if i0+i1 < i0 {
			panic(runtimeError("growslice: cap out of range"))
		}
		caller.setReg(ir, reflect.AppendSlice(v0, v1).Interface())

	case "copy": // copy([]T, []T) int or copy([]byte, string) int
		arg0 := caller.reg(ia[0])
		arg1 := caller.reg(ia[1])
		caller.setReg(ir, reflect.Copy(reflect.ValueOf(arg0), reflect.ValueOf(arg1)))

	case "close": // close(chan T)
		arg0 := caller.reg(ia[0])
		reflect.ValueOf(arg0).Close()

	case "delete": // delete(map[K]value, K)
		arg0 := caller.reg(ia[0])
		arg1 := caller.reg(ia[1])
		reflect.ValueOf(arg0).SetMapIndex(reflect.ValueOf(arg1), reflect.Value{})

	case "len":
		arg0 := caller.reg(ia[0])
		caller.setReg(ir, reflect.ValueOf(arg0).Len())

	case "cap":
		arg0 := caller.reg(ia[0])
		caller.setReg(ir, reflect.ValueOf(arg0).Cap())

	case "panic":
		// ssa.Panic handles most cases; this is only for "go
		// panic" or "defer panic".
		arg0 := caller.reg(ia[0])
		panic(targetPanic{arg0})

	case "recover":
		caller.setReg(ir, doRecover(caller))

	default:
		panic("unknown built-in: " + fn)
	}
}

//go:nocheckptr
func toUnsafePointer(v uintptr) unsafe.Pointer {
	return unsafe.Pointer(v)
}
