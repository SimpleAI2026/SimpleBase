package gointerp

import (
	"reflect"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)

func makeUnOpNOT(pfn *function, instr *ssa.UnOp) func(vm *goVm) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	typ := pfn.Interp.preToType(instr.Type())
	if typ.PkgPath() == "" {
		if kx == kindConst {
			v := !vx.(bool)
			return func(vm *goVm) {
				vm.setReg(ir, v)
			}
		}
		return func(vm *goVm) {
			v := !vm.reg(ix).(bool)
			vm.setReg(ir, v)
		}
	} else {
		if kx == kindConst {
			v := xtype.Not(vx)
			return func(vm *goVm) {
				vm.setReg(ir, v)
			}
		}
		return func(vm *goVm) {
			v := xtype.Not(vm.reg(ix))
			vm.setReg(ir, v)
		}
	}
}

func makeUnOpMUL(pfn *function, instr *ssa.UnOp) func(vm *goVm) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	if kx == kindGlobal {
		v := reflect.ValueOf(vx)
		return func(vm *goVm) {
			elem := v.Elem()
			if !elem.IsValid() {
				panic(runtimeError("invalid memory address or nil pointer dereference"))
			}
			vm.setReg(ir, elem.Interface())
		}
	}
	return func(vm *goVm) {
		elem := reflect.ValueOf(vm.reg(ix)).Elem()
		if !elem.IsValid() {
			panic(runtimeError("invalid memory address or nil pointer dereference"))
		}
		vm.setReg(ir, elem.Interface())
	}
}

func makeUnOpARROW(pfn *function, instr *ssa.UnOp) func(vm *goVm) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	typ := pfn.Interp.preToType(instr.X.Type()).Elem()
	if kx == kindGlobal {
		x := reflect.ValueOf(vx)
		if instr.CommaOk {
			return func(vm *goVm) {
				v, ok := x.Recv()
				if !ok {
					v = reflect.New(typ).Elem()
				}
				vm.setReg(ir, tuple{v.Interface(), ok})
			}
		}
		return func(vm *goVm) {
			v, ok := x.Recv()
			if !ok {
				v = reflect.New(typ).Elem()
			}
			vm.setReg(ir, v.Interface())
		}
	}
	if instr.CommaOk {
		return func(vm *goVm) {
			x := reflect.ValueOf(vm.reg(ix))
			v, ok := x.Recv()
			if !ok {
				v = reflect.New(typ).Elem()
			}
			vm.setReg(ir, tuple{v.Interface(), ok})
		}
	}
	return func(vm *goVm) {
		x := reflect.ValueOf(vm.reg(ix))
		v, ok := x.Recv()
		if !ok {
			v = reflect.New(typ).Elem()
		}
		vm.setReg(ir, v.Interface())
	}
}
