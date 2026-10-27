// 文件说明：std_fmt.go — gofunction/packages 的内部实现（stdFmt 等）
//
// 用法
//
//	同包调用：stdFmt() → packages.go
//
// 调用架构
//
//	[上游] gofunction/bench, gofunction/cmd/gofunction, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/packages/std_fmt.go
//	         │ 依赖
//	         ▼
//	[下游] gofunction/importer
//	[同包] packages.go, std_encoding.go, std_http.go, std_math.go, std_misc.go 等 7 个
//	[关键引用] stdFmt() ← packages.go
//
// 主要符号
//
//	stdFmt() []*importer.Object

package packages

import (
	"fmt"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
)

// stdFmt fmt 包注册清单
func stdFmt() []*importer.Object {
	return []*importer.Object{
		funcObj("Sprintf", fmt.Sprintf),
		funcObj("Sprint", fmt.Sprint),
		funcObj("Sprintln", fmt.Sprintln),
		funcObj("Sscanf", fmt.Sscanf),
		funcObj("Sscan", fmt.Sscan),
		funcObj("Sscanln", fmt.Sscanln),
		funcObj("Printf", fmt.Printf),
		funcObj("Println", fmt.Println),
		funcObj("Print", fmt.Print),
		funcObj("Errorf", fmt.Errorf),
	}
}
