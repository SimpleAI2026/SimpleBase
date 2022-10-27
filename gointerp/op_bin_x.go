package interp

import (
	"reflect"

	"golang.org/x/tools/go/ssa"
)

func makeBinOpEQL(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	xtyp := pfn.Interp.preToType(instr.X.Type())
	ytyp := pfn.Interp.preToType(instr.Y.Type())
	if xtyp == nil && ytyp == nil {
		return func(fr *frame) {
			fr.setReg(ir, true)
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
			return func(fr *frame) {
				fr.setReg(ir, v)
			}
		} else if kx == kindConst {
			return func(fr *frame) {
				fr.setReg(ir, vx == fr.reg(iy))
			}
		} else if ky == kindConst {
			return func(fr *frame) {
				fr.setReg(ir, fr.reg(ix) == vy)
			}
		}
		return func(fr *frame) {
			fr.setReg(ir, fr.reg(ix) == fr.reg(iy))
		}
	case reflect.Interface:
		return func(fr *frame) {
			fr.setReg(ir, fr.reg(ix) == fr.reg(iy))
		}
	case reflect.Array:
		if xtyp == ytyp {
			return func(fr *frame) {
				fr.setReg(ir, fr.reg(ix) == fr.reg(iy))
			}
		}
		return func(fr *frame) {
			x := fr.reg(ix)
			y := fr.reg(iy)
			fr.setReg(ir, x == reflect.ValueOf(y).Convert(xtyp).Interface())
		}
	case reflect.Struct:
		return func(fr *frame) {
			fr.setReg(ir, fr.reg(ix) == fr.reg(iy))
		}
	case reflect.UnsafePointer:
		return func(fr *frame) {
			fr.setReg(ir, fr.reg(ix) == fr.reg(iy))
		}
	case reflect.Chan:
		xdir := xtyp.ChanDir()
		ydir := ytyp.ChanDir()
		if xdir != ydir {
			if xdir == reflect.BothDir {
				return func(fr *frame) {
					x := fr.reg(ix)
					x = reflect.ValueOf(x).Convert(ytyp).Interface()
					fr.setReg(ir, x == fr.reg(iy))
				}
			} else if ydir == reflect.BothDir {
				return func(fr *frame) {
					y := fr.reg(iy)
					y = reflect.ValueOf(y).Convert(xtyp).Interface()
					fr.setReg(ir, fr.reg(ix) == y)
				}
			}
		}
		return func(fr *frame) {
			fr.setReg(ir, fr.reg(ix) == fr.reg(iy))
		}
	case reflect.Ptr:
		return func(fr *frame) {
			x := fr.reg(ix)
			y := fr.reg(iy)
			fr.setReg(ir, reflect.ValueOf(x).Pointer() == reflect.ValueOf(y).Pointer())
		}
	case reflect.Slice, reflect.Map, reflect.Func:
		return func(fr *frame) {
			x := fr.reg(ix)
			y := fr.reg(iy)
			b := reflect.ValueOf(x).IsNil() && reflect.ValueOf(y).IsNil()
			fr.setReg(ir, b)
		}
	default:
		panic("unreachable")
	}
}

func makeBinOpNEQ(pfn *function, instr *ssa.BinOp) func(fr *frame) {
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	xtyp := pfn.Interp.preToType(instr.X.Type())
	ytyp := pfn.Interp.preToType(instr.Y.Type())
	if xtyp == nil && ytyp == nil {
		return func(fr *frame) {
			fr.setReg(ir, false)
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
			return func(fr *frame) {
				fr.setReg(ir, v)
			}
		} else if kx == kindConst {
			return func(fr *frame) {
				fr.setReg(ir, vx != fr.reg(iy))
			}
		} else if ky == kindConst {
			return func(fr *frame) {
				fr.setReg(ir, fr.reg(ix) != vy)
			}
		}
		return func(fr *frame) {
			fr.setReg(ir, fr.reg(ix) != fr.reg(iy))
		}
	case reflect.Interface:
		return func(fr *frame) {
			fr.setReg(ir, fr.reg(ix) != fr.reg(iy))
		}
	case reflect.Array:
		if xtyp == ytyp {
			return func(fr *frame) {
				fr.setReg(ir, fr.reg(ix) != fr.reg(iy))
			}
		}
		return func(fr *frame) {
			x := fr.reg(ix)
			y := fr.reg(iy)
			fr.setReg(ir, x != reflect.ValueOf(y).Convert(xtyp).Interface())
		}
	case reflect.Struct:
		return func(fr *frame) {
			fr.setReg(ir, fr.reg(ix) != fr.reg(iy))
		}
	case reflect.UnsafePointer:
		return func(fr *frame) {
			fr.setReg(ir, fr.reg(ix) != fr.reg(iy))
		}
	case reflect.Chan:
		xdir := xtyp.ChanDir()
		ydir := ytyp.ChanDir()
		if xdir != ydir {
			if xdir == reflect.BothDir {
				return func(fr *frame) {
					x := fr.reg(ix)
					x = reflect.ValueOf(x).Convert(ytyp).Interface()
					fr.setReg(ir, x != fr.reg(iy))
				}
			} else if ydir == reflect.BothDir {
				return func(fr *frame) {
					y := fr.reg(iy)
					y = reflect.ValueOf(y).Convert(xtyp).Interface()
					fr.setReg(ir, fr.reg(ix) != y)
				}
			}
		}
		return func(fr *frame) {
			fr.setReg(ir, fr.reg(ix) != fr.reg(iy))
		}
	case reflect.Ptr:
		return func(fr *frame) {
			x := fr.reg(ix)
			y := fr.reg(iy)
			fr.setReg(ir, reflect.ValueOf(x).Pointer() != reflect.ValueOf(y).Pointer())
		}
	case reflect.Slice, reflect.Map, reflect.Func:
		return func(fr *frame) {
			x := fr.reg(ix)
			y := fr.reg(iy)
			b := reflect.ValueOf(x).IsNil() && reflect.ValueOf(y).IsNil()
			fr.setReg(ir, !b)
		}
	default:
		panic("unreachable")
	}
}
