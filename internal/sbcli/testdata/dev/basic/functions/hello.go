//go:build ignore

// 文件说明：hello.go — internal/sbcli/testdata/dev/basic/functions 中 HelloInput、HelloOutput 等符号的实现
//
// 用法
//
//	type HelloInput struct{…}
//	type HelloOutput struct{…}
//	Hello(in HelloInput) HelloOutput  // Hello 是可调用导出：大写开头，恰好一个入参、一个返回值
//
// 调用架构
//
//	[上游] 无（可执行入口）
//	[本文件] internal/sbcli/testdata/dev/basic/functions/hello.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] 无
//
// 主要符号
//
//	type HelloInput struct{…}
//	type HelloOutput struct{…}
//	Hello(in HelloInput) HelloOutput

package main

import "fmt"

type HelloInput struct {
	Name string `json:"name"`
}

type HelloOutput struct {
	Message string `json:"message"`
}

// Hello 是可调用导出：大写开头，恰好一个入参、一个返回值。
func Hello(in HelloInput) HelloOutput {
	name := in.Name
	if name == "" {
		name = "world"
	}
	return HelloOutput{Message: fmt.Sprintf("hello, %s", name)}
}
