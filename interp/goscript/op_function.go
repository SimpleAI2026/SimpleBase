package goscript

import (
	"fmt"
	"go/token"
	"go/types"
	"io"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/linkxzhou/gongx/interp/goscript/loader"
	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)

/*
type Op int

const (
	OpInvalid Op = iota
	// Value-defining instructions
	OpAlloc
	OpPhi
	OpCall
	OpBinOp
	OpUnOp
	OpChangeType
	OpConvert
	OpChangeInterface
	OpSliceToArrayPointer
	OpMakeInterface
	OpMakeClosure
	OpMakeMap
	OpMakeChan
	OpMakeSlice
	OpSlice
	OpFieldAddr
	OpField
	OpIndexAddr
	OpIndex
	OpLookup
	OpSelect
	OpRange
	OpNext
	OpTypeAssert
	OpExtract
	// Instructions executed for effect
	OpJump
	OpIf
	OpReturn
	OpRunDefers
	OpPanic
	OpGo
	OpDefer
	OpSend
	OpStore
	OpMapUpdate
	OpDebugRef
)
*/

type kind int

func (k kind) isStatic() bool {
	return k == kindConst || k == kindGlobal || k == kindFunction
}

const (
	kindInvalid kind = iota
	kindConst
	kindGlobal
	kindFunction
)

type Value = value
type Tuple = tuple

type register int
type value = interface{}
type tuple []value

type closure struct {
	pfn *function
	env []value
}

type function struct {
	Interp    *Interp
	Fn        *ssa.Function        // ssa function
	Main      *ssa.BasicBlock      // Fn.Blocks[0]
	pool      *sync.Pool           // create frame pool
	index     map[ssa.Value]uint32 // stack index
	Instrs    []func(vm *goVm)     // main instrs
	InstrsLen int
	Recover   []func(vm *goVm)  // recover instrs
	Blocks    []int             // block offset
	stack     []value           // results args envs datas
	ssaInstrs []ssa.Instruction // org ssa instr
	nres      int               // results count
	narg      int               // arguments count
	nenv      int               // closure free vars count
	used      int32             // function used count
	cached    int32             // enable cached by pool
}

func (p *function) initPool() {
	p.pool = &sync.Pool{}
	p.pool.New = func() interface{} {
		vm := &goVm{pfn: p, block: p.Main}
		vm.stack = append([]value{}, p.stack...)
		return vm
	}
}

func (p *function) allocFrame(caller *goVm) *goVm {
	var vm *goVm
	if atomic.LoadInt32(&p.cached) == 1 {
		vm = p.pool.Get().(*goVm)
	} else {
		if atomic.AddInt32(&p.used, 1) > int32(p.Interp.ctx.CallForPool) {
			atomic.StoreInt32(&p.cached, 1)
		}
		vm = &goVm{pfn: p, block: p.Main}
		vm.stack = append([]value{}, p.stack...)
	}
	vm.caller = caller
	if caller != nil {
		vm.deferid = caller.deferid
	}
	if vm.pc == -1 {
		vm.block = p.Main
		vm.defers = nil
		vm.panicking = nil
		vm.pc = 0
		vm.pred = 0
	}
	return vm
}

func (p *function) deleteFrame(vm *goVm) {
	if atomic.LoadInt32(&p.cached) == 1 {
		p.pool.Put(vm)
	}
	vm = nil
}

func (p *function) regIndex3(v ssa.Value) (register, kind, value) {
	instr := p.regInstr(v)
	index := int(instr & 0xffffff)
	return register(index), kind(instr >> 24), p.stack[index]
}

func (p *function) regIndex(v ssa.Value) register {
	instr := p.regInstr(v)
	return register(instr & 0xffffff)
}

func (p *function) regInstr(v ssa.Value) uint32 {
	if i, ok := p.index[v]; ok {
		return i
	}
	var vs interface{}
	var vk kind
	switch v := v.(type) {
	case *ssa.Const: // find const var
		vs = p.Interp.constToValue(v)
		vk = kindConst
	case *ssa.Global: // find global var
		vs, _ = p.Interp.globalToValue(v)
		vk = kindGlobal
	case *ssa.Function:
		vk = kindFunction
		if v.Blocks != nil {
			typ := p.Interp.preToType(v.Type())
			pfn := p.Interp.loadFunction(v)
			vs = p.Interp.makeFunction(typ, pfn, nil).Interface()
		} else {
			if ext, ok := findExternFunc(p.Interp, v); !ok {
				if v.Name() != "init" {
					panic(fmt.Errorf("no code for function: %v", v))
				}
			} else {
				vs = ext.Interface()
			}
		}
	}
	i := uint32(len(p.stack) | int(vk<<24))
	p.stack = append(p.stack, vs) // store stack value
	p.index[v] = i                // store stack index
	return i
}

func findExternFunc(interp *Interp, fn *ssa.Function) (ext reflect.Value, ok bool) {
	fnName := fn.String()
	switch fnName {
	case "os.Exit":
		return reflect.ValueOf(func(code int) {
			if atomic.LoadInt32(&interp.goexited) == 1 {
				interp.chexit <- code
			} else {
				panic(exitPanic(code))
			}
		}), true
	case "runtime.Goexit":
		return reflect.ValueOf(func() {
			// main goroutine use panic
			if goroutineID() == interp.mainid {
				atomic.StoreInt32(&interp.goexited, 1)
				panic(goexitPanic(0))
			} else {
				runtime.Goexit()
			}
		}), true
	default:
		// pass
	}
	if ext, ok = interp.ctx.Override[fnName]; ok { // check override func
		return
	}
	if ext, ok = externValues[fnName]; ok { // check extern func
		return
	}
	if fn.Pkg != nil {
		if recv := fn.Signature.Recv(); recv == nil {
			if pkg, found := interp.ctx.Loader.Installed(fn.Pkg.Pkg.Path()); found {
				ext, ok = pkg.Funcs[fn.Name()]
			}
		} else if typ, found := interp.ctx.Loader.LookupReflect(recv.Type()); found {
			if m, found := typ.MethodByName(fn.Name()); found {
				ext, ok = m.Func, true
			}
		}
	}
	return
}

func makeInstr(interp *Interp, pfn *function, instr ssa.Instruction) func(vm *goVm) {
	switch instr := instr.(type) {
	case *ssa.Alloc:
		if instr.Heap {
			typ := interp.preToType(instr.Type())
			ir := pfn.regIndex(instr)
			t := xtype.TypeOfType(typ.Elem())
			pt := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				vm.setReg(ir, xtype.New(t, pt))
			}
		}
		typ := interp.preToType(instr.Type())
		ir := pfn.regIndex(instr)
		t := xtype.TypeOfType(typ.Elem())
		pt := xtype.TypeOfType(typ)
		elem := reflect.New(typ.Elem()).Elem()
		return func(vm *goVm) {
			if v := vm.reg(ir); v != nil {
				reflect.ValueOf(v).Elem().Set(elem)
			} else {
				vm.setReg(ir, xtype.New(t, pt))
			}
		}
	case *ssa.Phi:
		ir := pfn.regIndex(instr)
		ie := make([]register, len(instr.Edges))
		for i, v := range instr.Edges {
			ie[i] = pfn.regIndex(v)
		}
		return func(vm *goVm) {
			for i, pred := range instr.Block().Preds {
				if vm.pred == pred.Index {
					vm.setReg(ir, vm.reg(ie[i]))
					break
				}
			}
		}
	case *ssa.Call:
		return makeCallInstr(pfn, interp, instr, &instr.Call)
	case *ssa.BinOp:
		switch instr.Op {
		case token.ADD:
			return makeBinOpADD(pfn, instr)
		case token.SUB:
			return makeBinOpSUB(pfn, instr)
		case token.MUL:
			return makeBinOpMUL(pfn, instr)
		case token.QUO:
			return makeBinOpQUO(pfn, instr)
		case token.REM:
			return makeBinOpREM(pfn, instr)
		case token.AND:
			return makeBinOpAND(pfn, instr)
		case token.OR:
			return makeBinOpOR(pfn, instr)
		case token.XOR:
			return makeBinOpXOR(pfn, instr)
		case token.AND_NOT:
			return makeBinOpANDNOT(pfn, instr)
		case token.LSS:
			return makeBinOpLSS(pfn, instr)
		case token.LEQ:
			return makeBinOpLEQ(pfn, instr)
		case token.GTR:
			return makeBinOpGTR(pfn, instr)
		case token.GEQ:
			return makeBinOpGEQ(pfn, instr)
		case token.EQL:
			return makeBinOpEQL(pfn, instr)
		case token.NEQ:
			return makeBinOpNEQ(pfn, instr)
		case token.SHL:
			return makeBinOpSHL(pfn, instr)
		case token.SHR:
			return makeBinOpSHR(pfn, instr)
		default:
			panic("unreachable")
		}
	case *ssa.UnOp:
		switch instr.Op {
		case token.NOT:
			return makeUnOpNOT(pfn, instr)
		case token.SUB:
			return makeUnOpSUB(pfn, instr)
		case token.XOR:
			return makeUnOpXOR(pfn, instr)
		case token.ARROW:
			return makeUnOpARROW(pfn, instr)
		case token.MUL:
			return makeUnOpMUL(pfn, instr)
		default:
			panic("unreachable")
		}
	case *ssa.ChangeInterface:
		ir := pfn.regIndex(instr)
		ix := pfn.regIndex(instr.X)
		return func(vm *goVm) {
			vm.setReg(ir, vm.reg(ix))
		}
	case *ssa.ChangeType:
		return makeTypeChangeInstr(pfn, instr)
	case *ssa.Convert:
		return makeConvertInstr(pfn, interp, instr)
	case *ssa.MakeInterface:
		typ := interp.preToType(instr.Type())
		ir := pfn.regIndex(instr)
		ix, kx, vx := pfn.regIndex3(instr.X)
		if kx.isStatic() {
			if typ == loader.TypesEmptyInterfaceV2 {
				return func(vm *goVm) {
					vm.setReg(ir, vx)
				}
			}
			v := reflect.New(typ).Elem()
			if vx != nil {
				setValue(v, reflect.ValueOf(vx))
			}
			vx = v.Interface()
			return func(vm *goVm) {
				vm.setReg(ir, vx)
			}
		}
		if typ == loader.TypesEmptyInterfaceV2 {
			return func(vm *goVm) {
				vm.setReg(ir, vm.reg(ix))
			}
		}
		return func(vm *goVm) {
			v := reflect.New(typ).Elem()
			if x := vm.reg(ix); x != nil {
				vx := reflect.ValueOf(x)
				setValue(v, vx)
			}
			vm.setReg(ir, v.Interface())
		}
	case *ssa.MakeClosure:
		fn := instr.Fn.(*ssa.Function)
		typ := interp.preToType(fn.Type())
		ir := pfn.regIndex(instr)
		ib := make([]register, len(instr.Bindings))
		for i, v := range instr.Bindings {
			ib[i] = pfn.regIndex(v)
		}
		pfn := interp.loadFunction(fn)
		return func(vm *goVm) {
			var bindings []value
			for i := range instr.Bindings {
				bindings = append(bindings, vm.reg(ib[i]))
			}
			v := interp.makeFunction(typ, pfn, bindings)
			vm.setReg(ir, v.Interface())
		}
	case *ssa.MakeChan:
		typ := interp.preToType(instr.Type())
		ir := pfn.regIndex(instr)
		is := pfn.regIndex(instr.Size)
		return func(vm *goVm) {
			size := vm.reg(is)
			buffer := asInt(size)
			if buffer < 0 {
				panic(runtimeError("makechan: size out of range"))
			}
			vm.setReg(ir, reflect.MakeChan(typ, buffer).Interface())
		}
	case *ssa.MakeMap:
		typ := instr.Type()
		rtyp := interp.preToType(typ)
		ir := pfn.regIndex(instr)
		if instr.Reserve == nil {
			return func(vm *goVm) {
				vm.setReg(ir, reflect.MakeMap(rtyp).Interface())
			}
		}
		iv := pfn.regIndex(instr.Reserve)
		return func(vm *goVm) {
			reserve := asInt(vm.reg(iv))
			vm.setReg(ir, reflect.MakeMapWithSize(rtyp, reserve).Interface())
		}
	case *ssa.MakeSlice:
		typ := interp.preToType(instr.Type())
		ir := pfn.regIndex(instr)
		il := pfn.regIndex(instr.Len)
		ic := pfn.regIndex(instr.Cap)
		return func(vm *goVm) {
			ilen := asInt(vm.reg(il))
			if ilen < 0 || ilen >= maxMemLen {
				panic(runtimeError("makeslice: len out of range"))
			}
			icap := asInt(vm.reg(ic))
			if icap < 0 || icap >= maxMemLen {
				panic(runtimeError("makeslice: cap out of range"))
			}
			vm.setReg(ir, reflect.MakeSlice(typ, ilen, icap).Interface())
		}
	case *ssa.Slice:
		typ := interp.preToType(instr.Type())
		isNamed := typ.Kind() == reflect.Slice && typ != reflect.SliceOf(typ.Elem())
		_, makesliceCheck := instr.X.(*ssa.Alloc)
		ir := pfn.regIndex(instr)
		ix := pfn.regIndex(instr.X)
		ih := pfn.regIndex(instr.High)
		il := pfn.regIndex(instr.Low)
		im := pfn.regIndex(instr.Max)
		if isNamed {
			return func(vm *goVm) {
				vm.setReg(ir, slice(vm, instr, makesliceCheck, ix, ih, il, im).Convert(typ).Interface())
			}
		}
		return func(vm *goVm) {
			vm.setReg(ir, slice(vm, instr, makesliceCheck, ix, ih, il, im).Interface())
		}
	case *ssa.FieldAddr:
		ir := pfn.regIndex(instr)
		ix := pfn.regIndex(instr.X)
		return func(vm *goVm) {
			v, err := loader.FieldAddrX(vm.reg(ix), instr.Field)
			if err != nil {
				panic(runtimeError(err.Error()))
			}
			vm.setReg(ir, v)
		}
	case *ssa.Field:
		ir := pfn.regIndex(instr)
		ix := pfn.regIndex(instr.X)
		return func(vm *goVm) {
			v, err := loader.FieldX(vm.reg(ix), instr.Field)
			if err != nil {
				panic(runtimeError(err.Error()))
			}
			vm.setReg(ir, v)
		}
	case *ssa.IndexAddr:
		ir := pfn.regIndex(instr)
		ix := pfn.regIndex(instr.X)
		ii := pfn.regIndex(instr.Index)
		return func(vm *goVm) {
			x := vm.reg(ix)
			idx := vm.reg(ii)
			v := reflect.ValueOf(x)
			if v.Kind() == reflect.Ptr {
				v = v.Elem()
			}
			switch v.Kind() {
			case reflect.Slice:
			case reflect.Array:
			case reflect.Invalid:
				panic(runtimeError("invalid memory address or nil pointer dereference"))
			default:
				panic(fmt.Sprintf("unexpected x type in IndexAddr: %T", x))
			}
			index := asInt(idx)
			if index < 0 {
				panic(runtimeError(fmt.Sprintf("index out of range [%v]", index)))
			} else if length := v.Len(); index >= length {
				panic(runtimeError(fmt.Sprintf("index out of range [%v] with length %v", index, length)))
			}
			vm.setReg(ir, v.Index(index).Addr().Interface())
		}
	case *ssa.Index:
		ir := pfn.regIndex(instr)
		ix := pfn.regIndex(instr.X)
		ii := pfn.regIndex(instr.Index)
		return func(vm *goVm) {
			x := vm.reg(ix)
			idx := vm.reg(ii)
			v := reflect.ValueOf(x)
			vm.setReg(ir, v.Index(asInt(idx)).Interface())
		}
	case *ssa.Lookup:
		typ := interp.preToType(instr.X.Type())
		ir := pfn.regIndex(instr)
		ix := pfn.regIndex(instr.X)
		ii := pfn.regIndex(instr.Index)
		switch typ.Kind() {
		case reflect.String:
			return func(vm *goVm) {
				v := vm.reg(ix)
				idx := vm.reg(ii)
				vm.setReg(ir, reflect.ValueOf(v).String()[asInt(idx)])
			}
		case reflect.Map:
			return func(vm *goVm) {
				m := vm.reg(ix)
				idx := vm.reg(ii)
				rval := reflect.ValueOf(m)
				v := rval.MapIndex(reflect.ValueOf(idx))
				ok := v.IsValid()
				var rv value
				if ok {
					rv = v.Interface()
				} else {
					rv = reflect.New(typ.Elem()).Elem().Interface()
				}
				if instr.CommaOk {
					vm.setReg(ir, tuple{rv, ok})
				} else {
					vm.setReg(ir, rv)
				}
			}
		default:
			panic("unreachable")
		}
	case *ssa.Select:
		ir := pfn.regIndex(instr)
		ic := make([]register, len(instr.States))
		is := make([]register, len(instr.States))
		for i, state := range instr.States {
			ic[i] = pfn.regIndex(state.Chan)
			if state.Send != nil {
				is[i] = pfn.regIndex(state.Send)
			}
		}
		return func(vm *goVm) {
			var cases []reflect.SelectCase
			if !instr.Blocking {
				cases = append(cases, reflect.SelectCase{
					Dir: reflect.SelectDefault,
				})
			}
			for i, state := range instr.States {
				var dir reflect.SelectDir
				if state.Dir == types.RecvOnly {
					dir = reflect.SelectRecv
				} else {
					dir = reflect.SelectSend
				}
				ch := reflect.ValueOf(vm.reg(ic[i]))
				var send reflect.Value
				if state.Send != nil {
					v := vm.reg(is[i])
					if v == nil {
						send = reflect.New(ch.Type().Elem()).Elem()
					} else {
						send = reflect.ValueOf(v)
					}
				}
				cases = append(cases, reflect.SelectCase{
					Dir:  dir,
					Chan: ch,
					Send: send,
				})
			}
			chosen, recv, recvOk := reflect.Select(cases)
			if !instr.Blocking {
				chosen-- // default case should have index -1.
			}
			r := tuple{chosen, recvOk}
			for n, st := range instr.States {
				if st.Dir == types.RecvOnly {
					var v value
					if n == chosen && recvOk {
						// No need to copy since send makes an unaliased copy.
						v = recv.Interface()
					} else {
						typ := interp.toType(st.Chan.Type())
						v = reflect.New(typ.Elem()).Elem().Interface()
					}
					r = append(r, v)
				}
			}
			vm.setReg(ir, r)
		}
	case *ssa.SliceToArrayPointer:
		typ := interp.preToType(instr.Type())
		ir := pfn.regIndex(instr)
		ix := pfn.regIndex(instr.X)
		return func(vm *goVm) {
			x := vm.reg(ix)
			v := reflect.ValueOf(x)
			vLen := v.Len()
			tLen := typ.Elem().Len()
			if tLen > vLen {
				panic(runtimeError(fmt.Sprintf("cannot convert slice with length %v to pointer to array with length %v", vLen, tLen)))
			}
			vm.setReg(ir, v.Convert(typ).Interface())
		}
	case *ssa.Range:
		typ := interp.preToType(instr.X.Type())
		ir := pfn.regIndex(instr)
		ix := pfn.regIndex(instr.X)
		switch typ.Kind() {
		case reflect.String:
			return func(vm *goVm) {
				v := vm.string(ix)
				vm.setReg(ir, &stringIter{Reader: strings.NewReader(v)})
			}
		case reflect.Map:
			return func(vm *goVm) {
				v := vm.reg(ix)
				vm.setReg(ir, &mapIter{iter: reflect.ValueOf(v).MapRange()})
			}
		default:
			panic("unreachable")
		}
	case *ssa.Next:
		ir := pfn.regIndex(instr)
		ii := pfn.regIndex(instr.Iter)
		if instr.IsString {
			return func(vm *goVm) {
				vm.setReg(ir, vm.reg(ii).(*stringIter).next())
			}
		}
		return func(vm *goVm) {
			vm.setReg(ir, vm.reg(ii).(*mapIter).next())
		}
	case *ssa.TypeAssert:
		typ := interp.preToType(instr.AssertedType)
		ir := pfn.regIndex(instr)
		ix, kx, vx := pfn.regIndex3(instr.X)
		if kx.isStatic() {
			return func(vm *goVm) {
				vm.setReg(ir, typeAssert(interp, instr, typ, vx))
			}
		}
		return func(vm *goVm) {
			v := vm.reg(ix)
			vm.setReg(ir, typeAssert(interp, instr, typ, v))
		}
	case *ssa.Extract:
		if *instr.Referrers() == nil {
			return nil
		}
		ir := pfn.regIndex(instr)
		it := pfn.regIndex(instr.Tuple)
		return func(vm *goVm) {
			vm.setReg(ir, vm.reg(it).(tuple)[instr.Index])
		}
	// Instructions executed for effect
	case *ssa.Jump:
		return func(vm *goVm) {
			vm.pred, vm.block = vm.block.Index, vm.block.Succs[0]
			vm.pc = vm.pfn.Blocks[vm.block.Index]
		}
	case *ssa.If:
		ic, kc, vc := pfn.regIndex3(instr.Cond)
		if kc == kindConst {
			if xtype.Bool(vc) {
				return func(vm *goVm) {
					vm.pred = vm.block.Index
					vm.block = vm.block.Succs[0]
					vm.pc = vm.pfn.Blocks[vm.block.Index]
				}
			}
			return func(vm *goVm) {
				vm.pred = vm.block.Index
				vm.block = vm.block.Succs[1]
				vm.pc = vm.pfn.Blocks[vm.block.Index]
			}
		}
		switch instr.Cond.Type().(type) {
		case *types.Basic:
			return func(vm *goVm) {
				vm.pred = vm.block.Index
				if vm.reg(ic).(bool) {
					vm.block = vm.block.Succs[0]
				} else {
					vm.block = vm.block.Succs[1]
				}
				vm.pc = vm.pfn.Blocks[vm.block.Index]
			}
		default:
			return func(vm *goVm) {
				vm.pred = vm.block.Index
				if vm.bool(ic) {
					vm.block = vm.block.Succs[0]
				} else {
					vm.block = vm.block.Succs[1]
				}
				vm.pc = vm.pfn.Blocks[vm.block.Index]
			}
		}
	case *ssa.Return:
		switch n := pfn.nres; n {
		case 0:
			return func(vm *goVm) {
				vm.pc = -1
			}
		case 1:
			ir, ik, iv := pfn.regIndex3(instr.Results[0])
			if ik.isStatic() {
				return func(vm *goVm) {
					vm.stack[0] = iv
					vm.pc = -1
				}
			}
			return func(vm *goVm) {
				vm.stack[0] = vm.reg(ir)
				vm.pc = -1
			}
		case 2:
			r1, k1, v1 := pfn.regIndex3(instr.Results[0])
			r2, k2, v2 := pfn.regIndex3(instr.Results[1])
			if k1.isStatic() && k2.isStatic() {
				return func(vm *goVm) {
					vm.stack[0] = v1
					vm.stack[1] = v2
					vm.pc = -1
				}
			} else if k1.isStatic() {
				return func(vm *goVm) {
					vm.stack[0] = v1
					vm.stack[1] = vm.reg(r2)
					vm.pc = -1
				}
			} else if k2.isStatic() {
				return func(vm *goVm) {
					vm.stack[0] = vm.reg(r1)
					vm.stack[1] = v2
					vm.pc = -1
				}
			}
			return func(vm *goVm) {
				vm.stack[0] = vm.reg(r1)
				vm.stack[1] = vm.reg(r2)
				vm.pc = -1
			}
		default:
			ir := make([]register, n)
			for i, v := range instr.Results {
				ir[i] = pfn.regIndex(v)
			}
			return func(vm *goVm) {
				for i := 0; i < n; i++ {
					vm.stack[i] = vm.reg(ir[i])
				}
				vm.pc = -1
			}
		}
	case *ssa.RunDefers:
		return func(vm *goVm) {
			vm.runDefers()
		}
	case *ssa.Panic:
		ix := pfn.regIndex(instr.X)
		return func(vm *goVm) {
			panic(targetPanic{vm.reg(ix)})
		}
	case *ssa.Go:
		iv, ia, ib := getCallIndex(pfn, &instr.Call)
		return func(vm *goVm) {
			fn, args := interp.prepareCall(vm, &instr.Call, iv, ia, ib)
			atomic.AddInt32(&interp.goroutines, 1)
			go func() {
				interp.callDiscardsResult(nil, fn, args, instr.Call.Args)
				atomic.AddInt32(&interp.goroutines, -1)
			}()
		}
	case *ssa.Defer:
		iv, ia, ib := getCallIndex(pfn, &instr.Call)
		return func(vm *goVm) {
			fn, args := interp.prepareCall(vm, &instr.Call, iv, ia, ib)
			vm.defers = &deferred{
				fn:      fn,
				args:    args,
				ssaArgs: instr.Call.Args,
				instr:   instr,
				tail:    vm.defers,
			}
		}
	case *ssa.Send:
		ic := pfn.regIndex(instr.Chan)
		ix := pfn.regIndex(instr.X)
		return func(vm *goVm) {
			c := vm.reg(ic)
			x := vm.reg(ix)
			ch := reflect.ValueOf(c)
			if x == nil {
				ch.Send(reflect.New(ch.Type().Elem()).Elem())
			} else {
				ch.Send(reflect.ValueOf(x))
			}
		}
	case *ssa.Store:
		// skip struct field _
		if addr, ok := instr.Addr.(*ssa.FieldAddr); ok {
			if s, ok := addr.X.Type().(*types.Pointer).Elem().(*types.Struct); ok {
				if s.Field(addr.Field).Name() == "_" {
					return nil
				}
			}
		}
		ia := pfn.regIndex(instr.Addr)
		iv, kv, vv := pfn.regIndex3(instr.Val)
		if kv.isStatic() {
			if vv == nil {
				return func(vm *goVm) {
					x := reflect.ValueOf(vm.reg(ia))
					setValue(x.Elem(), reflect.New(x.Elem().Type()).Elem())
				}
			}
			return func(vm *goVm) {
				x := reflect.ValueOf(vm.reg(ia))
				setValue(x.Elem(), reflect.ValueOf(vv))
			}
		}
		return func(vm *goVm) {
			x := reflect.ValueOf(vm.reg(ia))
			val := vm.reg(iv)
			v := reflect.ValueOf(val)
			if v.IsValid() {
				setValue(x.Elem(), v)
			} else {
				setValue(x.Elem(), reflect.New(x.Elem().Type()).Elem())
			}
		}
	case *ssa.MapUpdate:
		im := pfn.regIndex(instr.Map)
		ik := pfn.regIndex(instr.Key)
		iv, kv, vv := pfn.regIndex3(instr.Value)
		if kv.isStatic() {
			return func(vm *goVm) {
				rval := reflect.ValueOf(vm.reg(im))
				vk := reflect.ValueOf(vm.reg(ik))
				rval.SetMapIndex(vk, reflect.ValueOf(vv))
			}
		}
		return func(vm *goVm) {
			rval := reflect.ValueOf(vm.reg(im))
			vk := reflect.ValueOf(vm.reg(ik))
			v := vm.reg(iv)
			rval.SetMapIndex(vk, reflect.ValueOf(v))
		}
	case *ssa.DebugRef:
		if v, ok := instr.Object().(*types.Var); ok {
			ix := pfn.regIndex(instr.X)
			return func(vm *goVm) {
				ref := &loader.DebugInfo{DebugRef: instr, FSet: interp.ctx.FileSet}
				ref.ToValue = func() (*types.Var, interface{}, bool) {
					return v, vm.reg(ix), true
				}
				interp.ctx.DebugFunc(ref)
			}
		}
		return func(vm *goVm) {
			ref := &loader.DebugInfo{DebugRef: instr, FSet: interp.ctx.FileSet}
			ref.ToValue = func() (*types.Var, interface{}, bool) {
				return nil, nil, false
			}
			interp.ctx.DebugFunc(ref)
		}
	default:
		panic(fmt.Errorf("unreachable %T", instr))
	}
}

func getCallIndex(pfn *function, call *ssa.CallCommon) (iv register, ia []register, ib []register) {
	iv = pfn.regIndex(call.Value)
	ia = make([]register, len(call.Args))
	for i, v := range call.Args {
		ia[i] = pfn.regIndex(v)
	}
	if f, ok := call.Value.(*ssa.MakeClosure); ok {
		ib = make([]register, len(f.Bindings))
		for i, binding := range f.Bindings {
			ib[i] = pfn.regIndex(binding)
		}
	}
	return
}

func makeCallInstr(pfn *function, interp *Interp, instr ssa.Value, call *ssa.CallCommon) func(vm *goVm) {
	ir := pfn.regIndex(instr)
	iv, ia, ib := getCallIndex(pfn, call)
	switch fn := call.Value.(type) {
	case *ssa.Builtin:
		fname := fn.Name()
		return func(vm *goVm) {
			interp.callBuiltinByStack(vm, fname, call.Args, ir, ia)
		}
	case *ssa.MakeClosure:
		ifn := interp.loadFunction(fn.Fn.(*ssa.Function))
		ia = append(ia, ib...)
		if ifn.Recover == nil {
			switch ifn.nres {
			case 0:
				return func(vm *goVm) {
					interp.callFunctionByStackNoRecover0(vm, ifn, ir, ia)
				}
			case 1:
				return func(vm *goVm) {
					interp.callFunctionByStackNoRecover1(vm, ifn, ir, ia)
				}
			default:
				return func(vm *goVm) {
					interp.callFunctionByStackNoRecoverN(vm, ifn, ir, ia)
				}
			}
		}
		switch ifn.nres {
		case 0:
			return func(vm *goVm) {
				interp.callFunctionByStack0(vm, ifn, ir, ia)
			}
		case 1:
			return func(vm *goVm) {
				interp.callFunctionByStack1(vm, ifn, ir, ia)
			}
		default:
			return func(vm *goVm) {
				interp.callFunctionByStackN(vm, ifn, ir, ia)
			}
		}
	case *ssa.Function:
		// "static func/method call"
		if fn.Blocks == nil {
			ext, ok := findExternFunc(interp, fn)
			if !ok {
				// skip pkg.init
				if fn.Pkg != nil && fn.Name() == "init" {
					return nil
				}
				panic(fmt.Errorf("no code for function: %v", fn))
			}
			return func(vm *goVm) {
				interp.callExternalByStack(vm, ext, ir, ia)
			}
		}
		ifn := interp.loadFunction(fn)
		if ifn.Recover == nil {
			switch ifn.nres {
			case 0:
				return func(vm *goVm) {
					interp.callFunctionByStackNoRecover0(vm, ifn, ir, ia)
				}
			case 1:
				return func(vm *goVm) {
					interp.callFunctionByStackNoRecover1(vm, ifn, ir, ia)
				}
			default:
				return func(vm *goVm) {
					interp.callFunctionByStackNoRecoverN(vm, ifn, ir, ia)
				}
			}
		}
		switch ifn.nres {
		case 0:
			return func(vm *goVm) {
				interp.callFunctionByStack0(vm, ifn, ir, ia)
			}
		case 1:
			return func(vm *goVm) {
				interp.callFunctionByStack1(vm, ifn, ir, ia)
			}
		default:
			return func(vm *goVm) {
				interp.callFunctionByStackN(vm, ifn, ir, ia)
			}
		}
	}
	// "dynamic method call" // ("invoke" mode)
	if call.IsInvoke() {
		return makeCallMethodInstr(interp, instr, call, ir, iv, ia)
	}
	// dynamic func call
	typ := interp.preToType(call.Value.Type())
	if typ.Kind() != reflect.Func {
		panic("unsupport")
	}
	return func(vm *goVm) {
		fn := vm.reg(iv)
		v := reflect.ValueOf(fn)
		interp.callExternalByStack(vm, v, ir, ia)
	}
}

func findUserMethod(typ reflect.Type, name string) (ext reflect.Value, ok bool) {
	if m, ok := typ.MethodByName(name); ok {
		return m.Func, true
	}
	return
}

func findExternMethod(typ reflect.Type, name string) (ext reflect.Value, ok bool) {
	if m, ok := typ.MethodByName(name); ok {
		return m.Func, true
	}
	return
}

func (i *Interp) findMethod(typ reflect.Type, mname string) (fn *ssa.Function, ok bool) {
	if mset, mok := i.msets[typ]; mok {
		fn, ok = mset[mname]
	}
	return
}

func makeCallMethodInstr(interp *Interp, instr ssa.Value, call *ssa.CallCommon, ir register, iv register, ia []register) func(vm *goVm) {
	mname := call.Method.Name()
	ia = append([]register{iv}, ia...)
	var found bool
	var ext reflect.Value
	return func(vm *goVm) {
		v := vm.reg(iv)
		rtype := reflect.TypeOf(v)
		// find user type method *ssa.Function
		if mset, ok := interp.msets[rtype]; ok {
			if fn, ok := mset[mname]; ok {
				interp.callFunctionByStack(vm, interp.funcs[fn], ir, ia)
				return
			}
			ext, found = findUserMethod(rtype, mname)
		} else {
			ext, found = findExternMethod(rtype, mname)
		}
		if !found {
			panic(fmt.Errorf("no code for method: %v.%v", rtype, mname))
		}
		interp.callExternalByStack(vm, ext, ir, ia)
	}
}

type stringIter struct {
	*strings.Reader
	i int
}

func (it *stringIter) next() tuple {
	okv := make(tuple, 3)
	ch, n, err := it.ReadRune()
	ok := err != io.EOF
	okv[0] = ok
	if ok {
		okv[1] = it.i
		okv[2] = ch
	}
	it.i += n
	return okv
}

type mapIter struct {
	iter *reflect.MapIter
	ok   bool
}

func (it *mapIter) next() tuple {
	it.ok = it.iter.Next()
	if !it.ok {
		return []value{false, nil, nil}
	}
	k, v := it.iter.Key().Interface(), it.iter.Value().Interface()
	return []value{true, k, v}
}
