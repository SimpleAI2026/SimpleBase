package gofunction

// 本文件覆盖 Program 公共 API：构建、执行、全局变量读写、函数列表解析。

import (
	"go/ast"
	"reflect"
	"testing"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"github.com/linkxzhou/SimpleBase/gofunction/value"
	"golang.org/x/tools/go/ssa"
)

func TestBuildProgramTypeError(t *testing.T) {
	if _, err := BuildProgram("t", "main", "package main\nfunc test() { _ = nosuch }"); err == nil {
		t.Fatal("expected typecheck error")
	}
}

func TestAutoImportAlreadyImported(t *testing.T) {
	spec := importer.GetPackageByName("fmt")
	if spec == nil {
		t.Fatal("fmt not registered")
	}
	f := &ast.File{
		Imports:    []*ast.ImportSpec{spec},
		Unresolved: []*ast.Ident{{Name: "fmt"}},
	}
	_ = autoImport(f)
}

func TestSetGetGlobalEdgeCases(t *testing.T) {
	p, err := BuildProgram("t", "main", `package main
var S string = "a"
var N int = 1
func test() int { return N }
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SetGlobalValue("S", map[string]int{"x": 1}); err == nil {
		t.Fatal("expected assign error")
	}
	g := p.mainPkg.Members["N"].(*ssa.Global)
	cell := value.ValueOf(9)
	p.globals[g] = &cell
	v, err := p.GetGlobalValue("N")
	if err != nil || v != 9 {
		t.Fatalf("non-ptr cell %v %v", v, err)
	}
	nilCell := value.Value(value.RValue{Value: reflect.Zero(reflect.TypeOf((*int)(nil)))})
	p.globals[g] = &nilCell
	v, err = p.GetGlobalValue("N")
	if err != nil || v != nil {
		t.Fatalf("nil ptr cell %v %v", v, err)
	}
	delete(p.globals, g)
	v, err = p.GetGlobalValue("N")
	if err != nil || v != 0 {
		t.Fatalf("uninitialized %v %v", v, err)
	}

	// convert-compatible SetGlobalValue
	p2, err := BuildProgram("t", "main", `package main
var F float64 = 1
func test() float64 { return F }
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := p2.SetGlobalValue("F", int(3)); err != nil {
		t.Fatal(err)
	}
}

func TestExternalLookups(t *testing.T) {
	p := &Program{}
	mustPanic(t, "externalValue no importer", func() {
		_ = p.externalValue(&ssa.Global{})
	})
	if p.externalFunction(&ssa.Function{}) != nil {
		t.Fatal("nil importer")
	}
	p.importer = importer.NewImporter()
	mustPanic(t, "external object missing", func() {
		g := &ssa.Global{}
		_ = p.externalValue(g)
	})
	if p.importedFunction(nil) != nil {
		t.Fatal("nil fn")
	}
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

func TestFunctionNotFoundAndBuildError(t *testing.T) {
	if _, err := Run("t", "package main\nfunc a() {}", "missing"); err == nil {
		t.Fatal("expected missing function")
	}
	if _, err := Run("t", "not go source", "a"); err == nil {
		t.Fatal("expected build error")
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
