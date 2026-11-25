//go:build ignore

// 文件说明：controlflow.go — gofunction/testdata 的内部实现（classify 等）
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
//	[本文件] gofunction/testdata/controlflow.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] basic.go, basics.go, flowcontrol.go, funclist.go, globals.go 等 13 个
//
// 主要符号
//
//	classify(n int) (result string)

package main

import "fmt"

func classify(n int) (result string) {
	sum := 0
	for i := 1; i <= n; i++ {
		sum += i
	}

	var kind string
	switch {
	case sum < 10:
		kind = "small"
	case sum < 100:
		kind = "medium"
	default:
		kind = "large"
	}

	if n%2 == 0 {
		kind = kind + "/even"
	} else {
		kind = kind + "/odd"
	}

	result = fmt.Sprintf("%s:%d", kind, sum)
	defer func() { result = result + "/ok" }()
	return
}
