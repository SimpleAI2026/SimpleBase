# gongx
golang脚本执行引擎，提供如下功能：

- [ ]引擎优化
- [ ]测试覆盖率
- [ ]性能测试
# examples
```
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
```

# benchmark
（1）测试用例