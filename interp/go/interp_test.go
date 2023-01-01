package interp_go

import (
	"fmt"
	"testing"

	"github.com/linkxzhou/gongx/interp/go/loader"
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
	iv1, err1 := NewInterp(c, p)
	fmt.Println("NewInterp err: ", err1, ", iv1: ", iv1)
	iv2, err2 := iv1.RunMain()
	fmt.Println("RunMain err: ", err2, ", iv2: ", iv2)
}

func TestNewInterpAny(t *testing.T) {
	sources := `
	package test

	func fib(i int) int {
		if i < 2 {
			return i
		}
		return fib(i - 1) + fib(i - 2)
	}	
	`
	c := loader.NewContext(loader.EnableDumpImports)
	p, err := c.LoadFile("__main__.go", sources)
	fmt.Println("p: ", p, ", err: ", err)
	iv1, err1 := NewInterp(c, p)
	fmt.Println("NewInterp err: ", err1, ", iv1: ", iv1)
	iv2, err2 := iv1.RunAny("fib", 37)
	fmt.Println("RunAny err: ", err2, ", iv2: ", iv2)
}

func TestNewInterpAny1(t *testing.T) {
	sources := `
package test

import (
	"encoding/json"
)

func Handle() interface{} {
	coronaVirusJSON := "` + `{'name':'covid-11'}` + `"
	var result map[string]interface{}
	json.Unmarshal([]byte(coronaVirusJSON), &result)
	return result
}
	`
	c := loader.NewContext(loader.EnableDumpImports)
	p, err := c.LoadFile("__main__.go", sources)
	fmt.Println("p: ", p, ", err: ", err)
	iv1, err1 := NewInterp(c, p)
	fmt.Println("NewInterp err: ", err1, ", iv1: ", iv1)
	iv2, err2 := iv1.RunAny("Handle")
	fmt.Println("RunAny err: ", err2, ", iv2: ", iv2)
}

func TestNewInterpAnyCache(t *testing.T) {
	sources := `
package test

import (
	"encoding/json"
)

func Handle() interface{} {
	coronaVirusJSON := "` + `{'name':'covid-11'}` + `"
	var result map[string]interface{}
	json.Unmarshal([]byte(coronaVirusJSON), &result)
	return result
}
	`
	c := loader.NewContext(loader.EnableDumpImports)
	iv1, _ := LoadFileWithCache(c, "__main__.go", sources)
	iv2, err2 := iv1.RunAny("Handle")
	fmt.Println("TestNewInterpAnyCache err: ", err2, ", iv2: ", iv2)

	iv11, _ := LoadFileWithCache(c, "__main__.go", sources)
	iv21, err21 := iv11.RunAny("Handle")
	fmt.Println("TestNewInterpAnyCache1 err: ", err21, ", iv21: ", iv21)
}

func BenchmarkNewInterpCache(b *testing.B) {
	sources := `
package test

import (
	"encoding/json"
)

func Handle() interface{} {
	coronaVirusJSON := "` + `{'name':'covid-11'}` + `"
	var result map[string]interface{}
	json.Unmarshal([]byte(coronaVirusJSON), &result)
	return result
}
	`
	c := loader.NewContext(loader.DisableRecover)
	for n := 0; n < b.N; n++ {
		iv, _ := LoadFileWithCache(c, "__main__.go", sources)
		iv.RunAny("Handle")
	}
}

func BenchmarkNewInterpNoCache(b *testing.B) {
	sources := `
package test

import (
	"encoding/json"
)

func Handle() interface{} {
	coronaVirusJSON := "` + `{'name':'covid-11'}` + `"
	var result map[string]interface{}
	json.Unmarshal([]byte(coronaVirusJSON), &result)
	return result
}
	`
	c := loader.NewContext(loader.DisableRecover)
	for n := 0; n < b.N; n++ {
		p, _ := c.LoadFile("__main__.go", sources)
		iv1, _ := NewInterp(c, p)
		iv1.RunAny("Handle")
	}
}
