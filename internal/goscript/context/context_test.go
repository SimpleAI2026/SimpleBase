package context

import (
	"fmt"
	"go/types"
	"testing"

	"github.com/linkxzhou/goedge/internal/goscript/context"
	"github.com/linkxzhou/goedge/internal/goscript/importer"
)

func setExternalConfig(c *context.Context) {
	typesLoader := importer.NewTypesLoader()
	c.SetExternalConfig(importer.NewImporter(c, typesLoader), func() []*types.Package {
		return typesLoader.Packages()
	})
}

func TestLoadFileMain(t *testing.T) {
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
		c := context.NewContext(context.EnableDumpInstr | context.EnableDumpImports)
		setExternalConfig(c)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
	{
		c := context.NewContext(context.DisableRecover)
		setExternalConfig(c)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
	{
		c := context.NewContext(context.EnableDumpImports)
		setExternalConfig(c)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
}

func TestLoadFileNoMain(t *testing.T) {
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
		c := context.NewContext(context.EnableDumpInstr | context.EnableDumpImports)
		setExternalConfig(c)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
	{
		c := context.NewContext(context.DisableRecover)
		setExternalConfig(c)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
	{
		c := context.NewContext(context.EnableDumpImports)
		setExternalConfig(c)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
}

func TestLoadFileHttp(t *testing.T) {
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
		c := context.NewContext(context.EnableDumpInstr | context.EnableDumpImports)
		setExternalConfig(c)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
	{
		c := context.NewContext(context.DisableRecover)
		setExternalConfig(c)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
	{
		c := context.NewContext(context.EnableDumpImports)
		setExternalConfig(c)
		p, err := c.LoadFile("__main__.go", sources)
		fmt.Println("p: ", p, ", err: ", err)
	}
}
