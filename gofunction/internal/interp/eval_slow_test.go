// 文件说明：eval_slow_test.go — gofunction/internal/interp 的单元测试（TestSlowEvalFallback 等）
//
// 用法
//
//	go test ./gofunction/internal/interp -run 'TestSlowEvalFallback'
//
// 调用架构
//
//	[上游] gofunction
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/internal/interp/eval_slow_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bench_test.go, cachetest_test.go, call.go, call_test.go, cheap_test.go 等 40 个
//
// 主要符号
//
//	TestSlowEvalFallback(t *testing.T)

package interp

import (
	"testing"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

func TestSlowEvalFallback(t *testing.T) {
	skipFastEval = true
	t.Cleanup(func() { skipFastEval = false })

	src := `package main
func All() int {
	var u uint = 8
	var f float64 = 2.5
	s := "a" + "b"
	i := 10 + 2 - 3*2/2%3
	i = i & 7 | 1 ^ 2
	i = i &^ 1
	i = (1 << 2) + (8 >> 1)
	_ = u + 1 - 1*1/1%2&3 | 1 ^ 1
	_ = u << 1 >> 1
	_ = f + 1 - 1*1/1
	_ = s < "c" && s <= "ab" && s > "a" && s >= "ab"
	_ = 1 < 2 && 2 <= 2 && 3 > 1 && 3 >= 3 && 1 == 1 && 1 != 0
	_ = u < 9 && u <= 8 && u > 1 && u >= 8
	_ = f < 3 && f <= 2.5 && f > 1 && f >= 2.5
	_ = -i + ^i
	_ = -u + ^u
	_ = -f
	_ = !false
	if s == "ab" && s != "x" {
		i++
	}
	return i + len(s)
}
`
	v, err := Run("slow", src, "All")
	if err != nil {
		t.Fatal(err)
	}
	if v == nil {
		t.Fatal("nil")
	}
}
