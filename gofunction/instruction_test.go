package gofunction

// 本文件覆盖 SSA 指令处理器：选择/通道/切片/类型断言/全局存储/range 迭代等。

import (
	"go/constant"
	"go/types"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

func TestStoreLookupExtractSelectEdges(t *testing.T) {
	src := `package main
func test() []interface{} {
	s := "hi"
	n, err := strconvAtoi("7")
	ch := make(chan int, 1)
	ch <- 5
	v := 0
	select {
	case v = <-ch:
	case <-make(chan int):
	}
	return []interface{}{s[1], n, err == nil, v}
}
func strconvAtoi(s string) (int, error) {
	if s == "7" {
		return 7, nil
	}
	return 0, nil
}
`
	got := runSrc(t, src, "test").([]interface{})
	if got[0] != byte('i') && got[0] != uint8('i') {
		t.Fatalf("string index %#v", got[0])
	}
	if got[1] != 7 || got[3] != 5 {
		t.Fatalf("got %#v", got)
	}

	// extract of a non-[]value.Value tuple
	key := &ssa.Alloc{}
	ex := &ssa.Extract{Tuple: key}
	// Index is a field on Extract
	ex.Index = 0
	fr := &frame{layout: testLayout(key, ex), env: make([]value.Value, 2)}
	fr.env[0] = value.ValueOf([]int{9, 8})
	runExtract(fr, ex)
	if fr.get(ex).Int() != 9 {
		t.Fatalf("extract %v", fr.get(ex).Interface())
	}

	// store panics
	p, err := BuildProgram("t", "main", `package main
var F float64 = 1
var N int = 0
func test() float64 { return F }
`)
	if err != nil {
		t.Fatal(err)
	}
	intConst := ssa.NewConst(constant.MakeInt64(2), types.Typ[types.Int])

	// 缺失地址：布局不含 Addr，get 返回零值触发 store panic
	stFr := &frame{program: p, context: newCallContext(), layout: &funcLayout{slots: map[ssa.Value]int{}}, env: nil}
	mustPanic(t, "store missing alloc", func() {
		runStore(stFr, &ssa.Store{Addr: &ssa.Alloc{}, Val: intConst})
	})
	g := p.mainPkg.Members["F"].(*ssa.Global)
	runStore(stFr, &ssa.Store{Addr: g, Val: intConst}) // int → float64 convert
	cell := value.ValueOf(1)
	p.globals[g] = &cell
	runStore(stFr, &ssa.Store{Addr: g, Val: intConst}) // non-ptr cell replace

	gN := p.mainPkg.Members["N"].(*ssa.Global)
	delete(p.globals, gN)
	mustPanic(t, "store missing global", func() {
		runStore(stFr, &ssa.Store{Addr: gN, Val: intConst})
	})

	addr := &ssa.FieldAddr{}
	stFr.layout = testLayout(addr)
	stFr.env = make([]value.Value, 1)
	stFr.env[0] = value.Value(value.RValue{})
	mustPanic(t, "store invalid addr", func() {
		runStore(stFr, &ssa.Store{Addr: addr, Val: intConst})
	})
	stFr.env[0] = value.ValueOf(1) // 非 ptr
	mustPanic(t, "store non-ptr", func() {
		runStore(stFr, &ssa.Store{Addr: addr, Val: intConst})
	})

	var boxed interface{} = []int{1, 2, 3}
	rv := reflect.ValueOf(&boxed).Elem()
	cv := containerValue(value.RValue{Value: rv})
	if cv.Kind() != reflect.Slice {
		t.Fatalf("container kind %s", cv.Kind())
	}
}

func TestStringIndexAndBlockingSelect(t *testing.T) {
	src := `package main
func test() int {
	ch := make(chan int, 1)
	ch <- 3
	select {
	case v := <-ch:
		return v
	}
}
`
	got := runSrc(t, src, "test")
	if got != 3 {
		t.Fatalf("got %#v", got)
	}
}

func TestAppendNilPointerElems(t *testing.T) {
	src := `package main
func test() int {
	s := []*int{}
	s = append(s, nil)
	return len(s)
}
`
	got := runSrc(t, src, "test")
	if got != 1 {
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
