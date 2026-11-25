// 文件说明：probe_stale_test.go — gofunction/internal/interp 的单元测试（TestProbeStaleContext 等）
//
// 用法
//
//	go test ./gofunction/internal/interp -run 'TestProbeStaleContext'
//
// 调用架构
//
//	[上游] gofunction
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/internal/interp/probe_stale_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bench_test.go, cachetest_test.go, call.go, call_test.go, cheap_test.go 等 40 个
//
// 主要符号
//
//	TestProbeStaleContext(t *testing.T)

package interp

import (
	"testing"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

// 探针：复用 Program 时函数值/全局闭包是否绑定了已取消的旧 context。
func TestProbeStaleContext(t *testing.T) {
	src := `package main
import "strings"
var g = func(x int) int { return x + 1 }
func up(r rune) rune {
	for i := 0; i < 3; i++ {
	}
	return r - 32
}
func UseGlobal() int { return g(1) }
func apply(f func(rune) rune, r rune) rune { return f(r) }
func UseFuncValue() string {
	return strings.ToUpper(string(apply(up, 'a')))
}
`
	p, err := BuildProgram("p", "main", src)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if v, err := p.Run("", "UseGlobal"); err != nil {
			t.Errorf("UseGlobal run %d: %v %v", i, v, err)
		}
		if v, err := p.Run("", "UseFuncValue"); err != nil || v != "A" {
			t.Errorf("UseFuncValue run %d: %v %v", i, v, err)
		}
	}
}
