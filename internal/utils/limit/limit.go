// 文件说明：limit.go — 分页参数的归一化处理
//
// 用法
//
//	Normalize(n, def, max int) int  // Normalize 把分页参数 n 归一为合法值：n 非正或超过 max 时返回 def
//	Clamp(n, def, max int) int  // Clamp 把 n 限制在 [1, max]：n 非正时返回 def，n 超过 max 时返回 max
//
// 调用架构
//
//	[上游] internal/audit, internal/auth, internal/catalog 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/utils/limit/limit.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//
// 主要符号
//
//	Normalize(n, def, max int) int
//	Clamp(n, def, max int) int

package limit

// Normalize 把分页参数 n 归一为合法值：n 非正或超过 max 时返回 def。
//
// max <= 0 表示不设上限。仓库里约 20 处 `if limit <= 0 || limit > N { limit = def }`
// 都由本函数承接，避免默认值与上限散落各包。
//
// 注意与 Clamp 的区别：Normalize 在超限时回落到默认值，Clamp 截到上限。
func Normalize(n, def, max int) int {
	if n <= 0 || (max > 0 && n > max) {
		return def
	}
	return n
}

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
