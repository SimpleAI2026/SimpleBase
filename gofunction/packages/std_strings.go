// 文件说明：std_strings.go — gofunction/packages 的内部实现（stdStrings 等）
//
// 用法
//
//	同包调用：stdStrings() → packages.go
//
// 调用架构
//
//	[上游] gofunction/bench, gofunction/cmd/gofunction, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/packages/std_strings.go
//	         │ 依赖
//	         ▼
//	[下游] gofunction/importer
//	[同包] packages.go, std_encoding.go, std_fmt.go, std_http.go, std_math.go 等 7 个
//	[关键引用] stdStrings() ← packages.go
//
// 主要符号
//
//	stdStrings() []*importer.Object

package packages

import (
	"strings"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
)

// stdStrings strings 包注册清单
func stdStrings() []*importer.Object {
	return []*importer.Object{
		funcObj("ToUpper", strings.ToUpper),
		funcObj("ToLower", strings.ToLower),
		funcObj("Contains", strings.Contains),
		funcObj("HasPrefix", strings.HasPrefix),
		funcObj("HasSuffix", strings.HasSuffix),
		funcObj("Split", strings.Split),
		funcObj("SplitN", strings.SplitN),
		funcObj("Fields", strings.Fields),
		funcObj("Join", strings.Join),
		funcObj("Replace", strings.Replace),
		funcObj("ReplaceAll", strings.ReplaceAll),
		funcObj("TrimSpace", strings.TrimSpace),
		funcObj("Trim", strings.Trim),
		funcObj("Index", strings.Index),
		funcObj("Count", strings.Count),
		funcObj("Repeat", strings.Repeat),
	}
}
