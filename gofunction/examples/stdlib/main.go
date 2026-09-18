// stdlib 演示 blank-import packages 后，脚本调用 fmt / strings / encoding/json。
package main

import (
	"fmt"
	"log"

	"github.com/linkxzhou/SimpleBase/gofunction"
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

func main() {
	src := `package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func demo(name string) string {
	upper := strings.ToUpper(name)
	msg := fmt.Sprintf("hello %s", upper)
	b, err := json.Marshal(map[string]string{"msg": msg})
	if err != nil {
		return err.Error()
	}
	return string(b)
}
`
	result, err := gofunction.Run("stdlib-seq", src, "demo", "gofunction")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)
}
