package goscript

import (
	"reflect"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)

func makeBinOpEQL(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	xtyp := pfn.Interp.preToType(instr.X.Type())
	ytyp := pfn.Interp.preToType(instr.Y.Type())
	if xtyp == nil && ytyp == nil {
		return func(vm *goVm) {
			vm.setReg(ir, true)
		}
	}
	switch xtyp.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128,
		reflect.String:
		if kx == kindConst && ky == kindConst {
			v := vx == vy
			return func(vm *goVm) {
				vm.setReg(ir, v)
			}
		} else if kx == kindConst {
			return func(vm *goVm) {
				vm.setReg(ir, vx == vm.reg(iy))
			}
		} else if ky == kindConst {
			return func(vm *goVm) {
				vm.setReg(ir, vm.reg(ix) == vy)
			}
		}
		return func(vm *goVm) {
			vm.setReg(ir, vm.reg(ix) == vm.reg(iy))
		}
	case reflect.Interface:
		return func(vm *goVm) {
			vm.setReg(ir, vm.reg(ix) == vm.reg(iy))
		}
	case reflect.Array:
		if xtyp == ytyp {
			return func(vm *goVm) {
				vm.setReg(ir, vm.reg(ix) == vm.reg(iy))
			}
		}
		return func(vm *goVm) {
			x := vm.reg(ix)
			y := vm.reg(iy)
			vm.setReg(ir, x == reflect.ValueOf(y).Convert(xtyp).Interface())
		}
	case reflect.Struct:
		return func(vm *goVm) {
			vm.setReg(ir, vm.reg(ix) == vm.reg(iy))
		}
	case reflect.UnsafePointer:
		return func(vm *goVm) {
			vm.setReg(ir, vm.reg(ix) == vm.reg(iy))
		}
	case reflect.Chan:
		xdir := xtyp.ChanDir()
		ydir := ytyp.ChanDir()
		if xdir != ydir {
			if xdir == reflect.BothDir {
				return func(vm *goVm) {
					x := vm.reg(ix)
					x = reflect.ValueOf(x).Convert(ytyp).Interface()
					vm.setReg(ir, x == vm.reg(iy))
				}
			} else if ydir == reflect.BothDir {
				return func(vm *goVm) {
					y := vm.reg(iy)
					y = reflect.ValueOf(y).Convert(xtyp).Interface()
					vm.setReg(ir, vm.reg(ix) == y)
				}
			}
		}
		return func(vm *goVm) {
			vm.setReg(ir, vm.reg(ix) == vm.reg(iy))
		}
	case reflect.Ptr:
		return func(vm *goVm) {
			x := vm.reg(ix)
			y := vm.reg(iy)
			vm.setReg(ir, reflect.ValueOf(x).Pointer() == reflect.ValueOf(y).Pointer())
		}
	case reflect.Slice, reflect.Map, reflect.Func:
		return func(vm *goVm) {
			x := vm.reg(ix)
			y := vm.reg(iy)
			b := reflect.ValueOf(x).IsNil() && reflect.ValueOf(y).IsNil()
			vm.setReg(ir, b)
		}
	default:
		panic("unreachable")
	}
}

func makeBinOpNEQ(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	xtyp := pfn.Interp.preToType(instr.X.Type())
	ytyp := pfn.Interp.preToType(instr.Y.Type())
	if xtyp == nil && ytyp == nil {
		return func(vm *goVm) {
			vm.setReg(ir, false)
		}
	}
	switch xtyp.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128,
		reflect.String:
		if kx == kindConst && ky == kindConst {
			v := vx != vy
			return func(vm *goVm) {
				vm.setReg(ir, v)
			}
		} else if kx == kindConst {
			return func(vm *goVm) {
				vm.setReg(ir, vx != vm.reg(iy))
			}
		} else if ky == kindConst {
			return func(vm *goVm) {
				vm.setReg(ir, vm.reg(ix) != vy)
			}
		}
		return func(vm *goVm) {
			vm.setReg(ir, vm.reg(ix) != vm.reg(iy))
		}
	case reflect.Interface:
		return func(vm *goVm) {
			vm.setReg(ir, vm.reg(ix) != vm.reg(iy))
		}
	case reflect.Array:
		if xtyp == ytyp {
			return func(vm *goVm) {
				vm.setReg(ir, vm.reg(ix) != vm.reg(iy))
			}
		}
		return func(vm *goVm) {
			x := vm.reg(ix)
			y := vm.reg(iy)
			vm.setReg(ir, x != reflect.ValueOf(y).Convert(xtyp).Interface())
		}
	case reflect.Struct:
		return func(vm *goVm) {
			vm.setReg(ir, vm.reg(ix) != vm.reg(iy))
		}
	case reflect.UnsafePointer:
		return func(vm *goVm) {
			vm.setReg(ir, vm.reg(ix) != vm.reg(iy))
		}
	case reflect.Chan:
		xdir := xtyp.ChanDir()
		ydir := ytyp.ChanDir()
		if xdir != ydir {
			if xdir == reflect.BothDir {
				return func(vm *goVm) {
					x := vm.reg(ix)
					x = reflect.ValueOf(x).Convert(ytyp).Interface()
					vm.setReg(ir, x != vm.reg(iy))
				}
			} else if ydir == reflect.BothDir {
				return func(vm *goVm) {
					y := vm.reg(iy)
					y = reflect.ValueOf(y).Convert(xtyp).Interface()
					vm.setReg(ir, vm.reg(ix) != y)
				}
			}
		}
		return func(vm *goVm) {
			vm.setReg(ir, vm.reg(ix) != vm.reg(iy))
		}
	case reflect.Ptr:
		return func(vm *goVm) {
			x := vm.reg(ix)
			y := vm.reg(iy)
			vm.setReg(ir, reflect.ValueOf(x).Pointer() != reflect.ValueOf(y).Pointer())
		}
	case reflect.Slice, reflect.Map, reflect.Func:
		return func(vm *goVm) {
			x := vm.reg(ix)
			y := vm.reg(iy)
			b := reflect.ValueOf(x).IsNil() && reflect.ValueOf(y).IsNil()
			vm.setReg(ir, !b)
		}
	default:
		panic("unreachable")
	}
}

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

func cvtComplex64(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = complex64
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			case reflect.Complex64:
				v = T(vm.reg(ix).(complex64))
			case reflect.Complex128:
				v = T(vm.reg(ix).(complex128))
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(vm *goVm) {
			var v T
			switch xkind {
			case reflect.Complex64:
				v = T(vm.complex64(ix))
			case reflect.Complex128:
				v = T(vm.complex128(ix))
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func cvtComplex128(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = complex128
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			case reflect.Complex64:
				v = T(vm.reg(ix).(complex64))
			case reflect.Complex128:
				v = T(vm.reg(ix).(complex128))
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(vm *goVm) {
			var v T
			switch xkind {
			case reflect.Complex64:
				v = T(vm.complex64(ix))
			case reflect.Complex128:
				v = T(vm.complex128(ix))
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func makeConvertInstr(pfn *function, interp *Interp, instr *ssa.Convert) func(vm *goVm) {
	typ := interp.preToType(instr.Type())
	xtyp := interp.preToType(instr.X.Type())
	kind := typ.Kind()
	xkind := xtyp.Kind()
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	switch kind {
	case reflect.UnsafePointer:
		if xkind == reflect.Uintptr {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				v := vm.uintptr(ix)
				vm.setReg(ir, xtype.ConvertPtr(t, toUnsafePointer(v)))
			}
		} else if xkind == reflect.Ptr {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				v := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertPtr(t, v))
			}
		}
	case reflect.Uintptr:
		if xkind == reflect.UnsafePointer {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				v := vm.pointer(ix)
				vm.setReg(ir, xtype.MakeUintptr(t, uintptr(v)))
			}
		}
	case reflect.Ptr:
		if xkind == reflect.UnsafePointer {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				v := vm.reg(ix)
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	case reflect.Slice:
		if xkind == reflect.String {
			t := xtype.TypeOfType(typ)
			elem := typ.Elem()
			switch elem.Kind() {
			case reflect.Uint8:
				return func(vm *goVm) {
					v := vm.string(ix)
					vm.setReg(ir, xtype.Make(t, []byte(v)))
				}
			case reflect.Int32:
				return func(vm *goVm) {
					v := vm.string(ix)
					vm.setReg(ir, xtype.Make(t, []rune(v)))
				}
			}
		}
	case reflect.String:
		if xkind == reflect.Slice {
			t := xtype.TypeOfType(typ)
			elem := xtyp.Elem()
			switch elem.Kind() {
			case reflect.Uint8:
				return func(vm *goVm) {
					v := vm.bytes(ix)
					vm.setReg(ir, xtype.Make(t, string(v)))
				}
			case reflect.Int32:
				return func(vm *goVm) {
					v := vm.runes(ix)
					vm.setReg(ir, xtype.Make(t, string(v)))
				}
			}
		}
	}
	if kx.isStatic() {
		v := reflect.ValueOf(vx)
		return func(vm *goVm) {
			vm.setReg(ir, v.Convert(typ).Interface())
		}
	}
	switch kind {
	case reflect.Int:
		return cvtInt(ir, ix, xkind, xtyp, typ)
	case reflect.Int8:
		return cvtInt8(ir, ix, xkind, xtyp, typ)
	case reflect.Int16:
		return cvtInt16(ir, ix, xkind, xtyp, typ)
	case reflect.Int32:
		return cvtInt32(ir, ix, xkind, xtyp, typ)
	case reflect.Int64:
		return cvtInt64(ir, ix, xkind, xtyp, typ)
	case reflect.Uint:
		return cvtUint(ir, ix, xkind, xtyp, typ)
	case reflect.Uint8:
		return cvtUint8(ir, ix, xkind, xtyp, typ)
	case reflect.Uint16:
		return cvtUint16(ir, ix, xkind, xtyp, typ)
	case reflect.Uint32:
		return cvtUint32(ir, ix, xkind, xtyp, typ)
	case reflect.Uint64:
		return cvtUint64(ir, ix, xkind, xtyp, typ)
	case reflect.Uintptr:
		return cvtUintptr(ir, ix, xkind, xtyp, typ)
	case reflect.Float32:
		return cvtFloat32(ir, ix, xkind, xtyp, typ)
	case reflect.Float64:
		return cvtFloat64(ir, ix, xkind, xtyp, typ)
	case reflect.Complex64:
		return cvtComplex64(ir, ix, xkind, xtyp, typ)
	case reflect.Complex128:
		return cvtComplex128(ir, ix, xkind, xtyp, typ)
	}
	return func(vm *goVm) {
		v := reflect.ValueOf(vm.reg(ix))
		vm.setReg(ir, v.Convert(typ).Interface())
	}
}
