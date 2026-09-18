package testdata

import (
	"fmt"
	"math"
)

type Vertex2 struct {
	X float64
	Y float64
}

func (v Vertex2) Abs() float64 {
	return math.Sqrt(v.X*v.X + v.Y*v.Y)
}
func (__testSet) Methods() interface{} {
	v := Vertex2{3, 4}
	return fmt.Sprint(v.Abs())
}

func abs(v Vertex2) float64 {
	return math.Sqrt(v.X*v.X + v.Y*v.Y)
}
func (__testSet) MethodsFuncs() interface{} {
	v := Vertex2{3, 4}
	return fmt.Sprint(abs(v))
}

type MyFloat float64

func (f MyFloat) Abs() float64 {
	if f < 0 {
		return float64(-f)
	}
	return float64(f)
}
func (__testSet) Methods2() interface{} {
	f := MyFloat(-math.Sqrt2)
	return fmt.Sprint(f.Abs())
}

func (v *Vertex2) Scale(f float64) {
	v.X = v.X * f
	v.Y = v.Y * f
}
func (__testSet) MethodsPointers() interface{} {
	v := Vertex2{3, 4}
	v.Scale(10)
	return fmt.Sprint(v.Abs())
}

type Abser interface {
	Abs() float64
}

type I interface {
	M() string
}

type T struct {
	S string
}

func (t *T) M() string {
	if t == nil {
		return "<nil>"
	}
	return t.S
}

func (__testSet) TypeAssertions() interface{} {
	var i interface{} = "hello"
	s1 := i.(string)
	s2, ok1 := i.(string)
	f, ok2 := i.(float64)
	return fmt.Sprint(s1, s2, ok1, f, ok2)
}

func (__testSet) TypeAssertions2() interface{} {
	m := map[string]interface{}{
		"a": "123",
		"b": nil,
	}
	i, ok2 := m["b"].(float64)
	return fmt.Sprint(m["a"].(string), i, ok2)
}

func do(i interface{}) string {
	switch v := i.(type) {
	case int:
		return fmt.Sprintf("Twice %v is %v\n", v, v*2)
	case string:
		return fmt.Sprintf("%q is %v bytes long\n", v, len(v))
	default:
		return fmt.Sprintf("I don't know about type %T!\n", v)
	}
}

func (__testSet) TypeSwitches() interface{} {
	return fmt.Sprint(do(21), do("hello"), do(true))
}
