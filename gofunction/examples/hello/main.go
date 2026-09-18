// hello 演示 gofunction.Run：编译一段 Go 源码并执行指定函数。
package main

import (
	"fmt"
	"log"

	"github.com/linkxzhou/SimpleBase/gofunction"
)

func main() {
	src := `package main

func add(a, b int) int {
	return a + b
}
`
	result, err := gofunction.Run("hello-seq", src, "add", 40, 2)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("add(40, 2) = %v\n", result)
}
