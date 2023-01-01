package ctype

import (
	"reflect"
	"unsafe"
)

type eface struct {
	typ  unsafe.Pointer
	word unsafe.Pointer
}

type Type unsafe.Pointer

func TypeOf(i interface{}) Type {
	p := (*eface)(unsafe.Pointer(&i))
	return Type(p.typ)
}

func TypeOfType(typ reflect.Type) Type {
	e := (*eface)(unsafe.Pointer(&typ))
	return Type(e.word)
}

func Bytes(i interface{}) []byte {
	return *(*[]byte)((*eface)(unsafe.Pointer(&i)).word)
}

func Runes(i interface{}) []rune {
	return *(*[]rune)((*eface)(unsafe.Pointer(&i)).word)
}

func Bool(i interface{}) bool {
	return *(*bool)((*eface)(unsafe.Pointer(&i)).word)
}

func Int(i interface{}) int {
	return *(*int)((*eface)(unsafe.Pointer(&i)).word)
}

func Int8(i interface{}) int8 {
	return *(*int8)((*eface)(unsafe.Pointer(&i)).word)
}

func Int16(i interface{}) int16 {
	return *(*int16)((*eface)(unsafe.Pointer(&i)).word)
}

func Int32(i interface{}) int32 {
	return *(*int32)((*eface)(unsafe.Pointer(&i)).word)
}

func Int64(i interface{}) int64 {
	return *(*int64)((*eface)(unsafe.Pointer(&i)).word)
}

func Uint(i interface{}) uint {
	return *(*uint)((*eface)(unsafe.Pointer(&i)).word)
}

func Uint8(i interface{}) uint8 {
	return *(*uint8)((*eface)(unsafe.Pointer(&i)).word)
}

func Uint16(i interface{}) uint16 {
	return *(*uint16)((*eface)(unsafe.Pointer(&i)).word)
}

func Uint32(i interface{}) uint32 {
	return *(*uint32)((*eface)(unsafe.Pointer(&i)).word)
}

func Uint64(i interface{}) uint64 {
	return *(*uint64)((*eface)(unsafe.Pointer(&i)).word)
}

func Uintptr(i interface{}) uintptr {
	return *(*uintptr)((*eface)(unsafe.Pointer(&i)).word)
}

func Float32(i interface{}) float32 {
	return *(*float32)((*eface)(unsafe.Pointer(&i)).word)
}

func Float64(i interface{}) float64 {
	return *(*float64)((*eface)(unsafe.Pointer(&i)).word)
}

func Complex64(i interface{}) complex64 {
	return *(*complex64)((*eface)(unsafe.Pointer(&i)).word)
}

func Complex128(i interface{}) complex128 {
	return *(*complex128)((*eface)(unsafe.Pointer(&i)).word)
}

func String(i interface{}) string {
	return *(*string)((*eface)(unsafe.Pointer(&i)).word)
}

func Pointer(i interface{}) unsafe.Pointer {
	return (*eface)(unsafe.Pointer(&i)).word
}

// Make change interface type and return
func Make(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	p.typ = unsafe.Pointer(typ)
	return i
}

func ConvertPtr(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: p.word,
	}))
}

//go:linkname typedmemmove reflect.typedmemmove
func typedmemmove(t Type, dst unsafe.Pointer, src unsafe.Pointer)

//go:linkname unsafe_New reflect.unsafe_New
func unsafe_New(t Type) unsafe.Pointer

// convert copy
func ConvertDirect(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	c := unsafe_New(typ)
	typedmemmove(typ, c, p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: c,
	}))
}

func ConvertBool(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*bool)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertInt(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*int)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertInt8(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*int8)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertInt16(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*int16)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertInt32(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*int32)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertInt64(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*int64)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertUint(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*uint)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertUint8(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*uint8)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertUint16(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*uint16)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertUint32(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*uint32)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertUint64(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*uint64)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertUintptr(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*uintptr)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertFloat32(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*float32)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertFloat64(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*float64)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertComplex64(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*complex64)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertComplex128(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*complex128)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func ConvertString(typ Type, i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := *(*string)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func Not(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := !*(*bool)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegInt(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*int)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegInt8(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*int8)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegInt16(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*int16)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegInt32(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*int32)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegInt64(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*int64)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegUint(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*uint)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegUint8(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*uint8)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegUint16(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*uint16)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegUint32(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*uint32)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegUint64(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*uint64)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegUintptr(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*uintptr)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegFloat32(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*float32)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegFloat64(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*float64)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegComplex64(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*complex64)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func NegComplex128(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := -*(*complex128)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func XorInt(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := ^*(*int)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func XorInt8(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := ^*(*int8)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func XorInt16(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := ^*(*int16)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func XorInt32(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := ^*(*int32)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func XorInt64(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := ^*(*int64)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func XorUint(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := ^*(*uint)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func XorUint8(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := ^*(*uint8)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func XorUint16(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := ^*(*uint16)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func XorUint32(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := ^*(*uint32)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func XorUint64(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := ^*(*uint64)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func XorUintptr(i interface{}) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	v := ^*(*uintptr)(p.word)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  p.typ,
		word: unsafe.Pointer(&v),
	}))
}

func MakeBool(typ Type, v bool) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeInt(typ Type, v int) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeInt8(typ Type, v int8) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeInt16(typ Type, v int16) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeInt32(typ Type, v int32) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeInt64(typ Type, v int64) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeUint(typ Type, v uint) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeUint8(typ Type, v uint8) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeUint16(typ Type, v uint16) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeUint32(typ Type, v uint32) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeUint64(typ Type, v uint64) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeUintptr(typ Type, v uintptr) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeFloat32(typ Type, v float32) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeFloat64(typ Type, v float64) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeComplex64(typ Type, v complex64) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeComplex128(typ Type, v complex128) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func MakeString(typ Type, v string) interface{} {
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: unsafe.Pointer(&v),
	}))
}

func Alloc(typ Type) interface{} {
	ptr := unsafe_New(typ)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(typ),
		word: ptr,
	}))
}

func New(typ, ptrto Type) interface{} {
	ptr := unsafe_New(typ)
	return *(*interface{})(unsafe.Pointer(&eface{
		typ:  unsafe.Pointer(ptrto),
		word: ptr,
	}))
}

func NewPointer(typ Type) unsafe.Pointer {
	return unsafe_New(typ)
}

func SetPointer(i interface{}, word unsafe.Pointer) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	p.word = word
	return i
}

func SetType(i interface{}, typ Type) interface{} {
	p := (*eface)(unsafe.Pointer(&i))
	p.typ = unsafe.Pointer(typ)
	return i
}
