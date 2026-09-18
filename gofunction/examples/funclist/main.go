// funclist 演示 ParseFuncList：只解析 AST，不编译、不执行。
package main

import (
	"fmt"
	"log"

	"github.com/linkxzhou/SimpleBase/gofunction"
)

func main() {
	src := `package main

func Exported() {}
func hidden() {}
func (s *S) Method() {}

type S struct{}

var x = 1
`
	exported, err := gofunction.ParseFuncList(src, false)
	if err != nil {
		log.Fatal(err)
	}
	all, err := gofunction.ParseFuncList(src, true)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("exportedOnly = %v\n", exported)
	fmt.Printf("exportedAll  = %v\n", all)
}
