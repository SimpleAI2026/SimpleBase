package interp

import (
	"reflect"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)

func makeTypeChangeInstr(pfn *function, instr *ssa.ChangeType) func(vm *goVm) {
	typ := pfn.Interp.preToType(instr.Type())
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	if kx.isStatic() {
		var v interface{}
		if vx == nil {
			v = reflect.New(typ).Elem().Interface()
		} else {
			v = reflect.ValueOf(vx).Convert(typ).Interface()
		}
		return func(vm *goVm) {
			vm.setReg(ir, v)
		}
	}
	kind := typ.Kind()
	switch kind {
	case reflect.Ptr, reflect.Chan, reflect.Map, reflect.Func, reflect.Slice:
		t := xtype.TypeOfType(typ)
		return func(vm *goVm) {
			x := vm.reg(ix)
			vm.setReg(ir, xtype.ConvertPtr(t, x))
		}
	case reflect.Struct, reflect.Array:
		t := xtype.TypeOfType(typ)
		return func(vm *goVm) {
			x := vm.reg(ix)
			vm.setReg(ir, xtype.ConvertDirect(t, x))
		}
	case reflect.Interface:
		return func(vm *goVm) {
			x := vm.reg(ix)
			if x == nil {
				vm.setReg(ir, reflect.New(typ).Elem().Interface())
			} else {
				vm.setReg(ir, reflect.ValueOf(x).Convert(typ).Interface())
			}
		}
	case reflect.Bool:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Bool(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertBool(t, x))
			}
		}
	case reflect.Int:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Int(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertInt(t, x))
			}
		}
	case reflect.Int8:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Int8(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertInt8(t, x))
			}
		}
	case reflect.Int16:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Int16(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertInt16(t, x))
			}
		}
	case reflect.Int32:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Int32(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertInt32(t, x))
			}
		}
	case reflect.Int64:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Int64(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertInt64(t, x))
			}
		}
	case reflect.Uint:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Uint(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertUint(t, x))
			}
		}
	case reflect.Uint8:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Uint8(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertUint8(t, x))
			}
		}
	case reflect.Uint16:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Uint16(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertUint16(t, x))
			}
		}
	case reflect.Uint32:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Uint32(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertUint32(t, x))
			}
		}
	case reflect.Uint64:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Uint64(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertUint64(t, x))
			}
		}
	case reflect.Uintptr:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Uintptr(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertUintptr(t, x))
			}
		}
	case reflect.Float32:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Float32(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertFloat32(t, x))
			}
		}
	case reflect.Float64:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Float64(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertFloat64(t, x))
			}
		}
	case reflect.Complex64:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Complex64(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertComplex64(t, x))
			}
		}
	case reflect.Complex128:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.Complex128(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertComplex128(t, x))
			}
		}
	case reflect.String:
		if typ.PkgPath() == "" {
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.String(x))
			}
		} else {
			t := xtype.TypeOfType(typ)
			return func(vm *goVm) {
				x := vm.reg(ix)
				vm.setReg(ir, xtype.ConvertString(t, x))
			}
		}
	case reflect.UnsafePointer:
		t := xtype.TypeOfType(typ)
		return func(vm *goVm) {
			x := vm.reg(ix)
			vm.setReg(ir, xtype.ConvertPtr(t, x))
		}
	}
	panic("unreachable")
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
