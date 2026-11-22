// 文件说明：strutil.go — 跨包复用的字符串规范化、截断与列表切分工具
//
// 用法
//
//	Normalize(s string) string  // Normalize 去掉首尾空白并转小写
//	Truncate(s string, max int) string  // Truncate 返回 s 的前 max 个字节
//	TruncateRunes(s string, max int) string  // TruncateRunes 返回 s 的前 max 个 Unicode 字符
//	TruncateEllipsis(s string, max int) string  // TruncateEllipsis 与 Truncate 相同，但发生截断时追加 "…"（额外 3 字节）
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/api, internal/auth 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/utils/strutil/strutil.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//
// 主要符号
//
//	Normalize(s string) string
//	Truncate(s string, max int) string
//	TruncateRunes(s string, max int) string
//	TruncateEllipsis(s string, max int) string
//	SplitCSV(csv string) []string

package strutil

import "strings"

// Normalize 去掉首尾空白并转小写，用于大小写无关的配置值和标识符。
func Normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Truncate 返回 s 的前 max 个字节。max <= 0 时返回空串。
//
// 按字节截断，可能切断多字节字符；按字符截断用 TruncateRunes，需要省略号时用 TruncateEllipsis。
// 用途是给日志 / 落库字段设上限，不改变短字符串。
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// TruncateRunes 返回 s 的前 max 个 Unicode 字符。max <= 0 时返回空串。
func TruncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// TruncateEllipsis 与 Truncate 相同，但发生截断时追加 "…"（额外 3 字节）。
//
// 用于错误信息等需要提示「还有后续内容」的展示位。
func TruncateEllipsis(s string, max int) string {
	if max <= 0 {
		return "…"
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// SplitCSV 按逗号切分并去掉每项首尾空白与空项（空输入返回空切片）。
//
// 对应历史的 splitNonEmpty / parsePermissions / envList 三份同构实现。
// 恒返回非 nil 的切片，因此可直接序列化为 JSON 的 []；需要 null 语义的
// 调用方（如配置项未设置）自行在调用前判空。
func SplitCSV(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return []string{}
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
