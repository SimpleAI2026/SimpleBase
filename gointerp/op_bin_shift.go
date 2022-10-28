package interp

import (
	"reflect"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)

func makeBinOpSHL(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	xtyp := pfn.Interp.preToType(instr.X.Type())
	ytyp := pfn.Interp.preToType(instr.Y.Type())
	xkind := xtyp.Kind()
	ykind := ytyp.Kind()
	if kx == kindConst && ky == kindConst {
		t := xtype.TypeOfType(xtyp)
		switch xkind {
		case reflect.Int:
			x := xtype.Int(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Int8:
			x := xtype.Int8(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Int16:
			x := xtype.Int16(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Int32:
			x := xtype.Int32(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Int64:
			x := xtype.Int64(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Uint:
			x := xtype.Uint(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Uint8:
			x := xtype.Uint8(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Uint16:
			x := xtype.Uint16(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Uint32:
			x := xtype.Uint32(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Uint64:
			x := xtype.Uint64(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Uintptr:
			x := xtype.Uintptr(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		}
	}
	if xtyp.PkgPath() == "" {
		switch xkind {
		case reflect.Int:
			if kx == kindConst {
				x := vx.(int)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)<<vm.uintptr(iy)) }
				}
			}
		case reflect.Int8:
			if kx == kindConst {
				x := vx.(int8)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)<<vm.uintptr(iy)) }
				}
			}
		case reflect.Int16:
			if kx == kindConst {
				x := vx.(int16)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)<<vm.uintptr(iy)) }
				}
			}
		case reflect.Int32:
			if kx == kindConst {
				x := vx.(int32)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)<<vm.uintptr(iy)) }
				}
			}
		case reflect.Int64:
			if kx == kindConst {
				x := vx.(int64)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)<<vm.uintptr(iy)) }
				}
			}
		case reflect.Uint:
			if kx == kindConst {
				x := vx.(uint)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)<<vm.uintptr(iy)) }
				}
			}
		case reflect.Uint8:
			if kx == kindConst {
				x := vx.(uint8)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)<<vm.uintptr(iy)) }
				}
			}
		case reflect.Uint16:
			if kx == kindConst {
				x := vx.(uint16)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)<<vm.uintptr(iy)) }
				}
			}
		case reflect.Uint32:
			if kx == kindConst {
				x := vx.(uint32)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)<<vm.uintptr(iy)) }
				}
			}
		case reflect.Uint64:
			if kx == kindConst {
				x := vx.(uint64)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)<<vm.uintptr(iy)) }
				}
			}
		case reflect.Uintptr:
			if kx == kindConst {
				x := vx.(uintptr)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x<<vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)<<vm.uintptr(iy)) }
				}
			}
		}
	} else {
		t := xtype.TypeOfType(xtyp)
		switch xkind {
		case reflect.Int:
			if kx == kindConst {
				x := xtype.Int(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)<<vm.uintptr(iy))) }
				}
			}
		case reflect.Int8:
			if kx == kindConst {
				x := xtype.Int8(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)<<vm.uintptr(iy))) }
				}
			}
		case reflect.Int16:
			if kx == kindConst {
				x := xtype.Int16(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)<<vm.uintptr(iy))) }
				}
			}
		case reflect.Int32:
			if kx == kindConst {
				x := xtype.Int32(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)<<vm.uintptr(iy))) }
				}
			}
		case reflect.Int64:
			if kx == kindConst {
				x := xtype.Int64(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)<<vm.uintptr(iy))) }
				}
			}
		case reflect.Uint:
			if kx == kindConst {
				x := xtype.Uint(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)<<vm.uintptr(iy))) }
				}
			}
		case reflect.Uint8:
			if kx == kindConst {
				x := xtype.Uint8(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)<<vm.uintptr(iy))) }
				}
			}
		case reflect.Uint16:
			if kx == kindConst {
				x := xtype.Uint16(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)<<vm.uintptr(iy))) }
				}
			}
		case reflect.Uint32:
			if kx == kindConst {
				x := xtype.Uint32(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)<<vm.uintptr(iy))) }
				}
			}
		case reflect.Uint64:
			if kx == kindConst {
				x := xtype.Uint64(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)<<vm.uintptr(iy))) }
				}
			}
		case reflect.Uintptr:
			if kx == kindConst {
				x := xtype.Uintptr(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x<<vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)<<vm.uintptr(iy))) }
				}
			}
		}
	}
	panic("unreachable")
}

func makeBinOpSHR(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	xtyp := pfn.Interp.preToType(instr.X.Type())
	ytyp := pfn.Interp.preToType(instr.Y.Type())
	xkind := xtyp.Kind()
	ykind := ytyp.Kind()
	if kx == kindConst && ky == kindConst {
		t := xtype.TypeOfType(xtyp)
		switch xkind {
		case reflect.Int:
			x := xtype.Int(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Int8:
			x := xtype.Int8(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Int16:
			x := xtype.Int16(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Int32:
			x := xtype.Int32(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Int64:
			x := xtype.Int64(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Uint:
			x := xtype.Uint(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Uint8:
			x := xtype.Uint8(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Uint16:
			x := xtype.Uint16(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Uint32:
			x := xtype.Uint32(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Uint64:
			x := xtype.Uint64(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		case reflect.Uintptr:
			x := xtype.Uintptr(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(vm *goVm) { vm.setReg(ir, v) }
			}
		}
	}
	if xtyp.PkgPath() == "" {
		switch xkind {
		case reflect.Int:
			if kx == kindConst {
				x := vx.(int)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int)>>vm.uintptr(iy)) }
				}
			}
		case reflect.Int8:
			if kx == kindConst {
				x := vx.(int8)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8)>>vm.uintptr(iy)) }
				}
			}
		case reflect.Int16:
			if kx == kindConst {
				x := vx.(int16)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16)>>vm.uintptr(iy)) }
				}
			}
		case reflect.Int32:
			if kx == kindConst {
				x := vx.(int32)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32)>>vm.uintptr(iy)) }
				}
			}
		case reflect.Int64:
			if kx == kindConst {
				x := vx.(int64)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64)>>vm.uintptr(iy)) }
				}
			}
		case reflect.Uint:
			if kx == kindConst {
				x := vx.(uint)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint)>>vm.uintptr(iy)) }
				}
			}
		case reflect.Uint8:
			if kx == kindConst {
				x := vx.(uint8)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8)>>vm.uintptr(iy)) }
				}
			}
		case reflect.Uint16:
			if kx == kindConst {
				x := vx.(uint16)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16)>>vm.uintptr(iy)) }
				}
			}
		case reflect.Uint32:
			if kx == kindConst {
				x := vx.(uint32)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32)>>vm.uintptr(iy)) }
				}
			}
		case reflect.Uint64:
			if kx == kindConst {
				x := vx.(uint64)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64)>>vm.uintptr(iy)) }
				}
			}
		case reflect.Uintptr:
			if kx == kindConst {
				x := vx.(uintptr)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, x>>vm.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>vm.int(iy)) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>vm.int8(iy)) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>vm.int16(iy)) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>vm.int32(iy)) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>vm.int64(iy)) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>vm.uint(iy)) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>vm.uint8(iy)) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>vm.uint16(iy)) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>vm.uint32(iy)) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>vm.uint64(iy)) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr)>>vm.uintptr(iy)) }
				}
			}
		}
	} else {
		t := xtype.TypeOfType(xtyp)
		switch xkind {
		case reflect.Int:
			if kx == kindConst {
				x := xtype.Int(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix)>>vm.uintptr(iy))) }
				}
			}
		case reflect.Int8:
			if kx == kindConst {
				x := xtype.Int8(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix)>>vm.uintptr(iy))) }
				}
			}
		case reflect.Int16:
			if kx == kindConst {
				x := xtype.Int16(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix)>>vm.uintptr(iy))) }
				}
			}
		case reflect.Int32:
			if kx == kindConst {
				x := xtype.Int32(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix)>>vm.uintptr(iy))) }
				}
			}
		case reflect.Int64:
			if kx == kindConst {
				x := xtype.Int64(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix)>>vm.uintptr(iy))) }
				}
			}
		case reflect.Uint:
			if kx == kindConst {
				x := xtype.Uint(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix)>>vm.uintptr(iy))) }
				}
			}
		case reflect.Uint8:
			if kx == kindConst {
				x := xtype.Uint8(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix)>>vm.uintptr(iy))) }
				}
			}
		case reflect.Uint16:
			if kx == kindConst {
				x := xtype.Uint16(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix)>>vm.uintptr(iy))) }
				}
			}
		case reflect.Uint32:
			if kx == kindConst {
				x := xtype.Uint32(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix)>>vm.uintptr(iy))) }
				}
			}
		case reflect.Uint64:
			if kx == kindConst {
				x := xtype.Uint64(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix)>>vm.uintptr(iy))) }
				}
			}
		case reflect.Uintptr:
			if kx == kindConst {
				x := xtype.Uintptr(vx)
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x>>vm.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>vm.int(iy))) }
				case reflect.Int8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>vm.int8(iy))) }
				case reflect.Int16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>vm.int16(iy))) }
				case reflect.Int32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>vm.int32(iy))) }
				case reflect.Int64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>vm.int64(iy))) }
				case reflect.Uint:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>vm.uint(iy))) }
				case reflect.Uint8:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>vm.uint8(iy))) }
				case reflect.Uint16:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>vm.uint16(iy))) }
				case reflect.Uint32:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>vm.uint32(iy))) }
				case reflect.Uint64:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>vm.uint64(iy))) }
				case reflect.Uintptr:
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix)>>vm.uintptr(iy))) }
				}
			}
		}
	}
	panic("unreachable")
}
