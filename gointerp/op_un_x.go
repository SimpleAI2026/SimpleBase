package interp

import (
	"reflect"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)

func makeUnOpNOT(pfn *function, instr *ssa.UnOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	typ := pfn.Interp.preToType(instr.Type())
	if typ.PkgPath() == "" {
		if kx == kindConst {
			v := !vx.(bool)
			return func(fr *frame) {
				fr.setReg(ir, v)
			}
		}
		return func(fr *frame) {
			v := !fr.reg(ix).(bool)
			fr.setReg(ir, v)
		}
	} else {
		if kx == kindConst {
			v := xtype.Not(vx)
			return func(fr *frame) {
				fr.setReg(ir, v)
			}
		}
		return func(fr *frame) {
			v := xtype.Not(fr.reg(ix))
			fr.setReg(ir, v)
		}
	}
}

func makeUnOpMUL(pfn *function, instr *ssa.UnOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	if kx == kindGlobal {
		v := reflect.ValueOf(vx)
		return func(fr *frame) {
			elem := v.Elem()
			if !elem.IsValid() {
				panic(runtimeError("invalid memory address or nil pointer dereference"))
			}
			fr.setReg(ir, elem.Interface())
		}
	}
	return func(fr *frame) {
		elem := reflect.ValueOf(fr.reg(ix)).Elem()
		if !elem.IsValid() {
			panic(runtimeError("invalid memory address or nil pointer dereference"))
		}
		fr.setReg(ir, elem.Interface())
	}
}

func makeUnOpARROW(pfn *function, instr *ssa.UnOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	typ := pfn.Interp.preToType(instr.X.Type()).Elem()
	if kx == kindGlobal {
		x := reflect.ValueOf(vx)
		if instr.CommaOk {
			return func(fr *frame) {
				v, ok := x.Recv()
				if !ok {
					v = reflect.New(typ).Elem()
				}
				fr.setReg(ir, tuple{v.Interface(), ok})
			}
		}
		return func(fr *frame) {
			v, ok := x.Recv()
			if !ok {
				v = reflect.New(typ).Elem()
			}
			fr.setReg(ir, v.Interface())
		}
	}
	if instr.CommaOk {
		return func(fr *frame) {
			x := reflect.ValueOf(fr.reg(ix))
			v, ok := x.Recv()
			if !ok {
				v = reflect.New(typ).Elem()
			}
			fr.setReg(ir, tuple{v.Interface(), ok})
		}
	}
	return func(fr *frame) {
		x := reflect.ValueOf(fr.reg(ix))
		v, ok := x.Recv()
		if !ok {
			v = reflect.New(typ).Elem()
		}
		fr.setReg(ir, v.Interface())
	}
}
