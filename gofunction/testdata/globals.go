//go:build ignore

// 文件说明：globals.go — gofunction/testdata 中 N 等符号的实现
//
// 用法
//
//	var N
//
// 调用架构
//
//	[上游] 仅测试引用：gofunction/internal/interp
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/testdata/globals.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] basic.go, basics.go, controlflow.go, flowcontrol.go, funclist.go 等 13 个
//
// 主要符号
//
//	var N
//	current() int

package main

var N = 1

func current() int { return N }
