package interp

import (
	"reflect"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)

func makeUnOpSUB(pfn *function, instr *ssa.UnOp) func(vm *goVm) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	typ := pfn.Interp.preToType(instr.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst {
				v := -vx.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(int)
					vm.setReg(ir, v)
				}
			}
		case reflect.Int8:
			if kx == kindConst {
				v := -vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(int8)
					vm.setReg(ir, v)
				}
			}
		case reflect.Int16:
			if kx == kindConst {
				v := -vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(int16)
					vm.setReg(ir, v)
				}
			}
		case reflect.Int32:
			if kx == kindConst {
				v := -vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(int32)
					vm.setReg(ir, v)
				}
			}
		case reflect.Int64:
			if kx == kindConst {
				v := -vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(int64)
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint:
			if kx == kindConst {
				v := -vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(uint)
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint8:
			if kx == kindConst {
				v := -vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(uint8)
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint16:
			if kx == kindConst {
				v := -vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(uint16)
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint32:
			if kx == kindConst {
				v := -vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(uint32)
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint64:
			if kx == kindConst {
				v := -vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(uint64)
					vm.setReg(ir, v)
				}
			}
		case reflect.Uintptr:
			if kx == kindConst {
				v := -vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(uintptr)
					vm.setReg(ir, v)
				}
			}
		case reflect.Float32:
			if kx == kindConst {
				v := -vx.(float32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(float32)
					vm.setReg(ir, v)
				}
			}
		case reflect.Float64:
			if kx == kindConst {
				v := -vx.(float64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(float64)
					vm.setReg(ir, v)
				}
			}
		case reflect.Complex64:
			if kx == kindConst {
				v := -vx.(complex64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(complex64)
					vm.setReg(ir, v)
				}
			}
		case reflect.Complex128:
			if kx == kindConst {
				v := -vx.(complex128)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := -vm.reg(ix).(complex128)
					vm.setReg(ir, v)
				}
			}
		}
	} else {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst {
				v := xtype.NegInt(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegInt(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Int8:
			if kx == kindConst {
				v := xtype.NegInt8(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegInt8(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Int16:
			if kx == kindConst {
				v := xtype.NegInt16(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegInt16(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Int32:
			if kx == kindConst {
				v := xtype.NegInt32(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegInt32(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Int64:
			if kx == kindConst {
				v := xtype.NegInt64(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegInt64(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint:
			if kx == kindConst {
				v := xtype.NegUint(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegUint(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint8:
			if kx == kindConst {
				v := xtype.NegUint8(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegUint8(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint16:
			if kx == kindConst {
				v := xtype.NegUint16(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegUint16(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint32:
			if kx == kindConst {
				v := xtype.NegUint32(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegUint32(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint64:
			if kx == kindConst {
				v := xtype.NegUint64(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegUint64(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Uintptr:
			if kx == kindConst {
				v := xtype.NegUintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegUintptr(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Float32:
			if kx == kindConst {
				v := xtype.NegFloat32(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegFloat32(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Float64:
			if kx == kindConst {
				v := xtype.NegFloat64(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegFloat64(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Complex64:
			if kx == kindConst {
				v := xtype.NegComplex64(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegComplex64(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Complex128:
			if kx == kindConst {
				v := xtype.NegComplex128(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.NegComplex128(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		}
	}
	panic("unreachable")
}
func makeUnOpXOR(pfn *function, instr *ssa.UnOp) func(vm *goVm) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	typ := pfn.Interp.preToType(instr.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst {
				v := ^vx.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := ^vm.reg(ix).(int)
					vm.setReg(ir, v)
				}
			}
		case reflect.Int8:
			if kx == kindConst {
				v := ^vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := ^vm.reg(ix).(int8)
					vm.setReg(ir, v)
				}
			}
		case reflect.Int16:
			if kx == kindConst {
				v := ^vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := ^vm.reg(ix).(int16)
					vm.setReg(ir, v)
				}
			}
		case reflect.Int32:
			if kx == kindConst {
				v := ^vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := ^vm.reg(ix).(int32)
					vm.setReg(ir, v)
				}
			}
		case reflect.Int64:
			if kx == kindConst {
				v := ^vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := ^vm.reg(ix).(int64)
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint:
			if kx == kindConst {
				v := ^vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := ^vm.reg(ix).(uint)
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint8:
			if kx == kindConst {
				v := ^vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := ^vm.reg(ix).(uint8)
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint16:
			if kx == kindConst {
				v := ^vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := ^vm.reg(ix).(uint16)
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint32:
			if kx == kindConst {
				v := ^vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := ^vm.reg(ix).(uint32)
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint64:
			if kx == kindConst {
				v := ^vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := ^vm.reg(ix).(uint64)
					vm.setReg(ir, v)
				}
			}
		case reflect.Uintptr:
			if kx == kindConst {
				v := ^vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := ^vm.reg(ix).(uintptr)
					vm.setReg(ir, v)
				}
			}
		}
	} else {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst {
				v := xtype.XorInt(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.XorInt(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Int8:
			if kx == kindConst {
				v := xtype.XorInt8(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.XorInt8(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Int16:
			if kx == kindConst {
				v := xtype.XorInt16(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.XorInt16(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Int32:
			if kx == kindConst {
				v := xtype.XorInt32(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.XorInt32(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Int64:
			if kx == kindConst {
				v := xtype.XorInt64(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.XorInt64(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint:
			if kx == kindConst {
				v := xtype.XorUint(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.XorUint(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint8:
			if kx == kindConst {
				v := xtype.XorUint8(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.XorUint8(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint16:
			if kx == kindConst {
				v := xtype.XorUint16(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.XorUint16(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint32:
			if kx == kindConst {
				v := xtype.XorUint32(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.XorUint32(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Uint64:
			if kx == kindConst {
				v := xtype.XorUint64(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.XorUint64(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		case reflect.Uintptr:
			if kx == kindConst {
				v := xtype.XorUintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else {
				return func(vm *goVm) {
					v := xtype.XorUintptr(vm.reg(ix))
					vm.setReg(ir, v)
				}
			}
		}
	}
	panic("unreachable")
}
