package goscript

import (
	"fmt"
	"go/constant"
	"go/types"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/linkxzhou/gongx/interp/goscript/loader"
	"github.com/petermattis/goid"
	"golang.org/x/tools/go/ssa"
)

type Interp struct {
	ctx          *loader.Context
	mainpkg      *ssa.Package                                // the SSA main package
	record       *loader.TypesRecord                         // lookup type and ToType
	globals      map[ssa.Value]value                         // addresses of global variables (immutable)
	preloadTypes map[types.Type]reflect.Type                 // preload types.Type -> reflect.Type
	funcs        map[*ssa.Function]*function                 // ssa.Function -> *function
	msets        map[reflect.Type](map[string]*ssa.Function) // user defined type method sets
	chexit       chan int                                    // call os.Exit code by chan for runtime.Goexit
	deferMap     sync.Map                                    // defer goroutine id -> call frame
	typesMutex   sync.RWMutex                                // findType/toType mutex
	mainid       int64                                       // main goroutine id
	exitCode     int                                         // call os.Exit code
	goroutines   int32                                       // atomically updated
	deferCount   int32                                       // fast has defer check
	goexited     int32                                       // is call runtime.Goexit
	exited       int32                                       // is call os.Exit
}

func (i *Interp) loadFunction(fn *ssa.Function) *function {
	if pfn, ok := i.funcs[fn]; ok {
		return pfn
	}
	pfn := &function{
		Interp: i,
		Fn:     fn,
		index:  make(map[ssa.Value]uint32),
		narg:   len(fn.Params),
		nenv:   len(fn.FreeVars),
	}
	if len(fn.Blocks) > 0 {
		pfn.Main = fn.Blocks[0]
	}
	if res := fn.Signature.Results(); res != nil {
		pfn.nres = res.Len()
		pfn.stack = make([]value, pfn.nres)
	}
	i.funcs[fn] = pfn
	return pfn
}

func (i *Interp) findType(rt reflect.Type, local bool) (types.Type, bool) {
	i.typesMutex.Lock()
	defer i.typesMutex.Unlock()
	if local {
		return i.record.LookupLocalTypes(rt)
	} else {
		return i.record.LookupTypes(rt)
	}
}

func (i *Interp) tryDeferFrame() *goVm {
	if atomic.LoadInt32(&i.deferCount) != 0 {
		if f, ok := i.deferMap.Load(goroutineID()); ok {
			return f.(*goVm)
		}
	}
	return nil
}

func (i *Interp) FindMethod(mtyp reflect.Type, fn *types.Func) func([]reflect.Value) []reflect.Value {
	typ := fn.Type().(*types.Signature).Recv().Type()
	if f := i.mainpkg.Prog.LookupMethod(typ, fn.Pkg(), fn.Name()); f != nil {
		pfn := i.loadFunction(f)
		return func(args []reflect.Value) []reflect.Value {
			return i.callFunctionByReflect(i.tryDeferFrame(), mtyp, pfn, args, nil)
		}
	}
	name := fn.FullName()
	if v, ok := externValues[name]; ok && v.Kind() == reflect.Func {
		if v.Type().IsVariadic() {
			return func(args []reflect.Value) []reflect.Value {
				return v.CallSlice(args)
			}
		}
		return func(args []reflect.Value) []reflect.Value {
			return v.Call(args)
		}
	}
	panic(fmt.Sprintf("Not found method %v", fn))
}

func (i *Interp) makeFunction(typ reflect.Type, pfn *function, env []value) reflect.Value {
	return reflect.MakeFunc(typ, func(args []reflect.Value) []reflect.Value {
		return i.callFunctionByReflect(i.tryDeferFrame(), typ, pfn, args, env)
	})
}

func SetValue(v reflect.Value, x reflect.Value) {
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(x.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(x.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(x.Uint())
	case reflect.Uintptr:
		v.SetUint(x.Uint())
	case reflect.Float32, reflect.Float64:
		v.SetFloat(x.Float())
	case reflect.Complex64, reflect.Complex128:
		v.SetComplex(x.Complex())
	case reflect.String:
		v.SetString(x.String())
	case reflect.UnsafePointer:
		v.SetPointer(unsafe.Pointer(x.Pointer()))
	default:
		v.Set(x)
	}
}

// prepareCall determines the function value and argument values for a
// function call in a Call, Go or Defer instruction, performing
// interface method lookup if needed.
func (i *Interp) prepareCall(vm *goVm, call *ssa.CallCommon, iv register, ia []register, ib []register) (fv value, args []value) {
	if call.Method == nil {
		switch f := call.Value.(type) {
		case *ssa.Builtin:
			fv = f
		case *ssa.Function:
			if f.Blocks == nil {
				ext, ok := findExternFunc(i, f)
				if !ok {
					if f.Pkg != nil && f.Name() == "init" {
						fv = func() {}
					} else {
						panic(fmt.Errorf("no code for function: %v", f))
					}
				} else {
					fv = ext
				}
			} else {
				fv = f
			}
		case *ssa.MakeClosure:
			var bindings []value
			for i := range f.Bindings {
				bindings = append(bindings, vm.reg(ib[i]))
			}
			fv = &closure{i.funcs[f.Fn.(*ssa.Function)], bindings}
		default:
			fv = vm.reg(iv)
		}
	} else {
		v := vm.reg(iv)
		rtype := reflect.TypeOf(v)
		mname := call.Method.Name()
		if mset, ok := i.msets[rtype]; ok {
			if f, ok := mset[mname]; ok {
				fv = f
			} else {
				ext, ok := findUserMethod(rtype, mname)
				if !ok {
					panic(fmt.Errorf("no code for method: %v.%v", rtype, mname))
				}
				fv = ext
			}
		} else {
			ext, ok := findExternMethod(rtype, mname)
			if !ok {
				panic(fmt.Errorf("no code for method: %v.%v", rtype, mname))
			}
			fv = ext
		}
		args = append(args, v)
	}
	for i := range call.Args {
		v := vm.reg(ia[i])
		args = append(args, v)
	}
	return
}

// call interprets a call to a function (function, builtin or closure)
// fn with arguments args, returning its result.
// callpos is the position of the callsite.
func (i *Interp) call(caller *goVm, fn value, args []value, ssaArgs []ssa.Value) value {
	switch fn := fn.(type) {
	case *ssa.Function:
		return i.callFunction(caller, i.funcs[fn], args, nil)
	case *ssa.Builtin:
		return i.callBuiltin(caller, fn, args, ssaArgs)
	case *closure:
		return i.callFunction(caller, fn.pfn, args, fn.env)
	case reflect.Value:
		return i.callExternal(caller, fn, args, nil)
	default:
		return i.callExternal(caller, reflect.ValueOf(fn), args, nil)
	}
}

// call interprets a call to a function (function, builtin or closure)
// fn with arguments args, returning its result.
// callpos is the position of the callsite.
func (i *Interp) callDiscardsResult(caller *goVm, fn value, args []value, ssaArgs []ssa.Value) {
	switch fn := fn.(type) {
	case *ssa.Function:
		i.callFunctionDiscardsResult(caller, i.funcs[fn], args, nil)
	case *closure:
		i.callFunctionDiscardsResult(caller, fn.pfn, args, fn.env)
	case *ssa.Builtin:
		i.callBuiltinDiscardsResult(caller, fn, args, ssaArgs)
	case reflect.Value:
		i.callExternalDiscardsResult(caller, fn, args, nil)
	default:
		i.callExternalDiscardsResult(caller, reflect.ValueOf(fn), args, nil)
	}
}

func (i *Interp) callFunction(caller *goVm, pfn *function, args []value, env []value) (result value) {
	vm := pfn.allocFrame(caller)
	defer pfn.deleteFrame(vm)

	for i := 0; i < pfn.narg; i++ {
		vm.stack[i+pfn.nres] = args[i]
	}
	for i := 0; i < pfn.nenv; i++ {
		vm.stack[pfn.narg+i+pfn.nres] = env[i]
	}
	vm.run()
	if pfn.nres == 1 {
		result = vm.stack[0]
	} else if pfn.nres > 1 {
		result = tuple(vm.stack[0:pfn.nres])
	}

	return
}

func (i *Interp) callFunctionByReflect(caller *goVm, typ reflect.Type, pfn *function, args []reflect.Value, env []value) (results []reflect.Value) {
	vm := pfn.allocFrame(caller)
	for i := 0; i < pfn.narg; i++ {
		vm.stack[i+pfn.nres] = args[i].Interface()
	}
	for i := 0; i < pfn.nenv; i++ {
		vm.stack[pfn.narg+i+pfn.nres] = env[i]
	}
	vm.run()
	if pfn.nres > 0 {
		results = make([]reflect.Value, pfn.nres)
		for i := 0; i < pfn.nres; i++ {
			v := vm.stack[i]
			if v == nil {
				results[i] = reflect.New(typ.Out(i)).Elem()
			} else {
				results[i] = reflect.ValueOf(v)
			}
		}
	}
	pfn.deleteFrame(vm)
	return
}

func (i *Interp) callFunctionDiscardsResult(caller *goVm, pfn *function, args []value, env []value) {
	vm := pfn.allocFrame(caller)
	for i := 0; i < pfn.narg; i++ {
		vm.stack[i+pfn.nres] = args[i]
	}
	for i := 0; i < pfn.nenv; i++ {
		vm.stack[pfn.narg+i+pfn.nres] = env[i]
	}
	vm.run()
	pfn.deleteFrame(vm)
}

func (i *Interp) callFunctionByStack0(caller *goVm, pfn *function, ir register, ia []register) {
	vm := pfn.allocFrame(caller)
	for i := 0; i < len(ia); i++ {
		vm.stack[i] = caller.reg(ia[i])
	}
	vm.run()
	pfn.deleteFrame(vm)
}

func (i *Interp) callFunctionByStack1(caller *goVm, pfn *function, ir register, ia []register) {
	vm := pfn.allocFrame(caller)
	for i := 0; i < len(ia); i++ {
		vm.stack[i+1] = caller.reg(ia[i])
	}
	vm.run()
	caller.setReg(ir, vm.stack[0])
	pfn.deleteFrame(vm)
}

func (i *Interp) callFunctionByStackN(caller *goVm, pfn *function, ir register, ia []register) {
	vm := pfn.allocFrame(caller)
	for i := 0; i < len(ia); i++ {
		vm.stack[i+pfn.nres] = caller.reg(ia[i])
	}
	vm.run()
	caller.setReg(ir, tuple(vm.stack[0:pfn.nres]))
	pfn.deleteFrame(vm)
}

func (i *Interp) callFunctionByStack(caller *goVm, pfn *function, ir register, ia []register) {
	vm := pfn.allocFrame(caller)
	for i := 0; i < len(ia); i++ {
		vm.stack[i+pfn.nres] = caller.reg(ia[i])
	}
	vm.run()
	if pfn.nres == 1 {
		caller.setReg(ir, vm.stack[0])
	} else if pfn.nres > 1 {
		caller.setReg(ir, tuple(vm.stack[0:pfn.nres]))
	}
	pfn.deleteFrame(vm)
}

func (i *Interp) callFunctionByStackNoRecover0(caller *goVm, pfn *function, ir register, ia []register) {
	vm := pfn.allocFrame(caller)
	for i := 0; i < len(ia); i++ {
		vm.stack[i] = caller.reg(ia[i])
	}
	for vm.pc != -1 && vm.pc < vm.pfn.InstrsLen {
		fn := vm.pfn.Instrs[vm.pc]
		vm.pc++
		fn(vm)
	}
	pfn.deleteFrame(vm)
}

func (i *Interp) callFunctionByStackNoRecover1(caller *goVm, pfn *function, ir register, ia []register) {
	vm := pfn.allocFrame(caller)
	for i := 0; i < len(ia); i++ {
		vm.stack[i+1] = caller.reg(ia[i])
	}
	for vm.pc != -1 && vm.pc < vm.pfn.InstrsLen {
		fn := vm.pfn.Instrs[vm.pc]
		vm.pc++
		fn(vm)
	}
	caller.setReg(ir, vm.stack[0])
	pfn.deleteFrame(vm)
}

func (i *Interp) callFunctionByStackNoRecoverN(caller *goVm, pfn *function, ir register, ia []register) {
	vm := pfn.allocFrame(caller)
	for i := 0; i < len(ia); i++ {
		vm.stack[i+pfn.nres] = caller.reg(ia[i])
	}
	for vm.pc != -1 && vm.pc < vm.pfn.InstrsLen {
		fn := vm.pfn.Instrs[vm.pc]
		vm.pc++
		fn(vm)
	}
	caller.setReg(ir, tuple(vm.stack[0:pfn.nres]))
	pfn.deleteFrame(vm)
}

func (i *Interp) callExternal(caller *goVm, fn reflect.Value, args []value, env []value) value {
	if caller != nil && caller.deferid != 0 {
		i.deferMap.Store(caller.deferid, caller)
	}
	var ins []reflect.Value
	typ := fn.Type()
	isVariadic := fn.Type().IsVariadic()
	if isVariadic {
		for i := 0; i < len(args)-1; i++ {
			if args[i] == nil {
				ins = append(ins, reflect.New(typ.In(i)).Elem())
			} else {
				ins = append(ins, reflect.ValueOf(args[i]))
			}
		}
		ins = append(ins, reflect.ValueOf(args[len(args)-1]))
	} else {
		ins = make([]reflect.Value, len(args))
		for i := 0; i < len(args); i++ {
			if args[i] == nil {
				ins[i] = reflect.New(typ.In(i)).Elem()
			} else {
				ins[i] = reflect.ValueOf(args[i])
			}
		}
	}
	var results []reflect.Value
	if isVariadic {
		results = fn.CallSlice(ins)
	} else {
		results = fn.Call(ins)
	}
	switch len(results) {
	case 0:
		return nil
	case 1:
		return results[0].Interface()
	default:
		var res []value
		for _, r := range results {
			res = append(res, r.Interface())
		}
		return tuple(res)
	}
}

func (i *Interp) callExternalDiscardsResult(caller *goVm, fn reflect.Value, args []value, env []value) {
	if caller != nil && caller.deferid != 0 {
		i.deferMap.Store(caller.deferid, caller)
	}
	var ins []reflect.Value
	typ := fn.Type()
	isVariadic := fn.Type().IsVariadic()
	if isVariadic {
		for i := 0; i < len(args)-1; i++ {
			if args[i] == nil {
				ins = append(ins, reflect.New(typ.In(i)).Elem())
			} else {
				ins = append(ins, reflect.ValueOf(args[i]))
			}
		}
		ins = append(ins, reflect.ValueOf(args[len(args)-1]))
		fn.CallSlice(ins)
	} else {
		ins = make([]reflect.Value, len(args))
		for i := 0; i < len(args); i++ {
			if args[i] == nil {
				ins[i] = reflect.New(typ.In(i)).Elem()
			} else {
				ins[i] = reflect.ValueOf(args[i])
			}
		}
		fn.Call(ins)
	}
}

func (i *Interp) callExternalByStack(caller *goVm, fn reflect.Value, ir register, ia []register) {
	if caller.deferid != 0 {
		i.deferMap.Store(caller.deferid, caller)
	}
	var ins []reflect.Value
	typ := fn.Type()
	isVariadic := fn.Type().IsVariadic()
	if isVariadic {
		var i int
		for n := len(ia) - 1; i < n; i++ {
			arg := caller.reg(ia[i])
			if arg == nil {
				ins = append(ins, reflect.New(typ.In(i)).Elem())
			} else {
				ins = append(ins, reflect.ValueOf(arg))
			}
		}
		ins = append(ins, reflect.ValueOf(caller.reg(ia[i])))
	} else {
		n := len(ia)
		ins = make([]reflect.Value, n)
		for i := 0; i < n; i++ {
			arg := caller.reg(ia[i])
			if arg == nil {
				ins[i] = reflect.New(typ.In(i)).Elem()
			} else {
				ins[i] = reflect.ValueOf(arg)
			}
		}
	}
	var results []reflect.Value
	if isVariadic {
		results = fn.CallSlice(ins)
	} else {
		results = fn.Call(ins)
	}
	switch len(results) {
	case 0:
	case 1:
		caller.setReg(ir, results[0].Interface())
	default:
		var res []value
		for _, r := range results {
			res = append(res, r.Interface())
		}
		caller.setReg(ir, tuple(res))
	}
}

// runFrame executes SSA instructions starting at fr.block and
// continuing until a return, a panic, or a recovered panic.
//
// After a panic, runFrame panics.
//
// After a normal return, fr.result contains the result of the call
// and fr.block is nil.
//
// A recovered panic in a function without named return parameters
// (NRPs) becomes a normal return of the zero value of the function's
// result type.
//
// After a recovered panic in a function with NRPs, fr.result is
// undefined and fr.block contains the block at which to resume
// control.
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
		// TODO(adonovan): support runtime.Goexit.
		switch p := p.(type) {
		case targetPanic:
			// The target program explicitly called panic().
			return p.v
		case runtime.Error:
			// The interpreter encountered a runtime error.
			return p
		case string:
			return p
		case plainError:
			return p
		case *reflect.ValueError:
			return p
		default:
			panic(fmt.Sprintf("unexpected panic type %T in target call to recover()", p))
		}
	}

	return nil //iface{}
}

// Interpret interprets the Go program whose main package is mainpkg.
// mode specifies various interpreter options.  filename and args are
// the initial values of os.Args for the target program.  sizes is the
// effective type-sizing function for this program.
//
// Interpret returns the exit code of the program: 2 for panic (like
// gc does), or the argument to os.Exit for normal termination.
//
// The SSA program must include the "runtime" package.
func NewInterp(ctx *loader.Context, mainpkg *ssa.Package) (*Interp, error) {
	return newInterp(ctx, mainpkg, nil)
}

func newInterp(ctx *loader.Context, mainpkg *ssa.Package, globals map[string]interface{}) (*Interp, error) {
	i := &Interp{
		ctx:          ctx,
		mainpkg:      mainpkg,
		globals:      make(map[ssa.Value]value),
		goroutines:   1,
		preloadTypes: make(map[types.Type]reflect.Type),
		funcs:        make(map[*ssa.Function]*function),
		msets:        make(map[reflect.Type](map[string]*ssa.Function)),
		chexit:       make(chan int),
		mainid:       goroutineID(),
	}
	i.record = loader.NewTypesRecord(i.ctx.Loader, i)
	i.record.Load(mainpkg)

	var pkgs []*ssa.Package
	for _, pkg := range mainpkg.Prog.AllPackages() {
		// skip external pkg
		if pkg.Func("init").Blocks == nil {
			continue
		}
		pkgs = append(pkgs, pkg)
		// Initialize global storage.
		for _, m := range pkg.Members {
			switch v := m.(type) {
			case *ssa.Global:
				typ := i.preToType(deref(v.Type()))
				i.globals[v] = reflect.New(typ).Interface()
			}
		}
	}
	if globals != nil {
		for k := range i.globals {
			if fv, ok := globals[k.String()]; ok {
				i.globals[k] = fv
			}
		}
	}

	// static types check
	err := checkPackages(i, pkgs)
	if err != nil {
		return i, err
	}
	return i, err
}

func (i *Interp) loadType(typ types.Type) {
	if _, ok := i.preloadTypes[typ]; !ok {
		i.preloadTypes[typ] = i.record.ToType(typ)
	}
}

func (i *Interp) preToType(typ types.Type) reflect.Type {
	if t, ok := i.preloadTypes[typ]; ok {
		return t
	}
	t := i.record.ToType(typ)
	i.preloadTypes[typ] = t
	return t
}

func (i *Interp) toType(typ types.Type) reflect.Type {
	if t, ok := i.preloadTypes[typ]; ok {
		return t
	}
	i.typesMutex.Lock()
	defer i.typesMutex.Unlock()
	return i.record.ToType(typ)
}

func (i *Interp) RunFunc(name string, args ...Value) (r Value, err error) {
	defer func() {
		if i.ctx.Mode&loader.DisableRecover != 0 {
			return
		}
		switch p := recover().(type) {
		case nil:
			// nothing
		case exitPanic:
			i.exitCode = int(p)
			atomic.StoreInt32(&i.exited, 1)
		case goexitPanic:
			// check goroutines
			if atomic.LoadInt32(&i.goroutines) == 1 {
				err = loader.ErrGoexitDeadlock
			} else {
				i.exitCode = <-i.chexit
				atomic.StoreInt32(&i.exited, 1)
			}
		case targetPanic:
			err = p
		case runtime.Error:
			err = p
		case string:
			err = plainError(p)
		case plainError:
			err = p
		default:
			err = fmt.Errorf("unexpected type: %T: %v", p, p)
		}
	}()

	if fn := i.mainpkg.Func(name); fn != nil {
		r = i.call(nil, fn, args, nil)
	} else {
		err = fmt.Errorf("no function %v", name)
	}

	return
}

func (i *Interp) ExitCode() int {
	return i.exitCode
}

func (i *Interp) RunInit() (err error) {
	i.goexited = 0
	i.exitCode = 0
	i.exited = 0
	_, err = i.RunFunc("init")
	return
}

func (i *Interp) RunMain() (exitCode int, err error) {
	if atomic.LoadInt32(&i.exited) == 1 {
		return i.exitCode, nil
	}
	_, err = i.RunFunc(loader.MainPkgPath)
	if err != nil {
		exitCode = 2
	}
	if atomic.LoadInt32(&i.exited) == 1 {
		exitCode = i.exitCode
	}
	return
}

func (i *Interp) RunAny(fn string, args ...Value) (r interface{}, err error) {
	if atomic.LoadInt32(&i.exited) == 1 {
		return i.exitCode, nil
	}
	r, err = i.RunFunc(fn, args...)
	return
}

func (i *Interp) GetFunc(key string) (interface{}, bool) {
	m, ok := i.mainpkg.Members[key]
	if !ok {
		return nil, false
	}
	fn, ok := m.(*ssa.Function)
	if !ok {
		return nil, false
	}
	return i.makeFunction(i.toType(fn.Type()), i.funcs[fn], nil).Interface(), true
}

func (i *Interp) GetVarAddr(key string) (interface{}, bool) {
	m, ok := i.mainpkg.Members[key]
	if !ok {
		return nil, false
	}
	v, ok := m.(*ssa.Global)
	if !ok {
		return nil, false
	}
	p, ok := i.globals[v]
	return p, ok
}

func (i *Interp) GetConst(key string) (constant.Value, bool) {
	m, ok := i.mainpkg.Members[key]
	if !ok {
		return nil, false
	}
	v, ok := m.(*ssa.NamedConst)
	if !ok {
		return nil, false
	}
	return v.Value.Value, true
}

func (i *Interp) GetType(key string) (reflect.Type, bool) {
	m, ok := i.mainpkg.Members[key]
	if !ok {
		return nil, false
	}
	t, ok := m.(*ssa.Type)
	if !ok {
		return nil, false
	}
	return i.toType(t.Type()), true
}

func (i *Interp) GetSymbol(key string) (m ssa.Member, v interface{}, ok bool) {
	ar := strings.Split(key, ".")
	var pkg *ssa.Package
	switch len(ar) {
	case 1:
		pkg = i.mainpkg
	case 2:
		pkgs := i.mainpkg.Prog.AllPackages()
		for _, p := range pkgs {
			if p.Pkg.Path() == ar[0] || p.Pkg.Name() == ar[0] {
				pkg = p
				break
			}
		}
		if pkg == nil {
			return
		}
		key = ar[1]
	default:
		return
	}
	m, ok = pkg.Members[key]
	if !ok {
		return
	}
	switch p := m.(type) {
	case *ssa.NamedConst:
		v = p.Value.Value
	case *ssa.Global:
		v, ok = i.globalToValue(p)
	case *ssa.Function:
		typ := i.toType(p.Type())
		v = i.makeFunction(typ, i.funcs[p], nil)
	case *ssa.Type:
		v = i.toType(p.Type())
	}
	return
}

func (i *Interp) Exit(code int) {
	if i != nil && atomic.LoadInt32(&i.goexited) == 1 {
		i.chexit <- code
	} else {
		panic(exitPanic(code))
	}
}

// constValue returns the value of the constant with the
// dynamic type tag appropriate for c.Type().
func (i *Interp) constToValue(c *ssa.Const) value {
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

func (i *Interp) globalToValue(key *ssa.Global) (interface{}, bool) {
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

// deref returns a pointer's element type; otherwise it returns typ.
// TODO(adonovan): Import from ssa?
func deref(typ types.Type) types.Type {
	if p, ok := typ.Underlying().(*types.Pointer); ok {
		return p.Elem()
	}
	return typ
}

func goroutineID() int64 {
	return goid.Get()
}
