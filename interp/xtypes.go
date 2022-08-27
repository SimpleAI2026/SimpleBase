package interp

import (
	"fmt"
	"go/token"
	"go/types"
	"log"
	"reflect"
	"strconv"
	"unsafe"

	"github.com/goplus/reflectx"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"

	cloader "github.com/linkxzhou/gongx/loader"
)

var (
	tyEmptyStruct = reflect.TypeOf((*struct{})(nil)).Elem()
	tyEmptyPtr    = reflect.TypeOf((*struct{})(nil))
	tyEmptyMap    = reflect.TypeOf((*map[struct{}]struct{})(nil)).Elem()
	tyEmptySlice  = reflect.TypeOf((*[]struct{})(nil)).Elem()
	tyEmptyArray  = reflect.TypeOf((*[0]struct{})(nil)).Elem()
	tyEmptyChan   = reflect.TypeOf((*chan struct{})(nil)).Elem()
	tyEmptyFunc   = reflect.TypeOf((*func())(nil)).Elem()
)

/*
	Array
	Chan
	Func
	Interface
	Map
	Ptr
	Slice
	String
	Struct
	UnsafePointer
*/

func emptyType(kind reflect.Kind) reflect.Type {
	switch kind {
	case reflect.Array:
		return tyEmptyArray
	case reflect.Chan:
		return tyEmptyChan
	case reflect.Func:
		return tyEmptyFunc
	case reflect.Interface:
		return cloader.TyEmptyInterface
	case reflect.Map:
		return tyEmptyMap
	case reflect.Ptr:
		return tyEmptyPtr
	case reflect.Slice:
		return tyEmptySlice
	case reflect.Struct:
		return tyEmptyStruct
	default:
		return xtypeTypes[kind]
	}
}

func toMockType(typ types.Type) reflect.Type {
	switch t := typ.(type) {
	case *types.Basic:
		kind := t.Kind()
		if kind > types.Invalid && kind < types.UntypedNil {
			return xtypeTypes[kind]
		}
		panic(fmt.Errorf("toMockType: invalid type %v", typ))
	case *types.Pointer:
		return tyEmptyPtr
	case *types.Slice:
		return tyEmptySlice
	case *types.Array:
		e := toMockType(t.Elem())
		return reflect.ArrayOf(int(t.Len()), e)
	case *types.Map:
		return tyEmptyMap
	case *types.Chan:
		return tyEmptyChan
	case *types.Struct:
		n := t.NumFields()
		fs := make([]reflect.StructField, n)
		for i := 0; i < n; i++ {
			ft := t.Field(i)
			fs[i].Name = "F" + strconv.Itoa(i)
			fs[i].Type = toMockType(ft.Type())
			fs[i].Anonymous = ft.Embedded()
		}
		return reflect.StructOf(fs)
	case *types.Named:
		return toMockType(typ.Underlying())
	case *types.Interface:
		return cloader.TyEmptyInterface
	case *types.Signature:
		in := t.Params().Len()
		out := t.Results().Len()
		if in+out == 0 {
			return tyEmptyFunc
		}
		ins := make([]reflect.Type, in)
		outs := make([]reflect.Type, out)
		variadic := t.Variadic()
		if variadic {
			for i := 0; i < in-1; i++ {
				ins[i] = tyEmptyStruct
			}
			ins[in-1] = tyEmptySlice
		} else {
			for i := 0; i < in; i++ {
				ins[i] = tyEmptyStruct
			}
		}
		for i := 0; i < out; i++ {
			outs[i] = tyEmptyStruct
		}
		return reflect.FuncOf(ins, outs, variadic)
	default:
		panic(fmt.Errorf("toEmptyType: unreachable %v", typ))
	}
}

var xtypeTypes = [...]reflect.Type{
	types.Bool:          reflect.TypeOf(false),
	types.Int:           reflect.TypeOf(0),
	types.Int8:          reflect.TypeOf(int8(0)),
	types.Int16:         reflect.TypeOf(int16(0)),
	types.Int32:         reflect.TypeOf(int32(0)),
	types.Int64:         reflect.TypeOf(int64(0)),
	types.Uint:          reflect.TypeOf(uint(0)),
	types.Uint8:         reflect.TypeOf(uint8(0)),
	types.Uint16:        reflect.TypeOf(uint16(0)),
	types.Uint32:        reflect.TypeOf(uint32(0)),
	types.Uint64:        reflect.TypeOf(uint64(0)),
	types.Uintptr:       reflect.TypeOf(uintptr(0)),
	types.Float32:       reflect.TypeOf(float32(0)),
	types.Float64:       reflect.TypeOf(float64(0)),
	types.Complex64:     reflect.TypeOf(complex64(0)),
	types.Complex128:    reflect.TypeOf(complex128(0)),
	types.String:        reflect.TypeOf(""),
	types.UnsafePointer: reflect.TypeOf(unsafe.Pointer(nil)),

	types.UntypedBool:    reflect.TypeOf(false),
	types.UntypedInt:     reflect.TypeOf(0),
	types.UntypedRune:    reflect.TypeOf('a'),
	types.UntypedFloat:   reflect.TypeOf(0.1),
	types.UntypedComplex: reflect.TypeOf(0 + 1i),
	types.UntypedString:  reflect.TypeOf(""),
}

type FindMethod interface {
	FindMethod(mtyp reflect.Type, fn *types.Func) func([]reflect.Value) []reflect.Value
}

type TypesRecord struct {
	loader cloader.Loader
	finder FindMethod
	rcache map[reflect.Type]types.Type
	tcache *typeutil.Map
}

func NewTypesRecord(loader cloader.Loader, finder FindMethod) *TypesRecord {
	return &TypesRecord{
		loader: loader,
		finder: finder,
		rcache: make(map[reflect.Type]types.Type),
		tcache: &typeutil.Map{},
	}
}

func (r *TypesRecord) LookupReflect(typ types.Type) (rt reflect.Type, ok bool) {
	rt, ok = r.loader.LookupReflect(typ)
	if !ok {
		if rt := r.tcache.At(typ); rt != nil {
			return rt.(reflect.Type), true
		}
	}
	return
}

func (r *TypesRecord) LookupLocalTypes(rt reflect.Type) (typ types.Type, ok bool) {
	typ, ok = r.rcache[rt]
	return
}

func (r *TypesRecord) LookupTypes(rt reflect.Type) (typ types.Type, ok bool) {
	typ, ok = r.loader.LookupTypes(rt)
	if !ok {
		typ, ok = r.rcache[rt]
	}
	return
}

func (r *TypesRecord) saveType(typ types.Type, rt reflect.Type) {
	r.tcache.Set(typ, rt)
	r.rcache[rt] = typ
}

func (r *TypesRecord) ToType(typ types.Type) reflect.Type {
	if rt, ok := r.LookupReflect(typ); ok {
		return rt
	}
	var rt reflect.Type
	switch t := typ.(type) {
	case *types.Basic:
		kind := t.Kind()
		if kind > types.Invalid && kind < types.UntypedNil {
			rt = xtypeTypes[kind]
		}
	case *types.Pointer:
		elem := r.ToType(t.Elem())
		rt = reflect.PtrTo(elem)
	case *types.Slice:
		elem := r.ToType(t.Elem())
		rt = reflect.SliceOf(elem)
	case *types.Array:
		elem := r.ToType(t.Elem())
		rt = reflect.ArrayOf(int(t.Len()), elem)
	case *types.Map:
		key := r.ToType(t.Key())
		elem := r.ToType(t.Elem())
		rt = reflect.MapOf(key, elem)
	case *types.Chan:
		elem := r.ToType(t.Elem())
		rt = reflect.ChanOf(toReflectChanDir(t.Dir()), elem)
	case *types.Struct:
		rt = r.toStructType(t)
	case *types.Named:
		rt = r.toNamedType(t)
	case *types.Interface:
		rt = r.toInterfaceType(t)
	case *types.Signature:
		in := r.ToTypeList(t.Params())
		out := r.ToTypeList(t.Results())
		b := t.Variadic()
		if b && len(in) > 0 {
			last := in[len(in)-1]
			if last.Kind() == reflect.String {
				in[len(in)-1] = reflect.TypeOf([]byte{})
			}
		}
		rt = reflect.FuncOf(in, out, b)
	case *types.Tuple:
		r.ToTypeList(t)
		rt = reflect.TypeOf((*_tuple)(nil)).Elem()
	default:
		panic(fmt.Errorf("ToType: not handled %v", typ))
	}
	r.saveType(typ, rt)
	return rt
}

type _tuple struct{}

func (r *TypesRecord) toInterfaceType(t *types.Interface) reflect.Type {
	n := t.NumMethods()
	if n == 0 {
		return cloader.TyEmptyInterface
	}
	ms := make([]reflect.Method, n)
	for i := 0; i < n; i++ {
		fn := t.Method(i)
		mtyp := r.ToType(fn.Type())
		ms[i] = reflect.Method{
			Name: fn.Name(),
			Type: mtyp,
		}
		if pkg := fn.Pkg(); pkg != nil {
			ms[i].PkgPath = pkg.Path()
		}
	}
	return reflectx.InterfaceOf(nil, ms)
}

func (r *TypesRecord) toNamedType(t *types.Named) reflect.Type {
	ut := t.Underlying()
	name := t.Obj()
	if name.Pkg() == nil {
		if name.Name() == "error" {
			return cloader.TyErrorInterface
		}
		return r.ToType(ut)
	}
	methods := IntuitiveMethodSet(t)
	numMethods := len(methods)
	if numMethods == 0 {
		styp := toMockType(t.Underlying())
		typ := reflectx.NamedTypeOf(name.Pkg().Path(), name.Name(), styp)
		r.saveType(t, typ)
		utype := r.ToType(ut)
		reflectx.SetUnderlying(typ, utype)
		return typ
	}
	var mcount, pcount int
	for i := 0; i < numMethods; i++ {
		sig := methods[i].Type().(*types.Signature)
		if !isPointer(sig.Recv().Type()) {
			mcount++
		}
		pcount++
	}
	// toMockType for size/align
	etyp := toMockType(ut)
	styp := reflectx.NamedTypeOf(name.Pkg().Path(), name.Name(), etyp)
	typ := reflectx.NewMethodSet(styp, mcount, pcount)
	r.saveType(t, typ)
	utype := r.ToType(ut)
	reflectx.SetUnderlying(typ, utype)
	if typ.Kind() != reflect.Interface {
		r.setMethods(typ, methods)
	}
	return typ
}

func (r *TypesRecord) toStructType(t *types.Struct) reflect.Type {
	n := t.NumFields()
	if n == 0 {
		return tyEmptyStruct
	}
	flds := make([]reflect.StructField, n)
	for i := 0; i < n; i++ {
		flds[i] = r.toStructField(t.Field(i), t.Tag(i))
	}
	typ := reflectx.StructOf(flds)
	methods := IntuitiveMethodSet(t)
	if numMethods := len(methods); numMethods != 0 {
		// anonymous structs with methods. struct { T }
		var mcount, pcount int
		for i := 0; i < numMethods; i++ {
			sig := methods[i].Type().(*types.Signature)
			if !isPointer(sig.Recv().Type()) {
				mcount++
			}
			pcount++
		}
		typ = reflectx.NewMethodSet(typ, mcount, pcount)
		r.setMethods(typ, methods)
	}
	return typ
}

func (r *TypesRecord) toStructField(v *types.Var, tag string) reflect.StructField {
	name := v.Name()
	typ := r.ToType(v.Type())
	fld := reflect.StructField{
		Name:      name,
		Type:      typ,
		Tag:       reflect.StructTag(tag),
		Anonymous: v.Anonymous(),
	}
	if !token.IsExported(name) {
		fld.PkgPath = v.Pkg().Path()
	}
	return fld
}

func (r *TypesRecord) ToTypeList(tuple *types.Tuple) []reflect.Type {
	n := tuple.Len()
	if n == 0 {
		return nil
	}
	list := make([]reflect.Type, n)
	for i := 0; i < n; i++ {
		list[i] = r.ToType(tuple.At(i).Type())
	}
	return list
}

func isPointer(typ types.Type) bool {
	_, ok := typ.Underlying().(*types.Pointer)
	return ok
}

func (r *TypesRecord) setMethods(typ reflect.Type, methods []*types.Selection) {
	numMethods := len(methods)
	var ms []reflectx.Method
	for i := 0; i < numMethods; i++ {
		fn := methods[i].Obj().(*types.Func)
		sig := methods[i].Type().(*types.Signature)
		pointer := isPointer(sig.Recv().Type())
		mtyp := r.ToType(sig)
		var mfn func(args []reflect.Value) []reflect.Value
		idx := methods[i].Index()
		if len(idx) > 1 {
			isptr := isPointer(fn.Type().Underlying().(*types.Signature).Recv().Type())
			variadic := mtyp.IsVariadic()
			mfn = func(args []reflect.Value) []reflect.Value {
				v := args[0]
				for v.Kind() == reflect.Ptr {
					v = v.Elem()
				}
				v = reflectx.FieldByIndexX(v, idx[:len(idx)-1])
				if isptr && v.Kind() != reflect.Ptr {
					v = v.Addr()
				}
				if v.Kind() == reflect.Interface {
					if variadic {
						return v.MethodByName(fn.Name()).CallSlice(args[1:])
					}
					return v.MethodByName(fn.Name()).Call(args[1:])
				}
				m, _ := reflectx.MethodByName(v.Type(), fn.Name())
				args[0] = v
				if variadic {
					return m.Func.CallSlice(args)
				}
				return m.Func.Call(args)
			}
		} else {
			mfn = r.finder.FindMethod(mtyp, fn)
		}
		var pkgpath string
		if pkg := fn.Pkg(); pkg != nil {
			pkgpath = pkg.Path()
		}
		ms = append(ms, reflectx.MakeMethod(fn.Name(), pkgpath, pointer, mtyp, mfn))
	}
	err := reflectx.SetMethodSet(typ, ms, false)
	if err != nil {
		log.Fatalf("SetMethodSet %v err, %v\n", typ, err)
	}
}

func toReflectChanDir(d types.ChanDir) reflect.ChanDir {
	switch d {
	case types.SendRecv:
		return reflect.BothDir
	case types.SendOnly:
		return reflect.SendDir
	case types.RecvOnly:
		return reflect.RecvDir
	}
	return 0
}

func (r *TypesRecord) LoadType(typ types.Type) reflect.Type {
	return r.ToType(typ)
}

func (r *TypesRecord) Load(pkg *ssa.Package) {
	checked := make(map[types.Type]bool)
	for _, v := range pkg.Members {
		typ := v.Type()
		if checked[typ] {
			continue
		}
		checked[typ] = true
		r.LoadType(typ)
	}
}

// IntuitiveMethodSet copy from golang.org/x/tools/go/types/typeutil.IntuitiveMethodSet
func IntuitiveMethodSet(T types.Type) []*types.Selection {
	isPointerToConcrete := func(T types.Type) bool {
		ptr, ok := T.(*types.Pointer)
		return ok && !types.IsInterface(ptr.Elem())
	}

	var result []*types.Selection
	mset := types.NewMethodSet(T)
	if types.IsInterface(T) || isPointerToConcrete(T) {
		for i, n := 0, mset.Len(); i < n; i++ {
			result = append(result, mset.At(i))
		}
	} else {
		// T is some other concrete type.
		// Report methods of T and *T, preferring those of T.
		pmset := types.NewMethodSet(types.NewPointer(T))
		for i, n := 0, pmset.Len(); i < n; i++ {
			meth := pmset.At(i)
			if m := mset.Lookup(meth.Obj().Pkg(), meth.Obj().Name()); m != nil {
				meth = m
			}
			result = append(result, meth)
		}
	}
	return result
}
