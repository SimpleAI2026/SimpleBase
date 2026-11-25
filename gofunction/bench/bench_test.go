// 文件说明：bench_test.go — gofunction/bench 的单元测试（BenchmarkNativeFib22、BenchmarkScriptFib22 等）
//
// 用法
//
//	go test ./gofunction/bench -run 'BenchmarkNativeFib22|BenchmarkScriptFib22|BenchmarkRunJSONFib22|BenchmarkNativeJSON'
//
// 调用架构
//
//	[上游] 无（可执行入口）
//	[本文件] gofunction/bench/bench_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] cache_bench_test.go, main.go
//
// 主要符号
//
//	BenchmarkNativeFib22(b *testing.B)
//	BenchmarkScriptFib22(b *testing.B)
//	BenchmarkRunJSONFib22(b *testing.B)
//	BenchmarkNativeJSON(b *testing.B)
//	BenchmarkScriptJSON(b *testing.B)
//	BenchmarkNativeText(b *testing.B)
//	BenchmarkScriptText(b *testing.B)

package main

// 进程内三路径基准：native / 预编译解释执行 / RunJSON 全链路。
// 与 HTTP 压测互补——剥离网络与 HTTP 框架开销，测纯逻辑耗时。
//
// 运行：
//
//	go test -run='^$' -bench=. -benchmem ./bench

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
