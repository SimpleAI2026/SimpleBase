//go:build ignore

// 文件说明：funclist.go — gofunction/testdata 中 Exported、S 等符号的实现
//
// 用法
//
//	Exported()
//	type S struct{…}
//
// 调用架构
//
//	[上游] 仅测试引用：gofunction/internal/interp
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/testdata/funclist.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] basic.go, basics.go, controlflow.go, flowcontrol.go, globals.go 等 13 个
//
// 主要符号
//
//	Exported()
//	hidden()
//	(s *S) Method()
//	type S struct{…}
//	var x

package main

func Exported()      {}
func hidden()        {}
func (s *S) Method() {}

type S struct{}

var x = 1
