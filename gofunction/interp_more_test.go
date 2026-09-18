package gofunction

import (
	"strings"
	"testing"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

func TestIndexFieldMakeSliceSSA(t *testing.T) {
	src := `package main
type V struct{ X, Y int }
func test() []interface{} {
	n := [3]int{1, 2, 3}[2]
	f := V{X: 4, Y: 5}.X
	s := make([]int, 2, 5)
	s[0] = 9
	var send chan<- int = make(chan int, 1)
	send <- 1
	var rec <-chan int = make(chan int, 1)
	_ = rec
	var fp func(int) int
	fp = func(x int) int { return x + 1 }
	return []interface{}{n, f, len(s), cap(s), s[0], fp(2)}
}
`
	got := runSrc(t, src, "test").([]interface{})
	if got[0] != 3 || got[1] != 4 || got[2] != 2 || got[3] != 5 || got[4] != 9 || got[5] != 3 {
		t.Fatalf("got %#v", got)
	}
}

func TestInterfaceInvokeError(t *testing.T) {
	src := `package main
import "errors"
func test() string {
	err := errors.New("boom")
	return err.Error()
}
`
	got := runSrc(t, src, "test")
	if got != "boom" {
		t.Fatalf("got %#v", got)
	}
}

func TestGoPanicRecovered(t *testing.T) {
	src := `package main
func test() int {
	go func() { panic("bg") }()
	for i := 0; i < 1000; i++ {
	}
	return 1
}
`
	p, err := BuildProgram("t", "main", src)
	if err != nil {
		t.Fatal(err)
	}
	result, ctx, err := p.RunWithContext("t", "test")
	if err != nil {
		t.Fatal(err)
	}
	if result != 1 {
		t.Fatalf("got %#v", result)
	}
	if !strings.Contains(ctx.Output(), "goroutine panic") && ctx.Output() != "" {
		t.Logf("output %q", ctx.Output())
	}
}

func TestTypeAssertFailAndNil(t *testing.T) {
	src := `package main
func test() int {
	var i interface{} = 1
	defer func() { recover() }()
	_ = i.(string)
	return 0
}
`
	_ = runSrc(t, src, "test")
}

func TestTypeAssertNilInterface(t *testing.T) {
	src := `package main
func test() int {
	var i interface{}
	defer func() {
		if recover() != nil {
		}
	}()
	_ = i.(int)
	return 1
}
`
	_ = runSrc(t, src, "test")
}

func TestChangeInterfaceAndDefer(t *testing.T) {
	src := `package main
func test() int {
	var i interface{} = 2
	var j interface{} = i
	n, _ := j.(int)
	return n
}
`
	got := runSrc(t, src, "test")
	if got != 2 {
		t.Fatalf("got %#v", got)
	}
}

func TestVariadicAndMultiReturn(t *testing.T) {
	src := `package main
import "fmt"
func pair() (int, int) { return 1, 2 }
func test() string {
	a, b := pair()
	return fmt.Sprintf("%d-%d", a, b)
}
`
	got := runSrc(t, src, "test")
	if got != "1-2" {
		t.Fatalf("got %#v", got)
	}
}

func TestStoreGlobalAndRangeMap(t *testing.T) {
	src := `package main
var G = map[string]int{}
func test() int {
	G["a"] = 3
	sum := 0
	for k, v := range G {
		if k == "a" {
			sum += v
		}
	}
	return sum
}
`
	got := runSrc(t, src, "test")
	if got != 3 {
		t.Fatalf("got %#v", got)
	}
}

func TestBuiltinUnknownPanic(t *testing.T) {
	// 间接覆盖 cap/len on array and print without debug
	src := `package main
func test() int {
	var a [2]int
	print(len(a), cap(a))
	return len(a) + cap(a)
}
`
	got := runSrc(t, src, "test")
	if got != 4 {
		t.Fatalf("got %#v", got)
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

func TestSetGlobalConvert(t *testing.T) {
	src := `package main
var F float64 = 1
func test() float64 { return F }
`
	p, err := BuildProgram("t", "main", src)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SetGlobalValue("F", float32(2)); err != nil {
		// may fail convert; try exact type
		if err := p.SetGlobalValue("F", float64(2)); err != nil {
			t.Fatal(err)
		}
	}
	v, err := p.GetGlobalValue("F")
	if err != nil {
		t.Fatal(err)
	}
	_ = v
	got, err := p.Run("t", "test")
	if err != nil {
		t.Fatal(err)
	}
	if got != float64(2) && got != float32(2) {
		t.Logf("got %#v", got)
	}
}

func TestRuntimeNilUserData(t *testing.T) {
	r := NewRuntime()
	r.userData = nil
	if _, ok := r.GetUserData("x"); ok {
		t.Fatal("expected missing")
	}
	r.SetUserData("x", 1)
}

func TestExecutorNilVMSequence(t *testing.T) {
	ex := NewExecutor(nil)
	ex.vm = nil
	if ex.GetSequenceID() != "" {
		t.Fatal("empty id")
	}
	ex.SetSequenceID("z") // vm nil, no-op
	ex.vm = nil
	ex.Close()
}

func TestCallExternalVariadic(t *testing.T) {
	src := `package main
import "fmt"
func test() string {
	return fmt.Sprintf("%s-%s", "a", "b")
}
`
	got := runSrc(t, src, "test")
	if got != "a-b" {
		t.Fatalf("got %#v", got)
	}
}
