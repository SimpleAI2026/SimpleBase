package gofunction

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func runExtraSrc(t *testing.T, src, fn string, args ...interface{}) interface{} {
	t.Helper()
	p, err := BuildProgram("seq-extra", "extra.go", src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.Run("seq-extra", fn, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSelectDefaultAndClosedRecv(t *testing.T) {
	src := `package main
func SelectDefault() int {
	ch := make(chan int)
	select {
	case <-ch:
		return 1
	default:
		return 2
	}
}
func RecvClosed() int {
	ch := make(chan int)
	close(ch)
	v, ok := <-ch
	if ok {
		return v
	}
	return -1
}
func SendRecv() int {
	ch := make(chan int, 1)
	ch <- 7
	return <-ch
}
func TypeSwitch(x interface{}) int {
	switch x.(type) {
	case int:
		return 1
	case string:
		return 2
	default:
		return 3
	}
}
func SliceOps() int {
	s := []int{1, 2, 3, 4}
	s = s[1:3:3]
	s = append(s, 9)
	return len(s) + cap(s) + s[0]
}
func MakeCap() int {
	s := make([]int, 2, 5)
	m := make(map[string]int, 4)
	m["a"] = 1
	return cap(s) + len(m)
}
func Pointer() int {
	x := 3
	p := &x
	*p = 8
	return *p
}
func MultiReturn() int {
	a, b := split(9)
	return a + b
}
func split(n int) (int, int) { return n / 2, n - n/2 }
`
	if v := runExtraSrc(t, src, "SelectDefault"); v != 2 {
		t.Fatalf("select=%v", v)
	}
	if v := runExtraSrc(t, src, "RecvClosed"); v != -1 {
		t.Fatalf("recv=%v", v)
	}
	if v := runExtraSrc(t, src, "SendRecv"); v != 7 {
		t.Fatalf("send=%v", v)
	}
	if v := runExtraSrc(t, src, "TypeSwitch", 1); v != 1 {
		t.Fatalf("int=%v", v)
	}
	if v := runExtraSrc(t, src, "TypeSwitch", "s"); v != 2 {
		t.Fatalf("str=%v", v)
	}
	if v := runExtraSrc(t, src, "TypeSwitch", 1.2); v != 3 {
		t.Fatalf("def=%v", v)
	}
	if v := runExtraSrc(t, src, "SliceOps"); v == nil {
		t.Fatal("slice")
	}
	if v := runExtraSrc(t, src, "MakeCap"); v != 6 {
		t.Fatalf("makecap=%v", v)
	}
	if v := runExtraSrc(t, src, "Pointer"); v != 8 {
		t.Fatalf("ptr=%v", v)
	}
	if v := runExtraSrc(t, src, "MultiReturn"); v != 9 {
		t.Fatalf("multi=%v", v)
	}
}

func TestComplexAndStringConvert(t *testing.T) {
	src := `package main
func Complex() complex128 { return complex(1, 2) }
func Bytes() string { return string([]byte{'a', 'b'}) }
func Runes() int { return len([]rune("你好")) }
func Shift() int { return (1 << 3) | (8 >> 1) }
func Unary() int {
	x := 5
	return +x + ^0 + -x
}
func Compare() int {
	if 1 < 2 && 2 <= 2 && 3 > 1 && 3 >= 3 && 1 != 0 && 2 == 2 {
		return 1
	}
	return 0
}
`
	_ = runExtraSrc(t, src, "Complex")
	if v := runExtraSrc(t, src, "Bytes"); v != "ab" {
		t.Fatalf("bytes=%v", v)
	}
	if v := runExtraSrc(t, src, "Runes"); v != 2 {
		t.Fatalf("runes=%v", v)
	}
	if v := runExtraSrc(t, src, "Shift"); v != 12 {
		t.Fatalf("shift=%v", v)
	}
	_ = runExtraSrc(t, src, "Unary")
	if v := runExtraSrc(t, src, "Compare"); v != 1 {
		t.Fatalf("cmp=%v", v)
	}
}

func TestRunJSONAndTimeout(t *testing.T) {
	src := `package main
func Hello(m map[string]interface{}) string { return "ok" }
`
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := RunJSON(ctx, "seq-json", "f.go", src, "Hello", []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `"ok"` {
		t.Fatalf("%s", out)
	}
	_ = reflect.TypeOf(out)
}

func TestDebugAndWaitEventLoop(t *testing.T) {
	e := NewExecutor(nil)
	e.SetWaitEventLoop(false)
	e.SetExecutionTimeout(-1)
	src := `package main
func F() int { return 42 }
`
	v, err := Run("seq-dbg", src, "F")
	if err != nil || v != 42 {
		t.Fatalf("%v %v", v, err)
	}
	if err := e.Compile("F", src, false); err != nil {
		t.Fatal(err)
	}
	out, err := e.Execute("F", src)
	if err != nil || out != 42 {
		t.Fatalf("exec %v %v", out, err)
	}
	_ = e.GetCostTime()
	_ = e.GetSequenceID()
	e.SetSequenceID("seq-x")
	e.ClearInterrupt()
	e.Close()
}
