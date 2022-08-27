package loader

import (
	"fmt"
	"testing"

	"github.com/linkxzhou/gongx/loader"
)

func Test_LoadFile(t *testing.T) {
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
