package interp

import (
	"reflect"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)

func makeBinOpSHL(pfn *function, instr *ssa.BinOp) func(fr *frame) {
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
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Int8:
			x := xtype.Int8(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Int16:
			x := xtype.Int16(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Int32:
			x := xtype.Int32(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Int64:
			x := xtype.Int64(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Uint:
			x := xtype.Uint(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Uint8:
			x := xtype.Uint8(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Uint16:
			x := xtype.Uint16(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Uint32:
			x := xtype.Uint32(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Uint64:
			x := xtype.Uint64(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Uintptr:
			x := xtype.Uintptr(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x<<xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x<<xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x<<xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x<<xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x<<xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x<<xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x<<xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x<<xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x<<xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x<<xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x<<xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
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
					return func(fr *frame) { fr.setReg(ir, x<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)<<fr.uintptr(iy)) }
				}
			}
		case reflect.Int8:
			if kx == kindConst {
				x := vx.(int8)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)<<fr.uintptr(iy)) }
				}
			}
		case reflect.Int16:
			if kx == kindConst {
				x := vx.(int16)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)<<fr.uintptr(iy)) }
				}
			}
		case reflect.Int32:
			if kx == kindConst {
				x := vx.(int32)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)<<fr.uintptr(iy)) }
				}
			}
		case reflect.Int64:
			if kx == kindConst {
				x := vx.(int64)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)<<fr.uintptr(iy)) }
				}
			}
		case reflect.Uint:
			if kx == kindConst {
				x := vx.(uint)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)<<fr.uintptr(iy)) }
				}
			}
		case reflect.Uint8:
			if kx == kindConst {
				x := vx.(uint8)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)<<fr.uintptr(iy)) }
				}
			}
		case reflect.Uint16:
			if kx == kindConst {
				x := vx.(uint16)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)<<fr.uintptr(iy)) }
				}
			}
		case reflect.Uint32:
			if kx == kindConst {
				x := vx.(uint32)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)<<fr.uintptr(iy)) }
				}
			}
		case reflect.Uint64:
			if kx == kindConst {
				x := vx.(uint64)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)<<fr.uintptr(iy)) }
				}
			}
		case reflect.Uintptr:
			if kx == kindConst {
				x := vx.(uintptr)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x<<fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)<<fr.uintptr(iy)) }
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
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)<<fr.uintptr(iy))) }
				}
			}
		case reflect.Int8:
			if kx == kindConst {
				x := xtype.Int8(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)<<fr.uintptr(iy))) }
				}
			}
		case reflect.Int16:
			if kx == kindConst {
				x := xtype.Int16(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)<<fr.uintptr(iy))) }
				}
			}
		case reflect.Int32:
			if kx == kindConst {
				x := xtype.Int32(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)<<fr.uintptr(iy))) }
				}
			}
		case reflect.Int64:
			if kx == kindConst {
				x := xtype.Int64(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)<<fr.uintptr(iy))) }
				}
			}
		case reflect.Uint:
			if kx == kindConst {
				x := xtype.Uint(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)<<fr.uintptr(iy))) }
				}
			}
		case reflect.Uint8:
			if kx == kindConst {
				x := xtype.Uint8(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)<<fr.uintptr(iy))) }
				}
			}
		case reflect.Uint16:
			if kx == kindConst {
				x := xtype.Uint16(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)<<fr.uintptr(iy))) }
				}
			}
		case reflect.Uint32:
			if kx == kindConst {
				x := xtype.Uint32(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)<<fr.uintptr(iy))) }
				}
			}
		case reflect.Uint64:
			if kx == kindConst {
				x := xtype.Uint64(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)<<fr.uintptr(iy))) }
				}
			}
		case reflect.Uintptr:
			if kx == kindConst {
				x := xtype.Uintptr(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x<<fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)<<fr.uintptr(iy))) }
				}
			}
		}
	}
	panic("unreachable")
}

func makeBinOpSHR(pfn *function, instr *ssa.BinOp) func(fr *frame) {
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
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Int8:
			x := xtype.Int8(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Int16:
			x := xtype.Int16(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Int32:
			x := xtype.Int32(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Int64:
			x := xtype.Int64(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Uint:
			x := xtype.Uint(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Uint8:
			x := xtype.Uint8(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Uint16:
			x := xtype.Uint16(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Uint32:
			x := xtype.Uint32(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Uint64:
			x := xtype.Uint64(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			}
		case reflect.Uintptr:
			x := xtype.Uintptr(vx)
			switch ykind {
			case reflect.Int:
				v := xtype.Make(t, x>>xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int8:
				v := xtype.Make(t, x>>xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int16:
				v := xtype.Make(t, x>>xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int32:
				v := xtype.Make(t, x>>xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Int64:
				v := xtype.Make(t, x>>xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint:
				v := xtype.Make(t, x>>xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint8:
				v := xtype.Make(t, x>>xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint16:
				v := xtype.Make(t, x>>xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint32:
				v := xtype.Make(t, x>>xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uint64:
				v := xtype.Make(t, x>>xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			case reflect.Uintptr:
				v := xtype.Make(t, x>>xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
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
					return func(fr *frame) { fr.setReg(ir, x>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)>>fr.uintptr(iy)) }
				}
			}
		case reflect.Int8:
			if kx == kindConst {
				x := vx.(int8)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)>>fr.uintptr(iy)) }
				}
			}
		case reflect.Int16:
			if kx == kindConst {
				x := vx.(int16)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)>>fr.uintptr(iy)) }
				}
			}
		case reflect.Int32:
			if kx == kindConst {
				x := vx.(int32)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)>>fr.uintptr(iy)) }
				}
			}
		case reflect.Int64:
			if kx == kindConst {
				x := vx.(int64)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)>>fr.uintptr(iy)) }
				}
			}
		case reflect.Uint:
			if kx == kindConst {
				x := vx.(uint)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)>>fr.uintptr(iy)) }
				}
			}
		case reflect.Uint8:
			if kx == kindConst {
				x := vx.(uint8)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)>>fr.uintptr(iy)) }
				}
			}
		case reflect.Uint16:
			if kx == kindConst {
				x := vx.(uint16)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)>>fr.uintptr(iy)) }
				}
			}
		case reflect.Uint32:
			if kx == kindConst {
				x := vx.(uint32)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)>>fr.uintptr(iy)) }
				}
			}
		case reflect.Uint64:
			if kx == kindConst {
				x := vx.(uint64)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)>>fr.uintptr(iy)) }
				}
			}
		case reflect.Uintptr:
			if kx == kindConst {
				x := vx.(uintptr)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, x>>fr.uintptr(iy)) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>y) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>y) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>y) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>y) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>y) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>y) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>y) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>y) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>y) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>y) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>y) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>fr.int(iy)) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>fr.int8(iy)) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>fr.int16(iy)) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>fr.int32(iy)) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>fr.int64(iy)) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>fr.uint(iy)) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>fr.uint8(iy)) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>fr.uint16(iy)) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>fr.uint32(iy)) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>fr.uint64(iy)) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)>>fr.uintptr(iy)) }
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
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)>>fr.uintptr(iy))) }
				}
			}
		case reflect.Int8:
			if kx == kindConst {
				x := xtype.Int8(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)>>fr.uintptr(iy))) }
				}
			}
		case reflect.Int16:
			if kx == kindConst {
				x := xtype.Int16(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)>>fr.uintptr(iy))) }
				}
			}
		case reflect.Int32:
			if kx == kindConst {
				x := xtype.Int32(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)>>fr.uintptr(iy))) }
				}
			}
		case reflect.Int64:
			if kx == kindConst {
				x := xtype.Int64(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)>>fr.uintptr(iy))) }
				}
			}
		case reflect.Uint:
			if kx == kindConst {
				x := xtype.Uint(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)>>fr.uintptr(iy))) }
				}
			}
		case reflect.Uint8:
			if kx == kindConst {
				x := xtype.Uint8(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)>>fr.uintptr(iy))) }
				}
			}
		case reflect.Uint16:
			if kx == kindConst {
				x := xtype.Uint16(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)>>fr.uintptr(iy))) }
				}
			}
		case reflect.Uint32:
			if kx == kindConst {
				x := xtype.Uint32(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)>>fr.uintptr(iy))) }
				}
			}
		case reflect.Uint64:
			if kx == kindConst {
				x := xtype.Uint64(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)>>fr.uintptr(iy))) }
				}
			}
		case reflect.Uintptr:
			if kx == kindConst {
				x := xtype.Uintptr(vx)
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x>>fr.uintptr(iy))) }
				}
			} else if ky == kindConst {
				switch ykind {
				case reflect.Int:
					y := xtype.Int(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>y)) }
				case reflect.Int8:
					y := xtype.Int8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>y)) }
				case reflect.Int16:
					y := xtype.Int16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>y)) }
				case reflect.Int32:
					y := xtype.Int32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>y)) }
				case reflect.Int64:
					y := xtype.Int64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>y)) }
				case reflect.Uint:
					y := xtype.Uint(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>y)) }
				case reflect.Uint8:
					y := xtype.Uint8(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>y)) }
				case reflect.Uint16:
					y := xtype.Uint16(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>y)) }
				case reflect.Uint32:
					y := xtype.Uint32(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>y)) }
				case reflect.Uint64:
					y := xtype.Uint64(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>y)) }
				case reflect.Uintptr:
					y := xtype.Uintptr(vy)
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>y)) }
				}
			} else {
				switch ykind {
				case reflect.Int:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>fr.int(iy))) }
				case reflect.Int8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>fr.int8(iy))) }
				case reflect.Int16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>fr.int16(iy))) }
				case reflect.Int32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>fr.int32(iy))) }
				case reflect.Int64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>fr.int64(iy))) }
				case reflect.Uint:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>fr.uint(iy))) }
				case reflect.Uint8:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>fr.uint8(iy))) }
				case reflect.Uint16:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>fr.uint16(iy))) }
				case reflect.Uint32:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>fr.uint32(iy))) }
				case reflect.Uint64:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>fr.uint64(iy))) }
				case reflect.Uintptr:
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)>>fr.uintptr(iy))) }
				}
			}
		}
	}
	panic("unreachable")
}
