
// This is generate file
package goscript

import (
	"fmt"
	"reflect"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)


func makeBinOpLSS(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	
	
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	
	typ := pfn.Interp.preToType(instr.X.Type())
	
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float32) < vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) < vm.reg(iy).(float32)) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float64) < vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) < vm.reg(iy).(float64)) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) < vy.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) < vm.reg(iy).(int)) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) < vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) < vm.reg(iy).(int16)) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) < vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) < vm.reg(iy).(int32)) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) < vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) < vm.reg(iy).(int64)) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) < vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) < vm.reg(iy).(int8)) }
			}
		
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(string) < vy.(string)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(string)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(string)) }
			} else if ky == kindConst {
				y := vy.(string)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(string) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(string) < vm.reg(iy).(string)) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) < vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) < vm.reg(iy).(uint)) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) < vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) < vm.reg(iy).(uint16)) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) < vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) < vm.reg(iy).(uint32)) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) < vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) < vm.reg(iy).(uint64)) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) < vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) < vm.reg(iy).(uint8)) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) < vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, x < vm.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) < y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) < vm.reg(iy).(uintptr)) }
			}
		
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float32) < vy.(float32)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.float32(iy))) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) < vm.float32(iy))) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float64) < vy.(float64)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.float64(iy))) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) < vm.float64(iy))) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) < vy.(int)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) < vm.int(iy))) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) < vy.(int16)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) < vm.int16(iy))) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) < vy.(int32)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) < vm.int32(iy))) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) < vy.(int64)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) < vm.int64(iy))) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) < vy.(int8)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) < vm.int8(iy))) }
			}
		
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(string) < vy.(string)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.String(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.string(iy))) }
			} else if ky == kindConst {
				y := xtype.String(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.string(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.string(ix) < vm.string(iy))) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) < vy.(uint)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) < vm.uint(iy))) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) < vy.(uint16)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) < vm.uint16(iy))) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) < vy.(uint32)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) < vm.uint32(iy))) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) < vy.(uint64)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) < vm.uint64(iy))) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) < vy.(uint8)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) < vm.uint8(iy))) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) < vy.(uintptr)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x < vm.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) < y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) < vm.uintptr(iy))) }
			}
		
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
}

func makeBinOpGEQ(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	
	
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	
	typ := pfn.Interp.preToType(instr.X.Type())
	
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float32) >= vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) >= vm.reg(iy).(float32)) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float64) >= vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) >= vm.reg(iy).(float64)) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) >= vy.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) >= vm.reg(iy).(int)) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) >= vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) >= vm.reg(iy).(int16)) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) >= vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) >= vm.reg(iy).(int32)) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) >= vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) >= vm.reg(iy).(int64)) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) >= vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) >= vm.reg(iy).(int8)) }
			}
		
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(string) >= vy.(string)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(string)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(string)) }
			} else if ky == kindConst {
				y := vy.(string)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(string) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(string) >= vm.reg(iy).(string)) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) >= vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) >= vm.reg(iy).(uint)) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) >= vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) >= vm.reg(iy).(uint16)) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) >= vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) >= vm.reg(iy).(uint32)) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) >= vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) >= vm.reg(iy).(uint64)) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) >= vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) >= vm.reg(iy).(uint8)) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) >= vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, x >= vm.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) >= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) >= vm.reg(iy).(uintptr)) }
			}
		
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float32) >= vy.(float32)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.float32(iy))) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) >= vm.float32(iy))) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float64) >= vy.(float64)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.float64(iy))) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) >= vm.float64(iy))) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) >= vy.(int)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) >= vm.int(iy))) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) >= vy.(int16)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) >= vm.int16(iy))) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) >= vy.(int32)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) >= vm.int32(iy))) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) >= vy.(int64)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) >= vm.int64(iy))) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) >= vy.(int8)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) >= vm.int8(iy))) }
			}
		
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(string) >= vy.(string)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.String(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.string(iy))) }
			} else if ky == kindConst {
				y := xtype.String(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.string(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.string(ix) >= vm.string(iy))) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) >= vy.(uint)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) >= vm.uint(iy))) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) >= vy.(uint16)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) >= vm.uint16(iy))) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) >= vy.(uint32)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) >= vm.uint32(iy))) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) >= vy.(uint64)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) >= vm.uint64(iy))) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) >= vy.(uint8)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) >= vm.uint8(iy))) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) >= vy.(uintptr)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x >= vm.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) >= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) >= vm.uintptr(iy))) }
			}
		
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
}

func makeBinOpSUB(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	
	
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	
	typ := pfn.Interp.preToType(instr.Type())
	
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(complex128) - vy.(complex128)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex128)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(complex128)) }
			} else if ky == kindConst {
				y := vy.(complex128)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex128) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex128) - vm.reg(iy).(complex128)) }
			}
		
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(complex64) - vy.(complex64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex64)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(complex64)) }
			} else if ky == kindConst {
				y := vy.(complex64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex64) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex64) - vm.reg(iy).(complex64)) }
			}
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float32) - vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) - vm.reg(iy).(float32)) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float64) - vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) - vm.reg(iy).(float64)) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) - vy.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) - vm.reg(iy).(int)) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) - vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) - vm.reg(iy).(int16)) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) - vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) - vm.reg(iy).(int32)) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) - vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) - vm.reg(iy).(int64)) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) - vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) - vm.reg(iy).(int8)) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) - vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) - vm.reg(iy).(uint)) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) - vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) - vm.reg(iy).(uint16)) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) - vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) - vm.reg(iy).(uint32)) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) - vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) - vm.reg(iy).(uint64)) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) - vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) - vm.reg(iy).(uint8)) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) - vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, x - vm.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) - y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) - vm.reg(iy).(uintptr)) }
			}
		
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Complex128(vx) - xtype.Complex128(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex128(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.complex128(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex128(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex128(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex128(ix) - vm.complex128(iy))) }
			}
		
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Complex64(vx) - xtype.Complex64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.complex64(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex64(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex64(ix) - vm.complex64(iy))) }
			}
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Float32(vx) - xtype.Float32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.float32(iy))) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) - vm.float32(iy))) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Float64(vx) - xtype.Float64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.float64(iy))) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) - vm.float64(iy))) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int(vx) - xtype.Int(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) - vm.int(iy))) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int16(vx) - xtype.Int16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) - vm.int16(iy))) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int32(vx) - xtype.Int32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) - vm.int32(iy))) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int64(vx) - xtype.Int64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) - vm.int64(iy))) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int8(vx) - xtype.Int8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) - vm.int8(iy))) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint(vx) - xtype.Uint(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) - vm.uint(iy))) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint16(vx) - xtype.Uint16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) - vm.uint16(iy))) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint32(vx) - xtype.Uint32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) - vm.uint32(iy))) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint64(vx) - xtype.Uint64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) - vm.uint64(iy))) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint8(vx) - xtype.Uint8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) - vm.uint8(iy))) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uintptr(vx) - xtype.Uintptr(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x - vm.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) - y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) - vm.uintptr(iy))) }
			}
		
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
}

func makeBinOpMUL(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	
	
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	
	typ := pfn.Interp.preToType(instr.Type())
	
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(complex128) * vy.(complex128)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex128)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(complex128)) }
			} else if ky == kindConst {
				y := vy.(complex128)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex128) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex128) * vm.reg(iy).(complex128)) }
			}
		
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(complex64) * vy.(complex64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex64)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(complex64)) }
			} else if ky == kindConst {
				y := vy.(complex64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex64) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex64) * vm.reg(iy).(complex64)) }
			}
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float32) * vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) * vm.reg(iy).(float32)) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float64) * vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) * vm.reg(iy).(float64)) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) * vy.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) * vm.reg(iy).(int)) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) * vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) * vm.reg(iy).(int16)) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) * vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) * vm.reg(iy).(int32)) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) * vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) * vm.reg(iy).(int64)) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) * vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) * vm.reg(iy).(int8)) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) * vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) * vm.reg(iy).(uint)) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) * vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) * vm.reg(iy).(uint16)) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) * vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) * vm.reg(iy).(uint32)) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) * vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) * vm.reg(iy).(uint64)) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) * vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) * vm.reg(iy).(uint8)) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) * vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, x * vm.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) * y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) * vm.reg(iy).(uintptr)) }
			}
		
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Complex128(vx) * xtype.Complex128(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex128(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.complex128(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex128(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex128(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex128(ix) * vm.complex128(iy))) }
			}
		
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Complex64(vx) * xtype.Complex64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.complex64(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex64(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex64(ix) * vm.complex64(iy))) }
			}
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Float32(vx) * xtype.Float32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.float32(iy))) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) * vm.float32(iy))) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Float64(vx) * xtype.Float64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.float64(iy))) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) * vm.float64(iy))) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int(vx) * xtype.Int(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) * vm.int(iy))) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int16(vx) * xtype.Int16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) * vm.int16(iy))) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int32(vx) * xtype.Int32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) * vm.int32(iy))) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int64(vx) * xtype.Int64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) * vm.int64(iy))) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int8(vx) * xtype.Int8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) * vm.int8(iy))) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint(vx) * xtype.Uint(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) * vm.uint(iy))) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint16(vx) * xtype.Uint16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) * vm.uint16(iy))) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint32(vx) * xtype.Uint32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) * vm.uint32(iy))) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint64(vx) * xtype.Uint64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) * vm.uint64(iy))) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint8(vx) * xtype.Uint8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) * vm.uint8(iy))) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uintptr(vx) * xtype.Uintptr(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x * vm.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) * y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) * vm.uintptr(iy))) }
			}
		
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
}

func makeBinOpREM(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	
	
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	
	typ := pfn.Interp.preToType(instr.Type())
	
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) % vy.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(vm *goVm) { vm.setReg(ir, x % vm.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) % y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) % vm.reg(iy).(int)) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) % vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, x % vm.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) % y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) % vm.reg(iy).(int16)) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) % vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, x % vm.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) % y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) % vm.reg(iy).(int32)) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) % vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, x % vm.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) % y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) % vm.reg(iy).(int64)) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) % vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, x % vm.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) % y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) % vm.reg(iy).(int8)) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) % vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, x % vm.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) % y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) % vm.reg(iy).(uint)) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) % vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, x % vm.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) % y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) % vm.reg(iy).(uint16)) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) % vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, x % vm.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) % y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) % vm.reg(iy).(uint32)) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) % vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, x % vm.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) % y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) % vm.reg(iy).(uint64)) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) % vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, x % vm.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) % y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) % vm.reg(iy).(uint8)) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) % vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, x % vm.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) % y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) % vm.reg(iy).(uintptr)) }
			}
		
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int(vx) % xtype.Int(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x % vm.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) % y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) % vm.int(iy))) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int16(vx) % xtype.Int16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x % vm.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) % y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) % vm.int16(iy))) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int32(vx) % xtype.Int32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x % vm.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) % y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) % vm.int32(iy))) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int64(vx) % xtype.Int64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x % vm.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) % y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) % vm.int64(iy))) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int8(vx) % xtype.Int8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x % vm.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) % y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) % vm.int8(iy))) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint(vx) % xtype.Uint(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x % vm.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) % y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) % vm.uint(iy))) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint16(vx) % xtype.Uint16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x % vm.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) % y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) % vm.uint16(iy))) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint32(vx) % xtype.Uint32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x % vm.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) % y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) % vm.uint32(iy))) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint64(vx) % xtype.Uint64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x % vm.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) % y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) % vm.uint64(iy))) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint8(vx) % xtype.Uint8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x % vm.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) % y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) % vm.uint8(iy))) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uintptr(vx) % xtype.Uintptr(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x % vm.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) % y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) % vm.uintptr(iy))) }
			}
		
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
}

func makeBinOpXOR(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	
	
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	
	typ := pfn.Interp.preToType(instr.Type())
	
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) ^ vy.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(vm *goVm) { vm.setReg(ir, x ^ vm.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) ^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) ^ vm.reg(iy).(int)) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) ^ vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, x ^ vm.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) ^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) ^ vm.reg(iy).(int16)) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) ^ vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, x ^ vm.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) ^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) ^ vm.reg(iy).(int32)) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) ^ vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, x ^ vm.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) ^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) ^ vm.reg(iy).(int64)) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) ^ vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, x ^ vm.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) ^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) ^ vm.reg(iy).(int8)) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) ^ vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, x ^ vm.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) ^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) ^ vm.reg(iy).(uint)) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) ^ vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, x ^ vm.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) ^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) ^ vm.reg(iy).(uint16)) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) ^ vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, x ^ vm.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) ^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) ^ vm.reg(iy).(uint32)) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) ^ vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, x ^ vm.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) ^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) ^ vm.reg(iy).(uint64)) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) ^ vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, x ^ vm.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) ^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) ^ vm.reg(iy).(uint8)) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) ^ vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, x ^ vm.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) ^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) ^ vm.reg(iy).(uintptr)) }
			}
		
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int(vx) ^ xtype.Int(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x ^ vm.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) ^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) ^ vm.int(iy))) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int16(vx) ^ xtype.Int16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x ^ vm.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) ^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) ^ vm.int16(iy))) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int32(vx) ^ xtype.Int32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x ^ vm.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) ^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) ^ vm.int32(iy))) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int64(vx) ^ xtype.Int64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x ^ vm.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) ^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) ^ vm.int64(iy))) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int8(vx) ^ xtype.Int8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x ^ vm.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) ^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) ^ vm.int8(iy))) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint(vx) ^ xtype.Uint(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x ^ vm.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) ^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) ^ vm.uint(iy))) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint16(vx) ^ xtype.Uint16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x ^ vm.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) ^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) ^ vm.uint16(iy))) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint32(vx) ^ xtype.Uint32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x ^ vm.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) ^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) ^ vm.uint32(iy))) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint64(vx) ^ xtype.Uint64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x ^ vm.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) ^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) ^ vm.uint64(iy))) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint8(vx) ^ xtype.Uint8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x ^ vm.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) ^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) ^ vm.uint8(iy))) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uintptr(vx) ^ xtype.Uintptr(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x ^ vm.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) ^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) ^ vm.uintptr(iy))) }
			}
		
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
}

func makeBinOpANDNOT(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	
	
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	
	typ := pfn.Interp.preToType(instr.Type())
	
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) &^ vy.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(vm *goVm) { vm.setReg(ir, x &^ vm.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) &^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) &^ vm.reg(iy).(int)) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) &^ vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, x &^ vm.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) &^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) &^ vm.reg(iy).(int16)) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) &^ vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, x &^ vm.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) &^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) &^ vm.reg(iy).(int32)) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) &^ vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, x &^ vm.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) &^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) &^ vm.reg(iy).(int64)) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) &^ vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, x &^ vm.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) &^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) &^ vm.reg(iy).(int8)) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) &^ vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, x &^ vm.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) &^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) &^ vm.reg(iy).(uint)) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) &^ vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, x &^ vm.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) &^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) &^ vm.reg(iy).(uint16)) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) &^ vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, x &^ vm.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) &^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) &^ vm.reg(iy).(uint32)) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) &^ vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, x &^ vm.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) &^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) &^ vm.reg(iy).(uint64)) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) &^ vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, x &^ vm.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) &^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) &^ vm.reg(iy).(uint8)) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) &^ vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, x &^ vm.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) &^ y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) &^ vm.reg(iy).(uintptr)) }
			}
		
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int(vx) &^ xtype.Int(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x &^ vm.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) &^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) &^ vm.int(iy))) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int16(vx) &^ xtype.Int16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x &^ vm.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) &^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) &^ vm.int16(iy))) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int32(vx) &^ xtype.Int32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x &^ vm.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) &^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) &^ vm.int32(iy))) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int64(vx) &^ xtype.Int64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x &^ vm.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) &^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) &^ vm.int64(iy))) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int8(vx) &^ xtype.Int8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x &^ vm.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) &^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) &^ vm.int8(iy))) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint(vx) &^ xtype.Uint(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x &^ vm.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) &^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) &^ vm.uint(iy))) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint16(vx) &^ xtype.Uint16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x &^ vm.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) &^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) &^ vm.uint16(iy))) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint32(vx) &^ xtype.Uint32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x &^ vm.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) &^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) &^ vm.uint32(iy))) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint64(vx) &^ xtype.Uint64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x &^ vm.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) &^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) &^ vm.uint64(iy))) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint8(vx) &^ xtype.Uint8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x &^ vm.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) &^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) &^ vm.uint8(iy))) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uintptr(vx) &^ xtype.Uintptr(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x &^ vm.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) &^ y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) &^ vm.uintptr(iy))) }
			}
		
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
}

func makeBinOpLEQ(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	
	
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	
	typ := pfn.Interp.preToType(instr.X.Type())
	
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float32) <= vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) <= vm.reg(iy).(float32)) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float64) <= vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) <= vm.reg(iy).(float64)) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) <= vy.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) <= vm.reg(iy).(int)) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) <= vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) <= vm.reg(iy).(int16)) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) <= vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) <= vm.reg(iy).(int32)) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) <= vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) <= vm.reg(iy).(int64)) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) <= vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) <= vm.reg(iy).(int8)) }
			}
		
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(string) <= vy.(string)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(string)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(string)) }
			} else if ky == kindConst {
				y := vy.(string)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(string) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(string) <= vm.reg(iy).(string)) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) <= vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) <= vm.reg(iy).(uint)) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) <= vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) <= vm.reg(iy).(uint16)) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) <= vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) <= vm.reg(iy).(uint32)) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) <= vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) <= vm.reg(iy).(uint64)) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) <= vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) <= vm.reg(iy).(uint8)) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) <= vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, x <= vm.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) <= y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) <= vm.reg(iy).(uintptr)) }
			}
		
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float32) <= vy.(float32)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.float32(iy))) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) <= vm.float32(iy))) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float64) <= vy.(float64)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.float64(iy))) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) <= vm.float64(iy))) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) <= vy.(int)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) <= vm.int(iy))) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) <= vy.(int16)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) <= vm.int16(iy))) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) <= vy.(int32)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) <= vm.int32(iy))) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) <= vy.(int64)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) <= vm.int64(iy))) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) <= vy.(int8)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) <= vm.int8(iy))) }
			}
		
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(string) <= vy.(string)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.String(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.string(iy))) }
			} else if ky == kindConst {
				y := xtype.String(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.string(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.string(ix) <= vm.string(iy))) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) <= vy.(uint)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) <= vm.uint(iy))) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) <= vy.(uint16)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) <= vm.uint16(iy))) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) <= vy.(uint32)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) <= vm.uint32(iy))) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) <= vy.(uint64)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) <= vm.uint64(iy))) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) <= vy.(uint8)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) <= vm.uint8(iy))) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) <= vy.(uintptr)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x <= vm.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) <= y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) <= vm.uintptr(iy))) }
			}
		
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
}

func makeBinOpGTR(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	
	
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	
	typ := pfn.Interp.preToType(instr.X.Type())
	
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float32) > vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) > vm.reg(iy).(float32)) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float64) > vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) > vm.reg(iy).(float64)) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) > vy.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) > vm.reg(iy).(int)) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) > vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) > vm.reg(iy).(int16)) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) > vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) > vm.reg(iy).(int32)) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) > vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) > vm.reg(iy).(int64)) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) > vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) > vm.reg(iy).(int8)) }
			}
		
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(string) > vy.(string)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(string)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(string)) }
			} else if ky == kindConst {
				y := vy.(string)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(string) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(string) > vm.reg(iy).(string)) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) > vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) > vm.reg(iy).(uint)) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) > vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) > vm.reg(iy).(uint16)) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) > vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) > vm.reg(iy).(uint32)) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) > vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) > vm.reg(iy).(uint64)) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) > vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) > vm.reg(iy).(uint8)) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) > vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, x > vm.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) > y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) > vm.reg(iy).(uintptr)) }
			}
		
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float32) > vy.(float32)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.float32(iy))) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) > vm.float32(iy))) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float64) > vy.(float64)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.float64(iy))) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) > vm.float64(iy))) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) > vy.(int)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) > vm.int(iy))) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) > vy.(int16)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) > vm.int16(iy))) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) > vy.(int32)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) > vm.int32(iy))) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) > vy.(int64)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) > vm.int64(iy))) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) > vy.(int8)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) > vm.int8(iy))) }
			}
		
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(string) > vy.(string)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.String(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.string(iy))) }
			} else if ky == kindConst {
				y := xtype.String(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.string(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.string(ix) > vm.string(iy))) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) > vy.(uint)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) > vm.uint(iy))) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) > vy.(uint16)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) > vm.uint16(iy))) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) > vy.(uint32)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) > vm.uint32(iy))) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) > vy.(uint64)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) > vm.uint64(iy))) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) > vy.(uint8)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) > vm.uint8(iy))) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) > vy.(uintptr)
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x > vm.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) > y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) > vm.uintptr(iy))) }
			}
		
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
}

func makeBinOpADD(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	
	
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	
	typ := pfn.Interp.preToType(instr.Type())
	
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(complex128) + vy.(complex128)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex128)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(complex128)) }
			} else if ky == kindConst {
				y := vy.(complex128)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex128) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex128) + vm.reg(iy).(complex128)) }
			}
		
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(complex64) + vy.(complex64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex64)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(complex64)) }
			} else if ky == kindConst {
				y := vy.(complex64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex64) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex64) + vm.reg(iy).(complex64)) }
			}
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float32) + vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) + vm.reg(iy).(float32)) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(float64) + vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) + vm.reg(iy).(float64)) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) + vy.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) + vm.reg(iy).(int)) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) + vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) + vm.reg(iy).(int16)) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) + vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) + vm.reg(iy).(int32)) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) + vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) + vm.reg(iy).(int64)) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) + vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) + vm.reg(iy).(int8)) }
			}
		
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(string) + vy.(string)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(string)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(string)) }
			} else if ky == kindConst {
				y := vy.(string)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(string) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(string) + vm.reg(iy).(string)) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) + vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) + vm.reg(iy).(uint)) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) + vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) + vm.reg(iy).(uint16)) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) + vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) + vm.reg(iy).(uint32)) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) + vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) + vm.reg(iy).(uint64)) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) + vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) + vm.reg(iy).(uint8)) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) + vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, x + vm.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) + y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) + vm.reg(iy).(uintptr)) }
			}
		
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Complex128(vx) + xtype.Complex128(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex128(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.complex128(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex128(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex128(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex128(ix) + vm.complex128(iy))) }
			}
		
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Complex64(vx) + xtype.Complex64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.complex64(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex64(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex64(ix) + vm.complex64(iy))) }
			}
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Float32(vx) + xtype.Float32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.float32(iy))) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) + vm.float32(iy))) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Float64(vx) + xtype.Float64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.float64(iy))) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) + vm.float64(iy))) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int(vx) + xtype.Int(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) + vm.int(iy))) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int16(vx) + xtype.Int16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) + vm.int16(iy))) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int32(vx) + xtype.Int32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) + vm.int32(iy))) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int64(vx) + xtype.Int64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) + vm.int64(iy))) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int8(vx) + xtype.Int8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) + vm.int8(iy))) }
			}
		
		case reflect.String:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.String(vx) + xtype.String(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.String(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.string(iy))) }
			} else if ky == kindConst {
				y := xtype.String(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.string(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.string(ix) + vm.string(iy))) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint(vx) + xtype.Uint(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) + vm.uint(iy))) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint16(vx) + xtype.Uint16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) + vm.uint16(iy))) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint32(vx) + xtype.Uint32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) + vm.uint32(iy))) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint64(vx) + xtype.Uint64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) + vm.uint64(iy))) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint8(vx) + xtype.Uint8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) + vm.uint8(iy))) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uintptr(vx) + xtype.Uintptr(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x + vm.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) + y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) + vm.uintptr(iy))) }
			}
		
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
}

func makeBinOpQUO(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	
	
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	
	typ := pfn.Interp.preToType(instr.Type())
	
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Complex128(vx)
				y := xtype.Complex128(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(complex128) / vy.(complex128)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex128)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(complex128)) }
			} else if ky == kindConst {
				y := vy.(complex128)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex128) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex128) / vm.reg(iy).(complex128)) }
			}
		
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Complex64(vx)
				y := xtype.Complex64(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(complex64) / vy.(complex64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(complex64)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(complex64)) }
			} else if ky == kindConst {
				y := vy.(complex64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex64) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(complex64) / vm.reg(iy).(complex64)) }
			}
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Float32(vx)
				y := xtype.Float32(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(float32) / vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float32)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(float32)) }
			} else if ky == kindConst {
				y := vy.(float32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float32) / vm.reg(iy).(float32)) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Float64(vx)
				y := xtype.Float64(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(float64) / vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(float64)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(float64)) }
			} else if ky == kindConst {
				y := vy.(float64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(float64) / vm.reg(iy).(float64)) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Int(vx)
				y := xtype.Int(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(int) / vy.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) / vm.reg(iy).(int)) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Int16(vx)
				y := xtype.Int16(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(int16) / vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) / vm.reg(iy).(int16)) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Int32(vx)
				y := xtype.Int32(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(int32) / vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) / vm.reg(iy).(int32)) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Int64(vx)
				y := xtype.Int64(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(int64) / vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) / vm.reg(iy).(int64)) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Int8(vx)
				y := xtype.Int8(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(int8) / vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) / vm.reg(iy).(int8)) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Uint(vx)
				y := xtype.Uint(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(uint) / vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) / vm.reg(iy).(uint)) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Uint16(vx)
				y := xtype.Uint16(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(uint16) / vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) / vm.reg(iy).(uint16)) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Uint32(vx)
				y := xtype.Uint32(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(uint32) / vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) / vm.reg(iy).(uint32)) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Uint64(vx)
				y := xtype.Uint64(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(uint64) / vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) / vm.reg(iy).(uint64)) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Uint8(vx)
				y := xtype.Uint8(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(uint8) / vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) / vm.reg(iy).(uint8)) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Uintptr(vx)
				y := xtype.Uintptr(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, x / y) }
				}
				
				v := vx.(uintptr) / vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, x / vm.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) / y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) / vm.reg(iy).(uintptr)) }
			}
		
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		
		case reflect.Complex128:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Complex128(vx)
				y := xtype.Complex128(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Complex128(vx) / xtype.Complex128(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex128(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.complex128(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex128(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex128(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex128(ix) / vm.complex128(iy))) }
			}
		
		case reflect.Complex64:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Complex64(vx)
				y := xtype.Complex64(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Complex64(vx) / xtype.Complex64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Complex64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.complex64(iy))) }
			} else if ky == kindConst {
				y := xtype.Complex64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex64(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.complex64(ix) / vm.complex64(iy))) }
			}
		
		case reflect.Float32:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Float32(vx)
				y := xtype.Float32(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Float32(vx) / xtype.Float32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.float32(iy))) }
			} else if ky == kindConst {
				y := xtype.Float32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float32(ix) / vm.float32(iy))) }
			}
		
		case reflect.Float64:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Float64(vx)
				y := xtype.Float64(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Float64(vx) / xtype.Float64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Float64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.float64(iy))) }
			} else if ky == kindConst {
				y := xtype.Float64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.float64(ix) / vm.float64(iy))) }
			}
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Int(vx)
				y := xtype.Int(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Int(vx) / xtype.Int(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) / vm.int(iy))) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Int16(vx)
				y := xtype.Int16(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Int16(vx) / xtype.Int16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) / vm.int16(iy))) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Int32(vx)
				y := xtype.Int32(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Int32(vx) / xtype.Int32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) / vm.int32(iy))) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Int64(vx)
				y := xtype.Int64(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Int64(vx) / xtype.Int64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) / vm.int64(iy))) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Int8(vx)
				y := xtype.Int8(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Int8(vx) / xtype.Int8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) / vm.int8(iy))) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Uint(vx)
				y := xtype.Uint(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Uint(vx) / xtype.Uint(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) / vm.uint(iy))) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Uint16(vx)
				y := xtype.Uint16(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Uint16(vx) / xtype.Uint16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) / vm.uint16(iy))) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Uint32(vx)
				y := xtype.Uint32(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Uint32(vx) / xtype.Uint32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) / vm.uint32(iy))) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Uint64(vx)
				y := xtype.Uint64(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Uint64(vx) / xtype.Uint64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) / vm.uint64(iy))) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Uint8(vx)
				y := xtype.Uint8(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Uint8(vx) / xtype.Uint8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) / vm.uint8(iy))) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				x := xtype.Uintptr(vx)
				y := xtype.Uintptr(vy)
				if y == 0 {
					return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / y)) }
				}
				v := xtype.Make(t, xtype.Uintptr(vx) / xtype.Uintptr(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x / vm.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) / y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) / vm.uintptr(iy))) }
			}
		
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
}

func makeBinOpAND(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	
	
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	
	typ := pfn.Interp.preToType(instr.Type())
	
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) & vy.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(vm *goVm) { vm.setReg(ir, x & vm.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) & y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) & vm.reg(iy).(int)) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) & vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, x & vm.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) & y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) & vm.reg(iy).(int16)) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) & vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, x & vm.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) & y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) & vm.reg(iy).(int32)) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) & vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, x & vm.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) & y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) & vm.reg(iy).(int64)) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) & vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, x & vm.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) & y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) & vm.reg(iy).(int8)) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) & vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, x & vm.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) & y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) & vm.reg(iy).(uint)) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) & vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, x & vm.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) & y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) & vm.reg(iy).(uint16)) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) & vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, x & vm.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) & y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) & vm.reg(iy).(uint32)) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) & vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, x & vm.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) & y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) & vm.reg(iy).(uint64)) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) & vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, x & vm.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) & y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) & vm.reg(iy).(uint8)) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) & vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, x & vm.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) & y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) & vm.reg(iy).(uintptr)) }
			}
		
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int(vx) & xtype.Int(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x & vm.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) & y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) & vm.int(iy))) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int16(vx) & xtype.Int16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x & vm.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) & y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) & vm.int16(iy))) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int32(vx) & xtype.Int32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x & vm.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) & y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) & vm.int32(iy))) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int64(vx) & xtype.Int64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x & vm.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) & y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) & vm.int64(iy))) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int8(vx) & xtype.Int8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x & vm.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) & y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) & vm.int8(iy))) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint(vx) & xtype.Uint(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x & vm.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) & y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) & vm.uint(iy))) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint16(vx) & xtype.Uint16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x & vm.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) & y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) & vm.uint16(iy))) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint32(vx) & xtype.Uint32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x & vm.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) & y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) & vm.uint32(iy))) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint64(vx) & xtype.Uint64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x & vm.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) & y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) & vm.uint64(iy))) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint8(vx) & xtype.Uint8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x & vm.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) & y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) & vm.uint8(iy))) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uintptr(vx) & xtype.Uintptr(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x & vm.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) & y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) & vm.uintptr(iy))) }
			}
		
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
}

func makeBinOpOR(pfn *function, instr *ssa.BinOp) func(vm *goVm) {
	
	
	ir := pfn.regIndex(instr)
	ix, kx, vx := pfn.regIndex3(instr.X)
	iy, ky, vy := pfn.regIndex3(instr.Y)
	
	typ := pfn.Interp.preToType(instr.Type())
	
	if typ.PkgPath() == "" {
		switch typ.Kind() {
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int) | vy.(int)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int)
				return func(vm *goVm) { vm.setReg(ir, x | vm.reg(iy).(int)) }
			} else if ky == kindConst {
				y := vy.(int)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) | y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int) | vm.reg(iy).(int)) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int16) | vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int16)
				return func(vm *goVm) { vm.setReg(ir, x | vm.reg(iy).(int16)) }
			} else if ky == kindConst {
				y := vy.(int16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) | y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int16) | vm.reg(iy).(int16)) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int32) | vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int32)
				return func(vm *goVm) { vm.setReg(ir, x | vm.reg(iy).(int32)) }
			} else if ky == kindConst {
				y := vy.(int32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) | y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int32) | vm.reg(iy).(int32)) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int64) | vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int64)
				return func(vm *goVm) { vm.setReg(ir, x | vm.reg(iy).(int64)) }
			} else if ky == kindConst {
				y := vy.(int64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) | y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int64) | vm.reg(iy).(int64)) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(int8) | vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(int8)
				return func(vm *goVm) { vm.setReg(ir, x | vm.reg(iy).(int8)) }
			} else if ky == kindConst {
				y := vy.(int8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) | y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(int8) | vm.reg(iy).(int8)) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint) | vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint)
				return func(vm *goVm) { vm.setReg(ir, x | vm.reg(iy).(uint)) }
			} else if ky == kindConst {
				y := vy.(uint)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) | y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint) | vm.reg(iy).(uint)) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint16) | vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint16)
				return func(vm *goVm) { vm.setReg(ir, x | vm.reg(iy).(uint16)) }
			} else if ky == kindConst {
				y := vy.(uint16)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) | y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint16) | vm.reg(iy).(uint16)) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint32) | vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint32)
				return func(vm *goVm) { vm.setReg(ir, x | vm.reg(iy).(uint32)) }
			} else if ky == kindConst {
				y := vy.(uint32)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) | y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint32) | vm.reg(iy).(uint32)) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint64) | vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint64)
				return func(vm *goVm) { vm.setReg(ir, x | vm.reg(iy).(uint64)) }
			} else if ky == kindConst {
				y := vy.(uint64)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) | y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint64) | vm.reg(iy).(uint64)) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uint8) | vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uint8)
				return func(vm *goVm) { vm.setReg(ir, x | vm.reg(iy).(uint8)) }
			} else if ky == kindConst {
				y := vy.(uint8)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) | y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uint8) | vm.reg(iy).(uint8)) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := vx.(uintptr) | vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := vx.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, x | vm.reg(iy).(uintptr)) }
			} else if ky == kindConst {
				y := vy.(uintptr)
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) | y) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, vm.reg(ix).(uintptr) | vm.reg(iy).(uintptr)) }
			}
		
		}
	} else {
		t := xtype.TypeOfType(typ)
		switch typ.Kind() {
		
		case reflect.Int:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int(vx) | xtype.Int(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x | vm.int(iy))) }
			} else if ky == kindConst {
				y := xtype.Int(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) | y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int(ix) | vm.int(iy))) }
			}
		
		case reflect.Int16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int16(vx) | xtype.Int16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x | vm.int16(iy))) }
			} else if ky == kindConst {
				y := xtype.Int16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) | y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int16(ix) | vm.int16(iy))) }
			}
		
		case reflect.Int32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int32(vx) | xtype.Int32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x | vm.int32(iy))) }
			} else if ky == kindConst {
				y := xtype.Int32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) | y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int32(ix) | vm.int32(iy))) }
			}
		
		case reflect.Int64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int64(vx) | xtype.Int64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x | vm.int64(iy))) }
			} else if ky == kindConst {
				y := xtype.Int64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) | y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int64(ix) | vm.int64(iy))) }
			}
		
		case reflect.Int8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Int8(vx) | xtype.Int8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Int8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x | vm.int8(iy))) }
			} else if ky == kindConst {
				y := xtype.Int8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) | y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.int8(ix) | vm.int8(iy))) }
			}
		
		case reflect.Uint:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint(vx) | xtype.Uint(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x | vm.uint(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) | y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint(ix) | vm.uint(iy))) }
			}
		
		case reflect.Uint16:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint16(vx) | xtype.Uint16(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint16(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x | vm.uint16(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint16(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) | y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint16(ix) | vm.uint16(iy))) }
			}
		
		case reflect.Uint32:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint32(vx) | xtype.Uint32(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint32(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x | vm.uint32(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint32(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) | y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint32(ix) | vm.uint32(iy))) }
			}
		
		case reflect.Uint64:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint64(vx) | xtype.Uint64(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint64(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x | vm.uint64(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint64(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) | y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint64(ix) | vm.uint64(iy))) }
			}
		
		case reflect.Uint8:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uint8(vx) | xtype.Uint8(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uint8(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x | vm.uint8(iy))) }
			} else if ky == kindConst {
				y := xtype.Uint8(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) | y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uint8(ix) | vm.uint8(iy))) }
			}
		
		case reflect.Uintptr:
			if kx == kindConst && ky == kindConst {
				
				v := xtype.Make(t, xtype.Uintptr(vx) | xtype.Uintptr(vy))
				
				return func(vm *goVm) { vm.setReg(ir, v) }
			} else if kx == kindConst {
				x := xtype.Uintptr(vx)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, x | vm.uintptr(iy))) }
			} else if ky == kindConst {
				y := xtype.Uintptr(vy)
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) | y)) }
			} else {
				return func(vm *goVm) { vm.setReg(ir, xtype.Make(t, vm.uintptr(ix) | vm.uintptr(iy))) }
			}
		
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", typ.Kind()))
}
