package vm

import (
	"strings"
	"unicode/utf8"
)

// TruncateUTF8 把命令输出转成合法 UTF-8，并按字节上限截断且不切断字符。
// max <= 0 表示不截断；bool 只报告是否删除字节，不报告非法输入替换。
func TruncateUTF8(b []byte, max int) (string, bool) {
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
