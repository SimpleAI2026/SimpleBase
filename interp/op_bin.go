package interp

import (
	"reflect"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)

func makeBinOpADD(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := vx.(int) + vy.(int)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)+fr.reg(iy).(int)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := vx.(int8) + vy.(int8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)+fr.reg(iy).(int8)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := vx.(int16) + vy.(int16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)+fr.reg(iy).(int16)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := vx.(int32) + vy.(int32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)+fr.reg(iy).(int32)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := vx.(int64) + vy.(int64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)+fr.reg(iy).(int64)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint) + vy.(uint)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)+fr.reg(iy).(uint)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint8) + vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)+fr.reg(iy).(uint8)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint16) + vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)+fr.reg(iy).(uint16)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint32) + vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)+fr.reg(iy).(uint32)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint64) + vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)+fr.reg(iy).(uint64)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := vx.(uintptr) + vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)+fr.reg(iy).(uintptr)) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := vx.(float32) + vy.(float32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32)+fr.reg(iy).(float32)) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := vx.(float64) + vy.(float64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64)+fr.reg(iy).(float64)) }
			}
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				v := vx.(complex64) + vy.(complex64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex64)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(complex64)) }
			} else if ky == kindConst {
				y := vy.(complex64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex64)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex64)+fr.reg(iy).(complex64)) }
			}
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				v := vx.(complex128) + vy.(complex128)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex128)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(complex128)) }
			} else if ky == kindConst {
				y := vy.(complex128)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex128)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex128)+fr.reg(iy).(complex128)) }
			}
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				v := vx.(string) + vy.(string)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(string)
				return func(fr *frame) { fr.setReg(ir, x+fr.reg(iy).(string)) }
			} else if ky == kindConst {
				y := vy.(string)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(string)+y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(string)+fr.reg(iy).(string)) }
			}
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int(vx)+xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)+fr.int(iy))) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int8(vx)+xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)+fr.int8(iy))) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int16(vx)+xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)+fr.int16(iy))) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int32(vx)+xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)+fr.int32(iy))) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int64(vx)+xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)+fr.int64(iy))) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint(vx)+xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)+fr.uint(iy))) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint8(vx)+xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)+fr.uint8(iy))) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint16(vx)+xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)+fr.uint16(iy))) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint32(vx)+xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)+fr.uint32(iy))) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint64(vx)+xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)+fr.uint64(iy))) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uintptr(vx)+xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)+fr.uintptr(iy))) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Float32(vx)+xtype.Float32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.float32(iy))) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float32(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float32(ix)+fr.float32(iy))) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Float64(vx)+xtype.Float64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.float64(iy))) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float64(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float64(ix)+fr.float64(iy))) }
			}
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Complex64(vx)+xtype.Complex64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.complex64(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex64(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex64(ix)+fr.complex64(iy))) }
			}
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Complex128(vx)+xtype.Complex128(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex128(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.complex128(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex128(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex128(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex128(ix)+fr.complex128(iy))) }
			}
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.String(vx)+xtype.String(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.String(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x+fr.string(iy))) }
			} else if ky == kindConst {
				y := xtype.String(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.string(ix)+y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.string(ix)+fr.string(iy))) }
			}
		}
	}
	panic("unreachable")
}
func makeBinOpSUB(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := vx.(int) - vy.(int)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)-fr.reg(iy).(int)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := vx.(int8) - vy.(int8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)-fr.reg(iy).(int8)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := vx.(int16) - vy.(int16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)-fr.reg(iy).(int16)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := vx.(int32) - vy.(int32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)-fr.reg(iy).(int32)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := vx.(int64) - vy.(int64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)-fr.reg(iy).(int64)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint) - vy.(uint)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)-fr.reg(iy).(uint)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint8) - vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)-fr.reg(iy).(uint8)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint16) - vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)-fr.reg(iy).(uint16)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint32) - vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)-fr.reg(iy).(uint32)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint64) - vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)-fr.reg(iy).(uint64)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := vx.(uintptr) - vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)-fr.reg(iy).(uintptr)) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := vx.(float32) - vy.(float32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32)-fr.reg(iy).(float32)) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := vx.(float64) - vy.(float64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64)-fr.reg(iy).(float64)) }
			}
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				v := vx.(complex64) - vy.(complex64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex64)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(complex64)) }
			} else if ky == kindConst {
				y := vy.(complex64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex64)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex64)-fr.reg(iy).(complex64)) }
			}
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				v := vx.(complex128) - vy.(complex128)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex128)
				return func(fr *frame) { fr.setReg(ir, x-fr.reg(iy).(complex128)) }
			} else if ky == kindConst {
				y := vy.(complex128)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex128)-y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex128)-fr.reg(iy).(complex128)) }
			}
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int(vx)-xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)-fr.int(iy))) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int8(vx)-xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)-fr.int8(iy))) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int16(vx)-xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)-fr.int16(iy))) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int32(vx)-xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)-fr.int32(iy))) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int64(vx)-xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)-fr.int64(iy))) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint(vx)-xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)-fr.uint(iy))) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint8(vx)-xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)-fr.uint8(iy))) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint16(vx)-xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)-fr.uint16(iy))) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint32(vx)-xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)-fr.uint32(iy))) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint64(vx)-xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)-fr.uint64(iy))) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uintptr(vx)-xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)-fr.uintptr(iy))) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Float32(vx)-xtype.Float32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.float32(iy))) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float32(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float32(ix)-fr.float32(iy))) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Float64(vx)-xtype.Float64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.float64(iy))) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float64(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float64(ix)-fr.float64(iy))) }
			}
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Complex64(vx)-xtype.Complex64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.complex64(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex64(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex64(ix)-fr.complex64(iy))) }
			}
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Complex128(vx)-xtype.Complex128(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex128(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x-fr.complex128(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex128(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex128(ix)-y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex128(ix)-fr.complex128(iy))) }
			}
		}
	}
	panic("unreachable")
}
func makeBinOpMUL(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := vx.(int) * vy.(int)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)*fr.reg(iy).(int)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := vx.(int8) * vy.(int8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)*fr.reg(iy).(int8)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := vx.(int16) * vy.(int16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)*fr.reg(iy).(int16)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := vx.(int32) * vy.(int32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)*fr.reg(iy).(int32)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := vx.(int64) * vy.(int64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)*fr.reg(iy).(int64)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint) * vy.(uint)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)*fr.reg(iy).(uint)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint8) * vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)*fr.reg(iy).(uint8)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint16) * vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)*fr.reg(iy).(uint16)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint32) * vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)*fr.reg(iy).(uint32)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint64) * vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)*fr.reg(iy).(uint64)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := vx.(uintptr) * vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)*fr.reg(iy).(uintptr)) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := vx.(float32) * vy.(float32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32)*fr.reg(iy).(float32)) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := vx.(float64) * vy.(float64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64)*fr.reg(iy).(float64)) }
			}
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				v := vx.(complex64) * vy.(complex64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex64)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(complex64)) }
			} else if ky == kindConst {
				y := vy.(complex64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex64)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex64)*fr.reg(iy).(complex64)) }
			}
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				v := vx.(complex128) * vy.(complex128)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex128)
				return func(fr *frame) { fr.setReg(ir, x*fr.reg(iy).(complex128)) }
			} else if ky == kindConst {
				y := vy.(complex128)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex128)*y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex128)*fr.reg(iy).(complex128)) }
			}
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int(vx)*xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)*fr.int(iy))) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int8(vx)*xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)*fr.int8(iy))) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int16(vx)*xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)*fr.int16(iy))) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int32(vx)*xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)*fr.int32(iy))) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int64(vx)*xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)*fr.int64(iy))) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint(vx)*xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)*fr.uint(iy))) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint8(vx)*xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)*fr.uint8(iy))) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint16(vx)*xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)*fr.uint16(iy))) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint32(vx)*xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)*fr.uint32(iy))) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint64(vx)*xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)*fr.uint64(iy))) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uintptr(vx)*xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)*fr.uintptr(iy))) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Float32(vx)*xtype.Float32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.float32(iy))) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float32(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float32(ix)*fr.float32(iy))) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Float64(vx)*xtype.Float64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.float64(iy))) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float64(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float64(ix)*fr.float64(iy))) }
			}
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Complex64(vx)*xtype.Complex64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.complex64(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex64(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex64(ix)*fr.complex64(iy))) }
			}
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Complex128(vx)*xtype.Complex128(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex128(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x*fr.complex128(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex128(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex128(ix)*y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex128(ix)*fr.complex128(iy))) }
			}
		}
	}
	panic("unreachable")
}
func makeBinOpQUO(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				x := vx.(int)
				y := vy.(int)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, x/y) }
				}
				v := x / y
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)/fr.reg(iy).(int)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				x := vx.(int8)
				y := vy.(int8)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, x/y) }
				}
				v := x / y
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)/fr.reg(iy).(int8)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				x := vx.(int16)
				y := vy.(int16)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, x/y) }
				}
				v := x / y
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)/fr.reg(iy).(int16)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				x := vx.(int32)
				y := vy.(int32)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, x/y) }
				}
				v := x / y
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)/fr.reg(iy).(int32)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				x := vx.(int64)
				y := vy.(int64)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, x/y) }
				}
				v := x / y
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)/fr.reg(iy).(int64)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				x := vx.(uint)
				y := vy.(uint)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, x/y) }
				}
				v := x / y
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)/fr.reg(iy).(uint)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				x := vx.(uint8)
				y := vy.(uint8)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, x/y) }
				}
				v := x / y
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)/fr.reg(iy).(uint8)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				x := vx.(uint16)
				y := vy.(uint16)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, x/y) }
				}
				v := x / y
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)/fr.reg(iy).(uint16)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				x := vx.(uint32)
				y := vy.(uint32)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, x/y) }
				}
				v := x / y
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)/fr.reg(iy).(uint32)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				x := vx.(uint64)
				y := vy.(uint64)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, x/y) }
				}
				v := x / y
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)/fr.reg(iy).(uint64)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				x := vx.(uintptr)
				y := vy.(uintptr)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, x/y) }
				}
				v := x / y
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)/fr.reg(iy).(uintptr)) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := vx.(float32) / vy.(float32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32)/fr.reg(iy).(float32)) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := vx.(float64) / vy.(float64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64)/fr.reg(iy).(float64)) }
			}
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				v := vx.(complex64) / vy.(complex64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex64)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(complex64)) }
			} else if ky == kindConst {
				y := vy.(complex64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex64)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex64)/fr.reg(iy).(complex64)) }
			}
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				v := vx.(complex128) / vy.(complex128)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex128)
				return func(fr *frame) { fr.setReg(ir, x/fr.reg(iy).(complex128)) }
			} else if ky == kindConst {
				y := vy.(complex128)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex128)/y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(complex128)/fr.reg(iy).(complex128)) }
			}
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				x := xtype.Int(vx)
				y := xtype.Int(vy)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/y)) }
				}
				v := xtype.Make(t, x/y)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)/fr.int(iy))) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				x := xtype.Int8(vx)
				y := xtype.Int8(vy)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/y)) }
				}
				v := xtype.Make(t, x/y)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)/fr.int8(iy))) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				x := xtype.Int16(vx)
				y := xtype.Int16(vy)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/y)) }
				}
				v := xtype.Make(t, x/y)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)/fr.int16(iy))) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				x := xtype.Int32(vx)
				y := xtype.Int32(vy)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/y)) }
				}
				v := xtype.Make(t, x/y)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)/fr.int32(iy))) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				x := xtype.Int64(vx)
				y := xtype.Int64(vy)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/y)) }
				}
				v := xtype.Make(t, x/y)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)/fr.int64(iy))) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				x := xtype.Uint(vx)
				y := xtype.Uint(vy)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/y)) }
				}
				v := xtype.Make(t, x/y)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)/fr.uint(iy))) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				x := xtype.Uint8(vx)
				y := xtype.Uint8(vy)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/y)) }
				}
				v := xtype.Make(t, x/y)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)/fr.uint8(iy))) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				x := xtype.Uint16(vx)
				y := xtype.Uint16(vy)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/y)) }
				}
				v := xtype.Make(t, x/y)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)/fr.uint16(iy))) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				x := xtype.Uint32(vx)
				y := xtype.Uint32(vy)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/y)) }
				}
				v := xtype.Make(t, x/y)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)/fr.uint32(iy))) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				x := xtype.Uint64(vx)
				y := xtype.Uint64(vy)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/y)) }
				}
				v := xtype.Make(t, x/y)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)/fr.uint64(iy))) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				x := xtype.Uintptr(vx)
				y := xtype.Uintptr(vy)
				if y == 0 {
					return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/y)) }
				}
				v := xtype.Make(t, x/y)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)/fr.uintptr(iy))) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Float32(vx)/xtype.Float32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.float32(iy))) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float32(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float32(ix)/fr.float32(iy))) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Float64(vx)/xtype.Float64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.float64(iy))) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float64(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.float64(ix)/fr.float64(iy))) }
			}
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Complex64(vx)/xtype.Complex64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.complex64(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex64(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex64(ix)/fr.complex64(iy))) }
			}
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Complex128(vx)/xtype.Complex128(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex128(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x/fr.complex128(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex128(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex128(ix)/y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.complex128(ix)/fr.complex128(iy))) }
			}
		}
	}
	panic("unreachable")
}
func makeBinOpREM(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := vx.(int) % vy.(int)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(fr *frame) { fr.setReg(ir, x%fr.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)%y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)%fr.reg(iy).(int)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := vx.(int8) % vy.(int8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(fr *frame) { fr.setReg(ir, x%fr.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)%y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)%fr.reg(iy).(int8)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := vx.(int16) % vy.(int16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(fr *frame) { fr.setReg(ir, x%fr.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)%y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)%fr.reg(iy).(int16)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := vx.(int32) % vy.(int32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(fr *frame) { fr.setReg(ir, x%fr.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)%y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)%fr.reg(iy).(int32)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := vx.(int64) % vy.(int64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(fr *frame) { fr.setReg(ir, x%fr.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)%y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)%fr.reg(iy).(int64)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint) % vy.(uint)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(fr *frame) { fr.setReg(ir, x%fr.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)%y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)%fr.reg(iy).(uint)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint8) % vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(fr *frame) { fr.setReg(ir, x%fr.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)%y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)%fr.reg(iy).(uint8)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint16) % vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(fr *frame) { fr.setReg(ir, x%fr.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)%y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)%fr.reg(iy).(uint16)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint32) % vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(fr *frame) { fr.setReg(ir, x%fr.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)%y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)%fr.reg(iy).(uint32)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint64) % vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(fr *frame) { fr.setReg(ir, x%fr.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)%y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)%fr.reg(iy).(uint64)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := vx.(uintptr) % vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(fr *frame) { fr.setReg(ir, x%fr.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)%y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)%fr.reg(iy).(uintptr)) }
			}
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int(vx)%xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x%fr.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)%y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)%fr.int(iy))) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int8(vx)%xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x%fr.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)%y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)%fr.int8(iy))) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int16(vx)%xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x%fr.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)%y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)%fr.int16(iy))) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int32(vx)%xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x%fr.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)%y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)%fr.int32(iy))) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int64(vx)%xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x%fr.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)%y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)%fr.int64(iy))) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint(vx)%xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x%fr.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)%y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)%fr.uint(iy))) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint8(vx)%xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x%fr.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)%y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)%fr.uint8(iy))) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint16(vx)%xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x%fr.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)%y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)%fr.uint16(iy))) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint32(vx)%xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x%fr.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)%y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)%fr.uint32(iy))) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint64(vx)%xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x%fr.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)%y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)%fr.uint64(iy))) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uintptr(vx)%xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x%fr.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)%y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)%fr.uintptr(iy))) }
			}
		}
	}
	panic("unreachable")
}
func makeBinOpAND(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := vx.(int) & vy.(int)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(fr *frame) { fr.setReg(ir, x&fr.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)&y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)&fr.reg(iy).(int)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := vx.(int8) & vy.(int8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(fr *frame) { fr.setReg(ir, x&fr.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)&y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)&fr.reg(iy).(int8)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := vx.(int16) & vy.(int16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(fr *frame) { fr.setReg(ir, x&fr.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)&y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)&fr.reg(iy).(int16)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := vx.(int32) & vy.(int32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(fr *frame) { fr.setReg(ir, x&fr.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)&y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)&fr.reg(iy).(int32)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := vx.(int64) & vy.(int64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(fr *frame) { fr.setReg(ir, x&fr.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)&y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)&fr.reg(iy).(int64)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint) & vy.(uint)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(fr *frame) { fr.setReg(ir, x&fr.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)&y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)&fr.reg(iy).(uint)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint8) & vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(fr *frame) { fr.setReg(ir, x&fr.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)&y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)&fr.reg(iy).(uint8)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint16) & vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(fr *frame) { fr.setReg(ir, x&fr.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)&y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)&fr.reg(iy).(uint16)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint32) & vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(fr *frame) { fr.setReg(ir, x&fr.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)&y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)&fr.reg(iy).(uint32)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint64) & vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(fr *frame) { fr.setReg(ir, x&fr.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)&y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)&fr.reg(iy).(uint64)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := vx.(uintptr) & vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(fr *frame) { fr.setReg(ir, x&fr.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)&y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)&fr.reg(iy).(uintptr)) }
			}
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int(vx)&xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&fr.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)&y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)&fr.int(iy))) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int8(vx)&xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&fr.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)&y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)&fr.int8(iy))) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int16(vx)&xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&fr.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)&y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)&fr.int16(iy))) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int32(vx)&xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&fr.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)&y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)&fr.int32(iy))) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int64(vx)&xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&fr.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)&y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)&fr.int64(iy))) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint(vx)&xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&fr.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)&y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)&fr.uint(iy))) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint8(vx)&xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&fr.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)&y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)&fr.uint8(iy))) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint16(vx)&xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&fr.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)&y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)&fr.uint16(iy))) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint32(vx)&xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&fr.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)&y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)&fr.uint32(iy))) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint64(vx)&xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&fr.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)&y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)&fr.uint64(iy))) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uintptr(vx)&xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&fr.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)&y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)&fr.uintptr(iy))) }
			}
		}
	}
	panic("unreachable")
}
func makeBinOpOR(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := vx.(int) | vy.(int)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(fr *frame) { fr.setReg(ir, x|fr.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)|y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)|fr.reg(iy).(int)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := vx.(int8) | vy.(int8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(fr *frame) { fr.setReg(ir, x|fr.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)|y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)|fr.reg(iy).(int8)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := vx.(int16) | vy.(int16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(fr *frame) { fr.setReg(ir, x|fr.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)|y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)|fr.reg(iy).(int16)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := vx.(int32) | vy.(int32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(fr *frame) { fr.setReg(ir, x|fr.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)|y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)|fr.reg(iy).(int32)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := vx.(int64) | vy.(int64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(fr *frame) { fr.setReg(ir, x|fr.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)|y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)|fr.reg(iy).(int64)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint) | vy.(uint)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(fr *frame) { fr.setReg(ir, x|fr.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)|y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)|fr.reg(iy).(uint)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint8) | vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(fr *frame) { fr.setReg(ir, x|fr.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)|y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)|fr.reg(iy).(uint8)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint16) | vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(fr *frame) { fr.setReg(ir, x|fr.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)|y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)|fr.reg(iy).(uint16)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint32) | vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(fr *frame) { fr.setReg(ir, x|fr.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)|y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)|fr.reg(iy).(uint32)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint64) | vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(fr *frame) { fr.setReg(ir, x|fr.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)|y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)|fr.reg(iy).(uint64)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := vx.(uintptr) | vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(fr *frame) { fr.setReg(ir, x|fr.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)|y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)|fr.reg(iy).(uintptr)) }
			}
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int(vx)|xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x|fr.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)|y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)|fr.int(iy))) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int8(vx)|xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x|fr.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)|y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)|fr.int8(iy))) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int16(vx)|xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x|fr.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)|y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)|fr.int16(iy))) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int32(vx)|xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x|fr.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)|y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)|fr.int32(iy))) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int64(vx)|xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x|fr.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)|y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)|fr.int64(iy))) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint(vx)|xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x|fr.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)|y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)|fr.uint(iy))) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint8(vx)|xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x|fr.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)|y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)|fr.uint8(iy))) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint16(vx)|xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x|fr.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)|y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)|fr.uint16(iy))) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint32(vx)|xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x|fr.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)|y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)|fr.uint32(iy))) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint64(vx)|xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x|fr.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)|y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)|fr.uint64(iy))) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uintptr(vx)|xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x|fr.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)|y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)|fr.uintptr(iy))) }
			}
		}
	}
	panic("unreachable")
}
func makeBinOpXOR(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := vx.(int) ^ vy.(int)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(fr *frame) { fr.setReg(ir, x^fr.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)^fr.reg(iy).(int)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := vx.(int8) ^ vy.(int8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(fr *frame) { fr.setReg(ir, x^fr.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)^fr.reg(iy).(int8)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := vx.(int16) ^ vy.(int16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(fr *frame) { fr.setReg(ir, x^fr.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)^fr.reg(iy).(int16)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := vx.(int32) ^ vy.(int32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(fr *frame) { fr.setReg(ir, x^fr.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)^fr.reg(iy).(int32)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := vx.(int64) ^ vy.(int64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(fr *frame) { fr.setReg(ir, x^fr.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)^fr.reg(iy).(int64)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint) ^ vy.(uint)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(fr *frame) { fr.setReg(ir, x^fr.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)^fr.reg(iy).(uint)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint8) ^ vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(fr *frame) { fr.setReg(ir, x^fr.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)^fr.reg(iy).(uint8)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint16) ^ vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(fr *frame) { fr.setReg(ir, x^fr.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)^fr.reg(iy).(uint16)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint32) ^ vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(fr *frame) { fr.setReg(ir, x^fr.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)^fr.reg(iy).(uint32)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint64) ^ vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(fr *frame) { fr.setReg(ir, x^fr.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)^fr.reg(iy).(uint64)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := vx.(uintptr) ^ vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(fr *frame) { fr.setReg(ir, x^fr.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)^fr.reg(iy).(uintptr)) }
			}
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int(vx)^xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x^fr.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)^fr.int(iy))) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int8(vx)^xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x^fr.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)^fr.int8(iy))) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int16(vx)^xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x^fr.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)^fr.int16(iy))) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int32(vx)^xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x^fr.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)^fr.int32(iy))) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int64(vx)^xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x^fr.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)^fr.int64(iy))) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint(vx)^xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x^fr.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)^fr.uint(iy))) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint8(vx)^xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x^fr.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)^fr.uint8(iy))) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint16(vx)^xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x^fr.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)^fr.uint16(iy))) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint32(vx)^xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x^fr.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)^fr.uint32(iy))) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint64(vx)^xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x^fr.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)^fr.uint64(iy))) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uintptr(vx)^xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x^fr.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)^fr.uintptr(iy))) }
			}
		}
	}
	panic("unreachable")
}
func makeBinOpANDNOT(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := vx.(int) &^ vy.(int)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(fr *frame) { fr.setReg(ir, x&^fr.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)&^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int)&^fr.reg(iy).(int)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := vx.(int8) &^ vy.(int8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(fr *frame) { fr.setReg(ir, x&^fr.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)&^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8)&^fr.reg(iy).(int8)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := vx.(int16) &^ vy.(int16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(fr *frame) { fr.setReg(ir, x&^fr.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)&^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16)&^fr.reg(iy).(int16)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := vx.(int32) &^ vy.(int32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(fr *frame) { fr.setReg(ir, x&^fr.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)&^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32)&^fr.reg(iy).(int32)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := vx.(int64) &^ vy.(int64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(fr *frame) { fr.setReg(ir, x&^fr.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)&^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64)&^fr.reg(iy).(int64)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint) &^ vy.(uint)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(fr *frame) { fr.setReg(ir, x&^fr.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)&^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint)&^fr.reg(iy).(uint)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint8) &^ vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(fr *frame) { fr.setReg(ir, x&^fr.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)&^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8)&^fr.reg(iy).(uint8)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint16) &^ vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(fr *frame) { fr.setReg(ir, x&^fr.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)&^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16)&^fr.reg(iy).(uint16)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint32) &^ vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(fr *frame) { fr.setReg(ir, x&^fr.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)&^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32)&^fr.reg(iy).(uint32)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint64) &^ vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(fr *frame) { fr.setReg(ir, x&^fr.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)&^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64)&^fr.reg(iy).(uint64)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := vx.(uintptr) &^ vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(fr *frame) { fr.setReg(ir, x&^fr.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)&^y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr)&^fr.reg(iy).(uintptr)) }
			}
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int(vx)&^xtype.Int(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&^fr.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)&^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int(ix)&^fr.int(iy))) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int8(vx)&^xtype.Int8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&^fr.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)&^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int8(ix)&^fr.int8(iy))) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int16(vx)&^xtype.Int16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&^fr.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)&^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int16(ix)&^fr.int16(iy))) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int32(vx)&^xtype.Int32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&^fr.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)&^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int32(ix)&^fr.int32(iy))) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Int64(vx)&^xtype.Int64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&^fr.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)&^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.int64(ix)&^fr.int64(iy))) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint(vx)&^xtype.Uint(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&^fr.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)&^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint(ix)&^fr.uint(iy))) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint8(vx)&^xtype.Uint8(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&^fr.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)&^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint8(ix)&^fr.uint8(iy))) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint16(vx)&^xtype.Uint16(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&^fr.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)&^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint16(ix)&^fr.uint16(iy))) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint32(vx)&^xtype.Uint32(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&^fr.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)&^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint32(ix)&^fr.uint32(iy))) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uint64(vx)&^xtype.Uint64(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&^fr.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)&^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uint64(ix)&^fr.uint64(iy))) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := xtype.Make(t, xtype.Uintptr(vx)&^xtype.Uintptr(vy))
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, x&^fr.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)&^y)) }
			} else {
				return func(fr *frame) { fr.setReg(ir, xtype.Make(t, fr.uintptr(ix)&^fr.uintptr(iy))) }
			}
		}
	}
	panic("unreachable")
}
func makeBinOpLSS(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.X.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := vx.(int) < vy.(int)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int) < fr.reg(iy).(int)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := vx.(int8) < vy.(int8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8) < fr.reg(iy).(int8)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := vx.(int16) < vy.(int16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16) < fr.reg(iy).(int16)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := vx.(int32) < vy.(int32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32) < fr.reg(iy).(int32)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := vx.(int64) < vy.(int64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64) < fr.reg(iy).(int64)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint) < vy.(uint)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint) < fr.reg(iy).(uint)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint8) < vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8) < fr.reg(iy).(uint8)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint16) < vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16) < fr.reg(iy).(uint16)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint32) < vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32) < fr.reg(iy).(uint32)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint64) < vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64) < fr.reg(iy).(uint64)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := vx.(uintptr) < vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr) < fr.reg(iy).(uintptr)) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := vx.(float32) < vy.(float32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32) < fr.reg(iy).(float32)) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := vx.(float64) < vy.(float64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64) < fr.reg(iy).(float64)) }
			}
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				v := vx.(string) < vy.(string)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(string)
				return func(fr *frame) { fr.setReg(ir, x < fr.reg(iy).(string)) }
			} else if ky == kindConst {
				y := vy.(string)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(string) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(string) < fr.reg(iy).(string)) }
			}
		}
	} else {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int(vx) < xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.int(iy)) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int(ix) < fr.int(iy)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int8(vx) < xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.int8(iy)) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int8(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int8(ix) < fr.int8(iy)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int16(vx) < xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.int16(iy)) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int16(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int16(ix) < fr.int16(iy)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int32(vx) < xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.int32(iy)) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int32(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int32(ix) < fr.int32(iy)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int64(vx) < xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.int64(iy)) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int64(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int64(ix) < fr.int64(iy)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint(vx) < xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.uint(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint(ix) < fr.uint(iy)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint8(vx) < xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.uint8(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint8(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint8(ix) < fr.uint8(iy)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint16(vx) < xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.uint16(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint16(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint16(ix) < fr.uint16(iy)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint32(vx) < xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.uint32(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint32(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint32(ix) < fr.uint32(iy)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint64(vx) < xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.uint64(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint64(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint64(ix) < fr.uint64(iy)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uintptr(vx) < xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.uintptr(iy)) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uintptr(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uintptr(ix) < fr.uintptr(iy)) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Float32(vx) < xtype.Float32(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.float32(iy)) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(fr *frame) { fr.setReg(ir, fr.float32(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.float32(ix) < fr.float32(iy)) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Float64(vx) < xtype.Float64(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.float64(iy)) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(fr *frame) { fr.setReg(ir, fr.float64(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.float64(ix) < fr.float64(iy)) }
			}
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				v := xtype.String(vx) < xtype.String(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.String(vx)
				return func(fr *frame) { fr.setReg(ir, x < fr.string(iy)) }
			} else if ky == kindConst {
				y := xtype.String(vy)
				return func(fr *frame) { fr.setReg(ir, fr.string(ix) < y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.string(ix) < fr.string(iy)) }
			}
		}
	}
	panic("unreachable")
}
func makeBinOpLEQ(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.X.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := vx.(int) <= vy.(int)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int) <= fr.reg(iy).(int)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := vx.(int8) <= vy.(int8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8) <= fr.reg(iy).(int8)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := vx.(int16) <= vy.(int16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16) <= fr.reg(iy).(int16)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := vx.(int32) <= vy.(int32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32) <= fr.reg(iy).(int32)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := vx.(int64) <= vy.(int64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64) <= fr.reg(iy).(int64)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint) <= vy.(uint)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint) <= fr.reg(iy).(uint)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint8) <= vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8) <= fr.reg(iy).(uint8)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint16) <= vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16) <= fr.reg(iy).(uint16)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint32) <= vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32) <= fr.reg(iy).(uint32)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint64) <= vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64) <= fr.reg(iy).(uint64)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := vx.(uintptr) <= vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr) <= fr.reg(iy).(uintptr)) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := vx.(float32) <= vy.(float32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32) <= fr.reg(iy).(float32)) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := vx.(float64) <= vy.(float64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64) <= fr.reg(iy).(float64)) }
			}
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				v := vx.(string) <= vy.(string)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(string)
				return func(fr *frame) { fr.setReg(ir, x <= fr.reg(iy).(string)) }
			} else if ky == kindConst {
				y := vy.(string)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(string) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(string) <= fr.reg(iy).(string)) }
			}
		}
	} else {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int(vx) <= xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.int(iy)) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int(ix) <= fr.int(iy)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int8(vx) <= xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.int8(iy)) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int8(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int8(ix) <= fr.int8(iy)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int16(vx) <= xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.int16(iy)) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int16(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int16(ix) <= fr.int16(iy)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int32(vx) <= xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.int32(iy)) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int32(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int32(ix) <= fr.int32(iy)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int64(vx) <= xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.int64(iy)) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int64(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int64(ix) <= fr.int64(iy)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint(vx) <= xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.uint(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint(ix) <= fr.uint(iy)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint8(vx) <= xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.uint8(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint8(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint8(ix) <= fr.uint8(iy)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint16(vx) <= xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.uint16(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint16(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint16(ix) <= fr.uint16(iy)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint32(vx) <= xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.uint32(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint32(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint32(ix) <= fr.uint32(iy)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint64(vx) <= xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.uint64(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint64(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint64(ix) <= fr.uint64(iy)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uintptr(vx) <= xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.uintptr(iy)) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uintptr(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uintptr(ix) <= fr.uintptr(iy)) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Float32(vx) <= xtype.Float32(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.float32(iy)) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(fr *frame) { fr.setReg(ir, fr.float32(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.float32(ix) <= fr.float32(iy)) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Float64(vx) <= xtype.Float64(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.float64(iy)) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(fr *frame) { fr.setReg(ir, fr.float64(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.float64(ix) <= fr.float64(iy)) }
			}
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				v := xtype.String(vx) <= xtype.String(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.String(vx)
				return func(fr *frame) { fr.setReg(ir, x <= fr.string(iy)) }
			} else if ky == kindConst {
				y := xtype.String(vy)
				return func(fr *frame) { fr.setReg(ir, fr.string(ix) <= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.string(ix) <= fr.string(iy)) }
			}
		}
	}
	panic("unreachable")
}
func makeBinOpGTR(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.X.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := vx.(int) > vy.(int)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int) > fr.reg(iy).(int)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := vx.(int8) > vy.(int8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8) > fr.reg(iy).(int8)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := vx.(int16) > vy.(int16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16) > fr.reg(iy).(int16)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := vx.(int32) > vy.(int32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32) > fr.reg(iy).(int32)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := vx.(int64) > vy.(int64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64) > fr.reg(iy).(int64)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint) > vy.(uint)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint) > fr.reg(iy).(uint)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint8) > vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8) > fr.reg(iy).(uint8)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint16) > vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16) > fr.reg(iy).(uint16)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint32) > vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32) > fr.reg(iy).(uint32)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint64) > vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64) > fr.reg(iy).(uint64)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := vx.(uintptr) > vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr) > fr.reg(iy).(uintptr)) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := vx.(float32) > vy.(float32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32) > fr.reg(iy).(float32)) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := vx.(float64) > vy.(float64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64) > fr.reg(iy).(float64)) }
			}
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				v := vx.(string) > vy.(string)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(string)
				return func(fr *frame) { fr.setReg(ir, x > fr.reg(iy).(string)) }
			} else if ky == kindConst {
				y := vy.(string)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(string) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(string) > fr.reg(iy).(string)) }
			}
		}
	} else {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int(vx) > xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.int(iy)) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int(ix) > fr.int(iy)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int8(vx) > xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.int8(iy)) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int8(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int8(ix) > fr.int8(iy)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int16(vx) > xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.int16(iy)) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int16(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int16(ix) > fr.int16(iy)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int32(vx) > xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.int32(iy)) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int32(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int32(ix) > fr.int32(iy)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int64(vx) > xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.int64(iy)) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int64(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int64(ix) > fr.int64(iy)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint(vx) > xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.uint(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint(ix) > fr.uint(iy)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint8(vx) > xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.uint8(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint8(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint8(ix) > fr.uint8(iy)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint16(vx) > xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.uint16(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint16(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint16(ix) > fr.uint16(iy)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint32(vx) > xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.uint32(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint32(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint32(ix) > fr.uint32(iy)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint64(vx) > xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.uint64(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint64(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint64(ix) > fr.uint64(iy)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uintptr(vx) > xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.uintptr(iy)) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uintptr(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uintptr(ix) > fr.uintptr(iy)) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Float32(vx) > xtype.Float32(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.float32(iy)) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(fr *frame) { fr.setReg(ir, fr.float32(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.float32(ix) > fr.float32(iy)) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Float64(vx) > xtype.Float64(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.float64(iy)) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(fr *frame) { fr.setReg(ir, fr.float64(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.float64(ix) > fr.float64(iy)) }
			}
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				v := xtype.String(vx) > xtype.String(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.String(vx)
				return func(fr *frame) { fr.setReg(ir, x > fr.string(iy)) }
			} else if ky == kindConst {
				y := xtype.String(vy)
				return func(fr *frame) { fr.setReg(ir, fr.string(ix) > y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.string(ix) > fr.string(iy)) }
			}
		}
	}
	panic("unreachable")
}
func makeBinOpGEQ(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	typ := pfn.Interp.preToType(instr.X.Type())
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := vx.(int) >= vy.(int)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int) >= fr.reg(iy).(int)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := vx.(int8) >= vy.(int8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int8) >= fr.reg(iy).(int8)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := vx.(int16) >= vy.(int16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int16) >= fr.reg(iy).(int16)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := vx.(int32) >= vy.(int32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int32) >= fr.reg(iy).(int32)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := vx.(int64) >= vy.(int64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(int64) >= fr.reg(iy).(int64)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint) >= vy.(uint)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint) >= fr.reg(iy).(uint)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint8) >= vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint8) >= fr.reg(iy).(uint8)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint16) >= vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint16) >= fr.reg(iy).(uint16)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint32) >= vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint32) >= fr.reg(iy).(uint32)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := vx.(uint64) >= vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uint64) >= fr.reg(iy).(uint64)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := vx.(uintptr) >= vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(uintptr) >= fr.reg(iy).(uintptr)) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := vx.(float32) >= vy.(float32)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float32) >= fr.reg(iy).(float32)) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := vx.(float64) >= vy.(float64)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(float64) >= fr.reg(iy).(float64)) }
			}
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				v := vx.(string) >= vy.(string)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(string)
				return func(fr *frame) { fr.setReg(ir, x >= fr.reg(iy).(string)) }
			} else if ky == kindConst {
				y := vy.(string)
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(string) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.reg(ix).(string) >= fr.reg(iy).(string)) }
			}
		}
	} else {
		switch typ.Kind() {
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int(vx) >= xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.int(iy)) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int(ix) >= fr.int(iy)) }
			}
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int8(vx) >= xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.int8(iy)) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int8(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int8(ix) >= fr.int8(iy)) }
			}
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int16(vx) >= xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.int16(iy)) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int16(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int16(ix) >= fr.int16(iy)) }
			}
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int32(vx) >= xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.int32(iy)) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int32(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int32(ix) >= fr.int32(iy)) }
			}
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Int64(vx) >= xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.int64(iy)) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(fr *frame) { fr.setReg(ir, fr.int64(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.int64(ix) >= fr.int64(iy)) }
			}
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint(vx) >= xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.uint(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint(ix) >= fr.uint(iy)) }
			}
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint8(vx) >= xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.uint8(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint8(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint8(ix) >= fr.uint8(iy)) }
			}
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint16(vx) >= xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.uint16(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint16(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint16(ix) >= fr.uint16(iy)) }
			}
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint32(vx) >= xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.uint32(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint32(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint32(ix) >= fr.uint32(iy)) }
			}
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uint64(vx) >= xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.uint64(iy)) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uint64(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uint64(ix) >= fr.uint64(iy)) }
			}
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				v := xtype.Uintptr(vx) >= xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.uintptr(iy)) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(fr *frame) { fr.setReg(ir, fr.uintptr(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.uintptr(ix) >= fr.uintptr(iy)) }
			}
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				v := xtype.Float32(vx) >= xtype.Float32(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.float32(iy)) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(fr *frame) { fr.setReg(ir, fr.float32(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.float32(ix) >= fr.float32(iy)) }
			}
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				v := xtype.Float64(vx) >= xtype.Float64(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.float64(iy)) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(fr *frame) { fr.setReg(ir, fr.float64(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.float64(ix) >= fr.float64(iy)) }
			}
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				v := xtype.String(vx) >= xtype.String(vy)
				return func(fr *frame) { fr.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.String(vx)
				return func(fr *frame) { fr.setReg(ir, x >= fr.string(iy)) }
			} else if ky == kindConst {
				y := xtype.String(vy)
				return func(fr *frame) { fr.setReg(ir, fr.string(ix) >= y) }
			} else {
				return func(fr *frame) { fr.setReg(ir, fr.string(ix) >= fr.string(iy)) }
			}
		}
	}
	panic("unreachable")
}
