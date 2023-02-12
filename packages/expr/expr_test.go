package expr

import (
	"fmt"
	"math"
	"testing"
)

func TestExample(t *testing.T) {
	pool, _ := NewPool(map[string]BuiltinFunc{
		"sum": func(args ...interface{}) (interface{}, error) {
			var sum int64
			for _, v := range args {
				if v1, ok := v.(int64); ok {
					sum += v1
				} else {
					return nil, fmt.Errorf("%v isn't int64", v)
				}
			}
			return sum, nil
		},
	})
	e, err := New(`(sum(1,2,3) + x + y + x*y + x/y) && false`, pool)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	val, err := e.Eval(map[string]interface{}{
		"x": 122.0,
		"y": 123.0,
	})
	if err != nil {
		t.Fatalf("eval error: `%v`", err)
	}
	t.Logf("val: `%v`", val.String())
}

func TestConstExpr(t *testing.T) {
	for _, x := range []struct {
		s   string
		val float64
	}{
		{"1", 1},
		{"1+2", 3},
		{"1*2+3", 5},
		{"1*(2+3)", 5},
	} {
		e, err := New(x.s, nil)
		if err != nil {
			t.Errorf("parse %q error: %v", x.s, err)
			continue
		}
		val, err := e.Eval(nil)
		if err != nil {
			t.Errorf("parse %q error: %v", x.s, err)
			continue
		}
		if math.Abs(val.Float()-x.val) > 1e-6 {
			t.Errorf("%q want %v, got %v", x.s, x.val, val)
		}
	}
}

func TestVarExpr(t *testing.T) {
	getter := Getter{
		"x": 1.5,
		"y": 2.5,
		"m": 5.0,
		"n": 2.0,
	}

	for _, x := range []struct {
		s     string
		val   float64
		isErr bool
	}{
		{"1", 1, false},
		{"x", 1.5, false},
		{"y", 2.5, false},
		{"x+y", 4, false},
		{"x-y", -1, false},
		{"x*y", 3.75, false},
		{"x/y", 0.6, false},
		{"m / n", 2.5, false},
		{"m / n // line comment", 2.5, false},
		{"m / /*multiline\n	comment*/ n", 2.5, false},
		{"m%n", 1, false},
		{"(1+x)*y/(m-n)^2", 4.34027776, false},
		{"(1+x)*y/((m-n)^2)", 0.69444444, false},

		{"undefined_func(x,y,m)", 0, true},
		{"max()", 0, true},
		{"m!-!n", 0, true},
		{"m / undefined_var", 0, true},
	} {
		e, err := New(x.s, nil)
		if err != nil {
			if !x.isErr {
				t.Errorf("parse %q error: %v", x.s, err)
			}
			continue
		}
		val, err := e.Eval(getter)
		if err != nil {
			if !x.isErr {
				t.Errorf("parse %q error: %v", x.s, err)
			}
			continue
		}
		if math.Abs(val.Float()-x.val) > 1e-6 {
			t.Errorf("%q want %f, got %f", x.s, x.val, val.Float())
		}
	}
}

func TestCustomFactory(t *testing.T) {
	pool, _ := NewPool(map[string]BuiltinFunc{
		"sum": func(args ...interface{}) (interface{}, error) {
			var sum float64
			for _, v := range args {
				if v1, ok := v.(float64); ok {
					sum += v1
				} else {
					return nil, fmt.Errorf("%v isn't float64", v)
				}
			}
			return sum, nil
		},
		"average": func(args ...interface{}) (interface{}, error) {
			var sum float64
			for _, v := range args {
				if v1, ok := v.(float64); ok {
					sum += v1
				} else {
					return nil, fmt.Errorf("%v isn't float64", v)
				}
			}
			return sum / float64(len(args)), nil
		},
	})
	getter := Getter{
		"x": 1.5,
		"y": 2.5,
	}
	for i, x := range []struct {
		s     string
		val   float64
		isErr bool
	}{
		{"sum(1,2,3)", 6, false},
		{"sum()", 0, false},
		{"sum(x, y, x)", 5.5, false},
		{"average(x)", 1.5, false},
		{"average(x,y)", 2, false},
		{"average()", 0, true},
	} {
		e, err := New(x.s, pool)
		if err != nil {
			if !x.isErr {
				t.Errorf("%dth: parse %q error: %v", i, x.s, err)
			}
			continue
		}
		val, err := e.Eval(getter)
		if err != nil {
			if !x.isErr {
				t.Errorf("%dth: parse %q error: %v", i, x.s, err)
			}
			continue
		}
		if math.Abs(val.Float()-x.val) > 1e-6 {
			t.Errorf("%dth: %q want %f, got %f", i, x.s, x.val, val.Float())
		}
	}
}

func TestOnVarMissing(t *testing.T) {
	defaults := map[string]Value{
		"a": Int(0),
		"b": Int(1),
	}
	pool, _ := NewPool()
	pool.SetOnVarMissing(func(varName string) (Value, error) {
		if dft, ok := defaults[varName]; ok {
			return dft, nil
		}
		return DefaultOnVarMissing(varName)
	})
	if _, err := Eval("2 / b + a + x", map[string]interface{}{"x": 1}, pool); err != nil {
		t.Errorf("err: %v", err)
	}
	if _, err := Eval("2 / b + a + undefined", nil, pool); err != nil {
		t.Errorf("err: %v", err)
	}
}

func TestOp(t *testing.T) {
	for i, tc := range []struct {
		s      string
		result Value
		err    error
	}{
		{`1`, varTrue, nil},
		{`0`, varFalse, nil},
		{`0.0`, varFalse, nil},
		{`1.0`, varTrue, nil},
		{`"a"`, varTrue, nil},
		{`'a'`, varTrue, nil},
		{`'a'`, String("a"), nil},
		{`2 > 1`, varTrue, nil},
		{`2 >= 1`, varTrue, nil},
		{`1 >= 1`, varTrue, nil},
		{`1 <= 1`, varTrue, nil},
		{`1 == 1`, varTrue, nil},
		{`1 >= 2`, varFalse, nil},
		{`1 && 0`, varFalse, nil},
		{`1 && 2`, varTrue, nil},
		{`1 || 0`, varTrue, nil},
		{`1 < 2`, varTrue, nil},
		{`1 < 1`, varFalse, nil},
		{`1 <= 2`, varTrue, nil},
		{`1 <= 0`, varFalse, nil},
		{`1 != 0`, varTrue, nil},
		{`1 != 1`, varFalse, nil},
		{`"a" != "b"`, varTrue, nil},
		{`"a" != "a"`, varFalse, nil},
		{`"a" == "a"`, varTrue, nil},
		{`"ab" < "ac"`, varTrue, nil},
		{`"ab" <= "ac"`, varTrue, nil},
		{`"ab" <= "ab"`, varTrue, nil},
		{`"ab" > "ab"`, varFalse, nil},
		{`'a' == 'a'`, varTrue, nil},

		{`"a" + "b"`, String("ab"), nil},
		{`"ab" + "bc"`, String("abbc"), nil},
		{`2 - 1 > 0`, varTrue, nil},
		{`2 - 1 > 2`, varFalse, nil},
		{`2 + 1 > 2`, varTrue, nil},
		{`2 - 1`, Int(1), nil},

		{`"a" - "b"`, varInvalid, ErrUnsupportedType},
		{`"a" + 1`, varInvalid, ErrUnsupportedType},
		{`"a" + 1.2`, varInvalid, ErrUnsupportedType},
		{`"a" - 1`, varInvalid, ErrUnsupportedType},
		{`"a" == 1`, varInvalid, ErrUnsupportedType},
		{`"a" != 1`, varInvalid, ErrUnsupportedType},
		{`"a" > 1`, varInvalid, ErrUnsupportedType},
		{`"a" < 1`, varInvalid, ErrUnsupportedType},
		{`"a" >= 1`, varInvalid, ErrUnsupportedType},
		{`"a" <= 1`, varInvalid, ErrUnsupportedType},
		{`1/0`, varInvalid, ErrDivideZero},
		{`1%0`, varInvalid, ErrDivideZero},
		{`0^2`, varInvalid, ErrPowOfZero},
		{`0.0^2`, varInvalid, ErrPowOfZero},
	} {
		e, err := New(tc.s, nil)
		if err != nil {
			t.Errorf("%dth: invalid expression `%s'", i, tc.s)
			continue
		}
		got, err := e.Eval(nil)
		if err != nil {
			if err != tc.err {
				t.Errorf("%dth: parse `%v` want error `%v', got error `%v'", i, tc.s, tc.err, err)
				continue
			}
		} else if tc.err != nil {
			t.Errorf("%dth: parse `%v` want error `%s', got nil", i, tc.s, tc.err)
			continue
		}
		eq, _ := tc.result.Ne(got)
		if eq.Bool() {
			t.Errorf("%dth: parse `%v` result error, want `%s', got `%s'", i, tc.s, tc.result.String(), got.String())
		}
	}
}

func TestOpWithGetter(t *testing.T) {
	pool, err := NewPool(map[string]BuiltinFunc{
		"contains": func(args ...interface{}) (interface{}, error) {
			return nil, ErrUnsupportedType
		},
	})
	if err != nil {
		panic(err)
	}
	getter := Getter{
		"a": "u",
		"b": "v",
		"c": "w",
		"x": 1,
		"y": 2,
		"z": 0,
	}
	for i, tc := range []struct {
		s      string
		result Value
		err    error
	}{
		{`a + b`, String("uv"), nil},
		{`x + y`, Int(3), nil},
		{`a + 1`, varInvalid, ErrUnsupportedType},
		{`a > 1`, varInvalid, ErrUnsupportedType},
	} {
		e, err := New(tc.s, pool)
		if err != nil {
			t.Errorf("%dth: invalid expression `%s'", i, tc.s)
			continue
		}
		got, err := e.Eval(getter)
		if err != nil {
			if err != tc.err {
				t.Errorf("%dth: want error `%v', got error `%v'", i, tc.err, err)
				continue
			}
		} else if tc.err != nil {
			t.Errorf("%dth: want error `%s', got nil", i, tc.err)
			continue
		}
		eq, _ := tc.result.Ne(got)
		if eq.Bool() {
			t.Errorf("%dth: result error, want `%s', got `%s'", i, tc.result.String(), got.String())
		}
	}
}
