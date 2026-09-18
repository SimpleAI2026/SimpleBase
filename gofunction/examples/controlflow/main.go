// controlflow 演示脚本内 if / for / switch / defer。
package main

import (
	"fmt"
	"log"

	"github.com/linkxzhou/SimpleBase/gofunction"
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

func main() {
	src := `package main

import "fmt"

func classify(n int) (result string) {
	sum := 0
	for i := 1; i <= n; i++ {
		sum += i
	}

	var kind string
	switch {
	case sum < 10:
		kind = "small"
	case sum < 100:
		kind = "medium"
	default:
		kind = "large"
	}

	if n%2 == 0 {
		kind = kind + "/even"
	} else {
		kind = kind + "/odd"
	}

	result = fmt.Sprintf("%s:%d", kind, sum)
	defer func() { result = result + "/ok" }()
	return
}
`
	result, err := gofunction.Run("cf-seq", src, "classify", 10)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)
}
