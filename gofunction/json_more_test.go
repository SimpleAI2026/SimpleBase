package gofunction

import (
	"context"
	"errors"
	"go/types"
	"strings"
	"testing"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"golang.org/x/tools/go/ssa"
)

func TestValidateHTTPFuncsUnsupportedAndMixed(t *testing.T) {
	src := `package main
func Good(m map[string]interface{}) string { return "ok" }
func Chan(c chan int) int { return 0 }
func Two() (int, int) { return 1, 2 }
`
	_, err := ValidateHTTPFuncs(src)
	if err == nil || !strings.Contains(err.Error(), "不符合约定") {
		t.Fatalf("mixed: %v", err)
	}

	src = `package main
func OnlyBad() (int, int) { return 0, 1 }
`
	_, err = ValidateHTTPFuncs(src)
	if err == nil || !strings.Contains(err.Error(), "至少导出一个合规") {
		t.Fatalf("only bad: %v", err)
	}
}

func TestRunJSONBranches(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RunJSON(ctx, "s", "f.go", `package main
func Hello(m map[string]interface{}) string { return "ok" }
`, "Hello", []byte(`{}`))
	if err == nil {
		t.Fatal("canceled")
	}

	src := `package main
func Hello(m map[string]interface{}) string { return "ok" }
func Ptr(m *map[string]interface{}) string { return "p" }
func Num(x int) int { return x }
func Zero() int { return 0 }
`
	_, err = RunJSON(context.Background(), "s", "f.go", src, "Zero", []byte(`{}`))
	if err == nil {
		t.Fatal("arity")
	}
	_, err = RunJSON(context.Background(), "s", "f.go", src, "Num", nil)
	if err == nil || !errors.Is(err, ErrBind) && !strings.Contains(err.Error(), "empty body") {
		t.Fatalf("empty int: %v", err)
	}
	out, err := RunJSON(context.Background(), "s", "f.go", src, "Ptr", []byte(`{"k":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `"p"` {
		t.Fatalf("%s", out)
	}

	src = `package main
func Hello(c chan int) int { return 0 }
`
	_, err = RunJSON(context.Background(), "s", "f.go", src, "Hello", []byte(`{}`))
	if err == nil {
		t.Fatal("chan param")
	}
}

func TestSafeTypeChangeRecovers(t *testing.T) {
	_, err := safeTypeChange(types.NewChan(types.SendRecv, types.Typ[types.Int]))
	// chan may or may not panic; just exercise
	_ = err
}

func TestCompileAndCallHelpers(t *testing.T) {
	compileProgram(nil)
	compileProgram(&Program{})
	cb := compileBlock(&ssa.BasicBlock{})
	if cb == nil {
		t.Fatal("empty block")
	}
	_ = getCompiledBlock(nil, &ssa.BasicBlock{})

	src := `package main
func F() int {
	s := make([]int, 2, 5)
	t := s[0:1]
	u := []int{1, 2, 3}[1:]
	type S struct{ A, B int }
	v := S{A: 1, B: 2}
	return cap(s) + len(t) + len(u) + v.A + v.B
}
func Conv() int {
	var x int64 = 3
	return int(x)
}
func Iface() int {
	var i interface{} = 4
	var j any = i
	n, _ := j.(int)
	return n
}
func Sel() int {
	ch := make(chan int, 1)
	ch <- 1
	select {
	case v := <-ch:
		return v
	default:
		return 0
	}
}
`
	if v, err := Run("c", src, "F"); err != nil || v != 5+1+2+1+2 {
		t.Fatalf("F %v %v", v, err)
	}
	if v, err := Run("c", src, "Conv"); err != nil || v != 3 {
		t.Fatalf("conv %v %v", v, err)
	}
	if v, err := Run("c", src, "Iface"); err != nil || v != 4 {
		t.Fatalf("iface %v %v", v, err)
	}
	if v, err := Run("c", src, "Sel"); err != nil || v != 1 {
		t.Fatalf("sel %v %v", v, err)
	}
}
