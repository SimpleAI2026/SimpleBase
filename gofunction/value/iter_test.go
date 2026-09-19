package value

import (
	"reflect"
	"testing"
)

func TestNewStringIter(t *testing.T) {
	iter := NewStringIter("hél")
	if !iter.IsSeq() {
		t.Fatal("string iter should be seq")
	}
	if iter.SeqLen() != 3 {
		t.Fatalf("rune len=%d", iter.SeqLen())
	}
	k, v := iter.SeqElem(1)
	if k.Int() != 1 || v.Interface() != 'é' {
		t.Fatalf("elem1=%v %v", k, v)
	}
	n := 0
	for {
		ok, _, _ := iter.next()
		if !ok {
			break
		}
		n++
	}
	if n != 3 {
		t.Fatalf("iterated %d", n)
	}
	if ok, _, _ := iter.next(); ok {
		t.Fatal("exhausted string iter")
	}
	res := NewStringIter("").Next()
	ok := res.RValue().Index(0).Interface().(Value).Bool()
	if ok {
		t.Fatal("empty string should yield no elements")
	}
}

func TestNewSliceIterPtrAndValue(t *testing.T) {
	xs := []int{10, 20}
	iter := NewSliceIter(ValueOf(&xs))
	if !iter.IsSeq() {
		t.Fatal("slice iter")
	}
	if iter.SeqLen() != 2 {
		t.Fatalf("ptr slice len=%d", iter.SeqLen())
	}
	k, v := iter.SeqElem(1)
	if k.Int() != 1 || v.Int() != 20 {
		t.Fatalf("ptr elem: %v %v", k, v)
	}
	got := 0
	for {
		res := iter.Next()
		ok := res.RValue().Index(0).Interface().(Value).Bool()
		if !ok {
			break
		}
		got++
	}
	if got != 2 {
		t.Fatalf("iterated %d", got)
	}

	iter2 := NewSliceIter(ValueOf([]string{"a"}))
	if iter2.SeqLen() != 1 {
		t.Fatal(iter2.SeqLen())
	}
	_, ev := iter2.SeqElem(0)
	if ev.String() != "a" {
		t.Fatal(ev)
	}
}

func TestMapIterEmptyNextAndSeqLenMap(t *testing.T) {
	iter := &MapIter{Value: ValueOf(map[string]int{}), Keys: nil}
	if iter.IsSeq() {
		t.Fatal("map iter is not seq")
	}
	if iter.SeqLen() != 0 {
		t.Fatalf("map SeqLen=%d", iter.SeqLen())
	}
	res := iter.Next()
	ok := res.RValue().Index(0).Interface().(Value).Bool()
	if ok {
		t.Fatal("empty map Next should be false")
	}
	if ok2, _, _ := iter.next(); ok2 {
		t.Fatal("empty next")
	}
}

func TestPackageUnpackageLeftovers(t *testing.T) {
	empty := Package(nil)
	if empty.Kind() != reflect.Slice || empty.Len() != 0 {
		t.Fatalf("empty package: %v %d", empty.Kind(), empty.Len())
	}
	un := Unpackage(empty)
	if len(un) != 0 {
		t.Fatalf("unpackage empty: %d", len(un))
	}

	packed := Package([]reflect.Value{reflect.ValueOf(1), reflect.ValueOf(true)})
	out := Unpackage(packed)
	if len(out) != 2 || out[0].Int() != 1 || !out[1].Bool() {
		t.Fatalf("roundtrip: %v", out)
	}

	scalar := Unpackage(ValueOf("z"))
	if len(scalar) != 1 || scalar[0].String() != "z" {
		t.Fatalf("scalar: %v", scalar)
	}
}

func TestRValueIsNilKinds(t *testing.T) {
	if !ValueOf((map[string]int)(nil)).IsNil() {
		t.Fatal("nil map")
	}
	if !ValueOf((chan int)(nil)).IsNil() {
		t.Fatal("nil chan")
	}
	if ValueOf(1).IsNil() {
		t.Fatal("int not nil")
	}
}
