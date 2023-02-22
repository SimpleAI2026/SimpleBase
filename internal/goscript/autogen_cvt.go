
// This is generate file
package goscript

import (
	"fmt"
	"reflect"

	"github.com/visualfc/xtype"
	"golang.org/x/tools/go/ssa"
)



func cvtFloat32(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = float32
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
			
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			
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
			
			case reflect.Float32:
				v = T(vm.float32(ix))
			
			case reflect.Float64:
				v = T(vm.float64(ix))
			
			case reflect.Int:
				v = T(vm.int(ix))
			
			case reflect.Int16:
				v = T(vm.int16(ix))
			
			case reflect.Int32:
				v = T(vm.int32(ix))
			
			case reflect.Int64:
				v = T(vm.int64(ix))
			
			case reflect.Int8:
				v = T(vm.int8(ix))
			
			case reflect.Uint:
				v = T(vm.uint(ix))
			
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func cvtFloat64(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = float64
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
			
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			
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
			
			case reflect.Float32:
				v = T(vm.float32(ix))
			
			case reflect.Float64:
				v = T(vm.float64(ix))
			
			case reflect.Int:
				v = T(vm.int(ix))
			
			case reflect.Int16:
				v = T(vm.int16(ix))
			
			case reflect.Int32:
				v = T(vm.int32(ix))
			
			case reflect.Int64:
				v = T(vm.int64(ix))
			
			case reflect.Int8:
				v = T(vm.int8(ix))
			
			case reflect.Uint:
				v = T(vm.uint(ix))
			
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func cvtInt(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = int
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
			
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			
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
			
			case reflect.Float32:
				v = T(vm.float32(ix))
			
			case reflect.Float64:
				v = T(vm.float64(ix))
			
			case reflect.Int:
				v = T(vm.int(ix))
			
			case reflect.Int16:
				v = T(vm.int16(ix))
			
			case reflect.Int32:
				v = T(vm.int32(ix))
			
			case reflect.Int64:
				v = T(vm.int64(ix))
			
			case reflect.Int8:
				v = T(vm.int8(ix))
			
			case reflect.Uint:
				v = T(vm.uint(ix))
			
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func cvtInt16(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = int16
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
			
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			
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
			
			case reflect.Float32:
				v = T(vm.float32(ix))
			
			case reflect.Float64:
				v = T(vm.float64(ix))
			
			case reflect.Int:
				v = T(vm.int(ix))
			
			case reflect.Int16:
				v = T(vm.int16(ix))
			
			case reflect.Int32:
				v = T(vm.int32(ix))
			
			case reflect.Int64:
				v = T(vm.int64(ix))
			
			case reflect.Int8:
				v = T(vm.int8(ix))
			
			case reflect.Uint:
				v = T(vm.uint(ix))
			
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func cvtInt32(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = int32
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
			
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			
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
			
			case reflect.Float32:
				v = T(vm.float32(ix))
			
			case reflect.Float64:
				v = T(vm.float64(ix))
			
			case reflect.Int:
				v = T(vm.int(ix))
			
			case reflect.Int16:
				v = T(vm.int16(ix))
			
			case reflect.Int32:
				v = T(vm.int32(ix))
			
			case reflect.Int64:
				v = T(vm.int64(ix))
			
			case reflect.Int8:
				v = T(vm.int8(ix))
			
			case reflect.Uint:
				v = T(vm.uint(ix))
			
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func cvtInt64(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = int64
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
			
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			
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
			
			case reflect.Float32:
				v = T(vm.float32(ix))
			
			case reflect.Float64:
				v = T(vm.float64(ix))
			
			case reflect.Int:
				v = T(vm.int(ix))
			
			case reflect.Int16:
				v = T(vm.int16(ix))
			
			case reflect.Int32:
				v = T(vm.int32(ix))
			
			case reflect.Int64:
				v = T(vm.int64(ix))
			
			case reflect.Int8:
				v = T(vm.int8(ix))
			
			case reflect.Uint:
				v = T(vm.uint(ix))
			
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func cvtInt8(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = int8
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
			
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			
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
			
			case reflect.Float32:
				v = T(vm.float32(ix))
			
			case reflect.Float64:
				v = T(vm.float64(ix))
			
			case reflect.Int:
				v = T(vm.int(ix))
			
			case reflect.Int16:
				v = T(vm.int16(ix))
			
			case reflect.Int32:
				v = T(vm.int32(ix))
			
			case reflect.Int64:
				v = T(vm.int64(ix))
			
			case reflect.Int8:
				v = T(vm.int8(ix))
			
			case reflect.Uint:
				v = T(vm.uint(ix))
			
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func cvtUint(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = uint
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
			
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			
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
			
			case reflect.Float32:
				v = T(vm.float32(ix))
			
			case reflect.Float64:
				v = T(vm.float64(ix))
			
			case reflect.Int:
				v = T(vm.int(ix))
			
			case reflect.Int16:
				v = T(vm.int16(ix))
			
			case reflect.Int32:
				v = T(vm.int32(ix))
			
			case reflect.Int64:
				v = T(vm.int64(ix))
			
			case reflect.Int8:
				v = T(vm.int8(ix))
			
			case reflect.Uint:
				v = T(vm.uint(ix))
			
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func cvtUint16(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = uint16
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
			
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			
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
			
			case reflect.Float32:
				v = T(vm.float32(ix))
			
			case reflect.Float64:
				v = T(vm.float64(ix))
			
			case reflect.Int:
				v = T(vm.int(ix))
			
			case reflect.Int16:
				v = T(vm.int16(ix))
			
			case reflect.Int32:
				v = T(vm.int32(ix))
			
			case reflect.Int64:
				v = T(vm.int64(ix))
			
			case reflect.Int8:
				v = T(vm.int8(ix))
			
			case reflect.Uint:
				v = T(vm.uint(ix))
			
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func cvtUint32(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = uint32
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
			
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			
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
			
			case reflect.Float32:
				v = T(vm.float32(ix))
			
			case reflect.Float64:
				v = T(vm.float64(ix))
			
			case reflect.Int:
				v = T(vm.int(ix))
			
			case reflect.Int16:
				v = T(vm.int16(ix))
			
			case reflect.Int32:
				v = T(vm.int32(ix))
			
			case reflect.Int64:
				v = T(vm.int64(ix))
			
			case reflect.Int8:
				v = T(vm.int8(ix))
			
			case reflect.Uint:
				v = T(vm.uint(ix))
			
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func cvtUint64(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = uint64
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
			
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			
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
			
			case reflect.Float32:
				v = T(vm.float32(ix))
			
			case reflect.Float64:
				v = T(vm.float64(ix))
			
			case reflect.Int:
				v = T(vm.int(ix))
			
			case reflect.Int16:
				v = T(vm.int16(ix))
			
			case reflect.Int32:
				v = T(vm.int32(ix))
			
			case reflect.Int64:
				v = T(vm.int64(ix))
			
			case reflect.Int8:
				v = T(vm.int8(ix))
			
			case reflect.Uint:
				v = T(vm.uint(ix))
			
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func cvtUint8(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = uint8
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
			
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			
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
			
			case reflect.Float32:
				v = T(vm.float32(ix))
			
			case reflect.Float64:
				v = T(vm.float64(ix))
			
			case reflect.Int:
				v = T(vm.int(ix))
			
			case reflect.Int16:
				v = T(vm.int16(ix))
			
			case reflect.Int32:
				v = T(vm.int32(ix))
			
			case reflect.Int64:
				v = T(vm.int64(ix))
			
			case reflect.Int8:
				v = T(vm.int8(ix))
			
			case reflect.Uint:
				v = T(vm.uint(ix))
			
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}

func cvtUintptr(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = uintptr
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
			
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			
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
			
			case reflect.Float32:
				v = T(vm.float32(ix))
			
			case reflect.Float64:
				v = T(vm.float64(ix))
			
			case reflect.Int:
				v = T(vm.int(ix))
			
			case reflect.Int16:
				v = T(vm.int16(ix))
			
			case reflect.Int32:
				v = T(vm.int32(ix))
			
			case reflect.Int64:
				v = T(vm.int64(ix))
			
			case reflect.Int8:
				v = T(vm.int8(ix))
			
			case reflect.Uint:
				v = T(vm.uint(ix))
			
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}



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
	case reflect.UnsafePointer:
		t := xtype.TypeOfType(typ)
		return func(vm *goVm) {
			x := vm.reg(ix)
			vm.setReg(ir, xtype.ConvertPtr(t, x))
		}
	}
	panic("unreachable kind: " + fmt.Sprintf("%v", kind))
}

