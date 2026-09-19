package gofunction

// 本文件覆盖类型转换辅助：typeChange/conv/deref 及异常路径。

import (
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"go/token"
	"go/types"
	"reflect"
	"testing"
)

func TestTypeChangeKindsAndConvNil(t *testing.T) {
	_ = typeChange(types.NewChan(types.RecvOnly, types.Typ[types.Int]))
	_ = typeChange(types.NewChan(types.SendOnly, types.Typ[types.Int]))
	_ = typeChange(types.NewChan(types.SendRecv, types.Typ[types.Int]))

	params := types.NewTuple(types.NewVar(token.NoPos, nil, "x", types.Typ[types.Int]))
	results := types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.String]))
	sig := types.NewSignatureType(nil, nil, nil, params, results, false)
	rt := typeChange(sig)
	if rt.Kind() != reflect.Func || rt.NumIn() != 1 || rt.NumOut() != 1 {
		t.Fatalf("signature type %s", rt)
	}
	variadic := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "a", types.NewSlice(types.Typ[types.Int]))),
		nil, true)
	if !typeChange(variadic).IsVariadic() {
		t.Fatal("expected variadic")
	}

	st := types.NewStruct([]*types.Var{
		types.NewVar(token.NoPos, nil, "A", types.Typ[types.Int]),
	}, []string{`json:"a"`})
	if typeChange(st).NumField() != 1 {
		t.Fatal("struct fields")
	}

	z := conv(nil, types.Typ[types.Int])
	if z.Int() != 0 {
		t.Fatalf("nil conv %v", z.Interface())
	}

	mustPanic(t, "invalid basic", func() {
		_ = typeChange(types.Typ[types.Invalid])
	})
	mustPanic(t, "unsupported type", func() {
		_ = typeChange(types.NewTuple())
	})
}

func TestTypeChangeUnsupportedPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = typeChange(nil)
}

func TestDerefNonPointer(t *testing.T) {
	got := deref(types.Typ[types.Int])
	if got != types.Typ[types.Int] {
		t.Fatalf("got %s", got)
	}
	ptr := types.NewPointer(types.Typ[types.Int])
	if deref(ptr) != types.Typ[types.Int] {
		t.Fatal("deref pointer")
	}
}

func TestConvNil(t *testing.T) {
	src := `package main
func test() *int {
	var p *int
	var q *int = p
	return q
}
`
	got := runSrc(t, src, "test")
	if got != (*int)(nil) {
		t.Fatalf("got %#v", got)
	}
}
