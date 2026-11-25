//go:build ignore

// 文件说明：hostfn.go — gofunction/testdata 的内部实现（demo 等）
//
// 用法
//
//	包内实现，无导出与跨文件调用
//
// 调用架构
//
//	[上游] 仅测试引用：gofunction/internal/interp
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/testdata/hostfn.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] basic.go, basics.go, controlflow.go, flowcontrol.go, funclist.go 等 13 个
//
// 主要符号
//
//	demo() int

package main

import "example.com/host"

func demo() int {
	_ = host.Label
	return host.Add(20, 22)
}
