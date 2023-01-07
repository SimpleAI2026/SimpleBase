package loader

import (
	"fmt"
	"testing"

	"github.com/linkxzhou/gongx/interp/goscript/loader"
)

func Test_LoadFileMain(t *testing.T) {
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
		test(37)
		fmt.Println("=====")
	}
	`
	{
		c := loader.NewContext(loader.EnableDumpInstr | loader.EnableDumpImports)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
	{
		c := loader.NewContext(loader.DisableRecover)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
	{
		c := loader.NewContext(loader.EnableDumpImports)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
}

func Test_LoadFileNoMain(t *testing.T) {
	sources := `
	package test

	func fib(i int) int {
		if i < 2 {
			return i
		}
		return fib(i - 1) + fib(i - 2)
	}
	
	func test(i int) int {
		return fib(i)
	}
	`
	{
		c := loader.NewContext(loader.EnableDumpInstr | loader.EnableDumpImports)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
	{
		c := loader.NewContext(loader.DisableRecover)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
	{
		c := loader.NewContext(loader.EnableDumpImports)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
}

func Test_LoadFileHttp(t *testing.T) {
	sources := `
	package main

	import (
		"encoding/json"
		"net/http"
	)

	func Handle(req http.Request) interface{} {
		coronaVirusJSON := ` + "`" + `{
			"name" : "covid-11",
			"country" : "China",
			"city" : "Wuhan",
			"reason" : "Non vedge Food"
		}` + "`" + `
		var result map[string]interface{}
		json.Unmarshal([]byte(coronaVirusJSON), &result)
		return result
	}
	`
	{
		c := loader.NewContext(loader.EnableDumpInstr | loader.EnableDumpImports)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
	{
		c := loader.NewContext(loader.DisableRecover)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
	{
		c := loader.NewContext(loader.EnableDumpImports)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
}
