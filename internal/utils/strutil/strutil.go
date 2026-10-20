// Package strutil 提供跨包复用的字符串裁剪与列表切分工具。
//
// 只允许 import 标准库；不得 import 根模块任何业务包（见 internal/utils/README.md）。
package strutil

import (
	"strings"
	"unicode/utf8"
)

// Truncate 返回 s 的前 max 个字节。max <= 0 时返回空串。
//
// 按字节截断，可能切断多字节字符；面向展示的文本用 TruncateRunes 或
// TruncateValidUTF8。用途是给日志 / 落库字段设上限，不改变短字符串。
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max]
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

// TruncateRunes 按字符（rune）截断到 max 个字符，保证结果始终是合法 UTF-8。
//
// 用于需要完整字符语义的场景（如截断用户可见的中文提示）；
// 需要严格字节上限时用 Truncate。
func TruncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	count := 0
	for i := range s {
		if count == max {
			return s[:i]
		}
		count++
	}
	return s
}

// TruncateValidUTF8 把 b 转成合法 UTF-8 并截到 max 字节以内，不截断半个字符。
//
// 返回的 bool 表示是否发生了截断（max <= 0 视为不截断）。非法字节序列经
// strings.ToValidUTF8 替换为 U+FFFD，因此结果长度可能与输入不同。
func TruncateValidUTF8(b []byte, max int) (string, bool) {
	s := strings.ToValidUTF8(string(b), "\uFFFD")
	if max <= 0 || len(s) <= max {
		return s, false
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
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
