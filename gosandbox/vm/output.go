// 文件说明：output.go — gosandbox/vm 中 TruncateUTF8 等符号的实现
//
// 用法
//
//	TruncateUTF8(b []byte, max int) (string, bool)  // TruncateUTF8 把命令输出转成合法 UTF-8，并按字节上限截断且不切断字符
//
// 调用架构
//
//	[上游] gosandbox/vm/box, gosandbox/vm/fake, gosandbox/vm/local, internal/api 等 7 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/output.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] driver.go, env.go, env_test.go, errors.go, output_test.go 等 9 个
//	[关键引用] TruncateUTF8 ← output_test.go
//
// 主要符号
//
//	TruncateUTF8(b []byte, max int) (string, bool)

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
