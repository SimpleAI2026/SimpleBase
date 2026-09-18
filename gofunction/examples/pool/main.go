// pool 演示 ExecutorPool：GetExecutor → Execute → Close 归还 → 再次获取。
package main

import (
	"fmt"
	"log"

	"github.com/linkxzhou/SimpleBase/gofunction"
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

func main() {
	pool := gofunction.NewDefaultExecutorPool()
	pool.SetServiceName("gofunction-examples")
	defer pool.Close()

	script := `package main
func answer() int { return 40 + 2 }
`

	ex, err := pool.GetExecutor()
	if err != nil {
		log.Fatal(err)
	}
	ex.SetSequenceID("pool-1")
	ex.SetExecutionTimeout(-1)

	result, err := ex.Execute("answer", script)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("first Execute = %v, cost=%dµs, seq=%s\n", result, ex.GetCostTime(), ex.GetSequenceID())
	ex.Close()

	active, idle := pool.GetStats()
	fmt.Printf("after Close: service=%s active=%d idle=%d\n", pool.GetServiceName(), active, idle)

	ex2, err := pool.GetExecutor()
	if err != nil {
		log.Fatal(err)
	}
	result2, err := ex2.Execute("answer", script)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("reused Execute = %v\n", result2)
	ex2.Close()
}
