package interp

import (
	"reflect"

	"github.com/visualfc/xtype"
)

func cvtInt(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = int
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.reg(ix).(int))
			case reflect.Int8:
				v = T(fr.reg(ix).(int8))
			case reflect.Int16:
				v = T(fr.reg(ix).(int16))
			case reflect.Int32:
				v = T(fr.reg(ix).(int32))
			case reflect.Int64:
				v = T(fr.reg(ix).(int64))
			case reflect.Uint:
				v = T(fr.reg(ix).(uint))
			case reflect.Uint8:
				v = T(fr.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(fr.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(fr.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(fr.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(fr.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(fr.reg(ix).(float32))
			case reflect.Float64:
				v = T(fr.reg(ix).(float64))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.int(ix))
			case reflect.Int8:
				v = T(fr.int8(ix))
			case reflect.Int16:
				v = T(fr.int16(ix))
			case reflect.Int32:
				v = T(fr.int32(ix))
			case reflect.Int64:
				v = T(fr.int64(ix))
			case reflect.Uint:
				v = T(fr.uint(ix))
			case reflect.Uint8:
				v = T(fr.uint8(ix))
			case reflect.Uint16:
				v = T(fr.uint16(ix))
			case reflect.Uint32:
				v = T(fr.uint32(ix))
			case reflect.Uint64:
				v = T(fr.uint64(ix))
			case reflect.Uintptr:
				v = T(fr.uintptr(ix))
			case reflect.Float32:
				v = T(fr.float32(ix))
			case reflect.Float64:
				v = T(fr.float64(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtInt8(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = int8
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.reg(ix).(int))
			case reflect.Int8:
				v = T(fr.reg(ix).(int8))
			case reflect.Int16:
				v = T(fr.reg(ix).(int16))
			case reflect.Int32:
				v = T(fr.reg(ix).(int32))
			case reflect.Int64:
				v = T(fr.reg(ix).(int64))
			case reflect.Uint:
				v = T(fr.reg(ix).(uint))
			case reflect.Uint8:
				v = T(fr.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(fr.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(fr.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(fr.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(fr.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(fr.reg(ix).(float32))
			case reflect.Float64:
				v = T(fr.reg(ix).(float64))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.int(ix))
			case reflect.Int8:
				v = T(fr.int8(ix))
			case reflect.Int16:
				v = T(fr.int16(ix))
			case reflect.Int32:
				v = T(fr.int32(ix))
			case reflect.Int64:
				v = T(fr.int64(ix))
			case reflect.Uint:
				v = T(fr.uint(ix))
			case reflect.Uint8:
				v = T(fr.uint8(ix))
			case reflect.Uint16:
				v = T(fr.uint16(ix))
			case reflect.Uint32:
				v = T(fr.uint32(ix))
			case reflect.Uint64:
				v = T(fr.uint64(ix))
			case reflect.Uintptr:
				v = T(fr.uintptr(ix))
			case reflect.Float32:
				v = T(fr.float32(ix))
			case reflect.Float64:
				v = T(fr.float64(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtInt16(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = int16
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.reg(ix).(int))
			case reflect.Int8:
				v = T(fr.reg(ix).(int8))
			case reflect.Int16:
				v = T(fr.reg(ix).(int16))
			case reflect.Int32:
				v = T(fr.reg(ix).(int32))
			case reflect.Int64:
				v = T(fr.reg(ix).(int64))
			case reflect.Uint:
				v = T(fr.reg(ix).(uint))
			case reflect.Uint8:
				v = T(fr.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(fr.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(fr.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(fr.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(fr.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(fr.reg(ix).(float32))
			case reflect.Float64:
				v = T(fr.reg(ix).(float64))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.int(ix))
			case reflect.Int8:
				v = T(fr.int8(ix))
			case reflect.Int16:
				v = T(fr.int16(ix))
			case reflect.Int32:
				v = T(fr.int32(ix))
			case reflect.Int64:
				v = T(fr.int64(ix))
			case reflect.Uint:
				v = T(fr.uint(ix))
			case reflect.Uint8:
				v = T(fr.uint8(ix))
			case reflect.Uint16:
				v = T(fr.uint16(ix))
			case reflect.Uint32:
				v = T(fr.uint32(ix))
			case reflect.Uint64:
				v = T(fr.uint64(ix))
			case reflect.Uintptr:
				v = T(fr.uintptr(ix))
			case reflect.Float32:
				v = T(fr.float32(ix))
			case reflect.Float64:
				v = T(fr.float64(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtInt32(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = int32
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.reg(ix).(int))
			case reflect.Int8:
				v = T(fr.reg(ix).(int8))
			case reflect.Int16:
				v = T(fr.reg(ix).(int16))
			case reflect.Int32:
				v = T(fr.reg(ix).(int32))
			case reflect.Int64:
				v = T(fr.reg(ix).(int64))
			case reflect.Uint:
				v = T(fr.reg(ix).(uint))
			case reflect.Uint8:
				v = T(fr.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(fr.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(fr.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(fr.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(fr.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(fr.reg(ix).(float32))
			case reflect.Float64:
				v = T(fr.reg(ix).(float64))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.int(ix))
			case reflect.Int8:
				v = T(fr.int8(ix))
			case reflect.Int16:
				v = T(fr.int16(ix))
			case reflect.Int32:
				v = T(fr.int32(ix))
			case reflect.Int64:
				v = T(fr.int64(ix))
			case reflect.Uint:
				v = T(fr.uint(ix))
			case reflect.Uint8:
				v = T(fr.uint8(ix))
			case reflect.Uint16:
				v = T(fr.uint16(ix))
			case reflect.Uint32:
				v = T(fr.uint32(ix))
			case reflect.Uint64:
				v = T(fr.uint64(ix))
			case reflect.Uintptr:
				v = T(fr.uintptr(ix))
			case reflect.Float32:
				v = T(fr.float32(ix))
			case reflect.Float64:
				v = T(fr.float64(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtInt64(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = int64
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.reg(ix).(int))
			case reflect.Int8:
				v = T(fr.reg(ix).(int8))
			case reflect.Int16:
				v = T(fr.reg(ix).(int16))
			case reflect.Int32:
				v = T(fr.reg(ix).(int32))
			case reflect.Int64:
				v = T(fr.reg(ix).(int64))
			case reflect.Uint:
				v = T(fr.reg(ix).(uint))
			case reflect.Uint8:
				v = T(fr.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(fr.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(fr.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(fr.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(fr.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(fr.reg(ix).(float32))
			case reflect.Float64:
				v = T(fr.reg(ix).(float64))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.int(ix))
			case reflect.Int8:
				v = T(fr.int8(ix))
			case reflect.Int16:
				v = T(fr.int16(ix))
			case reflect.Int32:
				v = T(fr.int32(ix))
			case reflect.Int64:
				v = T(fr.int64(ix))
			case reflect.Uint:
				v = T(fr.uint(ix))
			case reflect.Uint8:
				v = T(fr.uint8(ix))
			case reflect.Uint16:
				v = T(fr.uint16(ix))
			case reflect.Uint32:
				v = T(fr.uint32(ix))
			case reflect.Uint64:
				v = T(fr.uint64(ix))
			case reflect.Uintptr:
				v = T(fr.uintptr(ix))
			case reflect.Float32:
				v = T(fr.float32(ix))
			case reflect.Float64:
				v = T(fr.float64(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtUint(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = uint
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.reg(ix).(int))
			case reflect.Int8:
				v = T(fr.reg(ix).(int8))
			case reflect.Int16:
				v = T(fr.reg(ix).(int16))
			case reflect.Int32:
				v = T(fr.reg(ix).(int32))
			case reflect.Int64:
				v = T(fr.reg(ix).(int64))
			case reflect.Uint:
				v = T(fr.reg(ix).(uint))
			case reflect.Uint8:
				v = T(fr.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(fr.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(fr.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(fr.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(fr.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(fr.reg(ix).(float32))
			case reflect.Float64:
				v = T(fr.reg(ix).(float64))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.int(ix))
			case reflect.Int8:
				v = T(fr.int8(ix))
			case reflect.Int16:
				v = T(fr.int16(ix))
			case reflect.Int32:
				v = T(fr.int32(ix))
			case reflect.Int64:
				v = T(fr.int64(ix))
			case reflect.Uint:
				v = T(fr.uint(ix))
			case reflect.Uint8:
				v = T(fr.uint8(ix))
			case reflect.Uint16:
				v = T(fr.uint16(ix))
			case reflect.Uint32:
				v = T(fr.uint32(ix))
			case reflect.Uint64:
				v = T(fr.uint64(ix))
			case reflect.Uintptr:
				v = T(fr.uintptr(ix))
			case reflect.Float32:
				v = T(fr.float32(ix))
			case reflect.Float64:
				v = T(fr.float64(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtUint8(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = uint8
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.reg(ix).(int))
			case reflect.Int8:
				v = T(fr.reg(ix).(int8))
			case reflect.Int16:
				v = T(fr.reg(ix).(int16))
			case reflect.Int32:
				v = T(fr.reg(ix).(int32))
			case reflect.Int64:
				v = T(fr.reg(ix).(int64))
			case reflect.Uint:
				v = T(fr.reg(ix).(uint))
			case reflect.Uint8:
				v = T(fr.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(fr.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(fr.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(fr.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(fr.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(fr.reg(ix).(float32))
			case reflect.Float64:
				v = T(fr.reg(ix).(float64))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.int(ix))
			case reflect.Int8:
				v = T(fr.int8(ix))
			case reflect.Int16:
				v = T(fr.int16(ix))
			case reflect.Int32:
				v = T(fr.int32(ix))
			case reflect.Int64:
				v = T(fr.int64(ix))
			case reflect.Uint:
				v = T(fr.uint(ix))
			case reflect.Uint8:
				v = T(fr.uint8(ix))
			case reflect.Uint16:
				v = T(fr.uint16(ix))
			case reflect.Uint32:
				v = T(fr.uint32(ix))
			case reflect.Uint64:
				v = T(fr.uint64(ix))
			case reflect.Uintptr:
				v = T(fr.uintptr(ix))
			case reflect.Float32:
				v = T(fr.float32(ix))
			case reflect.Float64:
				v = T(fr.float64(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtUint16(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = uint16
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.reg(ix).(int))
			case reflect.Int8:
				v = T(fr.reg(ix).(int8))
			case reflect.Int16:
				v = T(fr.reg(ix).(int16))
			case reflect.Int32:
				v = T(fr.reg(ix).(int32))
			case reflect.Int64:
				v = T(fr.reg(ix).(int64))
			case reflect.Uint:
				v = T(fr.reg(ix).(uint))
			case reflect.Uint8:
				v = T(fr.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(fr.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(fr.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(fr.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(fr.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(fr.reg(ix).(float32))
			case reflect.Float64:
				v = T(fr.reg(ix).(float64))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.int(ix))
			case reflect.Int8:
				v = T(fr.int8(ix))
			case reflect.Int16:
				v = T(fr.int16(ix))
			case reflect.Int32:
				v = T(fr.int32(ix))
			case reflect.Int64:
				v = T(fr.int64(ix))
			case reflect.Uint:
				v = T(fr.uint(ix))
			case reflect.Uint8:
				v = T(fr.uint8(ix))
			case reflect.Uint16:
				v = T(fr.uint16(ix))
			case reflect.Uint32:
				v = T(fr.uint32(ix))
			case reflect.Uint64:
				v = T(fr.uint64(ix))
			case reflect.Uintptr:
				v = T(fr.uintptr(ix))
			case reflect.Float32:
				v = T(fr.float32(ix))
			case reflect.Float64:
				v = T(fr.float64(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtUint32(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = uint32
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.reg(ix).(int))
			case reflect.Int8:
				v = T(fr.reg(ix).(int8))
			case reflect.Int16:
				v = T(fr.reg(ix).(int16))
			case reflect.Int32:
				v = T(fr.reg(ix).(int32))
			case reflect.Int64:
				v = T(fr.reg(ix).(int64))
			case reflect.Uint:
				v = T(fr.reg(ix).(uint))
			case reflect.Uint8:
				v = T(fr.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(fr.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(fr.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(fr.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(fr.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(fr.reg(ix).(float32))
			case reflect.Float64:
				v = T(fr.reg(ix).(float64))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.int(ix))
			case reflect.Int8:
				v = T(fr.int8(ix))
			case reflect.Int16:
				v = T(fr.int16(ix))
			case reflect.Int32:
				v = T(fr.int32(ix))
			case reflect.Int64:
				v = T(fr.int64(ix))
			case reflect.Uint:
				v = T(fr.uint(ix))
			case reflect.Uint8:
				v = T(fr.uint8(ix))
			case reflect.Uint16:
				v = T(fr.uint16(ix))
			case reflect.Uint32:
				v = T(fr.uint32(ix))
			case reflect.Uint64:
				v = T(fr.uint64(ix))
			case reflect.Uintptr:
				v = T(fr.uintptr(ix))
			case reflect.Float32:
				v = T(fr.float32(ix))
			case reflect.Float64:
				v = T(fr.float64(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtUint64(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = uint64
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.reg(ix).(int))
			case reflect.Int8:
				v = T(fr.reg(ix).(int8))
			case reflect.Int16:
				v = T(fr.reg(ix).(int16))
			case reflect.Int32:
				v = T(fr.reg(ix).(int32))
			case reflect.Int64:
				v = T(fr.reg(ix).(int64))
			case reflect.Uint:
				v = T(fr.reg(ix).(uint))
			case reflect.Uint8:
				v = T(fr.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(fr.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(fr.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(fr.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(fr.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(fr.reg(ix).(float32))
			case reflect.Float64:
				v = T(fr.reg(ix).(float64))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.int(ix))
			case reflect.Int8:
				v = T(fr.int8(ix))
			case reflect.Int16:
				v = T(fr.int16(ix))
			case reflect.Int32:
				v = T(fr.int32(ix))
			case reflect.Int64:
				v = T(fr.int64(ix))
			case reflect.Uint:
				v = T(fr.uint(ix))
			case reflect.Uint8:
				v = T(fr.uint8(ix))
			case reflect.Uint16:
				v = T(fr.uint16(ix))
			case reflect.Uint32:
				v = T(fr.uint32(ix))
			case reflect.Uint64:
				v = T(fr.uint64(ix))
			case reflect.Uintptr:
				v = T(fr.uintptr(ix))
			case reflect.Float32:
				v = T(fr.float32(ix))
			case reflect.Float64:
				v = T(fr.float64(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtUintptr(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = uintptr
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.reg(ix).(int))
			case reflect.Int8:
				v = T(fr.reg(ix).(int8))
			case reflect.Int16:
				v = T(fr.reg(ix).(int16))
			case reflect.Int32:
				v = T(fr.reg(ix).(int32))
			case reflect.Int64:
				v = T(fr.reg(ix).(int64))
			case reflect.Uint:
				v = T(fr.reg(ix).(uint))
			case reflect.Uint8:
				v = T(fr.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(fr.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(fr.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(fr.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(fr.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(fr.reg(ix).(float32))
			case reflect.Float64:
				v = T(fr.reg(ix).(float64))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.int(ix))
			case reflect.Int8:
				v = T(fr.int8(ix))
			case reflect.Int16:
				v = T(fr.int16(ix))
			case reflect.Int32:
				v = T(fr.int32(ix))
			case reflect.Int64:
				v = T(fr.int64(ix))
			case reflect.Uint:
				v = T(fr.uint(ix))
			case reflect.Uint8:
				v = T(fr.uint8(ix))
			case reflect.Uint16:
				v = T(fr.uint16(ix))
			case reflect.Uint32:
				v = T(fr.uint32(ix))
			case reflect.Uint64:
				v = T(fr.uint64(ix))
			case reflect.Uintptr:
				v = T(fr.uintptr(ix))
			case reflect.Float32:
				v = T(fr.float32(ix))
			case reflect.Float64:
				v = T(fr.float64(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtFloat32(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = float32
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.reg(ix).(int))
			case reflect.Int8:
				v = T(fr.reg(ix).(int8))
			case reflect.Int16:
				v = T(fr.reg(ix).(int16))
			case reflect.Int32:
				v = T(fr.reg(ix).(int32))
			case reflect.Int64:
				v = T(fr.reg(ix).(int64))
			case reflect.Uint:
				v = T(fr.reg(ix).(uint))
			case reflect.Uint8:
				v = T(fr.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(fr.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(fr.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(fr.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(fr.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(fr.reg(ix).(float32))
			case reflect.Float64:
				v = T(fr.reg(ix).(float64))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.int(ix))
			case reflect.Int8:
				v = T(fr.int8(ix))
			case reflect.Int16:
				v = T(fr.int16(ix))
			case reflect.Int32:
				v = T(fr.int32(ix))
			case reflect.Int64:
				v = T(fr.int64(ix))
			case reflect.Uint:
				v = T(fr.uint(ix))
			case reflect.Uint8:
				v = T(fr.uint8(ix))
			case reflect.Uint16:
				v = T(fr.uint16(ix))
			case reflect.Uint32:
				v = T(fr.uint32(ix))
			case reflect.Uint64:
				v = T(fr.uint64(ix))
			case reflect.Uintptr:
				v = T(fr.uintptr(ix))
			case reflect.Float32:
				v = T(fr.float32(ix))
			case reflect.Float64:
				v = T(fr.float64(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtFloat64(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = float64
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.reg(ix).(int))
			case reflect.Int8:
				v = T(fr.reg(ix).(int8))
			case reflect.Int16:
				v = T(fr.reg(ix).(int16))
			case reflect.Int32:
				v = T(fr.reg(ix).(int32))
			case reflect.Int64:
				v = T(fr.reg(ix).(int64))
			case reflect.Uint:
				v = T(fr.reg(ix).(uint))
			case reflect.Uint8:
				v = T(fr.reg(ix).(uint8))
			case reflect.Uint16:
				v = T(fr.reg(ix).(uint16))
			case reflect.Uint32:
				v = T(fr.reg(ix).(uint32))
			case reflect.Uint64:
				v = T(fr.reg(ix).(uint64))
			case reflect.Uintptr:
				v = T(fr.reg(ix).(uintptr))
			case reflect.Float32:
				v = T(fr.reg(ix).(float32))
			case reflect.Float64:
				v = T(fr.reg(ix).(float64))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Int:
				v = T(fr.int(ix))
			case reflect.Int8:
				v = T(fr.int8(ix))
			case reflect.Int16:
				v = T(fr.int16(ix))
			case reflect.Int32:
				v = T(fr.int32(ix))
			case reflect.Int64:
				v = T(fr.int64(ix))
			case reflect.Uint:
				v = T(fr.uint(ix))
			case reflect.Uint8:
				v = T(fr.uint8(ix))
			case reflect.Uint16:
				v = T(fr.uint16(ix))
			case reflect.Uint32:
				v = T(fr.uint32(ix))
			case reflect.Uint64:
				v = T(fr.uint64(ix))
			case reflect.Uintptr:
				v = T(fr.uintptr(ix))
			case reflect.Float32:
				v = T(fr.float32(ix))
			case reflect.Float64:
				v = T(fr.float64(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtComplex64(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = complex64
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Complex64:
				v = T(fr.reg(ix).(complex64))
			case reflect.Complex128:
				v = T(fr.reg(ix).(complex128))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Complex64:
				v = T(fr.complex64(ix))
			case reflect.Complex128:
				v = T(fr.complex128(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
func cvtComplex128(ir, ix register, xkind reflect.Kind, xtyp reflect.Type, typ reflect.Type) func(fr *frame) {
	type T = complex128
	t := xtype.TypeOfType(typ)
	isBasic := typ.PkgPath() == ""
	if xtyp.PkgPath() == "" {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Complex64:
				v = T(fr.reg(ix).(complex64))
			case reflect.Complex128:
				v = T(fr.reg(ix).(complex128))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	} else {
		return func(fr *frame) {
			var v T
			switch xkind {
			case reflect.Complex64:
				v = T(fr.complex64(ix))
			case reflect.Complex128:
				v = T(fr.complex128(ix))
			}
			if isBasic {
				fr.setReg(ir, v)
			} else {
				fr.setReg(ir, xtype.Make(t, v))
			}
		}
	}
}
