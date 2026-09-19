package main

// 进程内三路径基准：native / 预编译解释执行 / RunJSON 全链路。
// 与 HTTP 压测互补——剥离网络与 echo 开销，测纯逻辑耗时。
//
// 运行：
//
//	go test -bench=. -benchmem ./examples/gofunction

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/linkxzhou/SimpleBase/gofunction"
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

// benchNativeFib22 原生 Go fib(22)（性能上限）
func BenchmarkNativeFib22(b *testing.B) {
	for b.Loop() {
		_ = nativeFib(22)
	}
}

// benchScriptFib22 预编译程序重复执行 fib(22)（解释执行净耗时）
func BenchmarkScriptFib22(b *testing.B) {
	p, err := gofunction.BuildProgram("bench-fib", "main", scriptSource)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := p.Run("", "Compute", 22); err != nil {
		b.Fatal(err) // 预热
	}
	for b.Loop() {
		if _, err := p.Run("", "Compute", 22); err != nil {
			b.Fatal(err)
		}
	}
}

// benchRunJSONFib22 RunJSON 全链路（含每请求 BuildProgram，生产调用路径）
func BenchmarkRunJSONFib22(b *testing.B) {
	for b.Loop() {
		if _, err := gofunction.RunJSON(context.Background(), "b", "main", scriptSource, "Compute", []byte("22")); err != nil {
			b.Fatal(err)
		}
	}
}

// benchNativeJSON 原生订单处理
func BenchmarkNativeJSON(b *testing.B) {
	var o nativeOrder
	if err := json.Unmarshal([]byte(orderJSON), &o); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		_ = nativeJSON(o)
	}
}

// benchScriptJSON 脚本订单处理（string 入参，脚本内 json.Unmarshal）
func BenchmarkScriptJSON(b *testing.B) {
	p, err := gofunction.BuildProgram("bench-json", "main", scriptSource)
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if _, err := p.Run("", "JSON", orderJSON); err != nil {
			b.Fatal(err)
		}
	}
}

// benchNativeText 原生日志解析
func BenchmarkNativeText(b *testing.B) {
	for b.Loop() {
		_ = nativeText(logText)
	}
}

// benchScriptText 脚本日志解析
func BenchmarkScriptText(b *testing.B) {
	p, err := gofunction.BuildProgram("bench-text", "main", scriptSource)
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if _, err := p.Run("", "Text", logText); err != nil {
			b.Fatal(err)
		}
	}
}
