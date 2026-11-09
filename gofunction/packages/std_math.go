// 文件说明：std_math.go — gofunction/packages 的内部实现（stdMath 等）
//
// 用法
//
//	同包调用：stdMath() → packages.go
//
// 调用架构
//
//	[上游] gofunction/bench, gofunction/cmd/gofunction, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/packages/std_math.go
//	         │ 依赖
//	         ▼
//	[下游] gofunction/importer
//	[同包] packages.go, std_encoding.go, std_fmt.go, std_http.go, std_misc.go 等 7 个
//	[关键引用] stdMath() ← packages.go
//
// 主要符号
//
//	stdMath() []*importer.Object

package packages

import (
	"math"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
)

// stdMath math 包注册清单
func stdMath() []*importer.Object {
	return []*importer.Object{
		funcObj("Sqrt", math.Sqrt),
		funcObj("Abs", math.Abs),
		funcObj("Floor", math.Floor),
		funcObj("Ceil", math.Ceil),
		funcObj("Round", math.Round),
		funcObj("Max", math.Max),
		funcObj("Min", math.Min),
		funcObj("Pow", math.Pow),
		funcObj("Mod", math.Mod),
		funcObj("Trunc", math.Trunc),
		funcObj("Log", math.Log),
		funcObj("Log10", math.Log10),
		funcObj("Log2", math.Log2),
		funcObj("Exp", math.Exp),
		funcObj("Sin", math.Sin),
		funcObj("Cos", math.Cos),
		funcObj("Tan", math.Tan),
		constObj("Pi", math.Pi),
		constObj("E", math.E),
		constObj("Ln10", math.Ln10),
		constObj("Ln2", math.Ln2),
		constObj("Sqrt2", math.Sqrt2),
		constObj("MaxInt", math.MaxInt),
		constObj("MinInt", math.MinInt),
	}
}
