// hostfn 演示在编译前向 GlobalRegistry 注册宿主函数，供脚本 import。
package main

import (
	"fmt"
	"log"

	"github.com/linkxzhou/SimpleBase/gofunction"
	"github.com/linkxzhou/SimpleBase/gofunction/importer"
)

func main() {
	err := importer.RegisterPackage("example.com/host", "host",
		importer.CreateFunction("Add", func(a, b int) int { return a + b }, "a+b"),
		importer.CreateConstant("Label", "host-add", "demo label"),
	)
	if err != nil {
		log.Fatal(err)
	}

	src := `package main

import "example.com/host"

func demo() int {
	_ = host.Label
	return host.Add(20, 22)
}
`
	result, err := gofunction.Run("hostfn-seq", src, "demo")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("host.Add(20, 22) = %v\n", result)
}
