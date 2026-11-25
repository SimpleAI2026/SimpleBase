//go:build ignore

// 文件说明：leaky.go — internal/sbcli/testdata/dev/invalid/functions 中 Leak 等符号的实现
//
// 用法
//
//	Leak(in string) string
//
// 调用架构
//
//	[上游] 无（可执行入口）
//	[本文件] internal/sbcli/testdata/dev/invalid/functions/leaky.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] 无
//
// 主要符号
//
//	var leaked
//	var leakedSB
//	Leak(in string) string

package main

// 假密钥，仅用于校验 fixture。
var leaked = "sk-aaaaaaaaaaaaaaaaaaaaaaaa"

var leakedSB = "sb_live_00000000000000000000000000000000"

func Leak(in string) string { return leaked + leakedSB + in }
