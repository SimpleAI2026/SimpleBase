//go:build ignore

// 文件说明：stdlib.go — gofunction/testdata 的内部实现（demo 等）
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
//	[本文件] gofunction/testdata/stdlib.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] basic.go, basics.go, controlflow.go, flowcontrol.go, funclist.go 等 13 个
//
// 主要符号
//
//	demo(name string) string

package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func demo(name string) string {
	upper := strings.ToUpper(name)
	msg := fmt.Sprintf("hello %s", upper)
	b, err := json.Marshal(map[string]string{"msg": msg})
	if err != nil {
		return err.Error()
	}
	return string(b)
}
