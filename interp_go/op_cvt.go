package interp_go

import (
	"reflect"

	"github.com/visualfc/xtype"
)

func cvtInt(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = int
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
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
			case reflect.Int:
				v = T(vm.int(ix))
			case reflect.Int8:
				v = T(vm.int8(ix))
			case reflect.Int16:
				v = T(vm.int16(ix))
			case reflect.Int32:
				v = T(vm.int32(ix))
			case reflect.Int64:
				v = T(vm.int64(ix))
			case reflect.Uint:
				v = T(vm.uint(ix))
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			case reflect.Float32:
				v = T(vm.float32(ix))
			case reflect.Float64:
				v = T(vm.float64(ix))
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
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
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
			case reflect.Int:
				v = T(vm.int(ix))
			case reflect.Int8:
				v = T(vm.int8(ix))
			case reflect.Int16:
				v = T(vm.int16(ix))
			case reflect.Int32:
				v = T(vm.int32(ix))
			case reflect.Int64:
				v = T(vm.int64(ix))
			case reflect.Uint:
				v = T(vm.uint(ix))
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			case reflect.Float32:
				v = T(vm.float32(ix))
			case reflect.Float64:
				v = T(vm.float64(ix))
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
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
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
			case reflect.Int:
				v = T(vm.int(ix))
			case reflect.Int8:
				v = T(vm.int8(ix))
			case reflect.Int16:
				v = T(vm.int16(ix))
			case reflect.Int32:
				v = T(vm.int32(ix))
			case reflect.Int64:
				v = T(vm.int64(ix))
			case reflect.Uint:
				v = T(vm.uint(ix))
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			case reflect.Float32:
				v = T(vm.float32(ix))
			case reflect.Float64:
				v = T(vm.float64(ix))
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
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
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
			case reflect.Int:
				v = T(vm.int(ix))
			case reflect.Int8:
				v = T(vm.int8(ix))
			case reflect.Int16:
				v = T(vm.int16(ix))
			case reflect.Int32:
				v = T(vm.int32(ix))
			case reflect.Int64:
				v = T(vm.int64(ix))
			case reflect.Uint:
				v = T(vm.uint(ix))
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			case reflect.Float32:
				v = T(vm.float32(ix))
			case reflect.Float64:
				v = T(vm.float64(ix))
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
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
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
			case reflect.Int:
				v = T(vm.int(ix))
			case reflect.Int8:
				v = T(vm.int8(ix))
			case reflect.Int16:
				v = T(vm.int16(ix))
			case reflect.Int32:
				v = T(vm.int32(ix))
			case reflect.Int64:
				v = T(vm.int64(ix))
			case reflect.Uint:
				v = T(vm.uint(ix))
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			case reflect.Float32:
				v = T(vm.float32(ix))
			case reflect.Float64:
				v = T(vm.float64(ix))
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
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
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
			case reflect.Int:
				v = T(vm.int(ix))
			case reflect.Int8:
				v = T(vm.int8(ix))
			case reflect.Int16:
				v = T(vm.int16(ix))
			case reflect.Int32:
				v = T(vm.int32(ix))
			case reflect.Int64:
				v = T(vm.int64(ix))
			case reflect.Uint:
				v = T(vm.uint(ix))
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			case reflect.Float32:
				v = T(vm.float32(ix))
			case reflect.Float64:
				v = T(vm.float64(ix))
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
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
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
			case reflect.Int:
				v = T(vm.int(ix))
			case reflect.Int8:
				v = T(vm.int8(ix))
			case reflect.Int16:
				v = T(vm.int16(ix))
			case reflect.Int32:
				v = T(vm.int32(ix))
			case reflect.Int64:
				v = T(vm.int64(ix))
			case reflect.Uint:
				v = T(vm.uint(ix))
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			case reflect.Float32:
				v = T(vm.float32(ix))
			case reflect.Float64:
				v = T(vm.float64(ix))
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
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
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
			case reflect.Int:
				v = T(vm.int(ix))
			case reflect.Int8:
				v = T(vm.int8(ix))
			case reflect.Int16:
				v = T(vm.int16(ix))
			case reflect.Int32:
				v = T(vm.int32(ix))
			case reflect.Int64:
				v = T(vm.int64(ix))
			case reflect.Uint:
				v = T(vm.uint(ix))
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			case reflect.Float32:
				v = T(vm.float32(ix))
			case reflect.Float64:
				v = T(vm.float64(ix))
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
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
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
			case reflect.Int:
				v = T(vm.int(ix))
			case reflect.Int8:
				v = T(vm.int8(ix))
			case reflect.Int16:
				v = T(vm.int16(ix))
			case reflect.Int32:
				v = T(vm.int32(ix))
			case reflect.Int64:
				v = T(vm.int64(ix))
			case reflect.Uint:
				v = T(vm.uint(ix))
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			case reflect.Float32:
				v = T(vm.float32(ix))
			case reflect.Float64:
				v = T(vm.float64(ix))
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
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
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
			case reflect.Int:
				v = T(vm.int(ix))
			case reflect.Int8:
				v = T(vm.int8(ix))
			case reflect.Int16:
				v = T(vm.int16(ix))
			case reflect.Int32:
				v = T(vm.int32(ix))
			case reflect.Int64:
				v = T(vm.int64(ix))
			case reflect.Uint:
				v = T(vm.uint(ix))
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			case reflect.Float32:
				v = T(vm.float32(ix))
			case reflect.Float64:
				v = T(vm.float64(ix))
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
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
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
			case reflect.Int:
				v = T(vm.int(ix))
			case reflect.Int8:
				v = T(vm.int8(ix))
			case reflect.Int16:
				v = T(vm.int16(ix))
			case reflect.Int32:
				v = T(vm.int32(ix))
			case reflect.Int64:
				v = T(vm.int64(ix))
			case reflect.Uint:
				v = T(vm.uint(ix))
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			case reflect.Float32:
				v = T(vm.float32(ix))
			case reflect.Float64:
				v = T(vm.float64(ix))
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtFloat32(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(vm *goVm) {
	type T = float32
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(vm *goVm) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
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
			case reflect.Int:
				v = T(vm.int(ix))
			case reflect.Int8:
				v = T(vm.int8(ix))
			case reflect.Int16:
				v = T(vm.int16(ix))
			case reflect.Int32:
				v = T(vm.int32(ix))
			case reflect.Int64:
				v = T(vm.int64(ix))
			case reflect.Uint:
				v = T(vm.uint(ix))
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			case reflect.Float32:
				v = T(vm.float32(ix))
			case reflect.Float64:
				v = T(vm.float64(ix))
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
			case reflect.Int:
				v = T(vm.reg(ix).(int))
			case reflect.Int8:
				v = T(vm.reg(ix).(int8))
			case reflect.Int16:
				v = T(vm.reg(ix).(int16))
			case reflect.Int32:
				v = T(vm.reg(ix).(int32))
			case reflect.Int64:
				v = T(vm.reg(ix).(int64))
			case reflect.Uint:
				v = T(vm.reg(ix).(uint))
			case reflect.Uint8:
				v = T(vm.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(vm.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(vm.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(vm.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(vm.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(vm.reg(ix).(float32))
			case reflect.Float64:
				v = T(vm.reg(ix).(float64))
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
			case reflect.Int:
				v = T(vm.int(ix))
			case reflect.Int8:
				v = T(vm.int8(ix))
			case reflect.Int16:
				v = T(vm.int16(ix))
			case reflect.Int32:
				v = T(vm.int32(ix))
			case reflect.Int64:
				v = T(vm.int64(ix))
			case reflect.Uint:
				v = T(vm.uint(ix))
			case reflect.Uint8:
				v = T(vm.uint8(ix))
			case reflect.Uint16:
				v = T(vm.uint16(ix))
			case reflect.Uint32:
				v = T(vm.uint32(ix))
			case reflect.Uint64:
				v = T(vm.uint64(ix))
			case reflect.Uintptr:
				v = T(vm.uintptr(ix))
			case reflect.Float32:
				v = T(vm.float32(ix))
			case reflect.Float64:
				v = T(vm.float64(ix))
			}
			if isBasic {
				vm.setReg(ir, v)
			} else {
				vm.setReg(ir, xtype.Make(t, v))
			}
		}
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
