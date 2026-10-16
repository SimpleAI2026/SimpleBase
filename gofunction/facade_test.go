package gofunction

import (
	"context"
	"errors"
	"strings"
	"testing"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

// 门面层测试：确认导出符号确实转发到 internal/interp，
// 且类型别名保持方法集可用（调用方 import 路径不变）。
// 解释器行为本身由 internal/interp 的测试覆盖。

const src = `package main

type Req struct{ A, B int }

func Add(r Req) int { return r.A + r.B }

func Hello(name string) string { return "hi " + name }
`

func TestFacadeRun(t *testing.T) {
	got, err := Run("facade", src, "Hello", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hi bob" {
		t.Fatalf("Run = %v", got)
	}
}

func TestFacadeBuildProgramAndGlobals(t *testing.T) {
	p, err := BuildProgram("facade", "main", "package main\nvar N = 1\nfunc Get() int { return N }\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SetGlobalValue("N", 41); err != nil {
		t.Fatal(err)
	}
	v, err := p.GetGlobalValue("N")
	if err != nil || v != 41 {
		t.Fatalf("GetGlobalValue = %v, %v", v, err)
	}
	if p.Package() == nil {
		t.Fatal("Package() 为 nil")
	}
}

// 类型别名必须让 *Program / *Executor 等方法可直接调用。
func TestFacadeTypeAliases(t *testing.T) {
	var p *Program
	if p != nil {
		t.Fatal("零值 Program 应为 nil")
	}
	ex := NewExecutor(nil)
	if ex == nil {
		t.Fatal("NewExecutor 返回 nil")
	}
	ex.SetSequenceID("seq-facade")
	if ex.GetSequenceID() != "seq-facade" {
		t.Fatalf("GetSequenceID = %q", ex.GetSequenceID())
	}
	ex.Close()

	pool := NewDefaultExecutorPool()
	defer pool.Close()
	got, err := pool.GetExecutor()
	if err != nil || got == nil {
		t.Fatalf("GetExecutor = %v, %v", got, err)
	}
	got.Close()

	var cfg ExecutorPoolConfig = DefaultExecutorPoolConfig
	if cfg.MaxIdle <= 0 {
		t.Fatalf("默认池配置异常: %+v", cfg)
	}

	rt := NewRuntime()
	rt.Initialize("seq-rt")
	rt.SetUserData("k", "v")
	if v, ok := rt.GetUserData("k"); !ok || v != "v" {
		t.Fatalf("GetUserData = %v, %v", v, ok)
	}
	rt.Reset()
	rt.Cleanup()
}

func TestFacadeParseFuncList(t *testing.T) {
	names, err := ParseFuncList(src, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "Add" || names[1] != "Hello" {
		t.Fatalf("ParseFuncList = %v", names)
	}
}

func TestFacadeValidateAndRunJSON(t *testing.T) {
	infos, err := ValidateHTTPFuncs(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 || infos[0].Name != "Add" {
		t.Fatalf("ValidateHTTPFuncs = %+v", infos)
	}

	out, err := RunJSON(context.Background(), "facade", "main", src, "Add", []byte(`{"A":20,"B":22}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "42" {
		t.Fatalf("RunJSON = %s", out)
	}
}

// 错误哨兵必须与实现包是同一个值，errors.Is 才能跨包匹配。
func TestFacadeErrorSentinels(t *testing.T) {
	_, err := RunJSON(context.Background(), "facade", "main", src, "Nope", nil)
	if !errors.Is(err, ErrFunctionNotFound) {
		t.Fatalf("期望 ErrFunctionNotFound，得 %v", err)
	}
	if _, err := ValidateHTTPFuncs("package main\nfunc Bad() {}\n"); err == nil {
		t.Fatal("不合规脚本应报错")
	}
	for _, e := range []error{ErrTimeout, ErrBind, ErrCompile, ErrFunctionNotFound} {
		if e == nil || e.Error() == "" {
			t.Fatalf("哨兵为空: %v", e)
		}
	}
}

func TestFacadeSetLoggerAndDebugOutput(t *testing.T) {
	// nil 入参为 no-op，不得 panic。
	SetLogger(nil)
	SetDebugOutput(nil)
}
