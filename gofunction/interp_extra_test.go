package gofunction

import (
	"go/types"
	"io"
	"strings"
	"testing"
	"time"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

func runSrc(t *testing.T, src, fn string, params ...interface{}) interface{} {
	t.Helper()
	result, err := Run("t", src, fn, params...)
	if err != nil {
		t.Fatalf("%s: %v", fn, err)
	}
	return result
}

func TestParseFuncList(t *testing.T) {
	src := `package main
func Exported() {}
func hidden() {}
func (s *S) Method() {}
type S struct{}
var x = 1
`
	all, err := ParseFuncList(src, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 3 {
		t.Fatalf("exportedAll got %v", all)
	}
	pub, err := ParseFuncList(src, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(pub) != 1 || pub[0] != "Exported" {
		t.Fatalf("exported only got %v", pub)
	}
	if _, err := ParseFuncList("not go", false); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestSetGetGlobalValue(t *testing.T) {
	src := `package main
var N = 1
func test() int { return N }
`
	p, err := BuildProgram("t", "main", src)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SetGlobalValue("N", 9); err != nil {
		t.Fatal(err)
	}
	v, err := p.GetGlobalValue("N")
	if err != nil {
		t.Fatal(err)
	}
	if v != 9 {
		t.Fatalf("got %#v", v)
	}
	if err := p.SetGlobalValue("missing", 1); err == nil {
		t.Fatal("expected missing global error")
	}
	if _, err := p.GetGlobalValue("missing"); err == nil {
		t.Fatal("expected missing global error")
	}
	got, err := p.Run("t", "test")
	if err != nil {
		t.Fatal(err)
	}
	if got != 9 {
		t.Fatalf("run got %#v", got)
	}
	_ = p.Package()
}

func TestBuiltinsAppendCopyLenCapDeletePrint(t *testing.T) {
	src := `package main
func test() []interface{} {
	s := []int{1}
	s = append(s, 2, 3)
	n := copy(s[:1], []int{9})
	m := map[string]int{"a": 1, "b": 2}
	delete(m, "a")
	println("hi", n)
	return []interface{}{len(s), cap(s), n, len(m), s[0]}
}
`
	SetDebugOutput(io.Discard)
	debugging = true
	defer func() { debugging = false }()
	p, err := BuildProgram("t", "main", src)
	if err != nil {
		t.Fatal(err)
	}
	result, ctx, err := p.RunWithContext("t", "test")
	if err != nil {
		t.Fatal(err)
	}
	out := ctx.Output()
	if !strings.Contains(out, "hi") {
		t.Fatalf("println output %q", out)
	}
	got := result.([]interface{})
	if got[0] != 3 || got[2] != 1 || got[3] != 1 || got[4] != 9 {
		t.Fatalf("got %#v", got)
	}
}

func TestBuiltinCloseAndSelect(t *testing.T) {
	src := `package main
func test() int {
	ch := make(chan int, 1)
	ch <- 3
	close(ch)
	select {
	case v, ok := <-ch:
		if !ok {
			return -1
		}
		return v
	default:
		return 0
	}
}
`
	got := runSrc(t, src, "test")
	if got != 3 {
		t.Fatalf("got %#v", got)
	}
}

func TestSelectDefaultAndSend(t *testing.T) {
	src := `package main
func test() int {
	ch := make(chan int)
	select {
	case ch <- 1:
		return 1
	default:
		return 2
	}
}
`
	got := runSrc(t, src, "test")
	if got != 2 {
		t.Fatalf("got %#v", got)
	}
}

func TestGoStatementAndChannel(t *testing.T) {
	src := `package main
func test() int {
	ch := make(chan int, 1)
	go func() { ch <- 11 }()
	return <-ch
}
`
	got := runSrc(t, src, "test")
	if got != 11 {
		t.Fatalf("got %#v", got)
	}
}

func TestPanicRecoverNamed(t *testing.T) {
	src := `package main
func test() (ret int) {
	defer func() {
		if recover() != nil {
			ret = 7
		}
	}()
	panic("boom")
	return 0
}
`
	got := runSrc(t, src, "test")
	if got != 7 {
		t.Fatalf("got %#v", got)
	}
}

func TestTypeAssertCommaOk(t *testing.T) {
	src := `package main
func test() []interface{} {
	var i interface{} = "x"
	s, ok1 := i.(string)
	n, ok2 := i.(int)
	return []interface{}{s, ok1, n, ok2}
}
`
	got := runSrc(t, src, "test").([]interface{})
	if got[0] != "x" || got[1] != true || got[3] != false {
		t.Fatalf("got %#v", got)
	}
}

func TestSlice3AndIndexAndField(t *testing.T) {
	src := `package main
type P struct{ A, B int }
func test() []interface{} {
	s := []int{1, 2, 3, 4}
	u := s[1:3:4]
	arr := [2]int{5, 6}
	p := P{A: 8, B: 9}
	return []interface{}{len(u), cap(u), arr[1], p.A, p.B}
}
`
	got := runSrc(t, src, "test").([]interface{})
	if got[0] != 2 || got[2] != 6 || got[3] != 8 {
		t.Fatalf("got %#v", got)
	}
}

func TestMapLookupCommaOk(t *testing.T) {
	src := `package main
func test() []interface{} {
	m := map[string]int{"a": 1}
	v, ok := m["a"]
	_, ok2 := m["z"]
	return []interface{}{v, ok, ok2}
}
`
	got := runSrc(t, src, "test").([]interface{})
	if got[0] != 1 || got[1] != true || got[2] != false {
		t.Fatalf("got %#v", got)
	}
}

func TestUnaryOpsAndConvert(t *testing.T) {
	src := `package main
func test() []interface{} {
	x := 3
	f := float64(x)
	b := !false
	n := -x
	u := uint(4)
	return []interface{}{f, b, n, u ^ 1}
}
`
	got := runSrc(t, src, "test").([]interface{})
	if got[1] != true || got[2] != -3 {
		t.Fatalf("got %#v", got)
	}
}

func TestFunctionNotFoundAndBuildError(t *testing.T) {
	if _, err := Run("t", "package main\nfunc a() {}", "missing"); err == nil {
		t.Fatal("expected missing function")
	}
	if _, err := Run("t", "not go source", "a"); err == nil {
		t.Fatal("expected build error")
	}
}

func TestChanRecvCommaOk(t *testing.T) {
	src := `package main
func test() []interface{} {
	ch := make(chan int, 1)
	ch <- 4
	close(ch)
	v, ok := <-ch
	v2, ok2 := <-ch
	return []interface{}{v, ok, v2, ok2}
}
`
	got := runSrc(t, src, "test").([]interface{})
	if got[0] != 4 || got[1] != true || got[3] != false {
		t.Fatalf("got %#v", got)
	}
}

func TestClosureFreeVars(t *testing.T) {
	src := `package main
func test() int {
	n := 1
	inc := func() { n++ }
	inc()
	inc()
	return n
}
`
	got := runSrc(t, src, "test")
	if got != 3 {
		t.Fatalf("got %#v", got)
	}
}

func TestBytesAppendString(t *testing.T) {
	src := `package main
func test() string {
	b := []byte("he")
	b = append(b, "llo"...)
	return string(b)
}
`
	got := runSrc(t, src, "test")
	if got != "hello" {
		t.Fatalf("got %#v", got)
	}
}

func TestWaitGoroutineTimeoutPath(t *testing.T) {
	ctx := newCallContext()
	ctx.goroutines = 1
	start := time.Now()
	ctx.waitGoroutines(5 * time.Millisecond)
	if time.Since(start) < 5*time.Millisecond {
		t.Fatal("expected wait to honor timeout")
	}
	ctx.cancelFunc()
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
