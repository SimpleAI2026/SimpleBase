package tests

import (
	"fmt"
	"testing"

	luaNew "github.com/Shopify/go-lua"
	"github.com/dop251/goja"
	"github.com/linkxzhou/gongx/interp/go/loader"
	"github.com/linkxzhou/gongx/interp_go"
	lua "github.com/yuin/gopher-lua"
)

func TestNewInterp(t *testing.T) {
	sources := `
	package test

	import "fmt"

	func fib(i int) int {
		if i < 2 {
			return i
		}
		return fib(i - 1) + fib(i - 2)
	}

	func test(i int) int {
		return fib(i)
	}
	
	func main() {
		fmt.Println("====", test(37))
	}
	`
	c := loader.NewContext(loader.EnableDumpImports)
	p, err := c.LoadFile("__main__.go", sources)
	fmt.Println("p: ", p, ", err: ", err)
	iv1, err1 := interp_go.NewInterp(c, p)
	fmt.Println("NewInterp err: ", err1, ", iv1: ", iv1)
	iv2, err2 := iv1.RunMain()
	fmt.Println("RunMain err: ", err2, ", iv2: ", iv2)
}

func TestGoja(t *testing.T) {
	vm := goja.New()
	_, err := vm.RunString(`
	function fib(i) {
		if (i < 2) {
			return i
		}
		return fib(i - 1) + fib(i - 2)
	}
	`)
	if err != nil {
		panic(err)
	}
	fib, ok := goja.AssertFunction(vm.Get("fib"))
	if !ok {
		panic("Not a function")
	}
	res, err := fib(goja.Undefined(), vm.ToValue(37))
	if err != nil {
		panic(err)
	}
	fmt.Println(res)
}

func TestLua(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	if err := L.DoString(`
	local function fib(n)
		if n == 1 or n == 2 then
			return 1
		else
			return fib(n - 1) + fib(n - 2)
		end
	end
	fib(37)
	`); err != nil {
		panic(err)
	}
}

func TestLuaNew(t *testing.T) {
	L := luaNew.NewState()
	if err := luaNew.LoadString(L, `
	local function fib(n)
		if n == 1 or n == 2 then
			return 1
		else
			return fib(n - 1) + fib(n - 2)
		end
	end
	fib(37)
	`); err != nil {
		panic(err)
	}
	L.Call(0, 0)
}
