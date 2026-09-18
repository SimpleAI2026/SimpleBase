// globals 演示 Program.SetGlobalValue / GetGlobalValue。
package main

import (
	"fmt"
	"log"

	"github.com/linkxzhou/SimpleBase/gofunction"
)

func main() {
	src := `package main

var N = 1

func current() int { return N }
`
	prog, err := gofunction.BuildProgram("globals-seq", "main", src)
	if err != nil {
		log.Fatal(err)
	}

	before, err := prog.GetGlobalValue("N")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("N after init = %v\n", before)

	if err := prog.SetGlobalValue("N", 9); err != nil {
		log.Fatal(err)
	}
	after, err := prog.GetGlobalValue("N")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("N after SetGlobalValue = %v\n", after)

	got, err := prog.Run("globals-seq", "current")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("current() = %v\n", got)
}
