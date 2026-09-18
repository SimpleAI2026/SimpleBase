package importer

import (
	"go/types"
	"reflect"
	"testing"
	"unsafe"

	"golang.org/x/tools/go/ssa"
)

type withMethod struct {
	N int
}

func (w withMethod) ValueMeth() int { return w.N }
func (w *withMethod) PtrMeth() int  { return w.N }

func TestKeywordsAndRegistryLookups(t *testing.T) {
	_ = Keywords()
	reg := NewRegistry()
	if err := reg.RegisterPackage("demo/path", "demo",
		CreateFunction("Add", func(a, b int) int { return a + b }, "add"),
		CreateConstant("C", 1, "c"),
		CreateVariable("V", new(int), reflect.TypeOf(0), "v"),
		CreateType("T", reflect.TypeOf(withMethod{}), "t"),
	); err != nil {
		t.Fatal(err)
	}
	if spec, ok := reg.GetPackageByName("demo"); !ok || spec == nil {
		t.Fatal("GetPackageByName")
	}
	if GetPackageByName("demo") != nil {
		// global registry may or may not contain demo
	}
	typ := types.Typ[types.Int]
	reg.SetExternalType(typ, reflect.TypeOf(0))
	if _, ok := reg.GetExternalType(typ); !ok {
		t.Fatal("GetExternalType")
	}
	_ = GetExternalType(typ)
	_ = GetCompletionItems()
}

func TestImporterTypeOfKinds(t *testing.T) {
	imp := NewImporter()
	_ = imp.SsaPackage("none")
	_ = imp.ExternalObjects()

	pkg := types.NewPackage("example.com/p", "p")
	ssaPkg := &ssa.Package{Pkg: pkg}
	imp2 := NewImporter(ssaPkg)
	if imp2.SsaPackage("p") != ssaPkg {
		t.Fatal("ssa by name")
	}
	got, err := imp2.Import("p")
	if err != nil || got != pkg {
		t.Fatalf("import ssa pkg: %v %v", got, err)
	}

	reg := NewRegistry()
	_ = reg.RegisterPackage("k/arr", "arr",
		CreateType("Arr", reflect.TypeOf([2]int{}), ""),
		CreateType("Ch", reflect.TypeOf((chan int)(nil)), ""),
		CreateType("SendCh", reflect.TypeOf((chan<- int)(nil)), ""),
		CreateType("RecvCh", reflect.TypeOf((<-chan int)(nil)), ""),
		CreateType("Fn", reflect.TypeOf(func(int) string { return "" }), ""),
		CreateType("Iface", reflect.TypeOf((*interface{ M() })(nil)).Elem(), ""),
		CreateType("Mp", reflect.TypeOf(map[string]int{}), ""),
		CreateType("Ptr", reflect.TypeOf((*int)(nil)), ""),
		CreateType("Sl", reflect.TypeOf([]int{}), ""),
		CreateType("St", reflect.TypeOf(struct {
			A int `json:"a"`
		}{}), ""),
		CreateType("Up", reflect.TypeOf(unsafe.Pointer(nil)), ""),
		CreateConstant("Bl", true, ""),
		CreateConstant("I", int64(1), ""),
		CreateConstant("U", uint64(1), ""),
		CreateConstant("F", 1.5, ""),
		CreateConstant("S", "x", ""),
		CreateConstant("Cx", complex128(1+2i), ""),
		CreateFunction("Methy", (withMethod).ValueMeth, ""),
		CreateType("WM", reflect.TypeOf(withMethod{}), ""),
	)
	imp3 := NewImporterWithRegistry(reg)
	if p := imp3.Package(""); p != nil {
		t.Fatal("empty path")
	}
	p := imp3.Package("k/arr")
	if p == nil {
		t.Fatal("package")
	}
	vendor := imp3.Package("x/vendor/k/arr")
	if vendor == nil {
		t.Fatal("vendor path")
	}
	_, _ = imp3.Import("k/arr")
	_, _ = imp3.Import("k/arr") // cache
	_ = imp3.typeOf(reflect.TypeOf(withMethod{}), p)
	_ = imp3.typeOf(reflect.TypeOf([2]int{}), p)
	_ = imp3.typeOf(reflect.TypeOf((chan int)(nil)), p)
	_ = imp3.typeOf(reflect.TypeOf((chan<- int)(nil)), p)
	_ = imp3.typeOf(reflect.TypeOf((<-chan int)(nil)), p)
	_ = imp3.typeOf(reflect.TypeOf(func(int) (string, error) { return "", nil }), p)
	_ = imp3.typeOf(reflect.TypeOf((*interface{ M() int })(nil)).Elem(), p)
	_ = imp3.typeOf(reflect.TypeOf(map[string]int{}), p)
	_ = imp3.typeOf(reflect.TypeOf((*int)(nil)), p)
	_ = imp3.typeOf(reflect.TypeOf([]byte{}), p)
	_ = imp3.typeOf(reflect.TypeOf(struct{ A int }{}), p)
	_ = imp3.typeOf(reflect.TypeOf(unsafe.Pointer(nil)), p)
}

func TestParseNameTypeCached(t *testing.T) {
	imp := NewImporter()
	rt := reflect.TypeOf(withMethod{})
	n1 := imp.parseNameType(rt)
	n2 := imp.parseNameType(rt)
	if n1 != n2 {
		t.Fatal("expected cached named type")
	}
	// builtin named type has empty PkgPath → pkg == nil branch
	_ = imp.parseNameType(reflect.TypeOf(int(0)))
}

func TestTypeOfUnexportedAndBuiltin(t *testing.T) {
	type mixed struct {
		A int
		b int
	}
	imp := NewImporter()
	pkg := types.NewPackage("example.com/m", "m")
	_ = imp.typeOf(reflect.TypeOf(mixed{}), pkg)
	_ = imp.typeOf(reflect.TypeOf(true), pkg)
	_ = imp.typeOf(reflect.TypeOf(int(0)), pkg)
}
