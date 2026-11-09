package value

import (
	"reflect"
	"testing"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
	"golang.org/x/tools/go/ssa"
)

func TestRValueHelpers(t *testing.T) {
	invalid := RValue{}
	if invalid.Interface() != nil || invalid.String() != "<invalid>" {
		t.Fatalf("invalid: %#v %q", invalid.Interface(), invalid.String())
	}
	if invalid.IsValid() {
		t.Fatal("invalid should not be valid")
	}

	n := ValueOf(7)
	if n.Type() != reflect.TypeOf(7) {
		t.Fatal(n.Type())
	}
	if !n.IsValid() || n.IsNil() {
		t.Fatal("int is valid and non-nil")
	}

	s := ValueOf([]int{1, 2, 3})
	if s.Len() != 3 || s.Cap() < 3 {
		t.Fatal(s.Len(), s.Cap())
	}
	if s.Index(0).Int() != 1 {
		t.Fatal(s.Index(0))
	}

	m := ValueOf(map[string]int{"k": 9})
	if m.MapIndex(ValueOf("k")).Int() != 9 {
		t.Fatal(m.MapIndex(ValueOf("k")))
	}

	st := ValueOf(struct{ A int }{4})
	if st.Field(0).Int() != 4 {
		t.Fatal(st.Field(0))
	}

	x := 3
	p := ValueOf(&x)
	if p.Elem().Int() != 3 {
		t.Fatal(p.Elem())
	}
	p.Elem().Set(ValueOf(5))
	if x != 5 {
		t.Fatal(x)
	}
	if ValueOf(uint(2)).Uint() != 2 {
		t.Fatal("uint")
	}
	if ValueOf(1.5).Float() != 1.5 {
		t.Fatal("float")
	}
	if !ValueOf((*int)(nil)).IsNil() {
		t.Fatal("nil pointer")
	}
	_ = NewRValueOf(reflect.ValueOf(1))
}

func TestPackageEmptyAndUnpackageScalar(t *testing.T) {
	packed := Package(nil)
	if Unpackage(packed) == nil && packed.Len() != 0 {
		t.Fatal(packed)
	}
	single := Unpackage(ValueOf(3))
	if len(single) != 1 || single[0].Int() != 3 {
		t.Fatal(single)
	}
}

func TestConverterRejects(t *testing.T) {
	_, err := DefaultTypeConverter.Convert(ValueOf("x"), reflect.TypeOf(0))
	if err == nil {
		t.Fatal("expected convert error")
	}
	same, err := DefaultTypeConverter.Convert(ValueOf(1), reflect.TypeOf(0))
	if err != nil || same.Int() != 1 {
		t.Fatal(same, err)
	}
}

func TestMapIterValueAPI(t *testing.T) {
	iter := &MapIter{Value: ValueOf(map[int]int{}), Keys: nil}
	_ = iter.Interface()
	_ = iter.String()
	_ = iter.Type()
	_ = iter.Kind()
	_ = iter.IsValid()
	_ = iter.IsNil()
	_ = iter.Len()
	_ = iter.Cap()
	_ = iter.RValue()
	mustPanic(t, "Int", func() { iter.Int() })
	mustPanic(t, "Uint", func() { iter.Uint() })
	mustPanic(t, "Float", func() { iter.Float() })
	mustPanic(t, "Bool", func() { iter.Bool() })
	mustPanic(t, "Index", func() { iter.Index(0) })
	mustPanic(t, "MapIndex", func() { iter.MapIndex(ValueOf(1)) })
	mustPanic(t, "Field", func() { iter.Field(0) })
	mustPanic(t, "Elem", func() { iter.Elem() })
	mustPanic(t, "Set", func() { iter.Set(ValueOf(1)) })
}

func TestExternalValueAPI(t *testing.T) {
	s := "hi"
	ev := NewExternalValue(reflect.ValueOf(s))
	if ev.Interface() != "hi" || ev.String() != "hi" {
		t.Fatal(ev.Interface(), ev.String())
	}
	if ev.Type() != reflect.TypeOf(s) || ev.Kind() != reflect.String {
		t.Fatal(ev.Type(), ev.Kind())
	}
	if !ev.IsValid() || ev.IsNil() {
		t.Fatal("string valid")
	}
	_ = NewExternalValue(reflect.ValueOf(true)).String()
	_ = NewExternalValue(reflect.ValueOf(false)).String()
	_ = NewExternalValue(reflect.ValueOf(int64(1))).String()
	_ = NewExternalValue(reflect.ValueOf(uint64(1))).String()
	_ = NewExternalValue(reflect.ValueOf(1.25)).String()
	_ = NewExternalValue(reflect.ValueOf(complex(1, 1))).String()
	_ = NewExternalValue(reflect.ValueOf([]int{1})).String()
	_ = NewExternalValue(reflect.ValueOf(3)).Int()
	_ = NewExternalValue(reflect.ValueOf(uint(3))).Uint()
	_ = NewExternalValue(reflect.ValueOf(1.5)).Float()
	_ = NewExternalValue(reflect.ValueOf(true)).Bool()
	sl := NewExternalValue(reflect.ValueOf([]int{1, 2}))
	if sl.Len() != 2 || sl.Cap() < 2 || sl.Index(0).Int() != 1 {
		t.Fatal("slice")
	}
	mp := NewExternalValue(reflect.ValueOf(map[string]int{"a": 1}))
	if mp.MapIndex(ValueOf("a")).Int() != 1 {
		t.Fatal("map")
	}
	st := NewExternalValue(reflect.ValueOf(struct{ A int }{2}))
	if st.Field(0).Int() != 2 {
		t.Fatal("field")
	}
	x := 4
	ptr := NewExternalValue(reflect.ValueOf(&x))
	if ptr.Elem().Int() != 4 {
		t.Fatal("elem")
	}
	_ = ptr.RValue()
	ptr.Set(ValueOf(8))
	if x != 8 {
		t.Fatal(x)
	}
	invalid := NewExternalValue(reflect.Value{})
	if !invalid.IsNil() {
		t.Fatal("invalid is nil")
	}

	imp := importer.NewImporter()
	_ = ExternalValueWrap(imp, (*ssa.Package)(nil))
}

func TestExternalValueStoreUnaddressable(t *testing.T) {
	ev := NewExternalValue(reflect.ValueOf(1))
	mustPanic(t, "Store", func() { ev.Store(ValueOf(2)) })
}

func TestExternalValueNilKindsAndAddrStore(t *testing.T) {
	if !NewExternalValue(reflect.ValueOf([]int(nil))).IsNil() {
		t.Fatal("nil slice")
	}
	x := 3
	ev := NewExternalValue(reflect.ValueOf(&x).Elem())
	ev.Store(ValueOf(4))
	if x != 4 {
		t.Fatal(x)
	}
}

func TestMapIterNextPointerMap(t *testing.T) {
	m := map[int]int{1: 2}
	iter := &MapIter{Value: ValueOf(&m), Keys: reflect.ValueOf(m).MapKeys()}
	res := iter.Next()
	ok := res.RValue().Index(0).Interface().(Value).Bool()
	if !ok {
		t.Fatal("expected entry")
	}
}

func mustPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s: expected panic", name)
		}
	}()
	fn()
}
