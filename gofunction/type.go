package gofunction

import (
	"fmt"
	"go/types"
	"reflect"
	"sync"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
	"github.com/linkxzhou/SimpleBase/gofunction/value"
)

// builtinTypes golang的内置类型
var builtinTypes = map[types.BasicKind]reflect.Type{
	types.Bool:       reflect.TypeOf(true),
	types.Int:        reflect.TypeOf(int(0)),
	types.Int8:       reflect.TypeOf(int8(0)),
	types.Int16:      reflect.TypeOf(int16(0)),
	types.Int32:      reflect.TypeOf(int32(0)),
	types.Int64:      reflect.TypeOf(int64(0)),
	types.Uint:       reflect.TypeOf(uint(0)),
	types.Uint8:      reflect.TypeOf(uint8(0)),
	types.Uint16:     reflect.TypeOf(uint16(0)),
	types.Uint32:     reflect.TypeOf(uint32(0)),
	types.Uint64:     reflect.TypeOf(uint64(0)),
	types.Uintptr:    reflect.TypeOf(uintptr(0)),
	types.Float32:    reflect.TypeOf(float32(0)),
	types.Float64:    reflect.TypeOf(float64(0)),
	types.Complex64:  reflect.TypeOf(complex64(0)),
	types.Complex128: reflect.TypeOf(complex128(0)),
	types.String:     reflect.TypeOf(""),

	types.UntypedBool:    reflect.TypeOf(true),
	types.UntypedInt:     reflect.TypeOf(int(0)),
	types.UntypedRune:    reflect.TypeOf(rune(0)),
	types.UntypedFloat:   reflect.TypeOf(float64(0)),
	types.UntypedComplex: reflect.TypeOf(complex128(0)),
	types.UntypedString:  reflect.TypeOf(""),
}

// interfaceType 空 interface 的 reflect 类型
var interfaceType = reflect.TypeOf((*interface{})(nil)).Elem()

// typeCache types.Type -> reflect.Type 缓存。
// types.Type 接口的动态值在 SSA 构建后稳定不变，可直接做 map 键；
// 避免每次指令执行都递归重建 reflect 类型（含 StructOf 等高开销路径）。
// 读多写少且构建后只读，用 RWMutex 保护并发 Run 同一 Program 的场景。
var (
	typeCacheMu sync.RWMutex
	typeCache   = make(map[types.Type]reflect.Type)
)

// typeChangeCached 带缓存的 typeChange
func typeChangeCached(typ types.Type) reflect.Type {
	typeCacheMu.RLock()
	rType, ok := typeCache[typ]
	typeCacheMu.RUnlock()
	if ok {
		return rType
	}
	rType = typeChange(typ)
	typeCacheMu.Lock()
	typeCache[typ] = rType
	typeCacheMu.Unlock()
	return rType
}

// typeChange 将types.Type转换为对应的reflect.Type类型
func typeChange(typ types.Type) reflect.Type {
	rType := importer.GetExternalType(typ)
	if rType != nil {
		return rType
	}
	switch t := typ.Underlying().(type) {
	case *types.Array:
		rType = reflect.ArrayOf(int(t.Len()), typeChange(t.Elem()))
	case *types.Basic:
		rtype := builtinTypes[t.Kind()]
		if rtype == nil {
			panic(fmt.Sprintf("typeChange: unsupported basic kind %v", t.Kind()))
		}
		rType = rtype
	case *types.Chan:
		var dir reflect.ChanDir
		switch t.Dir() {
		case types.RecvOnly:
			dir = reflect.RecvDir
		case types.SendOnly:
			dir = reflect.SendDir
		case types.SendRecv:
			dir = reflect.BothDir
		}
		rType = reflect.ChanOf(dir, typeChange(t.Elem()))
	case *types.Interface:
		// 空 interface 或任意接口统一映射为 interface{}，
		// 方法调用通过 ExternalValue/MethodByName 反射分发
		rType = interfaceType
	case *types.Map:
		rType = reflect.MapOf(typeChange(t.Key()), typeChange(t.Elem()))
	case *types.Pointer:
		rType = reflect.PointerTo(typeChange(t.Elem()))
	case *types.Slice:
		rType = reflect.SliceOf(typeChange(t.Elem()))
	case *types.Struct:
		fields := make([]reflect.StructField, t.NumFields())
		for i := range fields {
			field := t.Field(i)
			fields[i] = reflect.StructField{
				Name:      field.Name(),
				Type:      typeChange(t.Field(i).Type()),
				Tag:       reflect.StructTag(t.Tag(i)),
				Offset:    0,
				Index:     []int{i},
				Anonymous: field.Anonymous(),
			}
		}
		rType = reflect.StructOf(fields)
	case *types.Signature:
		in := make([]reflect.Type, t.Params().Len())
		for i := range in {
			in[i] = typeChange(t.Params().At(i).Type())
		}
		out := make([]reflect.Type, t.Results().Len())
		for i := range out {
			out[i] = typeChange(t.Results().At(i).Type())
		}
		rType = reflect.FuncOf(in, out, t.Variadic())
	default:
		panic(fmt.Sprintf("typeChange: unsupported type %T (%s)", typ, typ))
	}
	return rType
}

// conv 将变量v的类型转换为typ（走缓存查询 reflect 类型）
func conv(v interface{}, typ types.Type) value.Value {
	rtype := typeChangeCached(typ)
	if v == nil {
		return value.RValue{Value: reflect.Zero(rtype)}
	}
	rv := reflect.ValueOf(v)
	// 快路径：结果天然类型与目标一致时跳过 Convert（消除一次反射转换分配）
	if rv.Type() == rtype {
		return value.RValue{Value: rv}
	}
	return value.RValue{Value: rv.Convert(rtype)}
}
