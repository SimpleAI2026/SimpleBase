// 文件说明：limit.go — 分页参数的归一化处理
//
// 用法
//
//	Clamp(n, def, max int) int  // Clamp 把 n 限制在 [1, max]：n 非正时返回 def，n 超过 max 时返回 max
//
// 调用架构
//
//	[上游] godatabase/blob
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/internal/limit/limit.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//
// 主要符号
//
//	Clamp(n, def, max int) int

package limit

// Clamp 把 n 限制在 [1, max]：n 非正时返回 def，n 超过 max 时返回 max。
//
// 用于「先定默认值、再截上限」的两段式写法（如 sandbox 的列表分页）。
func Clamp(n, def, max int) int {
	if n <= 0 {
		n = def
	}
	if max > 0 && n > max {
		return max
	}
	return n
}
