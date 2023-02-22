package tests

import (
	"fmt"
	"io/ioutil"
	"testing"

	wasmer "github.com/wasmerio/wasmer-go/wasmer"
)

func TestSum(t *testing.T) {
	wasmBytes, err := ioutil.ReadFile("input.wasm")
	if err != nil {
		t.Fatalf("ReadFile err: %v", err)
	}
	engine := wasmer.NewEngine()
	store := wasmer.NewStore(engine)
	// Compiles the module
	module, _ := wasmer.NewModule(store, wasmBytes)
	// Instantiates the module
	importObject := wasmer.NewImportObject()
	instance, err := wasmer.NewInstance(module, importObject)
	if err != nil {
		t.Fatalf("NewInstance err: %v", err)
	}
	// Gets the `sum` exported function from the WebAssembly instance.
	fib_result, err := instance.Exports.GetFunction("fib_result")
	if err != nil {
		t.Fatalf("GetFunction err: %v", err)
	}
	// Calls that exported function with Go standard values. The WebAssembly
	// types are inferred and values are casted automatically.
	result, _ := fib_result(37)
	fmt.Println(result) // 42!
}

func TestLua(t *testing.T) {
	wasmBytes, err := ioutil.ReadFile("lua.wasm")
	if err != nil {
		t.Fatalf("ReadFile err: %v", err)
	}
	engine := wasmer.NewEngine()
	store := wasmer.NewStore(engine)
	// Compiles the module
	module, _ := wasmer.NewModule(store, wasmBytes)
	// Instantiates the module
	importObject := wasmer.NewImportObject()
	instance, err := wasmer.NewInstance(module, importObject)
	if err != nil {
		t.Fatalf("NewInstance err: %v", err)
	}
	// Gets the `sum` exported function from the WebAssembly instance.
	run_lua, err := instance.Exports.GetFunction("run_lua")
	if err != nil {
		t.Fatalf("GetFunction err: %v", err)
	}
	// Calls that exported function with Go standard values. The WebAssembly
	// types are inferred and values are casted automatically.
	result, _ := run_lua(`
local function fib(n)
	if n == 1 or n == 2 then
		return 1
	else
		return fib(n - 1) + fib(n - 2)
	end
end
fib(37)
	`)
	fmt.Println(result)
}
