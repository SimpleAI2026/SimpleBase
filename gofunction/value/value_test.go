package value

import (
	"reflect"
	"testing"
)

func TestValueOf(t *testing.T) {
	v := ValueOf(42)
	if v.Kind() != reflect.Int {
		t.Errorf("expected Int kind, got %v", v.Kind())
	}
	if v.Int() != 42 {
		t.Errorf("expected 42, got %d", v.Int())
	}
}

func TestRValueString(t *testing.T) {
	v := ValueOf("hello")
	if v.String() != "hello" {
		t.Errorf("expected hello, got %s", v.String())
	}
}

func TestRValueIndex(t *testing.T) {
	v := ValueOf([]int{1, 2, 3})
	elem := v.Index(1)
	if elem.Int() != 2 {
		t.Errorf("expected 2, got %d", elem.Int())
	}
}

func TestPackageUnpackage(t *testing.T) {
	// 单返回值
	packed := Package([]reflect.Value{reflect.ValueOf(7)})
	unpacked := Unpackage(packed)
	if len(unpacked) != 1 || unpacked[0].Int() != 7 {
		t.Errorf("single value pack/unpack failed: %v", unpacked)
	}

	// 多返回值
	packed = Package([]reflect.Value{reflect.ValueOf(1), reflect.ValueOf("x")})
	unpacked = Unpackage(packed)
	if len(unpacked) != 2 {
		t.Fatalf("expected 2 results, got %d", len(unpacked))
	}
	if unpacked[0].Int() != 1 || unpacked[1].String() != "x" {
		t.Errorf("multi value pack/unpack failed: %v %v", unpacked[0], unpacked[1])
	}
}

func TestMapIterNext(t *testing.T) {
	m := map[string]int{"a": 1}
	rv := reflect.ValueOf(m)
	iter := &MapIter{Value: ValueOf(m), Keys: rv.MapKeys()}
	n := 0
	for {
		res := iter.Next()
		tuple := res.RValue()
		ok := tuple.Index(0).Interface().(Value).Bool()
		if !ok {
			break
		}
		n++
	}
	if n != 1 {
		t.Errorf("expected 1 iteration, got %d", n)
	}

	// ValueOf(reflect.Value) 不应二次包装
	iter2 := &MapIter{Value: ValueOf(rv), Keys: rv.MapKeys()}
	if !iter2.Next().RValue().Index(0).Interface().(Value).Bool() {
		t.Fatal("expected one element from reflect.Value-wrapped map")
	}
}

func TestExternalValueStore(t *testing.T) {
	x := 10
	ev := NewExternalValue(reflect.ValueOf(&x))
	ev.Store(ValueOf(20))
	if x != 20 {
		t.Errorf("expected 20, got %d", x)
	}
	if ev.ToValue().Int() != 20 {
		t.Errorf("ToValue expected 20, got %v", ev.ToValue().Interface())
	}
}

func TestConverter(t *testing.T) {
	v := ValueOf(int32(5))
	converted, err := DefaultTypeConverter.Convert(v, reflect.TypeOf(int64(0)))
	if err != nil {
		t.Fatal(err)
	}
	if converted.Int() != 5 {
		t.Errorf("expected 5, got %d", converted.Int())
	}
}
